// Package fetch maps sigstore+https:// URIs to their plain https://
// equivalents and downloads targets and their Sigstore verification
// material. It works identically against a normal apt repo webserver and a
// GitHub-hosted one (e.g. raw.githubusercontent.com): both are just static
// HTTPS file servers from the downloader's point of view. The one
// GitHub-specific path is the Attestations API fallback, used only when a
// target has no sidecar bundle file of its own.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const schemePrefix = "sigstore+"

// apiBaseURL is the GitHub API base URL; overridden in tests.
var apiBaseURL = "https://api.github.com"

// MapURI converts an apt "sigstore+https://..." (or sigstore+http://) URI
// into the real URI to fetch.
func MapURI(uri string) (string, error) {
	if !strings.HasPrefix(uri, schemePrefix) {
		return "", fmt.Errorf("fetch: URI %q does not use a sigstore+ scheme", uri)
	}
	return strings.TrimPrefix(uri, schemePrefix), nil
}

// BundleURL returns the sidecar signature-bundle URL for a target URL. This
// mirrors the classic Debian detached-signature convention of publishing
// Release next to Release.gpg: the bundle is fetched from the same host and
// path, just with extension appended.
func BundleURL(targetURL, extension string) string {
	return targetURL + extension
}

// Downloader fetches targets and Sigstore material over HTTP(S).
type Downloader struct {
	Client *http.Client
}

func NewDownloader() *Downloader {
	return &Downloader{Client: &http.Client{Timeout: 2 * time.Minute}}
}

// FetchToFile GETs url and streams the response body into destPath,
// returning the size and hex-encoded SHA-256 digest of what was written.
// destPath's parent directory must already exist (APT creates lists/partial
// and archives/partial itself).
func (d *Downloader) FetchToFile(ctx context.Context, rawURL, destPath string) (size int64, sha256Hex string, err error) {
	body, err := d.get(ctx, rawURL)
	if err != nil {
		return 0, "", err
	}
	defer body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return 0, "", fmt.Errorf("fetch: creating %s: %w", destPath, err)
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), body)
	if err != nil {
		return 0, "", fmt.Errorf("fetch: downloading %s: %w", rawURL, err)
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// FetchBytes GETs url fully into memory, up to maxBytes (+1, to detect
// truncation) to bound memory use against an oversized or malicious
// response.
func (d *Downloader) FetchBytes(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	body, err := d.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch: reading %s: %w", rawURL, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("fetch: %s exceeds %d byte limit", rawURL, maxBytes)
	}
	return data, nil
}

func (d *Downloader) get(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch: building request for %s: %w", rawURL, err)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %s: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &HTTPStatusError{URL: rawURL, StatusCode: resp.StatusCode}
	}
	return resp.Body, nil
}

// HTTPStatusError is returned when a fetch gets a non-200 response. Callers
// use errors.As to detect a 404 (e.g. "no sidecar bundle published") and
// fall back to another bundle source.
type HTTPStatusError struct {
	URL        string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("fetch: %s: unexpected status %d", e.URL, e.StatusCode)
}

// IsGitHubHost reports whether u is a github.com/raw.githubusercontent.com
// (or api.github.com) endpoint.
func IsGitHubHost(u *url.URL) bool {
	switch u.Hostname() {
	case "github.com", "raw.githubusercontent.com", "api.github.com":
		return true
	}
	return false
}

// GitHubOwnerRepo extracts an "owner/repo" pair from a github.com or
// raw.githubusercontent.com URL path (e.g. /OWNER/REPO/refs/heads/main/...
// or /OWNER/REPO/releases/download/...). ok is false if the path doesn't
// have at least two segments.
func GitHubOwnerRepo(u *url.URL) (owner, repo string, ok bool) {
	segments := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return "", "", false
	}
	return segments[0], segments[1], true
}

// gitHubAttestationsResponse mirrors the relevant subset of GitHub's
// Artifact Attestations API response:
// GET /repos/{owner}/{repo}/attestations/{subjectDigest}
type gitHubAttestationsResponse struct {
	Attestations []struct {
		Bundle json.RawMessage `json:"bundle"`
	} `json:"attestations"`
}

// GitHubAttestations fetches Sigstore bundles from GitHub's native Artifact
// Attestations API for an artifact with the given SHA-256 digest, used as a
// fallback when a GitHub-hosted target has no sidecar bundle file of its
// own. token is optional (anonymous requests are rate-limited more
// strictly); each returned []byte is a standalone Sigstore bundle JSON
// document, exactly as accepted by internal/verify.
func (d *Downloader) GitHubAttestations(ctx context.Context, owner, repo, sha256Hex, token string) ([][]byte, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/attestations/sha256:%s", apiBaseURL, url.PathEscape(owner), url.PathEscape(repo), sha256Hex)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch: building attestations request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: attestations for %s/%s: %w", owner, repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{URL: apiURL, StatusCode: resp.StatusCode}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("fetch: reading attestations response: %w", err)
	}

	var parsed gitHubAttestationsResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("fetch: parsing attestations response: %w", err)
	}
	if len(parsed.Attestations) == 0 {
		return nil, fmt.Errorf("fetch: no attestations found for %s/%s digest sha256:%s", owner, repo, sha256Hex)
	}

	bundles := make([][]byte, 0, len(parsed.Attestations))
	for _, a := range parsed.Attestations {
		bundles = append(bundles, []byte(a.Bundle))
	}
	return bundles, nil
}
