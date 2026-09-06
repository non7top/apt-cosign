export DOCKER_UID := $(shell id -u)
export DOCKER_GID := $(shell id -g)

METHOD_NAME := sigstore+https
SIGN_NAME := apt-cosign-sign

.PHONY: build test lint tidy shell run stop destroy package matrix apt-repo-stage apt-repo-sign

# -buildvcs=false: under act (and other nested-docker CI runners) the build
# runs as a different uid than the one that owns the bind-mounted .git
# directory, which trips git's "dubious ownership" safety check and makes
# Go's VCS-stamping step fail; we don't use the embedded VCS info, so just
# skip it rather than depend on uids lining up.
build:
	docker compose run --rm dev go build -buildvcs=false -o bin/$(METHOD_NAME) ./cmd/apt-cosign-method
	docker compose run --rm dev go build -buildvcs=false -o bin/$(SIGN_NAME) ./cmd/apt-cosign-sign

test:
	docker compose run --rm dev go test -buildvcs=false ./...

lint:
	docker compose run --rm dev go vet ./...

tidy:
	docker compose run --rm dev go mod tidy

shell:
	docker compose run --rm dev bash

# Manually simulate an apt run against the compiled method binary (spec section 6):
# a 601 Configuration block (the method refuses to acquire anything without a
# policy) followed by a 600 URI Acquire against a real, unsigned repo -- so the
# expected, correct outcome is a well-formed 400 URI Failure ("no sidecar
# bundle"), not a crash or a silent bypass.
run: build
	docker compose run --rm dev sh -c '\
		printf "601 Configuration\nConfig-Item: Acquire::sigstore::CacheDir=/tmp/apt-cosign-cache\nConfig-Item: Acquire::sigstore::Enforce::Repo::Owner=debian\nConfig-Item: Acquire::sigstore::Enforce::Repo::Name=debian\nConfig-Item: Acquire::sigstore::Enforce::Repo::Pipeline=release.yml\n\n600 URI Acquire\nURI: sigstore+https://deb.debian.org/debian/dists/stable/InRelease\nFilename: /tmp/TestInRelease\n\n" \
		| ./bin/$(METHOD_NAME)'

package: build
	docker compose run --rm dev ./debian/build.sh

# Run each matrix service with `run --rm` rather than `up
# --abort-on-container-exit`: the latter tears down every other service the
# instant the first one exits, which is racy for independent one-shot
# verification jobs -- we hit this for real once (noble finishing first
# killed jammy mid-install before it could report success). `run --rm` gives
# each service its own real, individually-checked exit code.
matrix: package
	docker compose -f docker-compose.matrix.yml build
	docker compose -f docker-compose.matrix.yml run --rm jammy
	docker compose -f docker-compose.matrix.yml run --rm noble
	docker compose -f docker-compose.matrix.yml run --rm resolute

# Stages a flat apt repo (InRelease/Packages/the .deb) for hosting on
# raw.githubusercontent.com -- see debian/build-apt-repo.sh.
apt-repo-stage: package
	docker compose run --rm repo ./debian/build-apt-repo.sh

# Signs everything apt-repo-stage produced. Needs a real OIDC identity and
# runs natively, not in a container -- see debian/sign-apt-repo.sh.
apt-repo-sign: apt-repo-stage
	./debian/sign-apt-repo.sh

stop:
	docker compose down

destroy:
	docker compose down -v
