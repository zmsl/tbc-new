# Performance harness

Answers two questions about a change to the sim engine: **did it get faster**, and **did the
results move**. The second matters more. An optimization that changes DPS is not an
optimization, it is a bug with a good excuse.

## Workflow

```sh
make perf-baseline          # snapshot the tree before you touch it
# ... make the change ...
make perf-compare           # timings via benchstat, plus the equivalence check
```

Snapshots live under `perf/`, which is gitignored — a timing from one machine says nothing on
another, so they are never shared through git.

While iterating, `tools/perf/capture.sh <label> --quick` skips the slow half and gives you
timings only. Take a full snapshot before deciding a change is safe to keep.

## What it measures

**Timings** come from two benchmarks per spec, across five specs chosen to span the shapes the
engine handles — two melee with different resource systems, a caster, an energy/combo-point
class with the longest priority list, and a pet class that drives a second unit:

| Benchmark | What it reports |
| --- | --- |
| `BenchmarkSimulate` | Environment construction plus one iteration. Setup is paid once per `RunSim`, which means once per concurrency split and dozens of times across a stat weight run — a real cost with its own number. |
| `BenchmarkIteration` | The event loop alone, with setup amortized away over `b.N` iterations. |

Both use each spec's real APL preset rather than `TypeSimple`. The APL interpreter is by far the
largest cost in the profile, so a benchmark that bypasses it measures the wrong thing.

**Results** come from each spec's `Average` test, run at a high iteration count across five
seeds. `WOWSIM_PERF_ITERATIONS` and `WOWSIM_PERF_SEED` (honored in `sim/core/test_utils.go`)
drive this; unset, they change nothing about a normal test run.

## Reading the equivalence check

Comparison is **paired by seed**: the same seed on both builds means the same stream of random
numbers, so a change that does not touch logic produces a difference of exactly zero.

| Verdict | Meaning |
| --- | --- |
| `IDENTICAL` | Every seed matched to the last decimal place. The change is provably logic-free. This is what an optimization should produce. |
| `OK` | The mean moved less than 0.1% and by less than two standard errors. Consistent with noise from a deliberate change in evaluation order. |
| `SHIFTED` | The mean moved further than that. This is a logic change. Find out why before deciding anything else. |

`IDENTICAL` is the bar for anything that claims to be a pure optimization, and it is also what
`make test` checks for free — the golden files are the same gate, at lower iteration counts.
The equivalence check exists for the narrower case where evaluation order legitimately changes
and the golden files therefore have to be re-baselined; it is what tells you the re-baseline is
noise rather than a regression.

## Profiling

```sh
make perf-profile                       # merged CPU profile across all five specs
go tool pprof -top perf/cpu.prof
```

The profile is merged across specs deliberately: one gathered from a single spec biases the
optimizer toward that spec's code paths. The same file is what Go's profile-guided optimization
consumes as `default.pgo`.
