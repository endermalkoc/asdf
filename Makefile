BINARY  := cusp
PKG     := ./cmd/cusp

# Version metadata baked into the binary (mirrors what GoReleaser injects).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

GO_BUILD := CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)"

.PHONY: all build install test vet tidy fmt clean snapshot release-check help

all: build ## Build the binary (default)

build: ## Build ./cusp for the host platform
	$(GO_BUILD) -o $(BINARY) $(PKG)

install: ## go install the binary into GOBIN/GOPATH
	CGO_ENABLED=0 go install -ldflags "$(LDFLAGS)" $(PKG)

test: ## Run all tests
	go test ./...

vet: ## Run go vet
	go vet ./...

tidy: ## Prune go.mod/go.sum to what's actually used
	go mod tidy

fmt: ## Format all Go source
	go fmt ./...

clean: ## Remove build output
	rm -rf $(BINARY) dist/

snapshot: ## Build a full cross-platform release locally (no publish) into dist/
	goreleaser release --snapshot --clean

release-check: ## Validate .goreleaser.yaml
	goreleaser check

help: ## List targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
