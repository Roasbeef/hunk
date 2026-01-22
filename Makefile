PKG := github.com/roasbeef/hunk
TOOLS_DIR := tools
TOOLS_MOD := $(TOOLS_DIR)/go.mod

GOCC ?= go
PREFIX ?= /usr/local

GOTOOL := GOWORK=off $(GOCC) tool -modfile=$(TOOLS_MOD)

GOIMPORTS_PKG := github.com/rinchsan/gosimports/cmd/gosimports
GOLINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint

GO_BIN := ${GOPATH}/bin

COMMIT := $(shell git describe --tags --dirty 2>/dev/null || echo "dev")

GOBUILD := $(GOCC) build -v
GOINSTALL := $(GOCC) install -v
GOTEST := $(GOCC) test

GOFILES_NOVENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*" \
	-not -path "./tools/*")

RM := rm -f
MAKE := make
XARGS := xargs -L 1

# Build flags.
DEV_LDFLAGS := -ldflags "-X $(PKG)/build.Commit=$(COMMIT)"

# Testing flags.
TEST_FLAGS ?=
COVER_FLAGS := -coverprofile=coverage.txt -covermode=atomic -coverpkg=$(PKG)/...

# Linting.
ifneq ($(workers),)
LINT_WORKERS = --concurrency=$(workers)
endif

GREEN := "\\033[0;32m"
NC := "\\033[0m"
define print
	echo $(GREEN)$1$(NC)
endef

.PHONY: default
default: build

.PHONY: all
all: build check

# ============
# INSTALLATION
# ============

#? build: Build hunk binary
.PHONY: build
build:
	@$(call print, "Building hunk.")
	$(GOBUILD) $(DEV_LDFLAGS) -o hunk $(PKG)/cmd/hunk

#? install: Install hunk to $GOPATH/bin
.PHONY: install
install:
	@$(call print, "Installing hunk.")
	$(GOINSTALL) $(DEV_LDFLAGS) $(PKG)/cmd/hunk

# =======
# TESTING
# =======

#? check: Run all checks (lint + test)
.PHONY: check
check: lint unit

#? unit: Run unit tests
.PHONY: unit
unit:
	@$(call print, "Running unit tests.")
	$(GOTEST) $(TEST_FLAGS) ./...

#? unit-cover: Run unit tests with coverage
.PHONY: unit-cover
unit-cover:
	@$(call print, "Running unit tests with coverage.")
	$(GOTEST) $(COVER_FLAGS) $(TEST_FLAGS) ./...

#? unit-race: Run unit tests with race detector
.PHONY: unit-race
unit-race:
	@$(call print, "Running unit tests with race detector.")
	CGO_ENABLED=1 $(GOTEST) -race $(TEST_FLAGS) ./...

#? unit-verbose: Run unit tests with verbose output
.PHONY: unit-verbose
unit-verbose:
	@$(call print, "Running unit tests (verbose).")
	$(GOTEST) -v $(TEST_FLAGS) ./...

# =========
# UTILITIES
# =========

#? fmt: Format source code
.PHONY: fmt
fmt:
	@$(call print, "Formatting source.")
	$(GOTOOL) $(GOIMPORTS_PKG) -w $(GOFILES_NOVENDOR)
	gofmt -l -w -s $(GOFILES_NOVENDOR)

#? fmt-check: Check formatting
.PHONY: fmt-check
fmt-check: fmt
	@$(call print, "Checking fmt results.")
	@if test -n "$$(git status --porcelain)"; then \
		echo "Code not formatted. Run 'make fmt'."; \
		git status; \
		exit 1; \
	fi

#? lint: Run linter
.PHONY: lint
lint:
	@$(call print, "Linting source.")
	$(GOTOOL) $(GOLINT_PKG) run -v $(LINT_WORKERS)

#? tidy: Run go mod tidy
.PHONY: tidy
tidy:
	@$(call print, "Tidying modules.")
	$(GOCC) mod tidy
	cd $(TOOLS_DIR) && $(GOCC) mod tidy

#? tidy-check: Check that go.mod is tidy
.PHONY: tidy-check
tidy-check: tidy
	@$(call print, "Checking mod tidy results.")
	@if test -n "$$(git status --porcelain go.mod go.sum $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum)"; then \
		echo "Modules not tidy. Run 'make tidy'."; \
		git status go.mod go.sum $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum; \
		git diff go.mod go.sum $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum; \
		exit 1; \
	fi

#? clean: Remove build artifacts
.PHONY: clean
clean:
	@$(call print, "Cleaning.")
	$(RM) hunk coverage.txt

#? gen: Generate code (if any)
.PHONY: gen
gen:
	@$(call print, "Generating code.")
	$(GOCC) generate ./...

#? help: Show this help
.PHONY: help
help: Makefile
	@$(call print, "Available targets:")
	@sed -n 's/^#?//p' $< | column -t -s ':' | sort | sed -e 's/^/ /'
