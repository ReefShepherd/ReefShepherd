# Version stamped into the binary; falls back to "dev" outside a git checkout.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/reefshepherd/reefshepherd/internal/version.version=$(VERSION)
SQLC ?= sqlc

HAVE_WEB := $(wildcard web/package.json)

.PHONY: web generate build build-go test lint fmt dev dev-web dev-status dev-stop dev-web-stop clean

# Build the web UI into internal/webui/dist; skipped until web/ exists.
web:
ifeq ($(HAVE_WEB),)
	@echo "web/package.json not found, skipping web build"
else
	cd web && { [ -d node_modules ] || npm ci; } && npm run build
endif

# Regenerate sqlc code; skipped until sqlc.yaml exists.
generate:
	@if [ -f sqlc.yaml ]; then $(SQLC) generate; else echo "sqlc.yaml not found, skipping generate"; fi

# One binary with the web UI embedded.
build: web build-go

# Go binary only, with whatever UI is currently in internal/webui/dist.
build-go:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/reefshepherd ./cmd/reefshepherd

test:
	go test ./...
ifneq ($(HAVE_WEB),)
	cd web && npm test
endif

lint:
	golangci-lint run ./...
ifneq ($(HAVE_WEB),)
	cd web && npm run lint && npm run check
endif

fmt:
	gofmt -w .

# Dev servers: only one of each may run at a time (see the check before starting).
dev:
	@if pgrep -af '[r]eefshepherd serve --dev' >/dev/null; then echo "a dev server is already running"; exit 1; fi
	go run ./cmd/reefshepherd serve --dev --listen 127.0.0.1:8485 --data-dir ./.devdata

dev-web:
ifeq ($(HAVE_WEB),)
	@echo "web/package.json not found, nothing to run"
else
	@if pgrep -af '[v]ite dev --port 5173' >/dev/null; then echo "the Vite dev server is already running"; exit 1; fi
	cd web && npm run dev
endif

dev-status:
	@echo "reefshepherd dev server:"; pgrep -af '[r]eefshepherd serve --dev' || echo "  not running"
	@echo "vite dev server:"; pgrep -af '[v]ite dev --port 5173' || echo "  not running"

dev-stop:
	-pkill -f '[r]eefshepherd serve --dev'

dev-web-stop:
	-pkill -f '[v]ite dev --port 5173'

clean:
	rm -rf bin
