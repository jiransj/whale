SHELL := /bin/sh

BIN ?= bin/whale
GOCACHE_DIR ?= $(CURDIR)/.gocache
VERSION ?= dev
LDFLAGS := -X github.com/usewhale/whale/internal/build.Version=$(VERSION)

.PHONY: help build fmt-check vet test test-tui test-evals run clean

help:
	@echo "Targets:"
	@echo "  make build       Build $(BIN)"
	@echo "  make fmt-check   Check Go formatting with gofmt"
	@echo "  make vet         Run go vet"
	@echo "  make test        Run all offline Go tests"
	@echo "  make test-evals  Run the eval-focused subset"
	@echo "  make test-tui    Run the TUI-focused subset"
	@echo "  make run         Build and run the TUI"
	@echo "  make clean       Remove build output and local Go cache"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=v0.1.0   Inject version into the binary"
	@echo "  BIN=path         Override output binary path"

build:
	@mkdir -p "$(dir $(BIN))" "$(GOCACHE_DIR)"
	GOCACHE="$(GOCACHE_DIR)" go build -ldflags "$(LDFLAGS)" -o "$(BIN)" ./cmd/whale

fmt-check:
	@files="$$(find . -name '*.go' -not -path './.gocache/*' | sort)"; \
	out="$$(gofmt -l $$files)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt needs to be run on:"; \
		echo "$$out"; \
		exit 1; \
	fi

vet:
	@mkdir -p "$(GOCACHE_DIR)"
	GOCACHE="$(GOCACHE_DIR)" go vet ./...

test:
	@mkdir -p "$(GOCACHE_DIR)"
	GOCACHE="$(GOCACHE_DIR)" go test ./...

test-evals:
	@mkdir -p "$(GOCACHE_DIR)"
	GOCACHE="$(GOCACHE_DIR)" go test ./internal/evals

test-tui:
	@mkdir -p "$(GOCACHE_DIR)"
	GOCACHE="$(GOCACHE_DIR)" go test ./internal/tui ./internal/tui/render

run: build
	"$(BIN)"

clean:
	rm -rf bin .gocache
