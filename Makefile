BINARY := krci
MODULE := github.com/KubeRocketCI/cli

# Build-time variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

GO_BUILD_FLAGS ?= -trimpath
GO_LDFLAGS = -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

# Directories
DIST_DIR := dist
BIN_DIR := bin

# Tools - pinned versions
GOLANGCI_LINT_VERSION ?= v2.11.3
GORELEASER_VERSION ?= v2.10.2

# Cross-platform builds
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build test lint lint-fix clean

build: ## Build the CLI binary
	@mkdir -p $(DIST_DIR)
	go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o $(DIST_DIR)/$(BINARY) ./cmd/krci

build-all: ## Build for all platforms
	@for platform in $(PLATFORMS); do \
		echo "Building for $$platform..."; \
		os=$$(echo $$platform | cut -d'/' -f1); \
		arch=$$(echo $$platform | cut -d'/' -f2); \
		output="$(DIST_DIR)/$(BINARY)-$$os-$$arch"; \
		if [ "$$os" = "windows" ]; then output="$$output.exe"; fi; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o $$output ./cmd/krci; \
	done

test: ## Run tests with race detector and coverage
	go test -race -count=1 -cover -coverprofile=coverage.out ./...

test-coverage: test ## Run tests and show coverage report
	go tool cover -html=coverage.out -o coverage.html

lint: $(BIN_DIR)/golangci-lint ## Run golangci-lint
	$(BIN_DIR)/golangci-lint run

lint-fix: $(BIN_DIR)/golangci-lint ## Run golangci-lint with auto-fix
	$(BIN_DIR)/golangci-lint run --fix

clean: ## Remove build artifacts
	rm -rf $(DIST_DIR) $(BIN_DIR) coverage.out coverage.html

release-snapshot: ## Build snapshot release locally
	goreleaser release --snapshot --clean

release-test: ## Validate GoReleaser configuration
	goreleaser check

ci: lint test build ## Run full CI pipeline locally

# Tool installation
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(BIN_DIR)/golangci-lint: $(BIN_DIR)
	GOBIN=$(PWD)/$(BIN_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(BIN_DIR)/goreleaser: $(BIN_DIR)
	GOBIN=$(PWD)/$(BIN_DIR) go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

tools: $(BIN_DIR)/golangci-lint $(BIN_DIR)/goreleaser ## Install all development tools
