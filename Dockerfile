FROM golang:1.25-bookworm

ENV GOPATH=/go \
    GOCACHE=/go-cache \
    GOFLAGS=-mod=mod \
    CGO_ENABLED=0

# Mode 1777 so an arbitrary (non-root) uid:gid passed in at `docker compose run`
# time can still read/write the module cache, build cache and workdir.
RUN mkdir -p /go /go-cache /app && chmod 1777 /go /go-cache /app

WORKDIR /app
