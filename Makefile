.PHONY: build bin plugins mac-bins wasm fmt-check proto-check check check-electron check-e2e check-web check-connections serve clean launch vendor dist dist-mac dist-win stamp-version node-modules

# Every plugin kind with a binary in $(PLUGINS_DIR). This is the one list:
# `plugins` builds from it and `clean` removes from it.
ALL_PLUGIN_KINDS := fs proc gitlab pages hey gmail

# HOST_GOOS is what this machine builds for. It is a question, not a switch:
# the release builds run on native runners, one per OS, so nothing here
# cross-compiles a distribution.
HOST_GOOS := $(shell go env GOOS)

ifeq ($(HOST_GOOS),windows)
# Windows names a built binary <name>.exe; internal/cli/serve.go's
# exeSuffixFor and apps/desktop/src/main/paths.ts are the loader's side of
# the same fact.
EXE := .exe
# proc reads /proc. It is a unix plugin by design, so the Windows build
# ships without it rather than shipping one that can never answer.
PLUGIN_KINDS := $(filter-out proc,$(ALL_PLUGIN_KINDS))
else
EXE :=
PLUGIN_KINDS := $(ALL_PLUGIN_KINDS)
endif

BIN := ./gridwell$(EXE)
# clean removes every kind, whatever this host builds, so switching hosts in
# one checkout leaves nothing behind.
ALL_PLUGIN_BIN := $(addsuffix $(EXE),$(addprefix ./gridwell-plugin-,$(ALL_PLUGIN_KINDS)))

# VERSION is the release version, and the git tag is its ONE owner: the
# release workflow passes VERSION=$${GITHUB_REF_NAME#v}. Nothing in the tree
# carries a version between releases — no bump commit, no churn — so an
# unset VERSION is a development build and reports itself as "dev".
VERSION ?=
GO_LDFLAGS := -X github.com/josephburnett/gridwell/internal/cli.Version=$(VERSION)

# The plugins live in their own repository — gridwell owns the door, the
# plugins repo owns the plugins. PLUGINS_DIR is the one place that says where
# that checkout is: beside this one by default, overridable from the
# environment (PLUGINS_DIR=/path/to/gridwell-plugins make build).
PLUGINS_DIR ?= ../gridwell-plugins
WASM := ./web/gridwell.wasm
WASM_EXEC := ./web/wasm_exec.js
# Backslashes out: on Windows `go env GOROOT` answers C:\..., and every recipe
# here is a POSIX shell script, where a backslash is an escape and the path
# silently fails to exist. Git Bash reads C:/... fine.
GOROOT := $(subst \,/,$(shell go env GOROOT))

DESKTOP := apps/desktop

# Repo-local caches. A single online `make vendor` populates them; after that
# every `make dist` (and `make launch`) is fully offline — no GitHub, no npm
# registry, no network at all. Work on a plane. The vars are exported so the
# npm / electron / electron-builder toolchain underneath honours them.
CACHE := $(CURDIR)/$(DESKTOP)/.cache
NPM_CACHE := $(CACHE)/npm
export electron_config_cache := $(CACHE)/electron
export ELECTRON_BUILDER_CACHE := $(CACHE)/electron-builder

# `bin`, `plugins`, and `wasm` are phony so they always invoke `go build`. Go's
# build cache makes this fast when nothing changed, but it guarantees
# we never serve a stale binary or wasm artifact. Every plugin is its own
# separately-compiled go-plugin binary, laid out beside $(BIN) so the server
# resolves them by `gridwell-plugin-<kind>`.
build: bin plugins wasm

# CGO_ENABLED=0 makes the sidecar a fully static binary: modernc.org/sqlite is
# pure Go, so nothing pulls cgo and the result has no libc-version coupling —
# and since the web client (index.html, wasm, vendor) is EMBEDDED (web/embed.go),
# the built gridwell + gridwell-plugin-<kind> binaries are the whole distribution:
# copy them anywhere and the browser client serves from the binary itself.
# bin depends on wasm so the embed always carries the current client.
bin: wasm
	cd apps/gridwell && CGO_ENABLED=0 go build -ldflags "$(GO_LDFLAGS)" -o ../../gridwell$(EXE) .

# Phony so a source change always rebuilds (Go's build cache keeps it fast);
# file-target rules would skip the build whenever the binary already existed.
# The sources are in $(PLUGINS_DIR); the binaries land here, beside $(BIN),
# which is where the loader and every Go test look for them.
plugins:
	@test -d $(PLUGINS_DIR) || { \
		echo "$(PLUGINS_DIR) missing — the plugins live in their own repository:"; \
		echo "  git clone git@github.com:josephburnett/gridwell-plugins.git $(PLUGINS_DIR)"; \
		echo "(or point PLUGINS_DIR at an existing checkout)"; \
		exit 1; \
	}
	@set -e; for k in $(PLUGIN_KINDS); do \
		echo "cd $(PLUGINS_DIR)/$$k && go build -o $(CURDIR)/gridwell-plugin-$$k$(EXE)"; \
		(cd $(PLUGINS_DIR)/$$k && CGO_ENABLED=0 go build -o $(CURDIR)/gridwell-plugin-$$k$(EXE) ./cmd/gridwell-plugin-$$k); \
	done

# The .gz sidecar rides along: the server serves it with
# Content-Encoding: gzip when the client accepts it (staticOrSPA's
# serveGzipSidecar). The wasm is tens of megabytes raw and a fraction of
# that gzipped, and a phone on a relayed link downloads it every boot.
# gzip runs after the build so the sidecar is always at least as new as
# the raw file; the server refuses a stale one.
#
# Both embedded artifacts are written the same way: to a private temp name in
# the same directory, then renamed into place. Rename is atomic on one
# filesystem, so the published name only ever holds a complete file. This
# matters because `go build` rewrites its output incrementally over about a
# second: whoever reads the growing file — the `gzip` on the next line, a
# concurrent `make` in the same tree, a `go build` of the server doing the
# go:embed, a --static file server — used to see a prefix. A gzip of a prefix
# is a VALID gzip and exits 0, so nothing complained and the browser got a
# clean 200 of a truncated module.
#
# The temp name carries the shell's pid, so two concurrent makes in one tree
# do not share it either; no flock, which would buy only the sub-millisecond
# window between the two renames below and is not portable off Linux. The
# byte-comparison stays as the invariant: it is what makes a bad pair
# impossible rather than merely unlikely, and web/embed_test.go holds the same
# property over the bytes actually embedded.
wasm: $(WASM_EXEC)
	mkdir -p web
	@set -e; \
	tmp=$(WASM).$$$$.tmp; \
	trap 'rm -f "$$tmp" "$$tmp.gz"' EXIT; \
	echo "GOOS=js GOARCH=wasm go build -o $(WASM) ./client/wasm"; \
	GOOS=js GOARCH=wasm go build -o "$$tmp" ./client/wasm; \
	echo "gzip -9 $(WASM) -> $(WASM).gz"; \
	gzip -9 -c "$$tmp" > "$$tmp.gz"; \
	gzip -dc "$$tmp.gz" | cmp -s - "$$tmp" || { \
		echo "$(WASM).gz does not decompress to $(WASM) — the sidecar is short; rerun make wasm"; \
		exit 1; \
	}; \
	mv -f "$$tmp.gz" $(WASM).gz; \
	mv -f "$$tmp" $(WASM)

$(WASM_EXEC):
	mkdir -p web
	@set -e; \
	tmp=$(WASM_EXEC).$$$$.tmp; \
	trap 'rm -f "$$tmp"' EXIT; \
	if [ -f $(GOROOT)/lib/wasm/wasm_exec.js ]; then \
		cp $(GOROOT)/lib/wasm/wasm_exec.js "$$tmp"; \
	elif [ -f $(GOROOT)/misc/wasm/wasm_exec.js ]; then \
		cp $(GOROOT)/misc/wasm/wasm_exec.js "$$tmp"; \
	else \
		echo "wasm_exec.js not found in GOROOT"; exit 1; \
	fi; \
	mv -f "$$tmp" $(WASM_EXEC)

# fmt-check fails if any hand-written Go file isn't gofmt-clean (generated code
# under api/gen is excluded — it's regenerated, not hand-edited). It is the
# first check step so formatting drift cannot accumulate. Fix with `gofmt -w`.
fmt-check:
	@bad=$$(gofmt -l $$(git ls-files '*.go' | grep -v '/gen/')); \
	if [ -n "$$bad" ]; then echo "gofmt needed (run: gofmt -w <file>):"; echo "$$bad"; exit 1; fi

# proto-check regenerates the wire code (local buf plugins — offline) and
# fails when the generated set differs from the git INDEX or carries
# untracked files. GENERATED covers both halves: api/gen (the protobuf +
# connect code) and api/rpc/wire_gen.go (api/rpc's Go records and their
# conversions, derived from the same proto).
# This catches all three ways generated code goes wrong: a proto edit
# without `buf generate`; a hand-edit to generated code; and a partial
# `git add` of the generated set, which leaves every working-tree gate
# green while the pushed history does not compile. Staged-but-uncommitted
# generated files pass, since worktree == index is the invariant, so the
# normal edit, regen, git add, make check, commit loop is unaffected.
GENERATED := api/gen api/rpc/wire_gen.go

proto-check:
	@command -v buf >/dev/null || { echo "buf not found — install buf (+protoc-gen-go, -connect-go, -go-grpc) to run proto-check"; exit 1; }
	buf generate
	@git diff --exit-code -- $(GENERATED) || { echo "generated code differs from the index — run 'git add $(GENERATED)' (or commit the regen with the proto change)"; exit 1; }
	@untracked=$$(git ls-files --others --exclude-standard $(GENERATED)); \
	if [ -n "$$untracked" ]; then echo "untracked generated files — run 'git add $(GENERATED)':"; echo "$$untracked"; exit 1; fi

# check is the per-commit verification gate: every commit must leave all of these
# green. fmt-check enforces gofmt; the wasm build catches GOOS=js breakage that
# `go build ./...` (host arch) misses, and the windows and darwin builds catch
# the same class for the RELEASE targets — a `//go:build unix` half whose other
# half went stale does not fail on the dev box, and the release workflow is a
# tag away, too late; the typecheck catches Electron-side TS
# drift; `npm test` runs the desktop main-process unit tests (menu/geometry logic
# that never reaches the heavier display-bound gates); check-exception-owners
# fails when a declared exception field is read outside the predicate that
# owns the question; check-docpaths fails when a doc or workflow names a repo
# path that no longer exists. No display or network needed.
# MODULES lists every in-repo Go module beyond the root: the api and the
# shared nested modules. check builds and tests each one standalone
# (GOWORK=off) so no module can quietly lean on the workspace. The plugins
# are not here: they are another repository's modules, gated by its own
# `make check`.
MODULES := api internal/doctype apps/gridwell

# check depends on wasm: web/embed.go EMBEDS the built gridwell.wasm, so
# a fresh checkout (CI) cannot even `go build ./...` before one exists. It
# depends on plugins because the seam tests spawn the real binaries: the only
# door a plugin has into this repo.
check: fmt-check proto-check wasm plugins
	go build ./...
	go vet ./...
	go test ./...
	cd test/boundary && go test -count=1 .
	@for m in $(MODULES); do \
		echo "== module $$m (standalone)"; \
		(cd $$m && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...) || exit 1; \
	done
	GOOS=js GOARCH=wasm go build -o /tmp/gridwell.wasm ./client/wasm
	GOOS=windows GOARCH=amd64 go build -o /dev/null ./...
	GOOS=darwin GOARCH=arm64 go build -o /dev/null ./...
	./scripts/check-tracked-binaries.sh
	./scripts/check-vocabulary.sh
	./scripts/check-deadcode.sh
	./scripts/check-exception-owners.sh
	./scripts/check-docpaths.sh
	go tool staticcheck ./...
	cd $(DESKTOP) && npm run typecheck
	cd $(DESKTOP) && npm run typecheck:e2e
	cd $(DESKTOP) && npm test

# The heavy gates below are the one recipe for each gate: CI
# (.github/workflows/gates.yml) invokes these targets rather than
# re-spelling them, so the two cannot drift. PW_FLAGS passes extra
# Playwright flags through to check-e2e / check-web:
#   make check-e2e PW_FLAGS=--retries=1     # CI's one-retry flake discipline
PW_FLAGS ?=

# check-electron runs the live-tile harnesses under a virtual display. It is
# needed only for a change that touches the live url path, since it exercises
# the real Electron WebContentsView; shells ride a WebSocket on the web door,
# so check-web owns that path. Requires xvfb and a prior `make vendor` for
# node_modules. The npm scripts wrap xvfb-run themselves — do not wrap them
# again.
check-electron: node-modules
	cd $(DESKTOP) && npm run test:integration && npm run test:bridge

# check-e2e drives the real Electron app end to end: Playwright launches the
# same `electron .` as `make launch`, which spawns the Go sidecar, points it at
# a fresh throwaway home that serve mints its config into, and drives the wasm
# canvas with synthetic mouse input, asserting outcomes against the live server
# over Connect-RPC. It is the only test that exercises the full renderer, wasm,
# RPC, server, SQLite composition. It is heavier than `make check`, since it
# builds the binaries and boots Electron, so it is a pre-merge full-stack gate
# rather than part of the fast per-commit check. Requires xvfb and a prior
# `make vendor` for node_modules and Playwright.
check-e2e: build node-modules
	cd $(DESKTOP) && npm run build && xvfb-run -a npm run test:e2e -- $(PW_FLAGS)

# check-web drives the BROWSER-MODE client: `gridwell serve` + plain Chromium
# (the system /usr/bin/chromium — no browser download, so the repo stays
# offline-buildable). This is the only gate that sees the degraded phone/
# tablet client: no Electron bridge, caps-gated live-URL affordances, and the
# touch gesture layer (client/touchgest) driven by real injected TouchEvents.
# Headless — no xvfb needed. Run for any change to client/caps,
# client/touchgest, touch.go, or the browser-serving path.
check-web: build node-modules
	cd $(DESKTOP) && npm run test:e2e:web -- $(PW_FLAGS)

# check-connections is the spawn gate: the real binaries — gridwell serve and
# the go-plugin subprocesses — through a real ssh tunnel, with one write and
# read crossing every hop. The in-process seam tests cannot see go-plugin
# spawn, so a failure that only happens in a spawned process leaves them green.
# Guarded by the `connections` build tag so make check stays fast. Headless.
# Run it for any change to plugin spawn, the dialer, the node export, or
# routing.
check-connections: build
	cd test/connections && go test -tags connections -count=1 .

# serve runs the backend on its own. The desktop app spawns it as a sidecar;
# this target is for poking at the RPC surface or loading the wasm client in a
# plain browser, where live url tiles only work inside the Electron app. A
# missing ~/.gridwell/server.yaml is a fresh home: the first serve mints the
# node's id and writes the file.
serve: build
	$(BIN) serve $(SERVE_FLAGS)

# SERVE_FLAGS passes extra flags through, e.g.
# `make serve SERVE_FLAGS="--bind 0.0.0.0:8080"`.
SERVE_FLAGS ?=

# vendor is the ONE online step. It pins and caches everything the desktop
# build needs — npm packages (into $(NPM_CACHE) + node_modules), the Electron
# runtime zip (into electron_config_cache), and the electron-builder helper
# binaries incl. the AppImage runtime (into ELECTRON_BUILDER_CACHE) — by
# running `npm ci` against the committed lockfile and then building the
# AppImage once. After this completes, `make dist` needs no network.
vendor: build
	cd $(DESKTOP) && npm ci --cache $(NPM_CACHE)
	# Electron defers its binary download to first run, so materialize it here,
	# into the repo-local cache, or the first offline `make launch` or harness
	# run reaches for the network.
	cd $(DESKTOP) && node node_modules/electron/install.js
	$(MAKE) dist
	@echo "vendored: caches warm under $(CACHE); 'make dist' is now offline"

# stamp-version writes VERSION into the desktop package.json, which is where
# electron-builder reads the version it names every artifact with. This is a
# BUILD-TIME write and never a commit: the tree keeps its 0.0.0 placeholder,
# so cutting a release bumps no file and leaves no churn. An empty VERSION is
# a local build and leaves package.json alone.
stamp-version:
	@if [ -n "$(VERSION)" ]; then \
		(cd $(DESKTOP) && npm version "$(VERSION)" --no-git-tag-version --allow-same-version >/dev/null); \
		echo "stamped $(DESKTOP)/package.json version = $(VERSION)"; \
	else \
		echo "no VERSION: a development build, package.json untouched"; \
	fi

# dist, dist-mac and dist-win are the three release builds, one per OS. Each
# runs on a NATIVE runner (.github/workflows/release.yml sequences them, and
# the Makefile stays the one recipe): a dmg cannot be produced off macOS, and
# the portable exe wants a Windows host. Every one bundles the Electron
# runtime, the Go binaries as extraResources, and — through web/embed.go —
# the whole web client, so an artifact is self-contained.
#
# dist is also the offline AppImage build for local use, and assumes a prior
# `make vendor` warmed the caches and installed node_modules. It produces
# Gridwell-<ver>.AppImage under $(DESKTOP)/out/.
dist: build node-modules stamp-version
	cd $(DESKTOP) && npm run build && ./node_modules/.bin/electron-builder --linux AppImage
	@echo "AppImage: $(DESKTOP)/out/"

# dist-mac produces both dmgs, Gridwell-<ver>-arm64.dmg and -x64.dmg, from
# mac-bins' universal Go binaries. package.json's mac.identity is "-": ad-hoc
# signing, because an unsigned bundle refuses to launch on Apple Silicon at
# all. It is not notarized, so a first launch needs "Open Anyway" — see
# docs/release.md.
dist-mac: mac-bins wasm node-modules stamp-version
	cd $(DESKTOP) && npm run build && ./node_modules/.bin/electron-builder --mac
	@echo "dmg: $(DESKTOP)/out/"

# dist-win produces the portable Gridwell-<ver>.exe. Its accepted
# degradations are in docs/release.md: no live shells (shelldriver has no PTY
# there), no serve lock, no proc plugin, and no connection door, whose 0600
# unix socket has no Windows equivalent.
dist-win: build node-modules stamp-version
	cd $(DESKTOP) && npm run build && ./node_modules/.bin/electron-builder --win portable
	@echo "portable exe: $(DESKTOP)/out/"

# mac-bins makes the Go binaries UNIVERSAL. macOS ships two dmgs, one per
# arch, but extraResources is ONE set of files for both, so an arm64-only
# sidecar would ride inside the Intel dmg and never start. Each binary is
# compiled twice and lipo'd into one file, at exactly the path the Linux and
# Windows builds write, so package.json names one thing everywhere.
#
# Then codesign --sign -, because Apple Silicon refuses to exec an unsigned
# Mach-O outright. Go's linker ad-hoc signs each arm64 build, but lipo writes
# a NEW fat file, and the signature it carries no longer covers what is on
# disk. The app bundle's own signing pass does not reach a plain executable
# in Contents/Resources, so this is the one that counts for the sidecar and
# the six plugins.
mac-bins: wasm
	@set -e; tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	fat() { \
		out=$$1; dir=$$2; pkg=$$3; \
		echo "universal $$out"; \
		(cd "$$dir" && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(GO_LDFLAGS)" -o "$$tmp/$$out.amd64" "$$pkg"); \
		(cd "$$dir" && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(GO_LDFLAGS)" -o "$$tmp/$$out.arm64" "$$pkg"); \
		lipo -create -output "$(CURDIR)/$$out" "$$tmp/$$out.amd64" "$$tmp/$$out.arm64"; \
		codesign --force --sign - "$(CURDIR)/$$out"; \
	}; \
	fat gridwell apps/gridwell .; \
	for k in $(PLUGIN_KINDS); do fat gridwell-plugin-$$k $(PLUGINS_DIR)/$$k ./cmd/gridwell-plugin-$$k; done

# `make launch` is the one-shot dev run: build the sidecar and wasm, compile
# the TS, and launch Electron against ~/.gridwell, so your existing grids are
# right there. A home with no server.yaml is created on the first serve. Point
# at a different home with GRIDWELL_HOME. It runs with Chromium's OS sandbox
# on, since live url tiles load untrusted web content and the sandbox is the
# containment that matters. Needs a prior `make vendor` for node_modules.
#
#   make launch                                     # ~/.gridwell
#   GRIDWELL_HOME=/path/to/home make launch         # another home
launch: build node-modules
	cd $(DESKTOP) && npm run build && ./node_modules/.bin/electron .

# node-modules guards the offline targets: if the desktop deps aren't present,
# point the user at the single online bootstrap instead of silently reaching
# for the network.
node-modules:
	@test -d $(DESKTOP)/node_modules || { \
		echo "$(DESKTOP)/node_modules missing — run 'make vendor' once (online) first"; \
		exit 1; \
	}

clean:
	rm -f $(BIN) $(ALL_PLUGIN_BIN) $(WASM) $(WASM_EXEC)
	rm -rf $(DESKTOP)/dist $(DESKTOP)/out
