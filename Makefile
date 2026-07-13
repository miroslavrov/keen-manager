# keen-manager — build system
#
# Single static Go binary (daemon + embedded web UI + CLI) for Keenetic routers
# running Entware. Cross-compiled for mipsle/mips/arm64/arm, CGO disabled.
#
# This Makefile assumes `go` and `npm` are on PATH (it does NOT hardcode a Go
# location). GNU make is fine; the recipes are POSIX-sh.
#
# Common usage:
#   make web         # build the front-end into internal/webui/dist (go:embed'd)
#   make build       # build ONE target (from GOARCH/GOMIPS env) -> ./build/keen-manager
#   make build-all   # cross-compile all router targets -> ./build/keen-manager-<arch>
#   make dist        # web + build-all, then gzip each binary for release upload
#   make test vet    # go test / go vet
#   make clean       # remove build output (keeps the dist placeholder)

# ---- Module / injection targets -------------------------------------------
MODULE      := github.com/miroslavrov/keen-manager
MAIN_PKG    := ./cmd/keen-manager
BINARY      := keen-manager
BUILD_DIR   := build
WEB_DIR     := web
DIST_DIR    := internal/webui/dist
VERSION_PKG := $(MODULE)/internal/version

# ---- Version metadata (guarded against a missing git) ----------------------
# VERSION: `git describe --tags --always --dirty`, falling back to 0.0.0-dev.
# COMMIT:  short SHA, "unknown" if git is unavailable.
# DATE:    UTC RFC3339 build timestamp.
GIT        := $(shell command -v git 2>/dev/null)
ifeq ($(GIT),)
VERSION    ?= 0.0.0-dev
COMMIT     ?= unknown
else
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
endif
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# ldflags: strip symbols (-s -w) and inject version/commit/date.
LDFLAGS := -s -w \
	-X $(VERSION_PKG).Version=$(VERSION) \
	-X $(VERSION_PKG).Commit=$(COMMIT) \
	-X $(VERSION_PKG).Date=$(DATE)

# Common Go build flags. CGO must be off for static MIPS/ARM binaries.
GOFLAGS   := -trimpath
GO        := go
NPM       := npm

# Router targets: <suffix>:<GOARCH>:<GOMIPS>. GOMIPS is empty for arm/arm64.
# Mirrors internal/platform/arch.go (mipsle & mips are softfloat).
TARGETS := mipsle:mipsle:softfloat mips:mips:softfloat arm64:arm64: arm:arm:

# ---- Default: help ---------------------------------------------------------
.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "keen-manager build targets:"
	@echo "  make web         Build the React front-end into $(DIST_DIR) (go:embed'd)"
	@echo "  make build       Build one target into ./$(BUILD_DIR)/$(BINARY)"
	@echo "                   (honors GOARCH/GOMIPS env, e.g. GOARCH=mipsle GOMIPS=softfloat)"
	@echo "  make build-all   Cross-compile all router targets into ./$(BUILD_DIR)/"
	@echo "                   ($(BINARY)-mipsle, -mips, -arm64, -arm)"
	@echo "  make dist        web + build-all, then gzip each binary (-<arch>.gz) for release"
	@echo "  make test        go test ./..."
	@echo "  make vet         go vet ./..."
	@echo "  make clean       Remove ./$(BUILD_DIR) and built front-end (keeps dist placeholder)"
	@echo ""
	@echo "Version: $(VERSION)  Commit: $(COMMIT)"

# ---- Front-end -------------------------------------------------------------
# npm ci for reproducible installs, falling back to npm install if there is no
# lockfile. Vite is configured (web/vite.config.ts) to emit into $(DIST_DIR).
.PHONY: web
web:
	@echo ">> building front-end ($(WEB_DIR) -> $(DIST_DIR))"
	cd $(WEB_DIR) && ($(NPM) ci || $(NPM) install)
	cd $(WEB_DIR) && $(NPM) run build

# ---- Single-target build ---------------------------------------------------
# Builds whatever GOARCH/GOMIPS are set in the environment (default: host).
# Example: make build GOARCH=mipsle GOMIPS=softfloat
#
# Depends on `web`: the front-end is go:embed'd, so every build starts from a
# freshly compiled bundle. This is the guard against the "source fixed but the
# embedded bundle is stale" bug (the blank-tabs regression). Iterating on Go
# only? Call `go build ./cmd/keen-manager` directly to skip the web step.
.PHONY: build
build: web
	@echo ">> building $(BINARY) (GOARCH=$(GOARCH) GOMIPS=$(GOMIPS)) -> $(BUILD_DIR)/$(BINARY)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY) $(MAIN_PKG)

# ---- Cross-compile every router target ------------------------------------
# Also depends on `web` so a stale embedded bundle can never be cross-compiled
# into a release. `make dist` (web build-all) runs the web target once — make
# de-duplicates a shared prerequisite within a single invocation.
.PHONY: build-all
build-all: web
	@mkdir -p $(BUILD_DIR)
	@for t in $(TARGETS); do \
		suffix=$${t%%:*}; rest=$${t#*:}; goarch=$${rest%%:*}; gomips=$${rest#*:}; \
		out="$(BUILD_DIR)/$(BINARY)-$$suffix"; \
		echo ">> building $$out (GOARCH=$$goarch GOMIPS=$$gomips)"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$goarch GOMIPS=$$gomips \
			$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o "$$out" $(MAIN_PKG) || exit 1; \
	done
	@echo ">> build-all complete:"
	@ls -lh $(BUILD_DIR)/$(BINARY)-* 2>/dev/null || true

# ---- Release artifacts -----------------------------------------------------
# gzip each cross-compiled binary in place -> keen-manager-<arch>.gz. The
# installer downloads and gunzips these (…/releases/latest/download/…-<arch>.gz).
.PHONY: dist
dist: web build-all
	@echo ">> gzipping release binaries in $(BUILD_DIR)/"
	@for t in $(TARGETS); do \
		suffix=$${t%%:*}; \
		bin="$(BUILD_DIR)/$(BINARY)-$$suffix"; \
		if [ -f "$$bin" ]; then \
			gzip -9 -f -k "$$bin"; \
			echo "   $$bin.gz"; \
		fi; \
	done
	@echo ">> dist complete:"
	@ls -lh $(BUILD_DIR)/*.gz 2>/dev/null || true

# ---- IPK packaging (Entware/opkg) ------------------------------------------
# Builds .ipk packages for each router arch. Requires `ar` (binutils).
.PHONY: ipk
ipk: build-all
	@echo ">> building IPK packages in $(BUILD_DIR)/"
	@sh scripts/build-ipk.sh "$(VERSION)" "$(BUILD_DIR)"
	@echo ">> IPK packages:"
	@ls -lh $(BUILD_DIR)/*.ipk 2>/dev/null || true

# ---- QA --------------------------------------------------------------------
.PHONY: test
test:
	$(GO) test ./...

.PHONY: vet
vet:
	$(GO) vet ./...

# ---- Clean -----------------------------------------------------------------
# Remove build output and the compiled front-end, but restore the committed
# placeholder index.html so `//go:embed all:dist` still compiles afterwards.
.PHONY: clean
clean:
	@echo ">> cleaning $(BUILD_DIR) and $(DIST_DIR)"
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)/*
	@mkdir -p $(DIST_DIR)
	@printf '%s\n' \
		'<!doctype html>' \
		'<html lang="en">' \
		'  <head>' \
		'    <meta charset="utf-8" />' \
		'    <meta name="viewport" content="width=device-width, initial-scale=1" />' \
		'    <title>keen-manager</title>' \
		'  </head>' \
		'  <body>' \
		'    <!-- Placeholder. Replaced by the built React app (npm run build). -->' \
		'    <div id="root">keen-manager: front-end not built. Run `make web`.</div>' \
		'  </body>' \
		'</html>' > $(DIST_DIR)/index.html
	@echo ">> restored $(DIST_DIR)/index.html placeholder"
