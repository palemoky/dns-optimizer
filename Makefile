APP_NAME := dnspick
BUILD_DIR := builds

# Version info (injected into internal/buildinfo at build time)
PKG     := github.com/palemoky/dnspick/internal/buildinfo
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG).Version=$(VERSION) \
	-X $(PKG).Commit=$(COMMIT) \
	-X $(PKG).Date=$(DATE)
GO_BUILD_ENV := CGO_ENABLED=0

# Cross-compilation target platforms: GOOS/GOARCH
PLATFORMS := \
	linux/amd64 linux/arm64 \
	windows/amd64 windows/arm64 \
	darwin/amd64 darwin/arm64

# Colored output
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
CYAN := \033[0;36m
NC := \033[0m # No Color

.PHONY: all build build-all clean release help

## Default: local build
all: build

## Show available commands
help:
	@awk 'BEGIN{FS=":"} /^## /{desc=substr($$0,4); next} /^[a-zA-Z_-]+:/{if(desc){printf "  $(CYAN)%-12s$(NC) %s\n", $$1, desc; desc=""}}' $(MAKEFILE_LIST)

## Build the binary for the host platform
build:
	$(GO_BUILD_ENV) go build -trimpath -ldflags="$(LDFLAGS)" -o $(APP_NAME) .

## Cross-compile all platforms into builds/
build-all: clean
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		out="$(BUILD_DIR)/$(APP_NAME)-$$os-$$arch"; \
		if [ "$$os" = "windows" ]; then out="$$out.exe"; fi; \
		echo "--> building $$os/$$arch"; \
		$(GO_BUILD_ENV) GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags="$(LDFLAGS)" -o "$$out" . || exit 1; \
	done
	@echo "All builds completed in ./$(BUILD_DIR)"

## Clean build artifacts
clean:
	rm -rf $(BUILD_DIR) $(APP_NAME)

## Release a new version
release:  ## Create and push version tag
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "$(RED)Error: Working directory has uncommitted changes$(NC)"; \
		echo "$(YELLOW)Please commit or stash your changes before releasing$(NC)"; \
		exit 1; \
	fi; \
	LATEST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo "none"); \
	echo "$(BLUE)════════════════════════════════════════$(NC)"; \
	echo "$(BLUE)         Release New Version$(NC)"; \
	echo "$(BLUE)════════════════════════════════════════$(NC)"; \
	echo "$(CYAN)Current latest tag: $(GREEN)$$LATEST_TAG$(NC)"; \
	echo "$(BLUE)════════════════════════════════════════$(NC)"; \
	printf "$(YELLOW)Enter new version: $(NC)"; \
	read -r VERSION; \
	if [ -z "$$VERSION" ]; then \
		echo "$(RED)Error: Version cannot be empty$(NC)"; \
		exit 1; \
	fi; \
	if ! echo "$$VERSION" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "$(RED)Error: Invalid version format '$$VERSION'$(NC)"; \
		echo "$(YELLOW)Expected format: v1.0.0$(NC)"; \
		exit 1; \
	fi; \
	if git tag | grep -q "^$$VERSION$$"; then \
		echo "$(RED)Error: Tag $$VERSION already exists$(NC)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "$(YELLOW)About to create and push tag: $(GREEN)$$VERSION$(NC)"; \
	printf "$(YELLOW)Continue? [y/N] $(NC)"; \
	read -r CONFIRM; \
	if [ "$$CONFIRM" != "y" ] && [ "$$CONFIRM" != "Y" ]; then \
		echo "$(YELLOW)Aborted$(NC)"; \
		exit 1; \
	fi; \
	if [ "$$LATEST_TAG" != "none" ]; then \
		SMALLEST=$$(printf '%s\n%s\n' "$$LATEST_TAG" "$$VERSION" | sort -V | head -n1); \
		if [ "$$SMALLEST" = "$$VERSION" ]; then \
			echo "$(RED)Error: New version $$VERSION must be greater than $$LATEST_TAG$(NC)"; \
			exit 1; \
		fi; \
	fi; \
	if git config user.signingkey >/dev/null 2>&1 && command -v gpg >/dev/null 2>&1; then \
		echo "$(BLUE)Creating GPG signed tag $$VERSION...$(NC)"; \
		if git tag -s $$VERSION -m "Release $$VERSION" 2>/dev/null; then \
			echo "$(GREEN)✓ Signed tag $$VERSION created (Verified ✓)$(NC)"; \
		else \
			echo "$(YELLOW)⚠ GPG signing failed, using regular tag...$(NC)"; \
			git tag -a $$VERSION -m "Release $$VERSION"; \
			echo "$(GREEN)✓ Tag $$VERSION created$(NC)"; \
		fi \
	else \
		echo "$(BLUE)Creating tag $$VERSION...$(NC)"; \
		git tag -a $$VERSION -m "Release $$VERSION"; \
		echo "$(GREEN)✓ Tag $$VERSION created$(NC)"; \
		echo "$(YELLOW)💡 Tip: Configure GPG key to show Verified badge$(NC)"; \
	fi; \
	echo "$(BLUE)Pushing tag to remote...$(NC)"; \
	git push origin $$VERSION; \
	echo "$(GREEN)✓ Release $$VERSION completed$(NC)"
