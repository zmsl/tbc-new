#!/usr/bin/env bash
#
# Packs one .mcpb: a zip carrying the MCP server, the manifest describing it, and an icon.
# Claude Desktop installs a bundle by opening it, and reads compatibility.platforms to decide
# whether it can run on this machine, so there is one bundle per platform rather than one
# universal one.
#
#   tools/mcp/pack_bundle.sh <target> <goos> <goarch> <out-dir> [version]
#
# <target> is a name the bundle package knows (windows, arm64-darwin, amd64-darwin). Run from
# the repository root; the makefile's mcp-bundle target does.

set -euo pipefail

if [ "$#" -lt 4 ]; then
	echo "usage: $0 <target> <goos> <goarch> <out-dir> [version]" >&2
	exit 2
fi

target=$1
goos=$2
goarch=$3
out_dir=$(mkdir -p "$4" && cd "$4" && pwd)
version=${5:-}

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
staging="$out_dir/mcpb-$target"

rm -rf "$staging"
mkdir -p "$staging"

# The manifest is rendered first so that the binary can be placed at whatever entry point it
# declares. Reading the path back out means the bundle package stays the only place that decides
# the layout, and a bundle whose manifest points somewhere the binary is not cannot be built.
manifest_args=(-target "$target" -o "$staging/manifest.json")
if [ -n "$version" ]; then
	manifest_args+=(-version "$version")
fi
(cd "$repo_root/mcp" && go run --tags=with_db ./cmd/genmanifest "${manifest_args[@]}")

entry_point=$(sed -n 's/^[[:space:]]*"entry_point": "\(.*\)",\?$/\1/p' "$staging/manifest.json")
if [ -z "$entry_point" ]; then
	echo "$0: no entry_point in $staging/manifest.json" >&2
	exit 1
fi

mkdir -p "$staging/$(dirname "$entry_point")"
# v2 matches the baseline the release binaries are built against; it is meaningless off amd64.
goamd64=""
if [ "$goarch" = "amd64" ]; then
	goamd64="v2"
fi
(cd "$repo_root/mcp" && GOOS="$goos" GOARCH="$goarch" GOAMD64="$goamd64" go build \
	--tags=with_db \
	-o "$staging/$entry_point" \
	-ldflags="-X 'main.Version=${version:-development}' -s -w" .)

cp "$repo_root/assets/favicon_io/android-chrome-512x512.png" "$staging/icon.png"

bundle="$out_dir/wowsimmcp-$target.mcpb"
rm -f "$bundle"
# zip preserves the executable bit, which the macOS bundles need to run at all once unpacked.
(cd "$staging" && zip -qr "$bundle" .)

echo "packed $bundle"
