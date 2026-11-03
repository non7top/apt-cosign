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
	// A totally empty configuration is now valid at parse time -- it's
	// PolicyFor, not FromItems, that decides per-request whether a
	// GitHub-hosted URL can fall back to a derived identity.
	p, err := FromItems(map[string]string{})
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}
	if p.HasDefault || len(p.Sources) != 0 {
		t.Fatalf("expected an empty policy, got %+v", p)
	}

	if _, _, err := p.PolicyFor("https://example.com/dists/stable/InRelease"); err == nil {
		t.Fatal("expected error: nothing configured and not a GitHub-hosted URL")
	}
	enf, source, err := p.PolicyFor("https://raw.githubusercontent.com/octo-org/widgets/refs/heads/main/InRelease")
	if err != nil {
		t.Fatalf("PolicyFor (derived fallback): %v", err)
	}
	if source != "derived-from-url" || enf.Repo == nil {
		t.Fatalf("expected a derived Repo policy, got source=%q enf=%+v", source, enf)
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
	// Sorted descending by match length: len(".../nginx-modules/") > len(".../apt-cosign/").
	if p.Sources[0].Name != "nginx-modules" || p.Sources[1].Name != "apt-cosign" {
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

	// No source matches, no default is configured, but it's a GitHub-hosted
	// URL: falls back to deriving owner/repo from it rather than failing.
	enf, source, err = p.PolicyFor("https://raw.githubusercontent.com/someone-else/other-repo/refs/heads/main/InRelease")
	if err != nil {
		t.Fatalf("PolicyFor (derived fallback): %v", err)
	}
	if source != "derived-from-url" || enf.Repo == nil || enf.Repo.Owner != "" {
		t.Fatalf("expected an unpinned Repo policy for the derived fallback, got source=%q enf=%+v", source, enf)
	}

	// A non-GitHub-hosted URL has nothing to derive an identity from, so it
	// still fails closed.
	if _, _, err := p.PolicyFor("https://example.com/dists/stable/InRelease"); err == nil {
		t.Fatal("expected error: no source matches, no default, and not a GitHub-hosted URL")
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

func TestFromItems_SourceInvalidMatchURL(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Sources::bad::Match":                "not-a-url",
		"Acquire::sigstore::Sources::bad::Enforce::Repo::Owner": "octo-org",
		"Acquire::sigstore::Sources::bad::Enforce::Repo::Name":  "widgets",
	}
	if _, err := FromItems(items); err == nil {
		t.Fatal("expected error for non-URL Match")
	}
}

func TestMatchURLPrefix(t *testing.T) {
	tests := []struct {
		target string
		prefix string
		want   bool
	}{
		{"https://example.com/repo", "https://example.com/repo", true},
		{"https://example.com/repo/", "https://example.com/repo", true},
		{"https://example.com/repo", "https://example.com/repo/", true},
		{"https://example.com/repo/dists/Release", "https://example.com/repo", true},
		{"https://example.com/repo/dists/Release", "https://example.com/repo/", true},
		{"https://example.com/repo-evil/dists/Release", "https://example.com/repo", false},
		{"https://example.com/repo-evil/dists/Release", "https://example.com/repo/", false},
		{"https://example.com/repostuff", "https://example.com/repo", false},
		{"https://example.com", "https://example.com", true},
		{"https://example.com/", "https://example.com", true},
		{"https://example.com", "https://example.com/", true},
		{"https://example.com/dists/Release", "https://example.com", true},
		{"https://example.com.attacker.org/Release", "https://example.com", false},
		{"https://example.com:8080/Release", "https://example.com", false},
		{"https://example.com:8080/Release", "https://example.com:8080", true},
		{"https://example.com/repo?param=val", "https://example.com/repo", true},
	}
	for _, tt := range tests {
		got := matchURLPrefix(tt.target, tt.prefix)
		if got != tt.want {
			t.Errorf("matchURLPrefix(%q, %q) = %v, want %v", tt.target, tt.prefix, got, tt.want)
		}
	}
}

func TestFromItems_Sources_BoundaryEnforcement(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Sources::repo::Match":                "https://example.com/repo",
		"Acquire::sigstore::Sources::repo::Enforce::Repo::Owner": "example",
		"Acquire::sigstore::Sources::repo::Enforce::Repo::Name":  "repo",

		"Acquire::sigstore::Sources::domain::Match":                "https://trusted.org",
		"Acquire::sigstore::Sources::domain::Enforce::Repo::Owner": "example",
		"Acquire::sigstore::Sources::domain::Enforce::Repo::Name":  "domain",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}

	// Legitimate child paths match.
	if _, source, err := p.PolicyFor("https://example.com/repo/dists/Release"); err != nil || source != "repo" {
		t.Fatalf("expected match for repo child, got source=%q err=%v", source, err)
	}
	if _, source, err := p.PolicyFor("https://trusted.org/pool/main/p.deb"); err != nil || source != "domain" {
		t.Fatalf("expected match for domain child, got source=%q err=%v", source, err)
	}

	// Sibling paths and sibling domains do not match.
	if _, _, err := p.PolicyFor("https://example.com/repo-malicious/dists/Release"); err == nil {
		t.Fatal("expected no policy match for sibling path repo-malicious")
	}
	if _, _, err := p.PolicyFor("https://trusted.org.attacker.com/Release"); err == nil {
		t.Fatal("expected no policy match for sibling domain trusted.org.attacker.com")
	}
}

func TestFromItems_Sources_LongestPrefixOrder(t *testing.T) {
	items := map[string]string{
		"Acquire::sigstore::Sources::broad::Match":                "https://example.com/",
		"Acquire::sigstore::Sources::broad::Enforce::Repo::Owner": "example",
		"Acquire::sigstore::Sources::broad::Enforce::Repo::Name":  "broad",

		"Acquire::sigstore::Sources::strict::Match":                "https://example.com/repo/strict/",
		"Acquire::sigstore::Sources::strict::Enforce::Repo::Owner": "example",
		"Acquire::sigstore::Sources::strict::Enforce::Repo::Name":  "strict",

		// Same length as broad to test deterministic tie-breaking by name: "alpha" vs "broad"
		"Acquire::sigstore::Sources::alpha::Match":                "https://example.org/",
		"Acquire::sigstore::Sources::alpha::Enforce::Repo::Owner": "example",
		"Acquire::sigstore::Sources::alpha::Enforce::Repo::Name":  "alpha",
	}
	p, err := FromItems(items)
	if err != nil {
		t.Fatalf("FromItems: %v", err)
	}

	if len(p.Sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(p.Sources))
	}
	// "strict" has length 32; "alpha" and "broad" both have length 20, broken alphabetically.
	if p.Sources[0].Name != "strict" {
		t.Fatalf("expected p.Sources[0] to be strict, got %s", p.Sources[0].Name)
	}
	if p.Sources[1].Name != "alpha" {
		t.Fatalf("expected p.Sources[1] to be alpha, got %s", p.Sources[1].Name)
	}
	if p.Sources[2].Name != "broad" {
		t.Fatalf("expected p.Sources[2] to be broad, got %s", p.Sources[2].Name)
	}

	// Stricter prefix takes precedence over broader prefix even though "broad" < "strict" alphabetically.
	_, source, err := p.PolicyFor("https://example.com/repo/strict/InRelease")
	if err != nil {
		t.Fatalf("PolicyFor: %v", err)
	}
	if source != "strict" {
		t.Fatalf("expected source strict, got %q", source)
	}

	// URL outside strict falls back to broad.
	_, source, err = p.PolicyFor("https://example.com/repo/other/InRelease")
	if err != nil {
		t.Fatalf("PolicyFor: %v", err)
	}
	if source != "broad" {
		t.Fatalf("expected source broad, got %q", source)
	}
}
