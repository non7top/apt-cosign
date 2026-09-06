package config

import "testing"

func TestFromItems_RawFields(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::RekorServer":                              "https://sigstore.dev",
		"Acquire::sigstore::Enforce::Fields::rekorLogIndex":           "100000",
		"Acquire::sigstore::Enforce::Fields::certificate-oidc-issuer": "https://accounts.google.com",
		"Acquire::sigstore::Enforce::Fields::certificate-identity":    "someone@example.com",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	if p.Enforce.Repo != nil {
		t.Fatalf("expected no Repo policy, got %+v", p.Enforce.Repo)
	}
	if p.Enforce.MinRekorLogIndex != 100000 {
		t.Fatalf("MinRekorLogIndex = %d", p.Enforce.MinRekorLogIndex)
	}
	if p.Enforce.Issuer != "https://accounts.google.com" || p.Enforce.Identity != "someone@example.com" {
		t.Fatalf("unexpected issuer/identity: %+v", p.Enforce)
	}
	if p.BundleExtension != DefaultBundleExtension {
		t.Fatalf("expected default bundle extension, got %q", p.BundleExtension)
	}
}

func TestFromItems_RepoBlock(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Owner":    "octo-org",
		"Acquire::sigstore::Enforce::Repo::Name":     "widgets",
		"Acquire::sigstore::Enforce::Repo::Pipeline": "release.yml",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	if p.Enforce.Repo == nil {
		t.Fatal("expected Repo policy to be set")
	}
	issuer, sanExact, sanRegexp := p.Enforce.Repo.Identity()
	if issuer != githubActionsIssuer {
		t.Fatalf("issuer = %q", issuer)
	}
	if sanExact != "" {
		t.Fatalf("expected regexp-only match (no Ref pinned), got exact SAN %q", sanExact)
	}
	wantRegexp := `^https://github\.com/octo-org/widgets/\.github/workflows/release\.yml@.+$`
	if sanRegexp != wantRegexp {
		t.Fatalf("sanRegexp = %q, want %q", sanRegexp, wantRegexp)
	}
}

func TestFromItems_RepoBlockWithRef(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Owner":    "octo-org",
		"Acquire::sigstore::Enforce::Repo::Name":     "widgets",
		"Acquire::sigstore::Enforce::Repo::Pipeline": "release.yml",
		"Acquire::sigstore::Enforce::Repo::Ref":      "refs/heads/main",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	_, sanExact, sanRegexp := p.Enforce.Repo.Identity()
	want := "https://github.com/octo-org/widgets/.github/workflows/release.yml@refs/heads/main"
	if sanExact != want {
		t.Fatalf("sanExact = %q, want %q", sanExact, want)
	}
	if sanRegexp != "" {
		t.Fatalf("expected no regexp when Ref is pinned, got %q", sanRegexp)
	}
}

func TestFromItems_RepoBlockIncomplete(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Owner": "octo-org",
	}
	if _, err := FromItems(items); err == nil {
		t.Fatal("expected error for incomplete Repo block")
	}
}

func TestFromItems_MissingEnforcement(t *testing.T) {
	if _, err := FromItems(map[string]string{}); err == nil {
		t.Fatal("expected error when no enforcement policy is configured")
	}
}

func TestFromItems_RepoTakesPrecedenceOverRawFields(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Owner":                     "octo-org",
		"Acquire::sigstore::Enforce::Repo::Name":                      "widgets",
		"Acquire::sigstore::Enforce::Repo::Pipeline":                  "release.yml",
		"Acquire::sigstore::Enforce::Fields::certificate-oidc-issuer": "https://accounts.google.com",
		"Acquire::sigstore::Enforce::Fields::certificate-identity":    "someone@example.com",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	if p.Enforce.Repo == nil {
		t.Fatal("expected Repo to take precedence")
	}
	if p.Enforce.Issuer != "" || p.Enforce.Identity != "" {
		t.Fatalf("expected raw Fields to be ignored when Repo is set, got %+v", p.Enforce)
	}
}

func TestParseConfigItem(t *testing.T) {
	key, value, ok := ParseConfigItem("APT::Architecture=amd64")
	if !ok || key != "APT::Architecture" || value != "amd64" {
		t.Fatalf("got key=%q value=%q ok=%v", key, value, ok)
	}
	if _, _, ok := ParseConfigItem("no-equals-sign"); ok {
		t.Fatal("expected ok=false for missing '='")
	}
}

func TestFromItems_InvalidLogIndex(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Fields::rekorLogIndex":           "not-a-number",
		"Acquire::sigstore::Enforce::Fields::certificate-oidc-issuer": "https://accounts.google.com",
		"Acquire::sigstore::Enforce::Fields::certificate-identity":    "someone@example.com",
	}
	if _, err := FromItems(items); err == nil {
		t.Fatal("expected error for non-numeric rekorLogIndex")
	}
}
