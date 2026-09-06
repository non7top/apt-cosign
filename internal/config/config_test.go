package config

import (
	"net/url"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return u
}

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
	if !p.HasDefault {
		t.Fatal("expected HasDefault to be true")
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
	target := mustURL(t, "https://raw.githubusercontent.com/octo-org/widgets/refs/heads/main/InRelease")
	issuer, sanExact, sanRegexp, err := p.Enforce.Repo.Identity(target)
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
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
	target := mustURL(t, "https://raw.githubusercontent.com/octo-org/widgets/refs/heads/main/InRelease")
	_, sanExact, sanRegexp, err := p.Enforce.Repo.Identity(target)
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	want := "https://github.com/octo-org/widgets/.github/workflows/release.yml@refs/heads/main"
	if sanExact != want {
		t.Fatalf("sanExact = %q, want %q", sanExact, want)
	}
	if sanRegexp != "" {
		t.Fatalf("expected no regexp when everything is pinned, got %q", sanRegexp)
	}
}

func TestFromItems_RepoBlockOwnerNameDerivedFromURL(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Pipeline": "release.yml",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	target := mustURL(t, "https://raw.githubusercontent.com/octo-org/widgets/refs/heads/main/InRelease")
	_, _, sanRegexp, err := p.Enforce.Repo.Identity(target)
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	want := `^https://github\.com/octo-org/widgets/\.github/workflows/release\.yml@.+$`
	if sanRegexp != want {
		t.Fatalf("sanRegexp = %q, want %q", sanRegexp, want)
	}

	// A non-GitHub URL can't derive an owner/repo.
	badTarget := mustURL(t, "https://example.com/dists/stable/InRelease")
	if _, _, _, err := p.Enforce.Repo.Identity(badTarget); err == nil {
		t.Fatal("expected error deriving owner/repo from a non-GitHub URL")
	}
}

func TestFromItems_RepoBlockPipelineWildcarded(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Owner": "octo-org",
		"Acquire::sigstore::Enforce::Repo::Name":  "widgets",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	target := mustURL(t, "https://raw.githubusercontent.com/octo-org/widgets/refs/heads/main/InRelease")
	_, _, sanRegexp, err := p.Enforce.Repo.Identity(target)
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	want := `^https://github\.com/octo-org/widgets/\.github/workflows/.+@.+$`
	if sanRegexp != want {
		t.Fatalf("sanRegexp = %q, want %q", sanRegexp, want)
	}
}

func TestFromItems_RepoBlockIncomplete(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Owner": "octo-org",
	}
	if _, err := FromItems(items); err == nil {
		t.Fatal("expected error for Owner without Name")
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

func TestFromItems_Sources(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Sources::apt-cosign::Match":                "https://raw.githubusercontent.com/non7top/apt-cosign/",
		"Acquire::sigstore::Sources::apt-cosign::Enforce::Repo::Owner": "non7top",
		"Acquire::sigstore::Sources::apt-cosign::Enforce::Repo::Name":  "apt-cosign",

		"Acquire::sigstore::Sources::nginx-modules::Match":                   "https://raw.githubusercontent.com/non7top/nginx-modules/",
		"Acquire::sigstore::Sources::nginx-modules::Enforce::Repo::Owner":    "non7top",
		"Acquire::sigstore::Sources::nginx-modules::Enforce::Repo::Name":     "nginx-modules",
		"Acquire::sigstore::Sources::nginx-modules::Enforce::Repo::Pipeline": "build.yml",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	if len(p.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(p.Sources))
	}
	// Sorted by name: "apt-cosign" < "nginx-modules".
	if p.Sources[0].Name != "apt-cosign" || p.Sources[1].Name != "nginx-modules" {
		t.Fatalf("unexpected source order: %+v", p.Sources)
	}

	enf, source, err := p.PolicyFor("https://raw.githubusercontent.com/non7top/apt-cosign/refs/heads/apt-repo/InRelease")
	if err != nil {
		t.Fatalf("PolicyFor: %v", err)
	}
	if source != "apt-cosign" {
		t.Fatalf("source = %q", source)
	}
	if enf.Repo.Name != "apt-cosign" {
		t.Fatalf("unexpected enforce: %+v", enf)
	}

	enf, source, err = p.PolicyFor("https://raw.githubusercontent.com/non7top/nginx-modules/refs/heads/main/InRelease")
	if err != nil {
		t.Fatalf("PolicyFor: %v", err)
	}
	if source != "nginx-modules" || enf.Repo.Pipeline != "build.yml" {
		t.Fatalf("unexpected enforce for nginx-modules: source=%q enf=%+v", source, enf)
	}

	if _, _, err := p.PolicyFor("https://raw.githubusercontent.com/someone-else/other-repo/refs/heads/main/InRelease"); err == nil {
		t.Fatal("expected error: no source matches and no default is configured")
	}
}

func TestFromItems_SourcesFallBackToDefault(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Enforce::Repo::Owner":                   "default-org",
		"Acquire::sigstore::Enforce::Repo::Name":                    "default-repo",
		"Acquire::sigstore::Sources::special::Match":                "https://raw.githubusercontent.com/special-org/special-repo/",
		"Acquire::sigstore::Sources::special::Enforce::Repo::Owner": "special-org",
		"Acquire::sigstore::Sources::special::Enforce::Repo::Name":  "special-repo",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}

	_, source, err := p.PolicyFor("https://raw.githubusercontent.com/special-org/special-repo/refs/heads/main/InRelease")
	if err != nil || source != "special" {
		t.Fatalf("expected special source to match, got source=%q err=%v", source, err)
	}

	enf, source, err := p.PolicyFor("https://raw.githubusercontent.com/anything/else/refs/heads/main/InRelease")
	if err != nil {
		t.Fatalf("PolicyFor (default fallback): %v", err)
	}
	if source != "default" || enf.Repo.Owner != "default-org" {
		t.Fatalf("expected default fallback, got source=%q enf=%+v", source, enf)
	}
}

func TestFromItems_SourceMissingMatch(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Sources::bad::Enforce::Repo::Owner": "octo-org",
		"Acquire::sigstore::Sources::bad::Enforce::Repo::Name":  "widgets",
	}
	if _, err := FromItems(items); err == nil {
		t.Fatal("expected error for a Sources entry missing Match")
	}
}

func TestFromItems_SourceMissingEnforce(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Sources::bad::Match": "https://raw.githubusercontent.com/octo-org/widgets/",
	}
	if _, err := FromItems(items); err == nil {
		t.Fatal("expected error for a Sources entry with no Enforce policy")
	}
}
