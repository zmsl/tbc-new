OUT_DIR := dist/tbc
TS_CORE_SRC := $(shell find ui/core -name '*.ts' -type f)
ASSETS_INPUT := $(shell find assets/ -type f)
ASSETS := $(patsubst assets/%,$(OUT_DIR)/assets/%,$(ASSETS_INPUT))
# Recursive wildcard function. Needs to be '=' instead of ':=' because of recursion.
rwildcard = $(foreach d,$(wildcard $(1:=/*)),$(call rwildcard,$d,$2) $(filter $(subst *,%,$2),$d))
GOROOT := $(shell go env GOROOT)
# .json matters: gear sets, APL rotations, preset builds and the talent trees are all JSON
# imported straight into the bundle. Without it, editing a preset leaves the bundle stale and
# make reports nothing to do, so host/desktop builds silently ship the previous gear.
UI_SRC := $(shell find ui -name '*.ts' -o -name '*.tsx' -o -name '*.scss' -o -name '*.html' -o -name '*.json')

$(OUT_DIR)/.dirstamp: \
  $(OUT_DIR)/lib.wasm \
  ui/core/proto/api.ts \
  $(ASSETS) \
  $(OUT_DIR)/bundle/.dirstamp
	touch $@

$(OUT_DIR)/bundle/.dirstamp: \
  $(UI_SRC) \
  vite.config.mts \
  vite.build-workers.mts \
  tools/vite/spec_pages.mts \
  node_modules \
  tsconfig.json \
  ui/core/index.ts \
  ui/core/proto/api.ts
	node_modules/typescript/bin/tsc --noEmit
	npx tsx vite.build-workers.mts
# Vite hashes chunk filenames and never removes the previous build's, so the directory
# accumulates orphans that nothing references and every packaging target then embeds. Clearing
# it first took the sidecar binary from 88MB to 33MB. Only bundle/ is emptied: the rest of
# $(OUT_DIR) holds assets and lib.wasm that make builds through separate rules.
	rm -rf $(OUT_DIR)/bundle
	npx vite build
	touch $@

ui/core/index.ts: $(TS_CORE_SRC)
	find ui/core -name '*.ts' | \
	  awk -F 'ui/core/' '{ print "import \x22./" $$2 "\x22;" }' | \
	  sed 's/\.ts";$$/";/' | \
	  grep -v 'import "./index";' > $@

.PHONY: clean
clean:
	rm -rf ui/core/proto/*.ts \
	  sim/core/proto/*.pb.go \
	  wowsimtbc \
	  wowsimtbc-windows.exe \
	  wowsimtbc-amd64-darwin \
	  wowsimtbc-arm64-darwin \
	  wowsimtbc-amd64-linux \
	  dist \
	  binary_dist \
	  ui/core/index.ts \
	  ui/core/proto/*.ts \
	  node_modules \
	  desktop-dist \
	  desktop/build \
	  desktop/node_modules \
	  mcp/dist \
	  wowsimmcp \
	  wowsimmcp.exe \
	  wowsims-tbc.mcpb
	find . -name "*.results.tmp" -type f -delete
# The presets are copied out of ui/ by mcp-presets, so they are build output like the rest.
# The glob leaves .gitkeep alone -- it does not match dotfiles -- which matters because that
# one file is tracked, and a clean that deleted it would leave the working tree dirty.
	rm -rf $(MCP_PRESETS)/*

ui/core/proto/api.ts: proto/*.proto node_modules
	npx protoc --ts_opt generate_dependencies --ts_out ui/core/proto --proto_path proto proto/api.proto
	npx protoc --ts_out ui/core/proto --proto_path proto proto/test.proto
	npx protoc --ts_out ui/core/proto --proto_path proto proto/ui.proto

.PHONY: package.json

package.json:
# Checks if the system is FreeBSD and jq is installed. This is due to the need to switch out the vite package for rollup on FreeBSD.
ifeq ($(shell uname -s), FreeBSD)
	@if ! command -v jq > /dev/null; then \
		echo "jq is not installed. Please install jq to proceed."; \
		exit 1; \
	fi; \
	\
	echo "Checking and updating package.json for FreeBSD..."; \
	\
	if ! grep -q '"overrides"' package.json; then \
		jq '. + { "overrides": { "vite": { "rollup": "npm:@rollup/wasm-node@4.13.0" } } }' package.json > package.json.tmp && mv package.json.tmp package.json && npm install; \
	else \
		jq '.overrides += { "vite": { "rollup": "npm:@rollup/wasm-node@4.13.0" } }' package.json > package.json.tmp && mv package.json.tmp package.json && npm install; \
	fi
endif

package-lock.json:
	npm install

node_modules: package-lock.json
	npm ci

# Generic rule for hosting any class directory
.PHONY: host_%
host_%: $(OUT_DIR) node_modules
	npx http-server $(OUT_DIR)/..

.PHONY: wasm
wasm: $(OUT_DIR)/lib.wasm

# Builds the generic .wasm, with all items included.
WASM_FEATURES := --enable-sign-ext --enable-nontrapping-float-to-int --enable-mutable-globals --enable-bulk-memory
$(OUT_DIR)/lib.wasm: sim/wasm/* sim/core/proto/api.pb.go $(filter-out sim/core/items/all_items.go, $(call rwildcard,sim,*.go))
	@echo "Starting webassembly compile now..."
	@if GOWASM=satconv,signext GOOS=js GOARCH=wasm go build -o ./$(OUT_DIR)/lib.wasm ./sim/wasm/; then \
		printf "\033[1;32mWASM compile successful.\033[0m\n"; \
	else \
		printf "\033[1;31mWASM COMPILE FAILED\033[0m\n"; \
		exit 1; \
	fi
	@if command -v wasm-opt >/dev/null 2>&1; then \
		echo "Optimizing wasm with wasm-opt..."; \
		wasm-opt -O3 $(WASM_FEATURES) $(OUT_DIR)/lib.wasm -o $(OUT_DIR)/lib.wasm.tmp && mv -f $(OUT_DIR)/lib.wasm.tmp $(OUT_DIR)/lib.wasm; \
	else \
		printf "\033[1;33mwasm-opt not found -- skipping optimization (install binaryen to enable).\033[0m\n"; \
	fi

$(OUT_DIR)/assets/%: assets/%
	mkdir -p $(@D)
	cp $< $@
	rm -rf $(OUT_DIR)/assets/db_inputs


binary_dist/dist.go: sim/web/dist.go.tmpl
	mkdir -p binary_dist/tbc
	touch binary_dist/tbc/embedded
	cp sim/web/dist.go.tmpl binary_dist/dist.go

binary_dist: $(OUT_DIR)/.dirstamp
	rm -rf binary_dist
	mkdir -p binary_dist
	cp -r $(OUT_DIR) binary_dist/
	rm binary_dist/tbc/lib.wasm
	rm -rf binary_dist/tbc/assets/db_inputs
	rm binary_dist/tbc/assets/database/db.bin
	rm binary_dist/tbc/assets/database/leftover_db.bin

# Rebuild the protobuf generated code.
.PHONY: proto
proto: sim/core/proto/api.pb.go ui/core/proto/api.ts

# Builds the web server with the compiled client.
.PHONY: wowsimtbc
wowsimtbc: binary_dist devserver

.PHONY: devserver
devserver: sim/core/proto/api.pb.go sim/web/main.go binary_dist/dist.go
	@echo "Starting server compile now..."
	@if go build -o wowsimtbc ./sim/web/main.go ; then \
		printf "\033[1;32mBuild Completed Successfully\033[0m\n"; \
	else \
		printf "\033[1;31mBUILD FAILED\033[0m\n"; \
		exit 1; \
	fi

.PHONY: air
air:
ifeq ($(WATCH), 1)
	@if ! command -v air; then \
		echo "Missing air dependency. Please run \`make setup\`"; \
		exit 1; \
	fi
endif

rundevserver: air devserver
ifeq ($(WATCH), 1)
	npx tsx vite.build-workers.mts & npx vite build -m development --watch &
	ulimit -n 10240 && air -tmp_dir "/tmp" -build.include_ext "go,proto" -build.args_bin "--usefs=true --launch=false" -build.bin "./wowsimtbc" -build.cmd "make devserver" -build.exclude_dir "assets,dist,node_modules,ui,tools"
else
	./wowsimtbc --usefs=true --launch=false --host=":3333"
endif

wowsimtbc-windows.exe: wowsimtbc
# go build only considers syso files when invoked without specifying .go files: https://github.com/golang/go/issues/16090
	cp ./assets/favicon_io/icon-windows_amd64.syso ./sim/web/icon-windows_amd64.syso
	cd ./sim/web/ && GOOS=windows GOARCH=amd64 GOAMD64=v2 go build -o wowsimtbc-windows.exe -ldflags="-X 'main.Version=$(VERSION)' -s -w"
	cd ./cmd/wowsimcli && GOOS=windows GOARCH=amd64 GOAMD64=v2 go build -o wowsimcli-windows.exe --tags=with_db -ldflags="-X 'main.Version=$(VERSION)' -s -w"
	rm ./sim/web/icon-windows_amd64.syso
	mv ./sim/web/wowsimtbc-windows.exe ./wowsimtbc-windows.exe
	mv ./cmd/wowsimcli/wowsimcli-windows.exe ./wowsimcli-windows.exe

release: wowsimtbc wowsimtbc-windows.exe
	GOOS=darwin GOARCH=amd64 GOAMD64=v2 go build -o wowsimtbc-amd64-darwin -ldflags="-X 'main.Version=$(VERSION)' -s -w" ./sim/web/main.go
	GOOS=darwin GOARCH=arm64 go build -o wowsimtbc-arm64-darwin -ldflags="-X 'main.Version=$(VERSION)' -s -w" ./sim/web/main.go
	GOOS=darwin GOARCH=arm64 go build -o wowsimcli-arm64-darwin --tags=with_db -ldflags="-X 'main.Version=$(VERSION)' -s -w" ./cmd/wowsimcli/cli_main.go
	GOOS=linux GOARCH=amd64 GOAMD64=v2 go build -o wowsimtbc-amd64-linux   -ldflags="-X 'main.Version=$(VERSION)' -s -w" ./sim/web/main.go
	GOOS=linux GOARCH=amd64 GOAMD64=v2 go build -o wowsimcli-amd64-linux --tags=with_db -ldflags="-X 'main.Version=$(VERSION)' -s -w" ./cmd/wowsimcli/cli_main.go
# Now compress into a zip because the files are getting large.
	zip wowsimtbc-windows.exe.zip wowsimtbc-windows.exe
	zip wowsimtbc-amd64-darwin.zip wowsimtbc-amd64-darwin
	zip wowsimtbc-arm64-darwin.zip wowsimtbc-arm64-darwin
	zip wowsimtbc-amd64-linux.zip wowsimtbc-amd64-linux
	zip wowsimcli-amd64-linux.zip wowsimcli-amd64-linux
	zip wowsimcli-arm64-darwin.zip wowsimcli-arm64-darwin
	zip wowsimcli-windows.exe.zip wowsimcli-windows.exe


# ---- Desktop app (Electron shell) -------------------------------------------------------
# Deliberately off the default build path: `make`, `make test` and `make host` never touch
# any of this, and Electron stays out of the root package.json so `npm ci` -- and the four
# CI test shards that run it -- are unaffected.

DESKTOP_DIR     := desktop
DESKTOP_SIDECAR := $(DESKTOP_DIR)/build/sidecar
DESKTOP_ICON    := $(DESKTOP_DIR)/build/icon.png
DESKTOP_LDFLAGS := -X 'main.Version=$(VERSION)' -s -w

# Single source of truth for the app icon; electron-builder derives .ico and .icns from it.
$(DESKTOP_ICON): assets/favicon_io/android-chrome-512x512.png
	mkdir -p $(@D)
	cp $< $@

$(DESKTOP_DIR)/node_modules: $(DESKTOP_DIR)/package.json
	cd $(DESKTOP_DIR) && npm install
	touch $@

# electron-updater compares releases against the version in desktop/package.json while the
# sim server reports its -ldflags value. If the two drift, the app either offers an update
# it already has or never notices one.
.PHONY: desktop-version
desktop-version:
ifneq ($(VERSION),)
	cd $(DESKTOP_DIR) && npm version --no-git-tag-version --allow-same-version $(patsubst v%,%,$(VERSION))
endif

# Desktop-only: swap the cdnjs Font Awesome link for a bundled copy, in the embedded client
# tree. The web sources keep the CDN link untouched, so the site is unaffected.
.PHONY: desktop-bundle-fonts
desktop-bundle-fonts: $(DESKTOP_DIR)/node_modules
	node tools/desktop/bundle_fonts.mjs binary_dist/tbc

.PHONY: desktop-sidecar-win
desktop-sidecar-win: binary_dist binary_dist/dist.go desktop-bundle-fonts
	mkdir -p $(DESKTOP_SIDECAR)
	GOOS=windows GOARCH=amd64 GOAMD64=v2 go build -o $(DESKTOP_SIDECAR)/wowsimtbc-x64.exe -ldflags="$(DESKTOP_LDFLAGS)" ./sim/web/main.go

.PHONY: desktop-sidecar-mac
desktop-sidecar-mac: binary_dist binary_dist/dist.go desktop-bundle-fonts
	mkdir -p $(DESKTOP_SIDECAR)
	GOOS=darwin GOARCH=amd64 GOAMD64=v2 go build -o $(DESKTOP_SIDECAR)/wowsimtbc-x64   -ldflags="$(DESKTOP_LDFLAGS)" ./sim/web/main.go
	GOOS=darwin GOARCH=arm64                go build -o $(DESKTOP_SIDECAR)/wowsimtbc-arm64 -ldflags="$(DESKTOP_LDFLAGS)" ./sim/web/main.go

.PHONY: desktop-win
desktop-win: $(DESKTOP_DIR)/node_modules $(DESKTOP_ICON) desktop-version desktop-sidecar-win
	cd $(DESKTOP_DIR) && npx electron-builder --win nsis --publish never

.PHONY: desktop-mac
desktop-mac: $(DESKTOP_DIR)/node_modules $(DESKTOP_ICON) desktop-version desktop-sidecar-mac
	cd $(DESKTOP_DIR) && npx electron-builder --mac dmg zip --publish never

# Unpacked Windows app, for eyeballing the real window without building an installer.
# Unlike desktop-win this needs no wine, because it stops before NSIS assembly. Copy
# desktop-dist/win-unpacked somewhere under /mnt/c and run "WoWSims TBC.exe" from Windows.
.PHONY: desktop-preview-win
desktop-preview-win: $(DESKTOP_DIR)/node_modules $(DESKTOP_ICON) desktop-sidecar-win
	cd $(DESKTOP_DIR) && npx electron-builder --win --dir --publish never

# Runs the shell against the repo-root wowsimtbc build, for iterating on the shell itself.
.PHONY: desktop-dev
desktop-dev: $(DESKTOP_DIR)/node_modules wowsimtbc
	cd $(DESKTOP_DIR) && npm start

# ---- MCP server -------------------------------------------------------------------------
# Like the desktop shell, deliberately off the default build path. It is its own Go module, so
# the MCP SDK never enters the root go.mod and `go build ./...`, `make test` and the four CI
# shards (which list ./sim/...) never see any of it. The trade is that a breaking change in
# sim/core only shows up here, so run `make mcp-test` when changing the engine's API.

MCP_DIR     := mcp
MCP_LDFLAGS := -X 'main.Version=$(VERSION)' -s -w

MCP_PRESETS := $(MCP_DIR)/internal/presets/files

# Everything these targets produce goes here rather than the repository root, so a built tree
# stays tidy and `rm -rf mcp/dist` is the whole cleanup. The root binaries the other targets
# write (wowsimtbc and friends) are upstream's convention and the release workflow's contract,
# so they are left where they are.
MCP_OUT := $(MCP_DIR)/dist

# The gear sets, rotations, builds and talent presets are compiled into the binary so it needs
# nothing but itself at runtime. They are copied out of ui/ on every build rather than committed
# under mcp/: one copy in the repository, and no chance of the two drifting.
.PHONY: mcp-presets
mcp-presets:
	rm -rf $(MCP_PRESETS)
	mkdir -p $(MCP_PRESETS)
	touch $(MCP_PRESETS)/.gitkeep
	cd ui && find . -path './*/*/*' \( -name '*.gear.json' -o -name '*.apl.json' -o -name '*.build.json' \) -print \
	  -o -path './*/*/presets.ts' -print \
	  | while read -r f; do \
	      mkdir -p "../$(MCP_PRESETS)/$$(dirname "$$f")"; \
	      cp "$$f" "../$(MCP_PRESETS)/$$f"; \
	    done

# with_db is not optional in practice: without it every item lookup comes back empty.
.PHONY: mcp
mcp: sim/core/proto/api.pb.go mcp-presets
	mkdir -p $(MCP_OUT)
	cd $(MCP_DIR) && go build --tags=with_db -o dist/wowsimmcp -ldflags="$(MCP_LDFLAGS)" .
	@echo "built $(MCP_OUT)/wowsimmcp"

# Claude Desktop runs on Windows and macOS, and cannot execute a Linux binary sitting in WSL
# without going through wsl.exe. Building the .exe removes that hop.
.PHONY: mcp-windows
mcp-windows: sim/core/proto/api.pb.go mcp-presets
	mkdir -p $(MCP_OUT)
	cd $(MCP_DIR) && GOOS=windows GOARCH=amd64 GOAMD64=v2 go build --tags=with_db -o dist/wowsimmcp.exe -ldflags="$(MCP_LDFLAGS)" .
	@echo "built $(MCP_OUT)/wowsimmcp.exe"

.PHONY: mcp-test
mcp-test: sim/core/proto/api.pb.go mcp-presets
	cd $(MCP_DIR) && GOARCH=amd64 go test --tags=with_db ./...

# Packs the Claude Desktop bundles: a zip carrying the server and a manifest describing it,
# which installs by being opened. One per platform, because a manifest names a single entry
# point and Claude Desktop reads compatibility.platforms to decide whether it can install at all.
#
# Set MCPB_VERSION to stamp a version on the manifests. Claude Desktop compares that field
# against what is installed to decide whether an update exists, so a release passes its tag and
# a local build leaves it at the constant in mcp/internal/bundle.
MCPB_VERSION ?=

.PHONY: mcp-bundle
mcp-bundle: sim/core/proto/api.pb.go mcp-presets
	rm -rf $(MCP_OUT)/mcpb-* $(MCP_OUT)/*.mcpb
	tools/mcp/pack_bundle.sh windows      windows amd64 $(MCP_OUT) $(MCPB_VERSION)
	tools/mcp/pack_bundle.sh arm64-darwin darwin  arm64 $(MCP_OUT) $(MCPB_VERSION)
	tools/mcp/pack_bundle.sh amd64-darwin darwin  amd64 $(MCP_OUT) $(MCPB_VERSION)
	@echo "open one with Claude Desktop to install it"

# Regenerates mcp/docs/TOOLS.md from the registry. Never edit that file by hand.
.PHONY: mcp-docs
mcp-docs: sim/core/proto/api.pb.go
	cd $(MCP_DIR) && go run --tags=with_db ./cmd/gendocs

sim/core/proto/api.pb.go: proto/*.proto
	@if go version -m "$$(command -v protoc-gen-go)" 2>/dev/null | grep -qE '^[[:space:]]+mod[[:space:]]+github\.com/golang/protobuf[[:space:]]'; then \
		echo "ERROR: your protoc-gen-go is the deprecated github.com/golang/protobuf plugin;"; \
		echo "it generates code that no longer builds against this repo's protobuf version."; \
		echo "Fix:  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"; \
		echo "then: rm -f sim/core/proto/*.pb.go && retry"; \
		exit 1; \
	fi
# Distro protoc packages (e.g. Ubuntu/WSL's protobuf-compiler) bundle a
# descriptor.proto whose go_package still points at the deprecated
# github.com/golang/protobuf path, which is no longer a dependency. Pin the
# mapping so common.proto's MessageOptions extension resolves to descriptorpb.
	protoc -I=./proto \
		--go_opt=Mgoogle/protobuf/descriptor.proto=google.golang.org/protobuf/types/descriptorpb \
		--go_out=./sim/core ./proto/*.proto

# Only useful for building the lib on a host platform that matches the target platform
.PHONY: locallib
locallib: sim/core/proto/api.pb.go
	go build -buildmode=c-shared -o wowsimtbc.so --tags=with_db ./sim/lib/library.go

.PHONY: nixlib
nixlib: sim/core/proto/api.pb.go
	GOOS=linux GOARCH=amd64 GOAMD64=v2 go build -buildmode=c-shared -o wowsimtbc-linux.so --tags=with_db ./sim/lib/library.go

.PHONY: winlib
winlib: sim/core/proto/api.pb.go
	GOOS=windows GOARCH=amd64 GOAMD64=v2 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -buildmode=c-shared -o wowsimtbc-windows.dll --tags=with_db ./sim/lib/library.go

.PHONY: simdb
simdb: sim/core/items/all_items.go sim/core/proto/api.pb.go

CLIENTDATA_SETTINGS := $(shell realpath ./tools/database/generator-settings.json)
CLIENTDATAPTR_SETTINGS := $(shell realpath ./tools/database/ptr-generator-settings.json)
CLIENTDATA_OUTPUT   := $(shell realpath ./tools/database/wowsims.db)

.PHONY: db
db:
	@echo "Extracting client data"
	go run ./tools/db2tool -s $(CLIENTDATA_SETTINGS) --output $(CLIENTDATA_OUTPUT)
	@echo "Running DBC generation tool"
	go run tools/database/gen_db/*.go -outDir=./assets -gen=db

.PHONY: ptrdb
ptrdb:
	@echo "Extracting client data"
	go run ./tools/db2tool -s $(CLIENTDATAPTR_SETTINGS) --output $(CLIENTDATA_OUTPUT)
	@echo "Running DBC generation tool"
	go run tools/database/gen_db/*.go -outDir=./assets -gen=db

sim/core/items/all_items.go: $(call rwildcard,tools/database,*.go) $(call rwildcard,sim/core/proto,*.go)
	@test -f tools/database/wowsims.db || { \
		echo "ERROR: tools/database/wowsims.db is missing (gitignored, produced by 'make db')."; \
		echo "Run 'make db' to extract it from a local WoW install."; \
		exit 1; }
	@test -f tools/db2tool/listfile.csv || { \
		echo "tools/db2tool/listfile.csv is missing, downloading it..."; \
		curl -fL -o tools/db2tool/listfile.csv https://github.com/wowdev/wow-listfile/releases/latest/download/community-listfile.csv; }
	go run tools/database/gen_db/*.go -outDir=./assets -gen=db

.PHONY: test
test: $(OUT_DIR)/lib.wasm binary_dist/dist.go
	GOARCH=amd64 go test --tags=with_db ./sim/...

.PHONY: update-tests
update-tests:
	find . -name "*.results" -type f -delete
	find . -name "*.results.tmp" -exec bash -c 'cp "$$1" "$${1%.results.tmp}".results' _ {} \;

# Performance harness. Take a baseline before changing anything, then compare against it:
#
#   make perf-baseline          # snapshot the tree as it stands
#   ... make the change ...
#   make perf-compare           # timings via benchstat, plus a DPS equivalence check
#
# PERF_LABEL names the snapshot under perf/ (gitignored), so several can coexist.
PERF_LABEL ?= current

.PHONY: perf-baseline
perf-baseline:
	tools/perf/capture.sh baseline

.PHONY: perf-snapshot
perf-snapshot:
	tools/perf/capture.sh $(PERF_LABEL)

.PHONY: perf-compare
perf-compare: perf-snapshot
	tools/perf/compare.sh baseline $(PERF_LABEL)

# Regenerates the merged CPU profile the optimization work is aimed at, and the one PGO consumes.
.PHONY: perf-profile
perf-profile:
	tools/perf/profile.sh

.PHONY: fmt
fmt: tsfmt
	gofmt -w ./sim
	gofmt -w ./tools

.PHONY: tsfmt
tsfmt:
	npx oxfmt ui

# one time setup to install pre-commit hook for gofmt and npm install needed packages
setup:
	cp pre-commit .git/hooks
	chmod +x .git/hooks/pre-commit
	! command -v air && curl -sSfL https://raw.githubusercontent.com/cosmtrek/air/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin || true

# Host a local server, for dev testing
.PHONY: host
host: air $(OUT_DIR)/.dirstamp node_modules
ifeq ($(WATCH), 1)
	ulimit -n 10240 && air -tmp_dir "/tmp" -build.include_ext "go,ts,js,html" -build.bin "npx" -build.args_bin "http-server $(OUT_DIR)/.." -build.cmd "make" -build.exclude_dir "dist,node_modules,tools"
else
	# Intentionally serve one level up, so the local site has 'tbc' as the first
	# directory just like github pages.
	npx http-server $(OUT_DIR)/..
endif

devmode: air devserver
ifeq ($(WATCH), 1)
	npx tsx vite.build-workers.mts & npx vite serve --host &
	air -tmp_dir "/tmp" -build.include_ext "go,proto" -build.args_bin "--usefs=true --launch=false --wasm=false" -build.bin "./wowsimtbc" -build.cmd "make devserver" -build.exclude_dir "assets,dist,node_modules,ui,tools"
else
	./wowsimtbc --usefs=true --launch=false --host=":3333"
endif

webworkers:
	npx tsx vite.build-workers.mts --watch=$(if $(WATCH),true,false)
