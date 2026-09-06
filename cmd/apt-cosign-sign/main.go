// Command apt-cosign-sign is the repo-maintainer counterpart to
// apt-cosign-method: it keylessly signs a file (typically a repo's
// InRelease) with Sigstore and writes the resulting bundle next to it as
// FILE.sigstore -- the sidecar apt-cosign-method fetches and verifies,
// mirroring the classic Debian convention of publishing Release next to a
// detached Release.gpg signature.
//
// Identity is resolved, in order: an explicit -id-token; GitHub Actions'
// ambient OIDC token when running in a workflow with
// "permissions: id-token: write"; otherwise an interactive OAuth login
// (a browser by default, or -device for a headless machine).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore/pkg/oauthflow"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// defaultOIDCIssuer and defaultOIDCClientID are the Sigstore project's
	// public-good OAuth client for interactive/device-flow logins -- the
	// same ones cosign itself uses; there's no secret here, "sigstore" is a
	// public OAuth client ID with no client secret.
	defaultOIDCIssuer   = "https://oauth2.sigstore.dev/auth"
	defaultOIDCClientID = "sigstore"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "apt-cosign-sign:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		outPath    = flag.String("out", "", "output path for the bundle (default: FILE.sigstore)")
		device     = flag.Bool("device", false, "use the OAuth device flow (print a URL+code to enter on another machine) instead of opening a local browser")
		idTokenArg = flag.String("id-token", "", "use this OIDC identity token verbatim instead of obtaining one")
		noRekor    = flag.Bool("no-rekor", false, "don't record the signature in Rekor (NOT recommended: apt-cosign-method requires a transparency log entry to verify)")
	)
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	target := flag.Arg(0)
	if *outPath == "" {
		*outPath = target + ".sigstore"
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("reading %s: %w", target, err)
	}

	idToken, err := resolveIDToken(*idTokenArg, *device)
	if err != nil {
		return fmt.Errorf("obtaining OIDC identity token: %w", err)
	}

	tufClient, err := tuf.New(tuf.DefaultOptions())
	if err != nil {
		return fmt.Errorf("creating TUF client: %w", err)
	}
	trustedRoot, err := root.GetTrustedRoot(tufClient)
	if err != nil {
		return fmt.Errorf("fetching trusted root: %w", err)
	}
	signingConfig, err := root.GetSigningConfig(tufClient)
	if err != nil {
		return fmt.Errorf("fetching signing config: %w", err)
	}

	fulcioService, err := root.SelectService(signingConfig.FulcioCertificateAuthorityURLs(), sign.FulcioAPIVersions, time.Now())
	if err != nil {
		return fmt.Errorf("selecting Fulcio service: %w", err)
	}

	opts := sign.BundleOptions{
		TrustedRoot:         trustedRoot,
		CertificateProvider: sign.NewFulcio(&sign.FulcioOptions{BaseURL: fulcioService.URL, Timeout: 30 * time.Second, Retries: 1}),
		CertificateProviderOptions: &sign.CertificateProviderOptions{
			IDToken: idToken,
		},
	}

	if !*noRekor {
		rekorServices, err := root.SelectServices(signingConfig.RekorLogURLs(), signingConfig.RekorLogURLsConfig(), sign.RekorAPIVersions, time.Now())
		if err != nil {
			return fmt.Errorf("selecting Rekor service: %w", err)
		}
		for _, s := range rekorServices {
			opts.TransparencyLogs = append(opts.TransparencyLogs, sign.NewRekor(&sign.RekorOptions{
				BaseURL: s.URL,
				Timeout: 90 * time.Second,
				Retries: 1,
				Version: s.MajorAPIVersion,
			}))
		}
	}

	keypair, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		return fmt.Errorf("generating ephemeral keypair: %w", err)
	}

	bundle, err := sign.Bundle(&sign.PlainData{Data: data}, keypair, opts)
	if err != nil {
		return fmt.Errorf("signing: %w", err)
	}

	bundleJSON, err := protojson.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("marshaling bundle: %w", err)
	}
	if err := os.WriteFile(*outPath, bundleJSON, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", *outPath, err)
	}

	fmt.Printf("wrote %s\n", *outPath)
	return nil
}

// resolveIDToken picks an OIDC identity token: an explicit one, GitHub
// Actions' ambient token, or an interactive login.
func resolveIDToken(explicit string, useDevice bool) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	if token, ok, err := githubActionsIDToken(); ok {
		return token, err
	}

	var tokenGetter oauthflow.TokenGetter = oauthflow.DefaultIDTokenGetter
	if useDevice {
		tokenGetter = oauthflow.NewDeviceFlowTokenGetterForIssuer(defaultOIDCIssuer)
	}
	tok, err := oauthflow.OIDConnect(defaultOIDCIssuer, defaultOIDCClientID, "", "", tokenGetter)
	if err != nil {
		return "", fmt.Errorf("interactive OIDC login: %w", err)
	}
	return tok.RawString, nil
}

// githubActionsIDToken requests an OIDC token from GitHub Actions' runtime
// API, available when the workflow has "permissions: id-token: write". ok
// is false if the ambient request env vars aren't present at all (i.e. we're
// not running in such a workflow), in which case the caller should fall
// back to another identity source rather than treat it as an error.
func githubActionsIDToken() (token string, ok bool, err error) {
	reqURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	reqToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if reqURL == "" || reqToken == "" {
		return "", false, nil
	}

	u, err := url.Parse(reqURL)
	if err != nil {
		return "", true, fmt.Errorf("parsing ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", "sigstore")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", true, err
	}
	req.Header.Set("Authorization", "Bearer "+reqToken)
	req.Header.Set("Accept", "application/json; api-version=2.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", true, fmt.Errorf("requesting GitHub Actions OIDC token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", true, fmt.Errorf("GitHub Actions OIDC token endpoint returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", true, fmt.Errorf("decoding GitHub Actions OIDC token response: %w", err)
	}
	return parsed.Value, true, nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [OPTIONS] FILE_TO_SIGN

Signs FILE_TO_SIGN with Sigstore keyless signing and writes the resulting
bundle next to it as FILE_TO_SIGN.sigstore.

Options:
`, os.Args[0])
	flag.PrintDefaults()
}
