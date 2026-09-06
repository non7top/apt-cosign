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

Then point a source at the `sigstore+https` scheme instead of `https`:

```
deb sigstore+https://raw.githubusercontent.com/OWNER/REPO/refs/heads/main/dists/stable ./
```

## Sign a repo

`apt-cosign-sign` keylessly signs a file and writes the sidecar bundle
`apt-cosign-method` fetches and verifies, next to it as `FILE.sigstore` — the
same convention as the classic `Release`/`Release.gpg` pairing:

```sh
apt-cosign-sign dists/stable/InRelease
```

Identity is resolved in order: `-id-token` if given, GitHub Actions' ambient
OIDC token (when running in a workflow with `permissions: id-token: write`),
or an interactive OAuth login (`-device` for a headless machine).

## Hosting a repo on raw.githubusercontent.com

1. Create a (public) GitHub repo holding your apt tree, e.g.
   `dists/stable/{InRelease,Packages,...}`.
2. Build the tree with your usual tooling (`reprepro`, `aptly`, ...), commit,
   and push.
3. In a GitHub Actions workflow with `permissions: id-token: write`, run
   `apt-cosign-sign dists/stable/InRelease` and commit the resulting
   `InRelease.sigstore` alongside it.
4. Point clients at
   `sigstore+https://raw.githubusercontent.com/OWNER/REPO/refs/heads/main/dists/stable/InRelease`.
5. Set the client's `Enforce::Repo` block to that repo's `Owner`/`Name`/
   `Pipeline`, so only your CI's identity is accepted. If a target has no
   sidecar bundle, the method also falls back to GitHub's native Artifact
   Attestations API for GitHub-hosted URLs.

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
