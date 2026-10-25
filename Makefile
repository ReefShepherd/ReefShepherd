# Version stamped into the binary; falls back to "dev" outside a git checkout.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/reefshepherd/reefshepherd/internal/version.version=$(VERSION)
# Pinned sqlc release, run through the Go toolchain so nothing has to be installed.
SQLC_VERSION := v1.31.1
SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

HAVE_WEB := $(wildcard web/package.json)

.PHONY: web generate check-sql build build-go test lint fmt dev dev-web dev-status dev-stop dev-web-stop clean

# Build the web UI into internal/webui/dist; skipped until web/ exists.
web:
ifeq ($(HAVE_WEB),)
	@echo "web/package.json not found, skipping web build"
else
	cd web && { [ -d node_modules ] || npm ci; } && npm run build
endif

# Regenerate the sqlc code in internal/db/dbgen.
generate:
	$(SQLC) generate

# Checks on the generated code and the migration files, run by CI after generate.
check-sql:
	python3 -B -I scripts/check_generated.py
	python3 -B -I scripts/check_generated_test.py
	scripts/check-migrations_test.sh
	scripts/check-migrations.sh

# One binary with the web UI embedded.
build: web build-go

# Go binary only, with whatever UI is currently in internal/webui/dist.
build-go:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/reefshepherd ./cmd/reefshepherd

# Go packages under cmd and internal only, so Go files inside web/node_modules are never picked up.
GO_PACKAGES := ./cmd/... ./internal/...

test:
	go test $(GO_PACKAGES)
ifneq ($(HAVE_WEB),)
	cd web && npm test
endif

lint:
	golangci-lint run $(GO_PACKAGES)
ifneq ($(HAVE_WEB),)
	cd web && npm run lint && npm run check
endif

fmt:
	gofmt -w cmd internal

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
