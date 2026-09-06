// Package config turns APT's flattened "Acquire::sigstore::*" configuration
// tree into a policy the verifier can check a Sigstore bundle against.
package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	prefix = "Acquire::sigstore::"

	keyRekorServer     = prefix + "RekorServer"
	keyBundleExtension = prefix + "BundleExtension"
	keyGitHubToken     = prefix + "GitHubToken"
	keyCacheDir        = prefix + "CacheDir"

	keyEnforceLogIndex = prefix + "Enforce::Fields::rekorLogIndex"
	keyEnforceIssuer   = prefix + "Enforce::Fields::certificate-oidc-issuer"
	keyEnforceIdentity = prefix + "Enforce::Fields::certificate-identity"

	keyRepoOwner    = prefix + "Enforce::Repo::Owner"
	keyRepoName     = prefix + "Enforce::Repo::Name"
	keyRepoPipeline = prefix + "Enforce::Repo::Pipeline"
	keyRepoRef      = prefix + "Enforce::Repo::Ref"

	// GitHub Actions' fixed Fulcio OIDC issuer for workflow identities.
	githubActionsIssuer = "https://token.actions.githubusercontent.com"

	// Defaults.
	DefaultBundleExtension = ".sigstore"
	DefaultCacheDir        = "/var/lib/apt-cosign"
)

// RepoIdentity describes a GitHub Actions workflow identity to match a
// certificate against, in place of a raw certificate-identity string.
type RepoIdentity struct {
	Owner    string
	Name     string
	Pipeline string // workflow file, e.g. "release.yml"
	Ref      string // optional; if empty, any ref is accepted (regex match)
}

// Identity resolves the Repo block into the (issuer, exact SAN, SAN regexp)
// triple that verify.NewShortCertificateIdentity expects. The issuer is
// always matched exactly. The SAN is matched exactly when Ref is pinned;
// otherwise a regexp anchored to Owner/Name/Pipeline (but any ref) is used,
// since APT config values are attacker-adjacent and must not be interpreted
// as regexp metacharacters themselves.
func (r RepoIdentity) Identity() (issuer, sanExact, sanRegexp string) {
	base := fmt.Sprintf("https://github.com/%s/%s/.github/workflows/%s", r.Owner, r.Name, r.Pipeline)
	if r.Ref != "" {
		return githubActionsIssuer, base + "@" + r.Ref, ""
	}
	return githubActionsIssuer, "", "^" + regexp.QuoteMeta(base) + "@.+$"
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

// Policy is the fully parsed sigstore policy for one apt run.
type Policy struct {
	RekorServer     string
	BundleExtension string
	CacheDir        string
	GitHubToken     string
	Enforce         Enforce
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

	if v, ok := items[keyEnforceLogIndex]; ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("config: %s: invalid integer %q: %w", keyEnforceLogIndex, v, err)
		}
		p.Enforce.MinRekorLogIndex = n
	}

	owner, name, pipeline := items[keyRepoOwner], items[keyRepoName], items[keyRepoPipeline]
	if owner != "" || name != "" || pipeline != "" {
		if owner == "" || name == "" || pipeline == "" {
			return nil, fmt.Errorf("config: %s, %s and %s must all be set together", keyRepoOwner, keyRepoName, keyRepoPipeline)
		}
		p.Enforce.Repo = &RepoIdentity{
			Owner:    owner,
			Name:     name,
			Pipeline: pipeline,
			Ref:      items[keyRepoRef],
		}
	} else {
		p.Enforce.Issuer = items[keyEnforceIssuer]
		p.Enforce.Identity = items[keyEnforceIdentity]
	}

	if p.Enforce.Repo == nil && (p.Enforce.Issuer == "" || p.Enforce.Identity == "") {
		return nil, fmt.Errorf("config: must set either %s+%s+%s or both %s and %s",
			keyRepoOwner, keyRepoName, keyRepoPipeline, keyEnforceIssuer, keyEnforceIdentity)
	}

	return p, nil
}
