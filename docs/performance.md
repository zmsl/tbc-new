# Sim performance

Where the time goes, what has been done about it, and why the answer to "can we put this on a
GPU" is no.

All numbers below were measured with `tools/perf` on an i9-10850K (16 threads), `GOARCH=amd64`,
Go 1.25.4. Reproduce them with `make perf-baseline` and `make perf-profile`; see
[tools/perf/README.md](../tools/perf/README.md).

## The shape of the problem

A single 180-second iteration of the feral cat profile cost **4.63ms** before this work. Building
the environment for it cost roughly another 2.3ms, paid once per `RunSim` — which means once per
concurrency split and dozens of times across a stat weight run, not once per sim.

The engine is already parallel where parallelism is available. `runSimConcurrent`
(`sim/core/sim_concurrent.go`) splits the iterations across `runtime.NumCPU()` goroutines, and the
web build splits them across web workers. Monte Carlo iterations are independent, so this scales
close to linearly with cores. **The bottleneck was never the number of cores; it was what one
core spends its time on.**

A CPU profile of the event loop alone — `BenchmarkIteration`, which amortizes environment setup
away — answered that:

| | Share of iteration time |
| --- | --- |
| `APLRotation.DoNextAction` | **94%** |
| ├ `getNextAction` → `APLAction.IsReady` (evaluating conditions) | **87%** |
| │  ├ `APLValueAnd.GetBool` | 58% |
| │  ├ `APLValueCompare.GetBool` | 46% |
| │  └ `SpellCost.ApplyCostModifiers` | 21% |
| └ `Execute` → `Spell.Cast` | 10% |

Almost none of that is simulating anything. It is a tree-walking interpreter re-evaluating a
rotation whose *structure has not changed since the moment it was parsed*, several times per
in-game second, several million times per sim.

The feral cat priority list is the clearest case: 91 comparison nodes, 43 boolean `and` nodes, and
25 separate reads of a spell's current mana or energy cost, walked from the top on every rotation
decision.

## What changed

Every change below is bit-exact. The entire golden-file suite passes unmodified, which is the
strongest available proof that these are optimizations and not behaviour changes.

**Spell costs are cached** (`sim/core/spell.go`). `GetCurrentCost` recomputed the same value on
every read, and the expensive part was never the arithmetic — it was the walk out to
`spell.Unit.PseudoStats` that the arithmetic needs, two pointer hops into large structs, once per
read, across every spell the priority list mentions. The result is now cached on the `SpellCost`
itself and invalidated by the six spell-modifier sites in `spell_mod.go`, by
`Unit.InvalidateSpellCosts` wherever `PseudoStats.SpellCostPercentModifier` moves, and on
iteration reset.

**Comparison nodes resolve their operand type once** (`sim/core/apl_values_operators.go`).
`APLValueCompare.GetBool` called `value.lhs.Type()` through an interface on every single
evaluation, to choose a switch arm that is fixed once the rotation is parsed. It now resolves
that on first use and remembers it.

**Two- and three-operand `and`/`or` nodes are named rather than ranged over**, which covers
nearly every boolean node in a real rotation and lets the compiler drop the loop and its bounds
checks. Short-circuit order is unchanged.

**`AuraReference.Get` resolves its target once** (`sim/core/apl_helpers.go`) instead of twice.

### Result

| Spec | Per-iteration cost | Change |
| --- | --- | --- |
| Feral cat | 4.63ms → 3.48ms | **−25.0%** |
| Rogue | 3.28ms → 2.74ms | **−16.6%** |
| Feral bear | 410µs → 364µs | **−11.2%** |
| Elemental shaman | 1.11ms → 1.03ms | −7.6% |
| Warlock | 172µs → 166µs | −3.4% |

The gain tracks how much rotation logic a spec has, which is exactly what the profile predicted.

## Build options that did not pay

Both were measured rather than assumed, and both were rejected. They are recorded here so the
next person does not spend the afternoon finding out again.

**Profile-guided optimization: no effect.** A merged profile across all five benchmarked specs
(`make perf-profile`), fed back through `go test -pgo`, produced no significant change.

The mechanism is the more convincing evidence, and it predicts this. PGO's main lever on code
like ours is devirtualization: where a profile shows one concrete type dominating an interface
call site, the compiler emits a type check plus a direct, inlinable call and falls back to the
indirect call otherwise. That needs a *dominant* callee. `APLValue` has dozens of implementations
and the hot call sites -- `lhs.GetFloat`, `val.GetBool` -- genuinely see many of them, so there
is no majority type to bet on. There is no `default.pgo` in the repository, deliberately.

**`GOAMD64=v3`: no difference, and the first measurement of it was wrong.** Six runs put rogue
6.4% slower at p=0.004, which looked conclusive. It was not: twelve runs under matched conditions
put the same comparison at **p=0.84**, and disassembly settles it -- `go tool objdump` gives
byte-identical mnemonics for every hot function under v2 and v3, differing only in branch target
addresses, and there is not a single AVX instruction anywhere in the APL value code. There is
nothing in this workload to vectorize, so v3 has nothing to do; the apparent regression was code
laid out at different addresses, which is a well-known way to move a benchmark by several percent
without changing an instruction. The makefile stays on `v2` because v3 buys nothing, not because
it costs anything.

For the record, the full golden-file suite also passes under `GOAMD64=v3` with zero diffs, so the
concern that a wider instruction set might fuse a multiply-add and shift results does not apply
here -- Go emits no FMA in this code either way.

**GC tuning: nothing to tune.** Garbage collection does not appear anywhere in the top of the
event-loop profile. It shows up only in profiles dominated by environment construction, which is
a setup cost, not a per-iteration one.

### An ordering experiment that failed

`APLAction.IsReady` evaluates `condition && impl.IsReady`. Since `CanCastOrQueue` is a pure
predicate, the two can be swapped, and checking spell readiness first looked like it should skip
whole condition trees for spells that are on cooldown or unaffordable.

It made feral cat **41% slower** (p=0.002) and did nothing measurable for rogue. Conditions are
on average *cheaper* than the readiness check, and they fail early far more often, so the
existing order is already the better one. Recorded here because the argument for swapping is
persuasive and wrong.

## Why not CUDA

The question comes up because 10,000 iterations sounds like 10,000 independent tasks, which
sounds like a GPU. Four things get in the way, in increasing order of how hard they are to argue
with.

**The parallelism is already claimed.** Iteration-level independence is what the goroutine split
and the web worker pool already exploit. A GPU would have to accelerate the *inside* of one
iteration, and that is a discrete-event loop: interface dispatch, closure calls, pointer chasing
through a graph of auras and spells, and branches whose direction is decided by a random number.

**SIMT wants lockstep, and this diverges immediately.** Thirty-two lanes execute the same
instruction or they stall. Iteration 2 parts company with iteration 1 on the first crit roll and
never reconverges. The work is branchy scalar logic with no vectorizable inner loop.

**Reach.** The overwhelming majority of users run the sim in a browser. CUDA is not reachable
there at all. WebGPU is, and has the same divergence problem plus a per-thread state budget that
a raid's working set does not fit into without spilling to global memory, which erases the win.

**It aims at the wrong target.** We were not core-starved. We were spending most of each core
re-interpreting a static tree — which is why the changes above bought 25% on the heaviest spec
without touching parallelism at all.

For completeness: SimulationCraft has fielded this same request for fifteen years and has not
done it, for the same reasons.

### What would actually be worth accelerating

Not the event loop. If a hardware-acceleration spike is ever run again, the candidates are the
workloads that are *not* discrete-event simulation:

- **Bulk sim** (`ui/core/components/individual_sim_ui/bulk_tab.tsx`) — hundreds of gear
  permutations. Almost certainly worker-starved rather than compute-bound; see the CPU-core
  setting below before reaching for anything exotic.
- **The reforge optimizer** (`ui/worker/reforge_worker.ts`) — an LP solve, which is a genuinely
  different shape of problem from the sim.

The bar for either: **3x over the 16-thread CPU baseline end to end**, including transfer and
result aggregation, not kernel time in isolation.

## What is left

After the changes above, the same profile still puts 88% of an iteration inside `DoNextAction`.
`SpellCost.ApplyCostModifiers` has vanished from it entirely; what remains is spread across the
interpreter itself:

| | Flat share |
| --- | --- |
| `APLValueCompare.GetBool` | 12.2% |
| `APLValueAnd.GetBool` | 12.0% |
| `APLAction.IsReady` | 8.4% |
| `APLValueVariableRef.GetBool` | 4.7% |
| `APLValueOr.GetBool` | 4.2% |
| `UnitReference.Get` / `AuraReference.Get` | 6.3% combined |

The next real step is memoizing shared subexpressions within a single rotation decision. The
machinery already exists -- `evalGeneration` (`sim/core/apl.go`) -- but it is wired only to
variable references. A priority list routinely asks the same question from several entries in
one pass, and each entry pays for it again.

Environment construction is **not** worth chasing, which is worth writing down because it looks
like it should be. It costs about 1.0ms per `RunSim` -- 0.75ms building the environment, of which
roughly 60% is parsing the APL, plus 0.28ms of presim -- and it is paid once per `RunSim`, which
means once per concurrency split and once per stat-weight sub-sim. Against a split running even a
few hundred iterations at 3.5ms each, that is well under 1%. Reusing environments across splits
was on the plan for this work and was dropped after measurement: it is the riskiest change
available (shared mutable state leaking between splits produces wrong answers that still look
plausible) in exchange for a fraction of a percent.

## The browser

The web sim runs the same Go engine compiled to WebAssembly, so everything above applies to it
unchanged. Its own ceiling is a different one: it defaulted to `min(4, cores/2)` workers, which
leaves most of a modern machine idle. Each worker holds its own wasm heap and copy of the item
database, so the cautious default is not unreasonable for an unknown machine — but it is now
possible to opt out of it, with **Unlock All CPU Cores** in the settings menu.

The desktop app never had this limit: it runs the native sim binary as a sidecar, at full
`NumCPU`.

## Measuring a change

Never trust a timing without the equivalence check beside it. `make perf-baseline` before,
`make perf-compare` after; the DPS comparison is paired by seed, so a change that does not touch
logic produces a difference of exactly zero, and anything else has to argue with the noise.

`make test` is the same gate at lower iteration counts, and it is not optional. An optimization
that changes DPS is not an optimization.
