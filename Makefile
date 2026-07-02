BINARY  := cusp
# The Go module lives under src/cli/ (the TypeScript extension lives under
# src/extension/). All go commands run with `-C $(CLI)`; build output still
# lands at the repo root via an absolute -o path.
CLI     := src/cli
PKG     := ./cmd/cusp

# Version metadata baked into the binary (mirrors what GoReleaser injects).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

GO_BUILD := CGO_ENABLED=0 go build -C $(CLI) -ldflags "$(LDFLAGS)"

.PHONY: all build install test vet tidy fmt clean snapshot release-check help \
	cover cover-check cover-commands cover-html

all: build ## Build the binary (default)

build: ## Build ./cusp for the host platform
	$(GO_BUILD) -o $(CURDIR)/$(BINARY) $(PKG)

install: ## go install the binary into GOBIN/GOPATH
	CGO_ENABLED=0 go install -C $(CLI) -ldflags "$(LDFLAGS)" $(PKG)

test: ## Run all tests
	go test -C $(CLI) ./...

vet: ## Run go vet
	go vet -C $(CLI) ./...

tidy: ## Prune go.mod/go.sum to what's actually used
	go mod -C $(CLI) tidy

cover: ## Coverage over owned packages + per-package breakdown (needs dolt)
	@scripts/coverage.sh report

cover-check: ## Enforce the coverage ratchet floor — the CI gate (needs dolt)
	@scripts/coverage.sh check

cover-commands: ## Per-command-file coverage: which commands have tests (needs dolt)
	@scripts/coverage.sh commands

cover-html: ## Open the HTML coverage report (needs dolt)
	@scripts/coverage.sh html

fmt: ## Format all Go source
	go fmt -C $(CLI) ./...

clean: ## Remove build output
	rm -rf $(BINARY) dist/

snapshot: ## Build a full cross-platform release locally (no publish) into dist/
	goreleaser release --snapshot --clean

release-check: ## Validate .goreleaser.yaml
	goreleaser check

help: ## List targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
