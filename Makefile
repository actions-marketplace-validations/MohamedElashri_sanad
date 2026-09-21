.PHONY: help build test race lint staticcheck-install staticcheck bench fmt check update docs-release-notes docs-build docs-serve clean

.DEFAULT_GOAL := help

GOCACHE ?= $(CURDIR)/.gocache
GOMODCACHE ?= $(CURDIR)/.gomodcache
GO_TOOL_GOPATH ?= $(CURDIR)/.gopath
XDG_CACHE_HOME ?= $(CURDIR)/.cache
BIN_DIR ?= $(CURDIR)/bin
STATICCHECK ?= $(BIN_DIR)/staticcheck
STATICCHECK_VERSION ?= v0.8.1
GO_TOOLCHAIN_VERSION ?= $(shell go env GOVERSION)
STATICCHECK_STAMP ?= $(BIN_DIR)/.staticcheck-$(STATICCHECK_VERSION)-$(GO_TOOLCHAIN_VERSION)
NIDA ?= nida

help:
	@printf '%s\n' 'Sanad development targets:'
	@printf '  %-20s %s\n' 'make build' 'Build the sanad binary into ./bin'
	@printf '  %-20s %s\n' 'make test' 'Run the test suite'
	@printf '  %-20s %s\n' 'make race' 'Run tests with the race detector'
	@printf '  %-20s %s\n' 'make lint' 'Run go vet'
	@printf '  %-20s %s\n' 'make staticcheck-install' 'Install Staticcheck into ./bin'
	@printf '  %-20s %s\n' 'make staticcheck' 'Run Staticcheck'
	@printf '  %-20s %s\n' 'make bench' 'Run workflow extraction benchmark'
	@printf '  %-20s %s\n' 'make fmt' 'Format Go sources'
	@printf '  %-20s %s\n' 'make check' 'Format, lint, test, and build'
	@printf '  %-20s %s\n' 'make update' 'Update all Go dependencies'
	@printf '  %-20s %s\n' 'make docs-build' 'Generate release notes and build docs'
	@printf '  %-20s %s\n' 'make docs-serve' 'Generate release notes and serve docs'
	@printf '  %-20s %s\n' 'make clean' 'Remove local build, docs, and cache artifacts'
	@printf '  %-20s %s\n' 'make sync-version' 'Sync VERSION, update package manifests, rebuild bundle, and stamp README SHAs'

build:
	mkdir -p $(BIN_DIR)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go build -o $(BIN_DIR)/sanad ./cmd/sanad

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go test ./...

race:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go test -race ./...

lint:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go vet ./...

staticcheck-install: $(STATICCHECK_STAMP)

$(STATICCHECK_STAMP):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GO_TOOL_GOPATH) go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	touch $(STATICCHECK_STAMP)

staticcheck: staticcheck-install
	XDG_CACHE_HOME=$(XDG_CACHE_HOME) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GO_TOOL_GOPATH) $(STATICCHECK) ./...

bench:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go test -run '^$$' -bench BenchmarkExtractUsesFromLargeWorkflow ./internal/workflow

fmt:
	gofmt -w cmd internal

check: fmt lint test build

update:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go get -u ./...
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go mod tidy

docs-release-notes:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go run ./scripts/generate_release_notes.go

docs-build: docs-release-notes
	$(NIDA) build --site ./docs

docs-serve: docs-release-notes
	$(NIDA) serve --site ./docs

sync-version:
	scripts/sync-version
	npm run build --prefix action
	scripts/update-readme

clean:
	$(RM) -r sanad bin dist .cache .gocache .gomodcache .gopath docs/public docs/content/release-notes.md
