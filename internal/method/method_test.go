package method

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"apt-cosign/internal/aptmsg"
	"apt-cosign/internal/verify"

	"github.com/sigstore/sigstore-go/pkg/root"
)

// runMethod feeds input (already-framed protocol messages) to a Method and
// returns everything it wrote to stdout, after wiring in no-op/fake
// trusted-root loading and a verifyBundle stub controlled by verifyResult.
func runMethod(t *testing.T, input string, verifyResult func(bundleJSON []byte) (*verify.Result, error)) string {
	t.Helper()
	var out bytes.Buffer
	m := New(strings.NewReader(input), &out, nil)
	m.loadTrustedRoot = func(string) (root.TrustedMaterial, error) {
		return &fakeTrustedMaterial{}, nil
	}
	if verifyResult != nil {
		m.verifyBundle = func(_ root.TrustedMaterial, bundleJSON []byte, _ io.Reader, _ verify.IdentityPolicy) (*verify.Result, error) {
			return verifyResult(bundleJSON)
		}
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.String()
}

type fakeTrustedMaterial struct {
	root.BaseTrustedMaterial
}

func configMessage(items ...string) string {
	var b strings.Builder
	b.WriteString("601 Configuration\n")
	for _, item := range items {
		fmt.Fprintf(&b, "Config-Item: %s\n", item)
	}
	b.WriteString("\n")
	return b.String()
}

func repoConfigMessage() string {
	return configMessage(
		"Acquire::sigstore::Enforce::Repo::Owner=octo-org",
		"Acquire::sigstore::Enforce::Repo::Name=widgets",
		"Acquire::sigstore::Enforce::Repo::Pipeline=release.yml",
	)
}

func acquireMessage(uri, filename string) string {
	return fmt.Sprintf("600 URI Acquire\nURI: %s\nFilename: %s\n\n", uri, filename)
}

func firstMessage(t *testing.T, stream string) *aptmsg.Message {
	t.Helper()
	msg, err := aptmsg.NewReader(strings.NewReader(stream)).ReadMessage()
	if err != nil {
		t.Fatalf("reading message: %v", err)
	}
	return msg
}

func allMessages(t *testing.T, stream string) []*aptmsg.Message {
	t.Helper()
	r := aptmsg.NewReader(strings.NewReader(stream))
	var msgs []*aptmsg.Message
	for {
		msg, err := r.ReadMessage()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading message: %v", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

func TestHandshake_SendsCapabilities(t *testing.T) {
	out := runMethod(t, "", nil)
	msg := firstMessage(t, out)
	if msg.Code != codeCapabilities {
		t.Fatalf("code = %d, want %d", msg.Code, codeCapabilities)
	}
	if v, _ := msg.Get("Send-Config"); v != "true" {
		t.Fatalf("Send-Config = %q", v)
	}
}

func TestAcquire_WithoutConfiguration_Fails(t *testing.T) {
	input := acquireMessage("sigstore+https://example.com/InRelease", "/tmp/x")
	out := runMethod(t, input, nil)

	msgs := allMessages(t, out)
	last := msgs[len(msgs)-1]
	if last.Code != codeURIFailure {
		t.Fatalf("code = %d, want %d (%s)", last.Code, codeURIFailure, last.Description)
	}
	if msg, _ := last.Get("Message"); !strings.Contains(msg, "not configured") {
		t.Fatalf("Message = %q", msg)
	}
}

func TestAcquire_Success_SidecarBundle(t *testing.T) {
	const targetContent = "Origin: test\n"
	var sawSidecarRequest bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dists/stable/InRelease":
			_, _ = w.Write([]byte(targetContent))
		case "/dists/stable/InRelease.sigstore":
			sawSidecarRequest = true
			_, _ = w.Write([]byte(`{"fake":"bundle"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "InRelease")
	uri := "sigstore+" + srv.URL + "/dists/stable/InRelease"

	input := repoConfigMessage() + acquireMessage(uri, destFile)
	out := runMethod(t, input, func(bundleJSON []byte) (*verify.Result, error) {
		if !strings.Contains(string(bundleJSON), "fake") {
			t.Fatalf("unexpected bundle content passed to verifier: %s", bundleJSON)
		}
		return &verify.Result{LogIndex: 42}, nil
	})

	if !sawSidecarRequest {
		t.Fatal("expected the .sigstore sidecar to be requested")
	}

	msgs := allMessages(t, out)
	last := msgs[len(msgs)-1]
	if last.Code != codeURIDone {
		t.Fatalf("code = %d, want %d (%s)", last.Code, codeURIDone, last.Description)
	}
	if fn, _ := last.Get("Filename"); fn != destFile {
		t.Fatalf("Filename = %q, want %q", fn, destFile)
	}
	if sz, _ := last.Get("Size"); sz != fmt.Sprint(len(targetContent)) {
		t.Fatalf("Size = %q", sz)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != targetContent {
		t.Fatalf("downloaded content = %q", got)
	}
}

func TestAcquire_VerificationFailure_RemovesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/InRelease":
			_, _ = w.Write([]byte("content"))
		case "/InRelease.sigstore":
			_, _ = w.Write([]byte(`{"fake":"bundle"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	destFile := filepath.Join(t.TempDir(), "InRelease")
	uri := "sigstore+" + srv.URL + "/InRelease"

	input := repoConfigMessage() + acquireMessage(uri, destFile)
	out := runMethod(t, input, func([]byte) (*verify.Result, error) {
		return nil, fmt.Errorf("boom: identity mismatch")
	})

	msgs := allMessages(t, out)
	last := msgs[len(msgs)-1]
	if last.Code != codeURIFailure {
		t.Fatalf("code = %d, want %d", last.Code, codeURIFailure)
	}
	if msg, _ := last.Get("Message"); !strings.Contains(msg, "boom") {
		t.Fatalf("Message = %q", msg)
	}
	if _, err := os.Stat(destFile); !os.IsNotExist(err) {
		t.Fatalf("expected downloaded file to be removed after failed verification, stat err = %v", err)
	}
}

func TestAcquire_NoSidecar_NonGitHubHost_Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/InRelease":
			_, _ = w.Write([]byte("content"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	destFile := filepath.Join(t.TempDir(), "InRelease")
	uri := "sigstore+" + srv.URL + "/InRelease"

	input := repoConfigMessage() + acquireMessage(uri, destFile)
	out := runMethod(t, input, nil)

	msgs := allMessages(t, out)
	last := msgs[len(msgs)-1]
	if last.Code != codeURIFailure {
		t.Fatalf("code = %d, want %d", last.Code, codeURIFailure)
	}
	if msg, _ := last.Get("Message"); !strings.Contains(msg, "no sidecar bundle") {
		t.Fatalf("Message = %q", msg)
	}
}

func TestAcquire_BadURIScheme_Fails(t *testing.T) {
	input := repoConfigMessage() + acquireMessage("https://example.com/InRelease", "/tmp/x")
	out := runMethod(t, input, nil)

	msgs := allMessages(t, out)
	last := msgs[len(msgs)-1]
	if last.Code != codeURIFailure {
		t.Fatalf("code = %d, want %d", last.Code, codeURIFailure)
	}
}

func TestAcquire_MultipleSourcesDispatchByURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sigstore"):
			_, _ = w.Write([]byte(`{"fake":"bundle"}`))
		default:
			_, _ = w.Write([]byte("content for " + r.URL.Path))
		}
	}))
	defer srv.Close()

	input := configMessage(
		"Acquire::sigstore::Sources::repo-a::Match="+srv.URL+"/repo-a/",
		"Acquire::sigstore::Sources::repo-a::Enforce::Repo::Owner=org-a",
		"Acquire::sigstore::Sources::repo-a::Enforce::Repo::Name=repo-a",
		"Acquire::sigstore::Sources::repo-b::Match="+srv.URL+"/repo-b/",
		"Acquire::sigstore::Sources::repo-b::Enforce::Repo::Owner=org-b",
		"Acquire::sigstore::Sources::repo-b::Enforce::Repo::Name=repo-b",
	) +
		acquireMessage("sigstore+"+srv.URL+"/repo-a/InRelease", filepath.Join(t.TempDir(), "a")) +
		acquireMessage("sigstore+"+srv.URL+"/repo-b/InRelease", filepath.Join(t.TempDir(), "b"))

	var out bytes.Buffer
	m := New(strings.NewReader(input), &out, nil)
	m.loadTrustedRoot = func(string) (root.TrustedMaterial, error) {
		return &fakeTrustedMaterial{}, nil
	}
	var sawSANs []string
	m.verifyBundle = func(_ root.TrustedMaterial, _ []byte, _ io.Reader, pol verify.IdentityPolicy) (*verify.Result, error) {
		sawSANs = append(sawSANs, pol.SANRegexp)
		return &verify.Result{LogIndex: 1}, nil
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	msgs := allMessages(t, out.String())
	var done int
	for _, msg := range msgs {
		if msg.Code == codeURIDone {
			done++
		}
	}
	if done != 2 {
		t.Fatalf("expected 2 successful acquisitions, got %d (messages: %+v)", done, msgs)
	}

	if len(sawSANs) != 2 {
		t.Fatalf("expected 2 verifyBundle calls, got %d", len(sawSANs))
	}
	if !strings.Contains(sawSANs[0], "org-a/repo-a") {
		t.Fatalf("first request's policy = %q, want it scoped to org-a/repo-a", sawSANs[0])
	}
	if !strings.Contains(sawSANs[1], "org-b/repo-b") {
		t.Fatalf("second request's policy = %q, want it scoped to org-b/repo-b", sawSANs[1])
	}
}

func TestAcquire_ZeroConfig_GitHubHost_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sigstore"):
			_, _ = w.Write([]byte(`{"fake":"bundle"}`))
		default:
			_, _ = w.Write([]byte("content"))
		}
	}))
	defer srv.Close()

	// Redirects the real hostname raw.githubusercontent.com to the local
	// test server at the TCP level, so fetch.IsGitHubHost (which only
	// looks at the hostname) sees the genuine article while the request is
	// actually served locally.
	serverAddr := strings.TrimPrefix(srv.URL, "http://")
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if strings.HasPrefix(addr, "raw.githubusercontent.com:") {
					addr = serverAddr
				}
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}

	destFile := filepath.Join(t.TempDir(), "InRelease")
	uri := "sigstore+http://raw.githubusercontent.com/octo-org/widgets/InRelease"

	// A bare, empty 601 Configuration -- no Enforce/Sources at all.
	input := configMessage() + acquireMessage(uri, destFile)

	var out bytes.Buffer
	m := New(strings.NewReader(input), &out, nil)
	m.dl.Client = client
	m.loadTrustedRoot = func(string) (root.TrustedMaterial, error) {
		return &fakeTrustedMaterial{}, nil
	}
	var sawIssuer, sawSAN string
	m.verifyBundle = func(_ root.TrustedMaterial, _ []byte, _ io.Reader, pol verify.IdentityPolicy) (*verify.Result, error) {
		sawIssuer, sawSAN = pol.Issuer, pol.SANRegexp
		return &verify.Result{LogIndex: 1}, nil
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	msgs := allMessages(t, out.String())
	last := msgs[len(msgs)-1]
	if last.Code != codeURIDone {
		t.Fatalf("code = %d, want %d (%s): %+v", last.Code, codeURIDone, last.Description, msgs)
	}
	if sawIssuer != "https://token.actions.githubusercontent.com" {
		t.Fatalf("issuer = %q, want the GitHub Actions issuer (derived Repo policy)", sawIssuer)
	}
	if !strings.Contains(sawSAN, "octo-org/widgets") {
		t.Fatalf("SAN regexp = %q, want it scoped to octo-org/widgets (derived from the URL)", sawSAN)
	}
}

func TestHandleConfiguration_EmptyConfiguration_NoError(t *testing.T) {
	// An empty configuration is valid at parse time now: PolicyFor decides
	// per-request whether a GitHub-hosted URL can fall back to a derived
	// identity (see TestAcquire_ZeroConfig_GitHubHost_Succeeds below).
	var out bytes.Buffer
	m := New(strings.NewReader(""), &out, nil)
	m.handleConfiguration(&aptmsg.Message{
		Code: codeConfiguration, Description: "Configuration",
	})
	if m.policyErr != nil {
		t.Fatalf("expected no policyErr for an empty configuration, got %v", m.policyErr)
	}
}

func TestHandleConfiguration_InvalidPolicy_RecordsError(t *testing.T) {
	var out bytes.Buffer
	m := New(strings.NewReader(""), &out, nil)
	m.handleConfiguration(&aptmsg.Message{
		Code:        codeConfiguration,
		Description: "Configuration",
		Headers: []aptmsg.Header{
			aptmsg.H("Config-Item", "Acquire::sigstore::Enforce::Fields::rekorLogIndex=not-a-number"),
			aptmsg.H("Config-Item", "Acquire::sigstore::Enforce::Fields::certificate-oidc-issuer=https://accounts.google.com"),
			aptmsg.H("Config-Item", "Acquire::sigstore::Enforce::Fields::certificate-identity=someone@example.com"),
		},
	})
	if m.policyErr == nil {
		t.Fatal("expected policyErr to be set for a malformed rekorLogIndex")
	}
}
