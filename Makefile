export DOCKER_UID := $(shell id -u)
export DOCKER_GID := $(shell id -g)

METHOD_NAME := sigstore+https
SIGN_NAME := apt-cosign-sign

.PHONY: build test lint tidy shell run stop destroy package matrix

build:
	docker compose run --rm dev go build -o bin/$(METHOD_NAME) ./cmd/apt-cosign-method
	docker compose run --rm dev go build -o bin/$(SIGN_NAME) ./cmd/apt-cosign-sign

test:
	docker compose run --rm dev go test ./...

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

matrix: package
	docker compose -f docker-compose.matrix.yml build
	docker compose -f docker-compose.matrix.yml up --abort-on-container-exit

stop:
	docker compose down

destroy:
	docker compose down -v
