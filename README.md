# apt-cosign

A native apt acquire method (`sigstore+https://`) that verifies a Sigstore
bundle — a Rekor transparency-log entry and a Fulcio-issued OIDC certificate
identity — before handing a fetched file back to apt. Includes
`apt-cosign-sign`, the repo-maintainer counterpart that produces those
bundles.

## Install

Grab the `.deb` from a [release](../../releases) (or build it yourself: see
below) and install it:

```sh
sudo dpkg -i apt-cosign_*.deb
```

This installs `/usr/lib/apt/methods/sigstore+https`, `/usr/bin/apt-cosign-sign`,
and an example policy at `/etc/apt/apt.conf.d/99sigstore-policy.example`.

## Configure a client

The method refuses every acquisition until a policy is set. Copy the example
and edit it:

```sh
sudo cp /etc/apt/apt.conf.d/99sigstore-policy.example /etc/apt/apt.conf.d/99sigstore-policy
```

Pick one enforcement style inside — a GitHub Actions workflow identity
(`Enforce::Repo { Owner; Name; Pipeline; }`), or a raw
`Enforce::Fields { certificate-oidc-issuer; certificate-identity; }` pair
matched exactly.

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
