#!/usr/bin/env bash
#
# Captures one performance snapshot: benchmark timings and, optionally, high-iteration DPS
# figures for the equivalence check. Snapshots go under perf/<label>/, which is gitignored.
#
#   tools/perf/capture.sh <label> [--quick]
#
# Take a "baseline" snapshot before changing anything, then a second one afterwards and hand
# both to tools/perf/compare.sh. Run from the repository root; the makefile's perf targets do.
#
# --quick skips the DPS snapshot, which is the slow half. Use it while iterating; take a full
# snapshot before asking whether a change is safe to keep.

set -euo pipefail

if [ "$#" -lt 1 ]; then
	echo "usage: $0 <label> [--quick]" >&2
	exit 2
fi

label=$1
quick=${2:-}

# The five benchmarked specs, chosen to span the shapes the engine has to handle: two melee
# with different resource systems, a caster, an energy/combo-point class with the longest
# priority list, and a pet class that drives a second unit through the same event loop.
packages=(
	./sim/druid/feralcat
	./sim/druid/feralbear
	./sim/shaman/elemental
	./sim/rogue
	./sim/warlock
)

# Test names paired with the package that holds them, for the DPS snapshot.
dps_targets=(
	"./sim/druid/feralcat:TestFeralCat"
	"./sim/druid/feralbear:TestFeralBear"
	"./sim/shaman/elemental:TestElemental"
	"./sim/rogue:TestRogue"
	"./sim/warlock:TestAffliction"
)

# Seeds swept by the DPS snapshot. Five is enough to put a standard error on the mean without
# the snapshot taking longer than the change it is checking.
seeds=(101 202 303 404 505)
iterations=${WOWSIM_PERF_ITERATIONS:-10000}

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
out="$repo_root/perf/$label"

mkdir -p "$out"

# GOARCH is pinned for the same reason the golden files pin it: results are float-sensitive and
# amd64 is what they were generated on. GOMAXPROCS=1 keeps one benchmark from being measured
# against a background GC worker on another core.
export GOARCH=amd64

echo "==> timings -> $out/bench.txt"
GOMAXPROCS=1 go test --tags=with_db "${packages[@]}" \
	-run XXX -bench 'Benchmark(Simulate|Iteration)' -benchtime 200x -count 6 \
	| tee "$out/bench.txt"

if [ "$quick" = "--quick" ]; then
	echo "==> skipping DPS snapshot (--quick)"
	exit 0
fi

echo "==> DPS at $iterations iterations across ${#seeds[@]} seeds -> $out/dps.txt"
: > "$out/dps.txt"

for target in "${dps_targets[@]}"; do
	pkg=${target%%:*}
	test_name=${target##*:}
	for seed in "${seeds[@]}"; do
		# The suite always writes <name>.results.tmp, pass or fail, so the run's exit status
		# does not matter here -- at these iteration counts it will differ from the committed
		# golden file by design, and that difference is the measurement.
		WOWSIM_PERF_ITERATIONS="$iterations" WOWSIM_PERF_SEED="$seed" \
			go test --tags=with_db "$pkg" -run "$test_name/Average/^\$" -timeout 60m >/dev/null 2>&1 || true

		tmp="$repo_root/${pkg#./}/$test_name.results.tmp"
		if [ ! -f "$tmp" ]; then
			echo "no results written for $test_name at seed $seed" >&2
			exit 1
		fi

		# dps_results entries are prototext: a key line followed by the value block. Pair each
		# Average key with the dps line that follows it.
		awk -v seed="$seed" '
			/key: "/ { key = $0; sub(/^.*key: "/, "", key); sub(/".*$/, "", key); next }
			/^[[:space:]]*dps:/ && key != "" { print key "\t" seed "\t" $2; key = "" }
		' "$tmp" >> "$out/dps.txt"

		# Delete it immediately. These hold numbers from a deliberately non-standard iteration
		# count, and `make update-tests` promotes every .results.tmp it finds into a golden
		# file -- leaving one behind would silently corrupt the goldens on someone's next
		# unrelated test update.
		rm -f "$tmp"
	done
done

echo "==> snapshot complete: $out"
