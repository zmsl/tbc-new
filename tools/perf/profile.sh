#!/usr/bin/env bash
#
# Builds a merged CPU profile across the benchmarked specs. Two consumers:
#
#   - reading, to find what to optimize next (go tool pprof -top perf/cpu.prof)
#   - Go's profile-guided optimization, which reads default.pgo from the main package
#
# BenchmarkIteration is the subject rather than BenchmarkSimulate because it amortizes
# environment construction away, so the profile describes the event loop rather than character
# setup. Profiles are merged across specs on purpose: a profile gathered from one spec biases
# inlining toward that spec's code paths.
#
#   tools/perf/profile.sh [benchtime]
#
# Run from the repository root; the makefile's perf-profile target does.

set -euo pipefail

benchtime=${1:-2000x}

packages=(
	./sim/druid/feralcat
	./sim/druid/feralbear
	./sim/shaman/elemental
	./sim/rogue
	./sim/warlock
)

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
out="$repo_root/perf/profiles"

mkdir -p "$out"
rm -f "$out"/*.prof "$out"/*.test

export GOARCH=amd64

# -cpuprofile takes a single package at a time, so each spec is profiled separately and the
# results are merged below.
for pkg in "${packages[@]}"; do
	name=$(basename "$pkg")
	echo "==> profiling $pkg"
	go test --tags=with_db "$pkg" \
		-run XXX -bench BenchmarkIteration -benchtime "$benchtime" \
		-cpuprofile "$out/$name.prof" -o "$out/$name.test" >/dev/null
done

merged="$repo_root/perf/cpu.prof"
go tool pprof -proto "$out"/*.prof > "$merged" 2>/dev/null

echo
echo "merged profile -> perf/cpu.prof"
echo "  read it:   go tool pprof -top perf/cpu.prof"
echo "  use it:    cp perf/cpu.prof default.pgo   (Go picks it up automatically)"
