package verify

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
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
