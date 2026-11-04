// Package verify checks a Sigstore bundle against the trusted root
// (Fulcio CA / Rekor & CT log keys, refreshed live from the public-good TUF
// repository) and an identity/transparency-log policy: the certificate's
// OIDC issuer and Subject Alternative Name are checked cryptographically by
// sigstore-go, and the transparency log entry's index is checked against a
// configured minimum here, on top of that.
package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	sigbundle "github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	sigverify "github.com/sigstore/sigstore-go/pkg/verify"
)

// IdentityPolicy is the identity and transparency-log policy a bundle must
// satisfy. Exactly one of {Issuer, IssuerRegexp} and one of {SAN, SANRegexp}
// should normally be set; sigstore-go requires at least one of each pair.
type IdentityPolicy struct {
	Issuer       string
	IssuerRegexp string
	SAN          string
	SANRegexp    string

	// MinLogIndex, when > 0, requires the bundle's transparency log entry to
	// have at least this index.
	MinLogIndex int64
}

// Result is what a successful verification established.
type Result struct {
	// LogIndex is the lowest transparency log index across the bundle's
	// verified log entries.
	LogIndex int64
	Detail   *sigverify.VerificationResult
}

// fetchTrustedRoot is a swappable seam for FetchTrustedRootWithOptions.
var fetchTrustedRoot = func(opts *tuf.Options) (root.TrustedMaterial, error) {
	return root.FetchTrustedRootWithOptions(opts)
}

// LoadTrustedRoot fetches the current Sigstore trusted root (Fulcio CA,
// Rekor/CT log keys) from the public-good TUF repository, caching it under
// cacheDir. The cache is trusted for a day before being refreshed.
// If the TUF CDN is unreachable or returns a transient error during refresh,
// the fetch is retried with backoff respecting ctx; if network refresh
// fails completely, it falls back to loading the existing cached trusted
// root via ForceCache and logs a warning.
func LoadTrustedRoot(ctx context.Context, cacheDir string, logger *log.Logger) (root.TrustedMaterial, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	opts := tuf.DefaultOptions()
	opts.CachePath = cacheDir
	opts.CacheValidity = 1
	opts.Context = ctx

	backoffs := []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}
	var lastErr error
	for attempt := 0; attempt <= len(backoffs); attempt++ {
		tr, err := fetchTrustedRoot(opts)
		if err == nil {
			return tr, nil
		}
		lastErr = err

		if attempt < len(backoffs) {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("verify: context cancelled during trusted root refresh: %w", ctx.Err())
			case <-time.After(backoffs[attempt]):
			}
		}
	}

	// If refresh failed (e.g. TUF CDN unreachable), attempt to load
	// from existing local cache without refreshing.
	cachedOpts := tuf.DefaultOptions()
	cachedOpts.CachePath = cacheDir
	cachedOpts.ForceCache = true
	cachedOpts.Context = ctx

	tr, err := fetchTrustedRoot(cachedOpts)
	if err == nil {
		logger.Printf("warning: TUF trusted root refresh failed (%v); falling back to cached trusted root", lastErr)
		return tr, nil
	}

	return nil, fmt.Errorf("verify: fetching trusted root: %w", lastErr)
}

// Bundle verifies bundleJSON (a Sigstore bundle, whether from a repo's
// sidecar file or GitHub's Attestations API) against artifact and pol.
func Bundle(trustedMaterial root.TrustedMaterial, bundleJSON []byte, artifact io.Reader, pol IdentityPolicy) (*Result, error) {
	var b sigbundle.Bundle
	if err := json.Unmarshal(bundleJSON, &b); err != nil {
		return nil, fmt.Errorf("verify: parsing sigstore bundle: %w", err)
	}
	return verifyEntity(trustedMaterial, &b, artifact, pol)
}

// verifyEntity holds the actual verification logic, decoupled from JSON
// bundle parsing so it can be exercised in tests directly against
// sigstore-go's own offline VirtualSigstore/TestEntity fixtures.
func verifyEntity(trustedMaterial root.TrustedMaterial, entity sigverify.SignedEntity, artifact io.Reader, pol IdentityPolicy) (*Result, error) {
	verifier, err := sigverify.NewVerifier(trustedMaterial,
		sigverify.WithTransparencyLog(1),
		sigverify.WithObserverTimestamps(1),
	)
	if err != nil {
		return nil, fmt.Errorf("verify: building verifier: %w", err)
	}

	identity, err := sigverify.NewShortCertificateIdentity(pol.Issuer, pol.IssuerRegexp, pol.SAN, pol.SANRegexp)
	if err != nil {
		return nil, fmt.Errorf("verify: building identity policy: %w", err)
	}

	policy := sigverify.NewPolicy(sigverify.WithArtifact(artifact), sigverify.WithCertificateIdentity(identity))

	detail, err := verifier.Verify(entity, policy)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	logIndex, err := minLogIndex(entity)
	if err != nil {
		return nil, fmt.Errorf("verify: reading transparency log entries: %w", err)
	}
	if pol.MinLogIndex > 0 {
		if logIndex < 0 {
			return nil, fmt.Errorf("verify: no transparency log entry found, cannot enforce minimum log index %d", pol.MinLogIndex)
		}
		if logIndex < pol.MinLogIndex {
			return nil, fmt.Errorf("verify: rekor log index %d is below configured minimum %d", logIndex, pol.MinLogIndex)
		}
	}

	return &Result{LogIndex: logIndex, Detail: detail}, nil
}

func minLogIndex(entity sigverify.SignedEntity) (int64, error) {
	entries, err := entity.TlogEntries()
	if err != nil {
		return 0, err
	}
	min := int64(-1)
	for _, e := range entries {
		if min == -1 || e.LogIndex() < min {
			min = e.LogIndex()
		}
	}
	return min, nil
}
