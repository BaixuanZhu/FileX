.PHONY: build clean test test-integration install

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
# Commit date, not build date: the same commit must produce the same binary.
BUILD_TIME ?= $(shell git show -s --format=%cI HEAD 2>/dev/null || echo "unknown")

LDFLAGS := -s -w \
           -X filex/internal/buildinfo.Version=$(VERSION) \
           -X filex/internal/buildinfo.Commit=$(COMMIT) \
           -X filex/internal/buildinfo.BuildTime=$(BUILD_TIME)

BUILD_FLAGS := -trimpath -buildvcs=false

build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o filex.exe ./cmd/filex

clean:
	rm -f filex.exe

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/winenv/...

install:
	CGO_ENABLED=0 go install $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" ./cmd/filex
