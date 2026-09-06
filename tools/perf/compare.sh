#!/usr/bin/env bash
#
# Compares two snapshots taken by tools/perf/capture.sh: timings via benchstat, and DPS via a
# paired equivalence test that answers the only question that matters about an optimization --
# did it change how fast the sim runs without changing what the sim says?
#
#   tools/perf/compare.sh <baseline-label> <current-label>
#
# Run from the repository root; the makefile's perf-compare target does.

set -euo pipefail

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <baseline-label> <current-label>" >&2
	exit 2
fi

baseline=$1
current=$2

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
base_dir="$repo_root/perf/$baseline"
curr_dir="$repo_root/perf/$current"

for dir in "$base_dir" "$curr_dir"; do
	if [ ! -d "$dir" ]; then
		echo "no snapshot at $dir -- take one with tools/perf/capture.sh" >&2
		exit 1
	fi
done

echo "===== timings ====="
if command -v benchstat >/dev/null 2>&1; then
	benchstat "$base_dir/bench.txt" "$curr_dir/bench.txt"
else
	echo "benchstat not found; install it with:"
	echo "    go install golang.org/x/perf/cmd/benchstat@latest"
	echo "Falling back to raw ns/op:"
	paste <(grep -h '^Benchmark' "$base_dir/bench.txt") <(grep -h '^Benchmark' "$curr_dir/bench.txt")
fi

if [ ! -f "$base_dir/dps.txt" ] || [ ! -f "$curr_dir/dps.txt" ]; then
	echo
	echo "===== results ====="
	echo "one or both snapshots were taken with --quick; no DPS comparison to make."
	exit 0
fi

echo
echo "===== results ====="

# Paired by (test, seed): the same seed on both builds is the same stream of random numbers, so
# a bit-exact optimization produces a difference of exactly zero and anything else has to argue
# with the noise. Verdicts:
#
#   IDENTICAL  every seed matched to the last decimal place -- the change is provably logic-free
#   OK         the mean moved less than 0.1% and by less than 2 standard errors: noise
#   SHIFTED    the mean moved further than that, which is a logic change, not a speedup
awk -F'\t' '
	FILENAME == ARGV[1] { base[$1 "\t" $2] = $3; next }
	{
		key = $1 "\t" $2
		if (!(key in base)) next
		test = $1
		d = $3 - base[key]
		n[test]++
		sumB[test] += base[key]
		sumD[test] += d
		sumDD[test] += d * d
		if (d != 0) nonzero[test]++
	}
	END {
		printf "%-34s %5s %14s %12s %10s  %s\n", "TEST", "SEEDS", "BASELINE DPS", "MEAN DELTA", "REL", "VERDICT"
		failed = 0
		for (t in n) {
			cnt = n[t]
			meanB = sumB[t] / cnt
			meanD = sumD[t] / cnt
			rel = meanB != 0 ? (meanD / meanB) * 100 : 0
			absD = meanD < 0 ? -meanD : meanD
			absRel = rel < 0 ? -rel : rel

			# Sample standard deviation of the paired differences, then its standard error.
			var = cnt > 1 ? (sumDD[t] - cnt * meanD * meanD) / (cnt - 1) : 0
			if (var < 0) var = 0
			se = cnt > 1 ? sqrt(var / cnt) : 0

			if (!(t in nonzero)) {
				verdict = "IDENTICAL"
			} else if (absRel <= 0.1 && absD <= 2 * se) {
				verdict = "OK"
			} else {
				verdict = "SHIFTED"
				failed = 1
			}
			printf "%-34s %5d %14.3f %12.4f %9.4f%%  %s\n", t, cnt, meanB, meanD, rel, verdict
		}
		if (failed) {
			print ""
			print "At least one result SHIFTED. That is a logic change, not a speedup -- find out why"
			print "before deciding whether to re-baseline the golden files."
			exit 1
		}
	}
' "$base_dir/dps.txt" "$curr_dir/dps.txt"
