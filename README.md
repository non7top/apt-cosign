# apt-cosign

A native apt acquire method (`sigstore+https://`) that verifies a Sigstore
bundle — a Rekor transparency-log entry and a Fulcio-issued OIDC certificate
identity — before handing a fetched file back to apt. Includes
`apt-cosign-sign`, the repo-maintainer counterpart that produces those
bundles.

## What gets verified

Every file apt-cosign-method fetches (not just the top-level index —
`InRelease`, `Packages`, and each `.deb` are each checked independently)
must have all of:

- A **valid signature** over its exact bytes.
- A **valid Rekor transparency-log entry** for that signature, checked
  against the live, TUF-fetched public-good Sigstore trusted root — so
  it's publicly, tamper-evidently logged, not just cryptographically
  self-consistent.
- A **certificate identity matching your configured policy**: either an
  exact `certificate-oidc-issuer` + `certificate-identity` pair, or (the
  `Repo` shorthand) a GitHub Actions-issued certificate whose owner/repo
  (pinned, or derived from the request's own URL) and, optionally, workflow
  filename/ref all match.
- Optionally, a **minimum Rekor log index** (`rekorLogIndex`), rejecting
  anything logged before a given point (e.g. to invalidate everything
  signed before a known key/log incident).

What this does *not* do: vouch for what a package actually contains or
does, beyond confirming who published it and that the publication is a
matter of public record. It's a publisher-identity and transparency check,
not a malware scanner.

## Install

Bootstrap problem: verifying a `sigstore+https://` source needs
apt-cosign-method already installed, so the very first download of
apt-cosign itself can't go through apt-cosign-method — there's nothing
installed yet to do the verifying. Grab it from a
[release](../../releases) instead and verify it directly with the
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/) CLI
against the `.sigstore` bundle attached alongside it:

```sh
gh release download --repo non7top/apt-cosign --pattern '*.deb*'

cosign verify-blob \
  --bundle apt-cosign_*_amd64.deb.sigstore \
  --certificate-identity-regexp '^https://github\.com/non7top/apt-cosign/\.github/workflows/release\.yml@.+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  apt-cosign_*_amd64.deb
```

(no `gh`? the release page lists the exact `.deb` and `.deb.sigstore`
filenames to `curl -LO` instead.) Once that prints `Verified OK`, install it
normally:

```sh
sudo dpkg -i apt-cosign_*.deb
```

This installs `/usr/lib/apt/methods/sigstore+https`, `/usr/bin/apt-cosign-sign`,
and an example policy at `/etc/apt/apt.conf.d/99sigstore-policy.example`. From
here on, apt-cosign-method itself can verify future updates through a
`sigstore+https` source (see below) — this manual cosign step is only needed
once, for the first install.

## Configure a client

The method refuses every acquisition until a policy is set. Copy the example
and edit it:

```sh
sudo cp /etc/apt/apt.conf.d/99sigstore-policy.example /etc/apt/apt.conf.d/99sigstore-policy
```

Pick one enforcement style inside — a GitHub Actions workflow identity
(`Enforce::Repo { Owner; Name; Pipeline; }`), or a raw
`Enforce::Fields { certificate-oidc-issuer; certificate-identity; }` pair
matched exactly. In a `Repo` block:

- `Owner`/`Name` can be omitted (both, together) to derive them from each
  request's own URL instead of pinning them — one policy then covers every
  GitHub-hosted source using the same `Pipeline` convention, with no
  per-repo config. This doesn't weaken anything: the URL comes from your
  own `sources.list`, which is already the trust anchor here, and the
  Fulcio certificate's owner/repo is still checked exactly, just against a
  value read from the request instead of duplicated in apt.conf.
- `Pipeline`/`Ref` are independently optional too, wildcarding the workflow
  filename / git ref when left unset — `Owner`+`Name` alone (any workflow
  in that repo) is a complete, valid policy on its own, on top of the
  Rekor-log and signature checks every bundle already gets regardless.

To trust more than one identity at once (e.g. two different repos signed by
two different workflows), use named, URL-prefix-scoped blocks instead of
one flat `Enforce`:

```
Acquire::sigstore::Sources::apt-cosign {
    Match "https://raw.githubusercontent.com/non7top/apt-cosign/";
    Enforce::Repo { Owner "non7top"; Name "apt-cosign"; };
};
Acquire::sigstore::Sources::nginx-modules {
    Match "https://raw.githubusercontent.com/non7top/nginx-modules/";
    Enforce::Repo { Pipeline "build.yml"; };
};
```

`Match` is checked as a prefix against the mapped `https://` target (not
the `sigstore+https://` URI); the first matching `Sources::<name>` wins, in
name order, falling back to the flat `Enforce` block (if any) when nothing
matches.

(Not yet possible: putting this policy directly in the source's own
`.sources`/`sources.list` stanza instead of a separate apt.conf block. Apt's
method IPC protocol only forwards the `Acquire::*` apt.conf tree to a
method via `601 Configuration` — custom per-stanza fields aren't part of
that today. If apt ever starts forwarding them, apt-cosign-method could
read policy straight from the matching stanza instead; tracked in
[#9](https://github.com/non7top/apt-cosign/issues/9).)

Then point a source at the `sigstore+https` scheme instead of `https`, with
`[trusted=yes]` — apt's own GPG check has nothing to check here (there's no
`Release.gpg`, no inline-signed `InRelease`; the sigstore bundle *is* the
trust mechanism), so tell apt to skip it and rely on the method instead:

```
deb [trusted=yes] sigstore+https://raw.githubusercontent.com/non7top/apt-cosign/refs/heads/apt-repo/ ./
```

That's this project's own live demo repo (see below) — every apt-cosign
release republishes it, so you can add this source today and
`apt-get install apt-cosign` through it.

## Sign a repo

`apt-cosign-sign` keylessly signs a file and writes the sidecar bundle
`apt-cosign-method` fetches and verifies, next to it as `FILE.sigstore` — the
same convention as the classic `Release`/`Release.gpg` pairing:

```sh
apt-cosign-sign InRelease
```

Identity is resolved in order: `-id-token` if given, GitHub Actions' ambient
OIDC token (when running in a workflow with `permissions: id-token: write`),
or an interactive OAuth login (`-device` for a headless machine).

## Hosting a repo on raw.githubusercontent.com

apt-cosign hosts itself this way — `debian/build-apt-repo.sh` and
`debian/sign-apt-repo.sh` (wired up as `make apt-repo-stage` /
`make apt-repo-sign`, and as the `publish-apt-repo` job in
[`release.yml`](.github/workflows/release.yml)) are the real, working
version of these steps, not just documentation:

1. Stage a flat repo (no `dists/` hierarchy — just `deb URL/ ./`): a
   `Packages` index from `dpkg-scanpackages`, an `InRelease` with a `SHA256:`
   hash section over `Packages` and no PGP signature, and the `.deb`(s)
   themselves, all in one directory.
2. In a GitHub Actions workflow with `permissions: id-token: write`, run
   `apt-cosign-sign` on **all three** of `InRelease`, `Packages`, and each
   `.deb` — apt-cosign-method verifies every file it's asked to fetch
   independently, not just the top-level index, so each needs its own
   `.sigstore` bundle.
3. Publish that directory as the sole commit of a dedicated branch (a
   `gh-pages`-style force-push, regenerated whole on every release — see
   the `publish-apt-repo` job) so `raw.githubusercontent.com` serves it.
4. Point clients at
   `sigstore+https://raw.githubusercontent.com/OWNER/REPO/refs/heads/BRANCH/InRelease`
   with `[trusted=yes]`.
5. Set the client's `Enforce::Repo` block to that repo's `Owner`/`Name`/
   `Pipeline` (the workflow file that actually runs the signing, e.g.
   `release.yml`), so only your CI's identity is accepted. If a target has
   no sidecar bundle, the method also falls back to GitHub's native
   Artifact Attestations API for GitHub-hosted URLs.

## Building it yourself

Everything builds in a pinned, non-root Docker container — no local Go
install needed:

```sh
make build     # ./bin/sigstore+https, ./bin/apt-cosign-sign
make test
make lint
make package   # ./dist/apt-cosign_*.deb, targets resolute/26.04
make matrix    # verifies the .deb installs cleanly on jammy/22.04, noble/24.04, resolute/26.04
```

CI (`.github/workflows/build.yml`) runs the same steps on every push and
uploads the built `.deb` as a workflow artifact; tagged releases attach it
directly.
