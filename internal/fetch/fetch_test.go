package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapURI(t *testing.T) {
	got, err := MapURI("sigstore+https://example.com/dists/stable/InRelease")
	if err != nil {
		t.Fatalf("MapURI: %v", err)
	}
	if got != "https://example.com/dists/stable/InRelease" {
		t.Fatalf("got %q", got)
	}

	if _, err := MapURI("https://example.com/x"); err == nil {
		t.Fatal("expected error for URI without sigstore+ prefix")
	}
}

func TestBundleURL(t *testing.T) {
	got := BundleURL("https://example.com/dists/stable/InRelease", ".sigstore")
	want := "https://example.com/dists/stable/InRelease.sigstore"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFetchToFile(t *testing.T) {
	const content = "hello world"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out")

	d := NewDownloader()
	size, digest, err := d.FetchToFile(context.Background(), srv.URL, dest)
	if err != nil {
		t.Fatalf("FetchToFile: %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}
	want := sha256.Sum256([]byte(content))
	if digest != hex.EncodeToString(want[:]) {
		t.Fatalf("digest mismatch: got %s", digest)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(got) != content {
		t.Fatalf("file content = %q", got)
	}
}

func TestFetchToFile_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	d := NewDownloader()
	_, _, err := d.FetchToFile(context.Background(), srv.URL, filepath.Join(t.TempDir(), "out"))
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var statusErr *HTTPStatusError
	if !asHTTPStatusError(err, &statusErr) || statusErr.StatusCode != 404 {
		t.Fatalf("expected HTTPStatusError with 404, got %v", err)
	}
}

func TestFetchBytes_ExceedsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 100))
	}))
	defer srv.Close()

	d := NewDownloader()
	_, err := d.FetchBytes(context.Background(), srv.URL, 10)
	if err == nil {
		t.Fatal("expected error for response exceeding size limit")
	}
}

func TestIsGitHubHost(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/o/r":                     true,
		"https://raw.githubusercontent.com/o/r/x":    true,
		"https://api.github.com/repos/o/r":           true,
		"https://example.com/dists/stable/InRelease": false,
	}
	for raw, want := range cases {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parsing %q: %v", raw, err)
		}
		if got := IsGitHubHost(u); got != want {
			t.Errorf("IsGitHubHost(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestGitHubOwnerRepo(t *testing.T) {
	u, _ := url.Parse("https://raw.githubusercontent.com/octo-org/widgets/refs/heads/main/dists/stable/InRelease")
	owner, repo, ok := GitHubOwnerRepo(u)
	if !ok || owner != "octo-org" || repo != "widgets" {
		t.Fatalf("got owner=%q repo=%q ok=%v", owner, repo, ok)
	}

	u2, _ := url.Parse("https://example.com/")
	if _, _, ok := GitHubOwnerRepo(u2); ok {
		t.Fatal("expected ok=false for path with no segments")
	}
}

func TestGitHubAttestations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repos/octo-org/widgets/attestations/sha256:abc123") {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"attestations":[{"bundle":{"mediaType":"application/vnd.dev.sigstore.bundle+json;version=0.3"}}]}`))
	}))
	defer srv.Close()

	d := NewDownloader()
	orig := apiBaseURL
	apiBaseURL = srv.URL
	defer func() { apiBaseURL = orig }()

	bundles, err := d.GitHubAttestations(context.Background(), "octo-org", "widgets", "abc123", "tok")
	if err != nil {
		t.Fatalf("GitHubAttestations: %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("got %d bundles, want 1", len(bundles))
	}
	if !strings.Contains(string(bundles[0]), "sigstore.bundle") {
		t.Fatalf("unexpected bundle content: %s", bundles[0])
	}
}

// asHTTPStatusError is a tiny errors.As wrapper kept local to the test file
// to avoid importing "errors" just for this one assertion helper elsewhere.
func asHTTPStatusError(err error, target **HTTPStatusError) bool {
	if e, ok := err.(*HTTPStatusError); ok {
		*target = e
		return true
	}
	return false
}
