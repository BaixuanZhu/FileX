.PHONY: build clean test

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X filex/internal/buildinfo.Version=$(VERSION) \
           -X filex/internal/buildinfo.Commit=$(COMMIT) \
           -X filex/internal/buildinfo.BuildTime=$(BUILD_TIME)

build:
	go build -ldflags "$(LDFLAGS)" -o filex.exe ./cmd/filex

clean:
	rm -f filex.exe

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/winenv/...

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/filex
