SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

# CGO build tags (override from env if needed)
GO_TAGS ?=
GO ?= go
# Override GO when install-go just installed it (see install-go target)
-include .go-path.inc
BIN_DIR := bin
HOME ?= $(shell echo $$HOME)
SCRIPT_DIR := scripts

# Targets that require HOME to be set (install/update/uninstall)
_require_home = @if [ -z "$$HOME" ]; then echo "ERROR: HOME is not set"; exit 1; fi

.PHONY: run build build-cli build-daemon build-all test test-coverage lint lint-strict install-hooks clean install install-full install-go install-ollama update uninstall restart-daemon stop-daemon ollama-ping help
.DEFAULT_GOAL := help

# Run the example brain demo
run:
	$(GO) run $(GO_TAGS) ./cmd/example/

# Build the example binary
build: | $(BIN_DIR)
	$(GO) build $(GO_TAGS) -o $(BIN_DIR)/brain-example ./cmd/example/

# Build the CLI tool
build-cli: | $(BIN_DIR)
	$(GO) build $(GO_TAGS) -o $(BIN_DIR)/brain ./cmd/brain/

# Build the daemon
build-daemon: | $(BIN_DIR)
	$(GO) build $(GO_TAGS) -o $(BIN_DIR)/brain-daemon ./cmd/daemon/

# Order-only: ensure output directory exists (no timestamp dep)
$(BIN_DIR):
	mkdir -p $(BIN_DIR)

# Build both CLI and daemon
build-all: build-cli build-daemon

# Install Go if not present (required to build CLI and daemon)
install-go:
	@if command -v go >/dev/null 2>&1 && go version >/dev/null 2>&1; then \
		echo "Go already installed ($$(go version))"; \
		rm -f .go-path.inc; \
	elif [ "$$(uname -s)" = "Darwin" ]; then \
		if command -v brew >/dev/null 2>&1; then \
			echo "Installing Go via Homebrew..."; \
			brew install go; \
			echo "GO := $$(brew --prefix)/bin/go" > .go-path.inc; \
			echo "Go installed. Build will use $$(brew --prefix)/bin/go"; \
		else \
			echo ""; \
			echo "ERROR: Go not found. Install it with:"; \
			echo "  Install Homebrew (https://brew.sh) then run: brew install go"; \
			echo "  Or download from: https://go.dev/dl/"; \
			echo ""; exit 1; \
		fi; \
	elif [ "$$(uname -s)" = "Linux" ]; then \
		echo "Installing Go..."; \
		if command -v apt-get >/dev/null 2>&1; then \
			sudo apt-get update && sudo apt-get install -y golang-go; \
		elif command -v dnf >/dev/null 2>&1; then \
			sudo dnf install -y golang; \
		elif command -v pacman >/dev/null 2>&1; then \
			sudo pacman -S --noconfirm go; \
		else \
			echo ""; \
			echo "ERROR: Go not found. Install it with your package manager or from: https://go.dev/dl/"; \
			echo ""; exit 1; \
		fi; \
		rm -f .go-path.inc; \
		echo "Go installed."; \
	else \
		echo ""; \
		echo "ERROR: Go not found. Install from: https://go.dev/dl/"; \
		echo ""; exit 1; \
	fi

# Install Ollama if not present (required for brain LLM operations)
install-ollama:
	@if command -v ollama >/dev/null 2>&1; then \
		echo "Ollama already installed ($$(ollama --version 2>/dev/null || echo 'found in PATH'))"; \
	elif [ "$$(uname -s)" = "Darwin" ]; then \
		if command -v brew >/dev/null 2>&1; then \
			echo "Installing Ollama via Homebrew..."; \
			brew install ollama; \
			echo "Ollama installed. Start it with: ollama serve (or run the Ollama app)"; \
		else \
			echo ""; \
			echo "WARNING: Ollama not found. Brain needs an LLM (e.g. Ollama) for consolidation and profiles."; \
			echo "Install Homebrew (https://brew.sh) then run: brew install ollama"; \
			echo "Or download Ollama from: https://ollama.com/download"; \
			echo ""; \
		fi; \
	elif [ "$$(uname -s)" = "Linux" ]; then \
		echo "Installing Ollama..."; \
		curl -fsSL -o /tmp/ollama_install.sh https://ollama.com/install.sh && sh /tmp/ollama_install.sh; \
		rm -f /tmp/ollama_install.sh; \
		echo "Ollama installed. Start it with: ollama serve"; \
	else \
		echo ""; \
		echo "WARNING: Ollama not found. Brain needs an LLM for consolidation and profiles."; \
		echo "Install from: https://ollama.com/download"; \
		echo ""; \
	fi

# Install or update: if brain is already in ~/bin, run update; otherwise full install.
install:
	$(_require_home); \
	if [ -f $(HOME)/bin/brain ]; then \
		$(MAKE) update; \
	else \
		$(MAKE) install-full; \
	fi

# Build and install the CLI globally, initialize a default brain, and install AI skills
# install-go and install-ollama run first; recursive make ensures build sees newly installed Go
install-full: install-go install-ollama
	$(MAKE) build-cli build-daemon
	$(_require_home); \
	test -f $(BIN_DIR)/brain || { echo "ERROR: $(BIN_DIR)/brain missing (build failed?)"; exit 1; }; \
	test -f $(BIN_DIR)/brain-daemon || { echo "ERROR: $(BIN_DIR)/brain-daemon missing (build failed?)"; exit 1; }; \
	mkdir -p $(HOME)/bin; \
	cp $(BIN_DIR)/brain $(HOME)/bin/brain; \
	cp $(BIN_DIR)/brain-daemon $(HOME)/bin/brain-daemon; \
	if [ "$$(uname -s)" = "Darwin" ]; then \
		codesign --force -s - $(HOME)/bin/brain $(HOME)/bin/brain-daemon; \
	fi; \
	echo "Installed brain to ~/bin/brain"; \
	if ! echo "$$PATH" | grep -q "$(HOME)/bin"; then \
		echo ""; echo "WARNING: ~/bin is not on your PATH. Add it with:"; \
		echo "  echo 'export PATH=\"\$$HOME/bin:\$$PATH\"' >> ~/.zshrc && source ~/.zshrc"; echo ""; \
	fi; \
	mkdir -p $(HOME)/.brain; \
	if [ ! -f $(HOME)/.brain/agent.brain ]; then \
		$(HOME)/bin/brain init; \
		echo "Created default brain at ~/.brain/agent.brain"; \
	else \
		echo "Default brain already exists at ~/.brain/agent.brain"; \
	fi
	@test -x $(SCRIPT_DIR)/install-integrations.sh || { echo "ERROR: $(SCRIPT_DIR)/install-integrations.sh not found or not executable"; exit 1; }
	$(SCRIPT_DIR)/install-integrations.sh install
	$(HOME)/bin/brain install-service
	@echo ""
	@which brain >/dev/null 2>&1 || { echo "ERROR: brain is not on your PATH (which brain failed). Add ~/bin to PATH (see above), open a new terminal or run: source ~/.zshrc"; echo "Then run 'which brain' to verify."; exit 1; }
	@echo "Verified: brain is installed globally ($$(which brain))."
	@echo "Done. brain is ready to use in any project."

# Update the global brain binaries and skill files (assumes install was run once).
# Reinstalls AI skills for every agent currently present; if you install a new agent
# (e.g. Gemini CLI), running make update will add brain support for it automatically.
update: build-cli build-daemon
	$(_require_home); \
	test -f $(BIN_DIR)/brain || { echo "ERROR: $(BIN_DIR)/brain missing (build failed?)"; exit 1; }; \
	test -f $(BIN_DIR)/brain-daemon || { echo "ERROR: $(BIN_DIR)/brain-daemon missing (build failed?)"; exit 1; }; \
	mkdir -p $(HOME)/bin; \
	cp $(BIN_DIR)/brain $(HOME)/bin/brain; \
	cp $(BIN_DIR)/brain-daemon $(HOME)/bin/brain-daemon; \
	if [ "$$(uname -s)" = "Darwin" ]; then \
		codesign --force -s - $(HOME)/bin/brain $(HOME)/bin/brain-daemon 2>/dev/null || true; \
	fi; \
	echo "Updated ~/bin/brain and ~/bin/brain-daemon"
	@which brain >/dev/null 2>&1 || { echo "ERROR: brain is not on your PATH (which brain failed). Add ~/bin to PATH and open a new terminal."; exit 1; }
	@echo "Verified: brain is installed globally ($$(which brain))."
	@test -x $(SCRIPT_DIR)/install-integrations.sh || { echo "ERROR: $(SCRIPT_DIR)/install-integrations.sh not found or not executable"; exit 1; }
	$(SCRIPT_DIR)/install-integrations.sh update

# Uninstall the background daemon service, remove brain data, CLI, and AI skills
uninstall:
	$(_require_home)
	@echo "WARNING: This will permanently delete ~/bin/brain, ~/bin/brain-daemon, and all data in ~/.brain."
	@echo "Press Ctrl-C within 5 seconds to abort..."
	@sleep 5
	-$(HOME)/bin/brain uninstall-service 2>/dev/null || true
	rm -f $(HOME)/bin/brain $(HOME)/bin/brain-daemon
	rm -rf $(HOME)/.brain
	@test -x $(SCRIPT_DIR)/install-integrations.sh && $(SCRIPT_DIR)/install-integrations.sh uninstall || true
	@echo "Removed ~/bin/brain, ~/bin/brain-daemon, ~/.brain, and AI skills"

# Lint the codebase (automatically fixes formatting when possible)
lint:
	@echo "Running go fmt (auto-fixing formatting)..."
	@FMT_OUTPUT=$$($(GO) fmt ./... 2>&1); \
	if [ -n "$$FMT_OUTPUT" ]; then \
		echo "Fixed formatting in:"; \
		echo "$$FMT_OUTPUT"; \
	else \
		echo "Code is already properly formatted."; \
	fi
	@echo "Running go vet..."
	@$(GO) vet $(GO_TAGS) ./...
	@echo "Checking for staticcheck..."
	@if command -v staticcheck >/dev/null 2>&1; then \
		echo "Running staticcheck..."; \
		staticcheck $(GO_TAGS) ./...; \
	else \
		echo "staticcheck not found. Install with: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		echo "Skipping staticcheck..."; \
	fi
	@echo "Linting complete!"

# Strict lint via golangci-lint (used by pre-commit). Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
lint-strict:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(GO_TAGS) ./...; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

# Install git pre-commit hook (lint-strict, go vet, tests).
# Requires golangci-lint on PATH: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
install-hooks:
	@test -f $(SCRIPT_DIR)/pre-commit || { echo "ERROR: $(SCRIPT_DIR)/pre-commit not found"; exit 1; }
	@mkdir -p .git/hooks
	@cp $(SCRIPT_DIR)/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Installed pre-commit hook to .git/hooks/pre-commit"

# Ping Ollama with a simple "hi" and print response time (use before test to verify Ollama is responsive).
ollama-ping:
	@url="$${BRAIN_TEST_OLLAMA_URL:-http://localhost:11434}"; \
	model="$${BRAIN_TEST_OLLAMA_MODEL:-qwen2.5:1.5b}"; \
	echo "Pinging Ollama ($$url, model $$model)..."; \
	out=$$(curl -s -w "%{http_code}\n%{time_total}" -o /tmp/ollama_ping.json -X POST "$$url/v1/chat/completions" \
		-H "Content-Type: application/json" \
		-d "{\"model\":\"$$model\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"stream\":false}" 2>/dev/null); \
	res=$$(echo "$$out" | head -1); \
	time=$$(echo "$$out" | tail -1); \
	if [ "$$res" = "200" ]; then \
		echo "Ollama responded in $$time s (HTTP 200)."; \
	else \
		echo "Ollama ping failed (HTTP $$res). Check that Ollama is running and model $$model is available."; exit 1; \
	fi

# Stop the brain daemon so it does not compete with tests (e.g. for Ollama).
stop-daemon:
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		plist="$$HOME/Library/LaunchAgents/com.brain.daemon.plist"; \
		if [ -f "$$plist" ]; then \
			launchctl unload "$$plist" 2>/dev/null || true; \
			echo "Stopped brain daemon (launchd)."; \
		fi; \
	elif [ "$$(uname -s)" = "Linux" ]; then \
		systemctl --user stop brain-daemon 2>/dev/null || true; \
		echo "Stopped brain daemon (systemd)."; \
	fi

# Run all tests, always including integration tests (LLM/Ollama). Ollama is managed by
# Go test harness (TestMain) in internal/brain; the harness starts or uses existing ollama serve.
# Does not stop the brain daemon; if it is running, it may share Ollama with tests.
test:
	@echo "Running tests (including integration)..."
	@mkdir -p /tmp/brain-go-build-cache
	@BRAIN_TEST_OLLAMA_MODEL="$${BRAIN_TEST_OLLAMA_MODEL:-qwen2.5:1.5b}" \
		GOCACHE="$${GOCACHE:-/tmp/brain-go-build-cache}" \
		$(GO) test $(GO_TAGS) -tags integration -v -timeout 10m -count=1 ./...
	@echo "Tests complete!"

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@$(GO) test $(GO_TAGS) -v -count=1 -coverprofile=coverage.out ./...
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@$(GO) tool cover -func=coverage.out | tail -1

# Restart the background daemon service
restart-daemon:
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		launchctl kickstart -k gui/$$(id -u)/com.brain.daemon && echo "brain-daemon restarted"; \
	elif [ "$$(uname -s)" = "Linux" ]; then \
		systemctl --user restart brain-daemon && echo "brain-daemon restarted"; \
	else \
		echo "Unsupported OS: $$(uname -s)"; exit 1; \
	fi

# Remove build artifacts and test coverage files
clean:
	rm -rf $(BIN_DIR)/
	rm -f coverage.out coverage.html

# Show available targets and usage
help:
	@echo "brAIn Makefile — common targets:"
	@echo "  make              — show this help"
	@echo "  make build-all    — build CLI and daemon"
	@echo "  make run          — run example brain demo"
	@echo "  make test         — run tests (including integration)"
	@echo "  make test-coverage — run tests with coverage report"
	@echo "  make lint         — fmt, vet, staticcheck"
	@echo "  make lint-strict  — golangci-lint (required for hooks)"
	@echo "  make install      — install to ~/bin (installs Go and Ollama if missing, or run update if already installed)"
	@echo "  make update       — refresh ~/bin binaries and AI skills"
	@echo "  make install-hooks — install git pre-commit hook"
	@echo "  make clean        — remove bin/ and coverage files"
	@echo ""
	@echo "Override: GO_TAGS=... GO=... BIN_DIR=... HOME=..."
