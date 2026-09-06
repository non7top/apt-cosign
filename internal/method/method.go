// Package method implements the apt acquire-method main loop: handshake,
// configuration ingestion, and the fetch+verify loop for 600 URI Acquire
// requests.
//
// Status codes below match APT's actual method IPC protocol as implemented
// by apt-pkg/acquire-method.cc (Configuration is sent by apt as "601
// Configuration"; "101 Log" is a method->apt informational message, not
// configuration -- some early drafts of this project's spec had those
// swapped).
package method

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"apt-cosign/internal/aptmsg"
	"apt-cosign/internal/config"
	"apt-cosign/internal/fetch"
	"apt-cosign/internal/verify"

	"github.com/sigstore/sigstore-go/pkg/root"
)

const (
	codeCapabilities  = 100
	codeLog           = 101
	codeURIDone       = 201
	codeURIFailure    = 400
	codeURIAcquire    = 600
	codeConfiguration = 601
)

// DebugLogPath matches the spec's Task 1 requirement to log configuration
// and acquisition activity here for runtime inspection.
const DebugLogPath = "/tmp/apt-sigstore-debug.log"

// maxBundleBytes bounds how large a single Sigstore bundle (sidecar file or
// GitHub attestation) we'll read into memory; real bundles are a few KB to
// a few hundred KB even with a full cert chain and inclusion proof.
const maxBundleBytes = 1 << 20

// OpenDebugLog opens (or creates) the debug log file. Logging is a
// diagnostic aid, not a correctness requirement, so a failure to open it
// falls back to discarding log output rather than failing the method.
func OpenDebugLog() *log.Logger {
	f, err := os.OpenFile(DebugLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return log.New(io.Discard, "", 0)
	}
	return log.New(f, "", log.LstdFlags|log.Lmicroseconds)
}

// Method runs the apt acquire-method protocol loop over the given streams.
type Method struct {
	in     *aptmsg.Reader
	out    *aptmsg.Writer
	dl     *fetch.Downloader
	logger *log.Logger

	configItems map[string]string
	policy      *config.Policy
	policyErr   error

	trustedMaterial root.TrustedMaterial

	// loadTrustedRoot and verifyBundle are swappable seams for testing the
	// protocol/orchestration logic without live network access or real
	// Fulcio-issued certificates; they default to the real implementations.
	loadTrustedRoot func(cacheDir string) (root.TrustedMaterial, error)
	verifyBundle    func(tm root.TrustedMaterial, bundleJSON []byte, artifact io.Reader, pol verify.IdentityPolicy) (*verify.Result, error)
}

func New(r io.Reader, w io.Writer, logger *log.Logger) *Method {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Method{
		in:              aptmsg.NewReader(r),
		out:             aptmsg.NewWriter(w),
		dl:              fetch.NewDownloader(),
		logger:          logger,
		configItems:     map[string]string{},
		policyErr:       errors.New("no 601 Configuration message received yet"),
		loadTrustedRoot: verify.LoadTrustedRoot,
		verifyBundle:    verify.Bundle,
	}
}

// Run reads messages until EOF, handling 601 Configuration and 600 URI
// Acquire; it returns nil on a clean EOF.
func (m *Method) Run(ctx context.Context) error {
	if err := m.sendCapabilities(); err != nil {
		return fmt.Errorf("method: sending capabilities: %w", err)
	}

	for {
		msg, err := m.in.ReadMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("method: reading message: %w", err)
		}
		m.logger.Printf("recv %d %s", msg.Code, msg.Description)

		switch msg.Code {
		case codeConfiguration:
			m.handleConfiguration(msg)
		case codeURIAcquire:
			m.handleAcquire(ctx, msg)
		default:
			m.logger.Printf("ignoring unhandled message code %d %s", msg.Code, msg.Description)
		}
	}
}

func (m *Method) sendCapabilities() error {
	return m.out.WriteMessage(codeCapabilities, "Capabilities",
		aptmsg.H("Version", "1.2"),
		aptmsg.H("Single-Instance", "true"),
		aptmsg.H("Pipeline", "true"),
		aptmsg.H("Send-Config", "true"),
	)
}

func (m *Method) logMessage(uri, text string) {
	m.logger.Printf("%s", text)
	_ = m.out.WriteMessage(codeLog, "Log", aptmsg.H("URI", uri), aptmsg.H("Message", text))
}

func (m *Method) handleConfiguration(msg *aptmsg.Message) {
	for _, item := range msg.All("Config-Item") {
		key, value, ok := config.ParseConfigItem(item)
		if !ok {
			continue
		}
		m.configItems[key] = value
	}

	policy, err := config.FromItems(m.configItems)
	if err != nil {
		m.policy, m.policyErr = nil, err
		m.logger.Printf("sigstore policy configuration error: %v", err)
		return
	}
	m.policy, m.policyErr = policy, nil

	if policy.RekorServer != "" {
		m.logger.Printf("note: Acquire::sigstore::RekorServer=%q is configured but only the public-good Sigstore instance (live TUF trusted root) is currently supported; ignoring", policy.RekorServer)
	}
}

func (m *Method) handleAcquire(ctx context.Context, msg *aptmsg.Message) {
	uri, _ := msg.Get("URI")
	filename, _ := msg.Get("Filename")

	if err := m.acquire(ctx, uri, filename); err != nil {
		m.logger.Printf("acquire %s failed: %v", uri, err)
		_ = m.out.WriteMessage(codeURIFailure, "URI Failure",
			aptmsg.H("URI", uri),
			aptmsg.H("Message", err.Error()),
		)
	}
}

func (m *Method) acquire(ctx context.Context, uri, filename string) error {
	if m.policyErr != nil {
		return fmt.Errorf("sigstore policy not configured: %w", m.policyErr)
	}
	if uri == "" || filename == "" {
		return fmt.Errorf("acquire request missing URI or Filename")
	}

	targetURL, err := fetch.MapURI(uri)
	if err != nil {
		return err
	}
	parsedTarget, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("parsing target URL %q: %w", targetURL, err)
	}

	if m.trustedMaterial == nil {
		m.logMessage(uri, "loading sigstore trusted root")
		tm, err := m.loadTrustedRoot(m.policy.CacheDir)
		if err != nil {
			return fmt.Errorf("loading sigstore trusted root: %w", err)
		}
		m.trustedMaterial = tm
	}

	m.logMessage(uri, "downloading "+targetURL)
	size, sha256Hex, err := m.dl.FetchToFile(ctx, targetURL, filename)
	if err != nil {
		return fmt.Errorf("downloading target: %w", err)
	}

	bundles, source, err := m.candidateBundles(ctx, parsedTarget, targetURL, sha256Hex)
	if err != nil {
		_ = os.Remove(filename)
		return fmt.Errorf("fetching signature bundle: %w", err)
	}

	m.logMessage(uri, fmt.Sprintf("verifying sigstore bundle from %s", source))
	artifactData, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reopening downloaded file for verification: %w", err)
	}

	identityPolicy := m.identityPolicy()

	var result *verify.Result
	var verr error
	for _, b := range bundles {
		result, verr = m.verifyBundle(m.trustedMaterial, b, bytes.NewReader(artifactData), identityPolicy)
		if verr == nil {
			break
		}
	}
	if verr != nil {
		_ = os.Remove(filename)
		return fmt.Errorf("sigstore verification failed (bundle source: %s): %w", source, verr)
	}
	m.logMessage(uri, fmt.Sprintf("verified (rekor log index %d)", result.LogIndex))

	return m.out.WriteMessage(codeURIDone, "URI Done",
		aptmsg.H("URI", uri),
		aptmsg.H("Filename", filename),
		aptmsg.H("Size", strconv.FormatInt(size, 10)),
		aptmsg.H("SHA256-Hash", sha256Hex),
	)
}

// candidateBundles returns the Sigstore bundle(s) to try verifying the
// target against: the sidecar bundle file next to the target (the classic
// Debian detached-signature convention, e.g. Release/Release.gpg, just with
// a Sigstore bundle instead), or -- if that's missing and the target is
// GitHub-hosted -- bundles from GitHub's native Artifact Attestations API.
func (m *Method) candidateBundles(ctx context.Context, target *url.URL, targetURL, sha256Hex string) (bundles [][]byte, source string, err error) {
	sidecarURL := fetch.BundleURL(targetURL, m.policy.BundleExtension)
	data, err := m.dl.FetchBytes(ctx, sidecarURL, maxBundleBytes)
	if err == nil {
		return [][]byte{data}, sidecarURL, nil
	}

	var statusErr *fetch.HTTPStatusError
	if !(errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound) {
		return nil, "", err
	}
	if !fetch.IsGitHubHost(target) {
		return nil, "", fmt.Errorf("no sidecar bundle at %s: %w", sidecarURL, err)
	}

	owner, repo, ok := m.ownerRepoForAttestations(target)
	if !ok {
		return nil, "", fmt.Errorf("no sidecar bundle at %s (%v), and could not determine a GitHub owner/repo for the attestations API fallback", sidecarURL, err)
	}

	bundles, aerr := m.dl.GitHubAttestations(ctx, owner, repo, sha256Hex, m.policy.GitHubToken)
	if aerr != nil {
		return nil, "", fmt.Errorf("no sidecar bundle at %s (%v); GitHub attestations fallback also failed: %w", sidecarURL, err, aerr)
	}
	return bundles, fmt.Sprintf("GitHub attestations API (%s/%s)", owner, repo), nil
}

func (m *Method) ownerRepoForAttestations(u *url.URL) (owner, repo string, ok bool) {
	if r := m.policy.Enforce.Repo; r != nil {
		return r.Owner, r.Name, true
	}
	return fetch.GitHubOwnerRepo(u)
}

func (m *Method) identityPolicy() verify.IdentityPolicy {
	e := m.policy.Enforce
	pol := verify.IdentityPolicy{MinLogIndex: e.MinRekorLogIndex}
	if e.Repo != nil {
		pol.Issuer, pol.SAN, pol.SANRegexp = e.Repo.Identity()
		return pol
	}
	pol.Issuer = e.Issuer
	pol.SAN = e.Identity
	return pol
}
