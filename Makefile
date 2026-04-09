.PHONY: dev build test test-go test-electron lint clean install

GO_DIR      := $(CURDIR)/go
ELECTRON_DIR := $(CURDIR)/electron
GO_BINARY   := $(GO_DIR)/bistro-sync

# ── Development ──────────────────────────────────────────────────────────────

dev: build-go
	@echo "Starting Go sync engine and Electron in dev mode..."
	@echo "  Go:       http://localhost:8080 (WebSocket)"
	@echo "  Electron: pnpm dev"
	@cd $(GO_DIR) && ./bistro-sync --log-level debug &
	@cd $(ELECTRON_DIR) && pnpm dev

# ── Build ─────────────────────────────────────────────────────────────────────

build: build-go build-electron

build-go:
	@echo "Building Go sync engine..."
	cd $(GO_DIR) && go build -o bistro-sync ./cmd/bistro-sync

build-electron:
	@echo "Building Electron app..."
	cd $(ELECTRON_DIR) && pnpm build

# ── Tests ─────────────────────────────────────────────────────────────────────

test: test-go test-electron

test-go:
	@echo "Running Go tests..."
	cd $(GO_DIR) && go test ./...

test-electron:
	@echo "Running Electron tests..."
	cd $(ELECTRON_DIR) && pnpm test

# ── Lint ──────────────────────────────────────────────────────────────────────

lint:
	@echo "Linting Go..."
	cd $(GO_DIR) && go vet ./...
	@echo "Linting Electron..."
	cd $(ELECTRON_DIR) && pnpm lint

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	@echo "Cleaning Go build artifacts..."
	rm -f $(GO_BINARY)
	@echo "Cleaning Electron build artifacts..."
	rm -rf $(ELECTRON_DIR)/dist

# ── Install ───────────────────────────────────────────────────────────────────

install:
	@echo "Installing Go dependencies..."
	cd $(GO_DIR) && go mod download
	@echo "Installing Electron dependencies..."
	cd $(ELECTRON_DIR) && pnpm install
