package verify

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/tuf"
)

const (
	testIssuer   = "https://token.actions.githubusercontent.com"
	testIdentity = "https://github.com/octo-org/widgets/.github/workflows/release.yml@refs/heads/main"
)

func TestBundle_InvalidJSON(t *testing.T) {
	// No trusted material is needed: JSON parsing fails before it would ever
	// be consulted.
	_, err := Bundle(nil, []byte("not json"), bytes.NewReader(nil), IdentityPolicy{})
	if err == nil {
		t.Fatal("expected error for invalid bundle JSON")
	}
}

func TestVerifyEntity_ExactIdentityMatch(t *testing.T) {
	virtualSigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("NewVirtualSigstore: %v", err)
	}

	artifact := []byte("dists/stable/InRelease contents")
	entity, err := virtualSigstore.Sign(testIdentity, testIssuer, artifact)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	result, err := verifyEntity(virtualSigstore, entity, bytes.NewReader(artifact), IdentityPolicy{
		Issuer: testIssuer,
		SAN:    testIdentity,
	})
	if err != nil {
		t.Fatalf("verifyEntity: %v", err)
	}
	if result.LogIndex != 1000 {
		t.Fatalf("LogIndex = %d, want 1000 (VirtualSigstore's fixed test value)", result.LogIndex)
	}
}

func TestVerifyEntity_RegexpIdentityMatch(t *testing.T) {
	virtualSigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("NewVirtualSigstore: %v", err)
	}

	artifact := []byte("some artifact bytes")
	entity, err := virtualSigstore.Sign(testIdentity, testIssuer, artifact)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = verifyEntity(virtualSigstore, entity, bytes.NewReader(artifact), IdentityPolicy{
		Issuer:    testIssuer,
		SANRegexp: `^https://github\.com/octo-org/widgets/\.github/workflows/release\.yml@.+$`,
	})
	if err != nil {
		t.Fatalf("verifyEntity: %v", err)
	}
}

func TestVerifyEntity_IdentityMismatch(t *testing.T) {
	virtualSigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("NewVirtualSigstore: %v", err)
	}

	artifact := []byte("artifact")
	entity, err := virtualSigstore.Sign(testIdentity, testIssuer, artifact)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = verifyEntity(virtualSigstore, entity, bytes.NewReader(artifact), IdentityPolicy{
		Issuer: testIssuer,
		SAN:    "https://github.com/someone-else/other-repo/.github/workflows/release.yml@refs/heads/main",
	})
	if err == nil {
		t.Fatal("expected error for mismatched certificate identity")
	}
}

func TestVerifyEntity_WrongIssuer(t *testing.T) {
	virtualSigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("NewVirtualSigstore: %v", err)
	}

	artifact := []byte("artifact")
	entity, err := virtualSigstore.Sign(testIdentity, testIssuer, artifact)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = verifyEntity(virtualSigstore, entity, bytes.NewReader(artifact), IdentityPolicy{
		Issuer: "https://accounts.google.com",
		SAN:    testIdentity,
	})
	if err == nil {
		t.Fatal("expected error for mismatched OIDC issuer")
	}
}

func TestVerifyEntity_TamperedArtifact(t *testing.T) {
	virtualSigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("NewVirtualSigstore: %v", err)
	}

	entity, err := virtualSigstore.Sign(testIdentity, testIssuer, []byte("original artifact"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = verifyEntity(virtualSigstore, entity, bytes.NewReader([]byte("tampered artifact")), IdentityPolicy{
		Issuer: testIssuer,
		SAN:    testIdentity,
	})
	if err == nil {
		t.Fatal("expected error for artifact/signature mismatch")
	}
}

func TestVerifyEntity_MinLogIndexSatisfied(t *testing.T) {
	virtualSigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("NewVirtualSigstore: %v", err)
	}

	artifact := []byte("artifact")
	entity, err := virtualSigstore.Sign(testIdentity, testIssuer, artifact)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// VirtualSigstore always logs test entries at index 1000.
	result, err := verifyEntity(virtualSigstore, entity, bytes.NewReader(artifact), IdentityPolicy{
		Issuer:      testIssuer,
		SAN:         testIdentity,
		MinLogIndex: 1000,
	})
	if err != nil {
		t.Fatalf("verifyEntity: %v", err)
	}
	if result.LogIndex != 1000 {
		t.Fatalf("LogIndex = %d, want 1000", result.LogIndex)
	}
}

func TestVerifyEntity_MinLogIndexRejected(t *testing.T) {
	virtualSigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("NewVirtualSigstore: %v", err)
	}

	artifact := []byte("artifact")
	entity, err := virtualSigstore.Sign(testIdentity, testIssuer, artifact)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = verifyEntity(virtualSigstore, entity, bytes.NewReader(artifact), IdentityPolicy{
		Issuer:      testIssuer,
		SAN:         testIdentity,
		MinLogIndex: 1001, // one above the fixed test log index of 1000
	})
	if err == nil {
		t.Fatal("expected error: log index below configured minimum")
	}
	if !strings.Contains(err.Error(), "below configured minimum") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeMaterial struct {
	root.BaseTrustedMaterial
}

func TestLoadTrustedRoot_RetrySuccess(t *testing.T) {
	orig := fetchTrustedRoot
	defer func() { fetchTrustedRoot = orig }()

	attempts := 0
	fetchTrustedRoot = func(opts *tuf.Options) (root.TrustedMaterial, error) {
		attempts++
		if attempts < 2 {
			return nil, fmt.Errorf("transient network timeout")
		}
		return &fakeMaterial{}, nil
	}

	tr, err := LoadTrustedRoot(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("LoadTrustedRoot: %v", err)
	}
	if tr == nil {
		t.Fatal("expected trusted material, got nil")
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestLoadTrustedRoot_CacheFallback(t *testing.T) {
	orig := fetchTrustedRoot
	defer func() { fetchTrustedRoot = orig }()

	refreshCalls := 0
	cacheCalls := 0
	fetchTrustedRoot = func(opts *tuf.Options) (root.TrustedMaterial, error) {
		if opts.ForceCache {
			cacheCalls++
			return &fakeMaterial{}, nil
		}
		refreshCalls++
		return nil, fmt.Errorf("504 Gateway Timeout")
	}

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	tr, err := LoadTrustedRoot(context.Background(), t.TempDir(), logger)
	if err != nil {
		t.Fatalf("LoadTrustedRoot: %v", err)
	}
	if tr == nil {
		t.Fatal("expected trusted material from cache, got nil")
	}
	if refreshCalls != 3 {
		t.Fatalf("expected 3 refresh retries, got %d", refreshCalls)
	}
	if cacheCalls != 1 {
		t.Fatalf("expected 1 cache fallback call, got %d", cacheCalls)
	}
	if !strings.Contains(logBuf.String(), "falling back to cached trusted root") {
		t.Fatalf("expected fallback warning in log, got: %q", logBuf.String())
	}
}

func TestLoadTrustedRoot_ContextCancelled(t *testing.T) {
	orig := fetchTrustedRoot
	defer func() { fetchTrustedRoot = orig }()

	fetchTrustedRoot = func(opts *tuf.Options) (root.TrustedMaterial, error) {
		return nil, fmt.Errorf("timeout")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := LoadTrustedRoot(ctx, t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadTrustedRoot_AllFail(t *testing.T) {
	orig := fetchTrustedRoot
	defer func() { fetchTrustedRoot = orig }()

	fetchTrustedRoot = func(opts *tuf.Options) (root.TrustedMaterial, error) {
		if opts.ForceCache {
			return nil, fmt.Errorf("no cache on disk")
		}
		return nil, fmt.Errorf("connection refused")
	}

	_, err := LoadTrustedRoot(context.Background(), t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error when both refresh and cache fail")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected original refresh error, got: %v", err)
	}
}
