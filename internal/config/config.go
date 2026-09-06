// Package config turns APT's flattened "Acquire::sigstore::*" configuration
// tree into a policy the verifier can check a Sigstore bundle against.
package config

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"apt-cosign/internal/fetch"
)

const (
	prefix = "Acquire::sigstore::"

	keyRekorServer     = prefix + "RekorServer"
	keyBundleExtension = prefix + "BundleExtension"
	keyGitHubToken     = prefix + "GitHubToken"
	keyCacheDir        = prefix + "CacheDir"

	keyDefaultEnforceBase = prefix + "Enforce"
	keySourcesPrefix      = prefix + "Sources::"

	// GitHub Actions' fixed Fulcio OIDC issuer for workflow identities.
	githubActionsIssuer = "https://token.actions.githubusercontent.com"

	// Defaults.
	DefaultBundleExtension = ".sigstore"
	DefaultCacheDir        = "/var/lib/apt-cosign"
)

// RepoIdentity describes a GitHub Actions workflow identity to match a
// certificate against, in place of a raw certificate-identity string.
type RepoIdentity struct {
	// Owner and Name are optional together: if both are empty, they're
	// derived from the request's own URL (for a github.com/
	// raw.githubusercontent.com target) instead of being pinned in config.
	// This is deliberately safe rather than a shortcut: the URL comes from
	// the operator's own sources.list, which is already the trust anchor
	// here -- an attacker able to point it at a different repo, or edit it
	// at all, already has root and could edit any pinned Owner/Name here
	// just as easily.
	Owner string
	Name  string
	// Pipeline is optional: if empty, any workflow filename in Owner/Name
	// matches (the owner/repo identity check -- pinned or derived from the
	// URL -- plus the existing Rekor transparency-log and signature
	// validity checks are the actual security boundary; Pipeline/Ref just
	// narrow it further for anyone who wants that).
	Pipeline string
	Ref      string // optional; if empty, any ref is accepted (regex match)
}

// Identity resolves the Repo block into the (issuer, exact SAN, SAN regexp)
// triple that verify.NewShortCertificateIdentity expects, deriving
// Owner/Name from targetURL when they weren't pinned in config, and
// wildcarding Pipeline/Ref in the regexp when they weren't pinned either.
// The issuer is always matched exactly. The SAN is matched exactly only
// when Owner, Name, Pipeline and Ref are all pinned; any omitted piece
// falls back to a regexp, since APT config values are attacker-adjacent
// and must not be interpreted as regexp metacharacters themselves.
func (r RepoIdentity) Identity(targetURL *url.URL) (issuer, sanExact, sanRegexp string, err error) {
	owner, name := r.Owner, r.Name
	if owner == "" && name == "" {
		if !fetch.IsGitHubHost(targetURL) {
			return "", "", "", fmt.Errorf("cannot derive an owner/repo from %s (not a github.com or raw.githubusercontent.com URL); set Owner and Name explicitly for this source", targetURL)
		}
		var ok bool
		owner, name, ok = fetch.GitHubOwnerRepo(targetURL)
		if !ok {
			return "", "", "", fmt.Errorf("cannot derive an owner/repo from %s (path too short); set Owner and Name explicitly for this source", targetURL)
		}
	}

	if r.Pipeline != "" && r.Ref != "" {
		san := fmt.Sprintf("https://github.com/%s/%s/.github/workflows/%s@%s", owner, name, r.Pipeline, r.Ref)
		return githubActionsIssuer, san, "", nil
	}

	pipelinePattern := ".+"
	if r.Pipeline != "" {
		pipelinePattern = regexp.QuoteMeta(r.Pipeline)
	}
	refPattern := ".+"
	if r.Ref != "" {
		refPattern = regexp.QuoteMeta(r.Ref)
	}
	san := fmt.Sprintf("^https://github\\.com/%s/%s/\\.github/workflows/%s@%s$",
		regexp.QuoteMeta(owner), regexp.QuoteMeta(name), pipelinePattern, refPattern)
	return githubActionsIssuer, "", san, nil
}

// Enforce holds the identity/transparency-log policy a verified bundle must
// satisfy.
type Enforce struct {
	// MinRekorLogIndex, when > 0, rejects any bundle whose transparency log
	// entry has a lower index (e.g. to invalidate everything signed before a
	// known key/log incident).
	MinRekorLogIndex int64

	// Either Repo is set (GitHub Actions workflow identity), or Issuer/Identity
	// are set directly (matched exactly against the certificate's OIDC issuer
	// and Subject Alternative Name). Repo takes precedence if both are set.
	Repo     *RepoIdentity
	Issuer   string
	Identity string
}

// ScopedEnforce is one named, URL-prefix-scoped policy from an
// Acquire::sigstore::Sources::<name> block.
type ScopedEnforce struct {
	Name    string // the Sources::<name> key, for logging/error messages
	Match   string // URL prefix (matched against the mapped https:// target, not the sigstore+https:// URI)
	Enforce Enforce
}

// Policy is the fully parsed sigstore policy for one apt run.
type Policy struct {
	RekorServer     string
	BundleExtension string
	CacheDir        string
	GitHubToken     string

	// Enforce is the default/fallback policy, from the flat, unscoped
	// Acquire::sigstore::Enforce::* block. HasDefault is false if that
	// block wasn't configured at all (valid as long as at least one
	// Sources entry is).
	Enforce    Enforce
	HasDefault bool

	// Sources are named, URL-prefix-scoped policies from
	// Acquire::sigstore::Sources::<name> blocks, sorted by Name for
	// deterministic dispatch order.
	Sources []ScopedEnforce
}

// PolicyFor returns the Enforce policy that applies to targetURL (the
// mapped https:// target, i.e. after fetch.MapURI): the first (by Name) of
// Sources whose Match is a prefix of targetURL, the default Enforce block
// if none match and one is configured, or -- if neither is configured at
// all -- a policy derived entirely from targetURL itself for a
// github.com/raw.githubusercontent.com target (any workflow, any ref, for
// the owner/repo the URL names). That last case isn't a weaker fallback:
// sources.list already had to explicitly name this URL for the request to
// exist at all, so it's already the operator's own trust anchor, same
// reasoning as leaving Owner/Name unset in a configured Repo block. A
// non-GitHub-hosted target still has nothing to derive an identity from,
// so it still fails closed.
func (p *Policy) PolicyFor(targetURL string) (enf *Enforce, source string, err error) {
	for i := range p.Sources {
		if strings.HasPrefix(targetURL, p.Sources[i].Match) {
			return &p.Sources[i].Enforce, p.Sources[i].Name, nil
		}
	}
	if p.HasDefault {
		return &p.Enforce, "default", nil
	}
	if u, uerr := url.Parse(targetURL); uerr == nil && fetch.IsGitHubHost(u) {
		return &Enforce{Repo: &RepoIdentity{}}, "derived-from-url", nil
	}
	return nil, "", fmt.Errorf("no sigstore policy matches %s (no %s<name>::Match prefix, no default %s::* block configured, and it's not a GitHub-hosted URL an identity could be derived from)", targetURL, keySourcesPrefix, keyDefaultEnforceBase)
}

// ParseConfigItem splits an APT "Config-Item" header value ("Key=Value")
// into its key and value. ok is false if there is no "=".
func ParseConfigItem(item string) (key, value string, ok bool) {
	idx := strings.Index(item, "=")
	if idx < 0 {
		return "", "", false
	}
	return item[:idx], item[idx+1:], true
}

// FromItems builds a Policy from the flattened key/value configuration tree
// APT sends via repeated Config-Item lines in 101 Configuration messages.
func FromItems(items map[string]string) (*Policy, error) {
	p := &Policy{
		RekorServer:     items[keyRekorServer],
		BundleExtension: DefaultBundleExtension,
		CacheDir:        DefaultCacheDir,
		GitHubToken:     items[keyGitHubToken],
	}
	if v, ok := items[keyBundleExtension]; ok && v != "" {
		p.BundleExtension = v
	}
	if v, ok := items[keyCacheDir]; ok && v != "" {
		p.CacheDir = v
	}

	defaultEnforce, hasDefault, err := parseEnforce(items, keyDefaultEnforceBase)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	p.Enforce = defaultEnforce
	p.HasDefault = hasDefault

	sources, err := parseSources(items)
	if err != nil {
		return nil, err
	}
	p.Sources = sources

	// No error when neither is configured: PolicyFor still fails closed
	// per-request for anything that isn't a GitHub-hosted URL it can derive
	// an identity from.
	return p, nil
}

// parseEnforce reads one Enforce block (rekorLogIndex + either Repo or raw
// Fields) out of m, where every relevant key is prefixed by base+"::". has
// is false if no identity policy (Repo, or both Issuer and Identity) was
// configured under base at all.
func parseEnforce(m map[string]string, base string) (Enforce, bool, error) {
	var e Enforce

	if v, ok := m[base+"::Fields::rekorLogIndex"]; ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return Enforce{}, false, fmt.Errorf("%s::Fields::rekorLogIndex: invalid integer %q: %w", base, v, err)
		}
		e.MinRekorLogIndex = n
	}

	owner, name, pipeline, ref := m[base+"::Repo::Owner"], m[base+"::Repo::Name"], m[base+"::Repo::Pipeline"], m[base+"::Repo::Ref"]
	if owner != "" || name != "" || pipeline != "" || ref != "" {
		// Owner/Name must be both set (pinned) or both empty (derived per
		// request from the target URL) -- see RepoIdentity's doc comment.
		// Pipeline and Ref are independently optional (wildcarded when
		// empty), but at least one of the four must be set here, or this
		// isn't a Repo block at all -- it falls through to the raw Fields
		// case below.
		if (owner == "") != (name == "") {
			return Enforce{}, false, fmt.Errorf("%s::Repo::Owner and ::Name must be set together, or both left unset to derive them from each request's URL", base)
		}
		e.Repo = &RepoIdentity{Owner: owner, Name: name, Pipeline: pipeline, Ref: ref}
	} else {
		e.Issuer = m[base+"::Fields::certificate-oidc-issuer"]
		e.Identity = m[base+"::Fields::certificate-identity"]
	}

	has := e.Repo != nil || (e.Issuer != "" && e.Identity != "")
	return e, has, nil
}

// parseSources groups every Acquire::sigstore::Sources::<name>::* key by
// <name> and parses each group into a ScopedEnforce, sorted by name for
// deterministic dispatch order regardless of map iteration order.
func parseSources(items map[string]string) ([]ScopedEnforce, error) {
	groups := map[string]map[string]string{}
	for k, v := range items {
		rest, ok := strings.CutPrefix(k, keySourcesPrefix)
		if !ok {
			continue
		}
		idx := strings.Index(rest, "::")
		if idx < 0 {
			continue // malformed (a bare "Sources::name" with no sub-key); ignore
		}
		name, subkey := rest[:idx], rest[idx+2:]
		if groups[name] == nil {
			groups[name] = map[string]string{}
		}
		groups[name][subkey] = v
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	sources := make([]ScopedEnforce, 0, len(names))
	for _, name := range names {
		sub := groups[name]
		match := sub["Match"]
		if match == "" {
			return nil, fmt.Errorf("config: %s%s::Match must be set", keySourcesPrefix, name)
		}
		enf, has, err := parseEnforce(sub, "Enforce")
		if err != nil {
			return nil, fmt.Errorf("config: %s%s: %w", keySourcesPrefix, name, err)
		}
		if !has {
			return nil, fmt.Errorf("config: %s%s must configure an Enforce policy (Repo or Fields)", keySourcesPrefix, name)
		}
		sources = append(sources, ScopedEnforce{Name: name, Match: match, Enforce: enf})
	}
	return sources, nil
}
