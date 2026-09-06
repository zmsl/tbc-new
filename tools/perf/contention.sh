#!/usr/bin/env bash
#
# Measures how much throughput the sim loses when several run at once.
#
#   tools/perf/contention.sh [processes] [spec-package]
#
# The rest of this harness measures one sim on one core. That is not how anyone runs the sim:
# every real run splits across GOMAXPROCS goroutines, so what a user waits for is the *concurrent*
# throughput, and that turns out to be materially worse than the single-core number suggests.
#
# Each sim is run as its own process pinned to one core rather than as goroutines, so the only
# thing being shared is the memory system -- no scheduler effects, no GC coordination between
# them.
#
# BenchmarkRnds/Addition is the control. It is arithmetic on a couple of registers with no working
# set to speak of, so whatever it loses under N-way load is the CPU dropping its all-core turbo
# bin, not memory. Subtracting it from the sim's loss leaves the part attributable to cache and
# bandwidth. Without that control the two are indistinguishable, and the raw number is roughly an
# order of magnitude too pessimistic about memory.
#
# Run from the repository root.

set -euo pipefail

procs=${1:-8}
pkg=${2:-./sim/druid/feralcat}

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

export GOARCH=amd64
export GOMAXPROCS=1

# shellcheck disable=SC2016
# Full precision on the way out: the sim is measured in milliseconds and the control in single
# nanoseconds, so rounding here would erase the control entirely.
mean_of() { awk '{ s += $3; n++ } END { if (n) printf "%.4f", s / n }' "$@"; }

run_solo() {
	go test --tags=with_db "$1" -run XXX -bench "$2" -benchtime "$3" -count 3 2>&1 | grep -E "^$2" > "$out/solo.txt"
	mean_of "$out/solo.txt"
}

run_parallel() {
	local i
	for ((i = 1; i <= procs; i++)); do
		go test --tags=with_db "$1" -run XXX -bench "$2" -benchtime "$3" -count 3 2>&1 | grep -E "^$2" > "$out/par_$i.txt" &
	done
	wait
	mean_of "$out"/par_*.txt
}

cd "$repo_root"

echo "== sim ($pkg), 1 process vs $procs =="
sim_solo=$(run_solo "$pkg" BenchmarkIteration 300x)
sim_par=$(run_parallel "$pkg" BenchmarkIteration 300x)

echo "== control (arithmetic, no working set), 1 process vs $procs =="
ctl_solo=$(run_solo ./sim/core "BenchmarkRnds/Addition" 3000000x)
ctl_par=$(run_parallel ./sim/core "BenchmarkRnds/Addition" 3000000x)

awk -v ss="$sim_solo" -v sp="$sim_par" -v cs="$ctl_solo" -v cp="$ctl_par" -v n="$procs" '
BEGIN {
	sim = (sp / ss - 1) * 100
	ctl = (cp / cs - 1) * 100
	printf "\n%-28s %12s %12s %10s\n", "", "1 process", n " procs", "penalty"
	printf "%-28s %12s %12s %9.1f%%\n", "sim iteration (ns/op)", ss, sp, sim
	printf "%-28s %12s %12s %9.1f%%\n", "control (ns/op)", cs, cp, ctl
	printf "\n  frequency (from control):  %5.1f%%\n", ctl
	printf "  memory system:             %5.1f%%\n", sim - ctl
}'
