package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

const (
	defaultIterations = 2000
	// Above this a run is long enough that a client is likely to time out waiting for it, so it
	// has to go through the job tools instead.
	maxSyncIterations  = 20_000
	maxAsyncIterations = 1_000_000
)

type simRunInput struct {
	setupInput

	Iterations int32 `json:"iterations,omitempty" jsonschema:"fights to simulate. Defaults to 2000, which gives roughly half a percent of error on DPS. Above 20000 requires async."`
	Seed       int64 `json:"seed,omitempty" jsonschema:"RNG seed. Defaults to a fixed value, so repeating a call returns exactly the same numbers; change it to check a result is not a fluke of one seed."`
	Async      bool  `json:"async,omitempty" jsonschema:"start the run in the background and return a job id to poll with job_status"`
	SpellLimit int   `json:"spellLimit,omitempty" jsonschema:"how many spells to include in the damage breakdown. Defaults to 12."`
}

type simRunOutput struct {
	Result  *engine.ResultSummary `json:"result,omitempty" jsonschema:"the simulation result, when it ran synchronously"`
	Job     *jobs.Record          `json:"job,omitempty" jsonschema:"the job that was started, when async was set. Poll it with job_status and collect it with job_result."`
	Link    string                `json:"link" jsonschema:"share link for the setup that was simulated"`
	Summary settingsSummary       `json:"summary" jsonschema:"what was simulated"`
	Notes   []string              `json:"notes,omitempty" jsonschema:"which defaults were applied while assembling the setup"`
}

func simRun(config engine.Config, store jobs.Store) spec.Entry {
	return spec.Tool[simRunInput, simRunOutput]{
		Name:    "sim_run",
		Title:   "Run a simulation",
		Summary: "Simulates a setup and reports DPS with its error bars and a per-spell breakdown.",
		Details: "Results are deterministic: the same arguments produce the same numbers, because the seed is\n" +
			"fixed unless you change it. Compare two setups by running both at the same seed and iteration\n" +
			"count, and treat a gap smaller than the two standard errors as noise rather than a difference.\n\n" +
			"2000 iterations takes well under a second and is enough for most questions. Use async for\n" +
			"anything above 20000, then poll job_status.",
		Examples: []spec.Example{
			{Description: "sim the smite priest's phase 3 set", Args: `{"spec": "SmitePriest", "gearSet": "p3"}`},
			{Description: "sim a setup someone shared, precisely", Args: `{"link": "https://wowsims.com/tbc/priest/smite/#eJys...", "iterations": 50000, "async": true}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input simRunInput) (*mcp.CallToolResult, simRunOutput, error) {
			var output simRunOutput

			settings, notes, err := input.resolve(config)
			if err != nil {
				return nil, output, err
			}

			iterations := input.Iterations
			if iterations <= 0 {
				iterations = defaultIterations
			}
			limit := int32(maxSyncIterations)
			if input.Async {
				limit = maxAsyncIterations
			}
			if iterations > limit {
				return nil, output, fmt.Errorf("%d iterations is above the limit of %d; %s", iterations, limit,
					map[bool]string{true: "split the run", false: "set async to run it in the background"}[input.Async])
			}

			simRequest, err := core.BuildRaidSimRequest(settings, core.SimRequestOptions{
				Iterations: iterations,
				RandomSeed: input.Seed,
			})
			if err != nil {
				return nil, output, err
			}

			output.Summary = summarize(settings)
			output.Notes = notes
			if output.Link, err = shareLink(config, settings); err != nil {
				return nil, output, err
			}

			if input.Async {
				label := fmt.Sprintf("%s, %d iterations", output.Summary.Spec, iterations)
				record, err := store.Start("sim", label, output.Link, simRequest)
				if err != nil {
					return nil, output, err
				}
				output.Job = &record
				return nil, output, nil
			}

			result := core.RunRaidSimConcurrent(simRequest)
			if result.Error != nil {
				return nil, output, fmt.Errorf("simulation failed: %s", result.Error.Message)
			}

			summary := engine.SummarizeResult(simRequest, result, input.SpellLimit)
			output.Result = &summary
			return nil, output, nil
		},
	}
}

type statWeightsInput struct {
	setupInput

	Iterations int32    `json:"iterations,omitempty" jsonschema:"iterations per stat. Defaults to 1000. Weights are a difference between two sims, so they need more iterations than a single result to settle."`
	Seed       int64    `json:"seed,omitempty" jsonschema:"RNG seed. Defaults to a fixed value."`
	Stats      []string `json:"stats,omitempty" jsonschema:"stats to weigh, e.g. [\"SpellDamage\", \"SpellHitRating\"]. Defaults to a sensible set for the class."`
}

type statWeightsOutput struct {
	Weights map[string]statWeight `json:"weights" jsonschema:"DPS gained per point of each stat, with its error"`
	Link    string                `json:"link" jsonschema:"share link for the setup that was weighed"`
	Summary settingsSummary       `json:"summary" jsonschema:"what was weighed"`
	Notes   []string              `json:"notes,omitempty" jsonschema:"which defaults were applied"`
}

type statWeight struct {
	Weight float64 `json:"weight" jsonschema:"DPS per point of this stat"`
	Stdev  float64 `json:"stdev" jsonschema:"uncertainty in the weight; a weight smaller than this is indistinguishable from zero"`
	EP     float64 `json:"ep" jsonschema:"value relative to the reference stat, which is the class's main damage stat"`
}

const maxStatWeightIterations = 20_000

func statWeights(config engine.Config) spec.Entry {
	return spec.Tool[statWeightsInput, statWeightsOutput]{
		Name:    "sim_stat_weights",
		Title:   "Compute stat weights",
		Summary: "Measures how much DPS each point of each stat is worth for a setup.",
		Details: "Use the weights to rank gear cheaply -- score candidate items by their stats -- before spending\n" +
			"simulations on the few that look best. Weights are specific to the setup that produced them:\n" +
			"they shift with gear, talents and fight length, so recompute after a significant change.\n\n" +
			"This runs two simulations per stat, so it costs several times a single sim_run.",
		Examples: []spec.Example{
			{Description: "weights for a phase 3 smite priest", Args: `{"spec": "SmitePriest", "gearSet": "p3"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input statWeightsInput) (*mcp.CallToolResult, statWeightsOutput, error) {
			var output statWeightsOutput

			settings, notes, err := input.resolve(config)
			if err != nil {
				return nil, output, err
			}

			iterations := input.Iterations
			if iterations <= 0 {
				iterations = 1000
			}
			if iterations > maxStatWeightIterations {
				return nil, output, fmt.Errorf("%d iterations per stat is above the limit of %d", iterations, maxStatWeightIterations)
			}

			simRequest, err := core.BuildRaidSimRequest(settings, core.SimRequestOptions{
				Iterations: iterations,
				RandomSeed: input.Seed,
			})
			if err != nil {
				return nil, output, err
			}

			weighted, reference, err := statsToWeigh(settings, input.Stats)
			if err != nil {
				return nil, output, err
			}

			result := core.StatWeights(&proto.StatWeightsRequest{
				Player:          simRequest.Raid.Parties[0].Players[0],
				RaidBuffs:       simRequest.Raid.Buffs,
				PartyBuffs:      simRequest.Raid.Parties[0].Buffs,
				Debuffs:         simRequest.Raid.Debuffs,
				Encounter:       simRequest.Encounter,
				SimOptions:      simRequest.SimOptions,
				Tanks:           simRequest.Raid.Tanks,
				StatsToWeigh:    weighted,
				EpReferenceStat: reference,
			})
			if result.Error != nil {
				return nil, output, fmt.Errorf("stat weights failed: %s", result.Error.Message)
			}

			output.Weights = namedWeights(result.Dps, weighted)
			output.Summary = summarize(settings)
			output.Notes = notes
			if output.Link, err = shareLink(config, settings); err != nil {
				return nil, output, err
			}
			return nil, output, nil
		},
	}
}

// Which stats are worth weighing depends on the class: weighing strength for a priest wastes two
// simulations to prove it is zero.
func statsToWeigh(settings *proto.IndividualSimSettings, requested []string) ([]proto.Stat, proto.Stat, error) {
	if len(requested) > 0 {
		var stats []proto.Stat
		for _, name := range requested {
			value, ok := proto.Stat_value["Stat"+name]
			if !ok {
				return nil, proto.Stat_StatStrength, fmt.Errorf("unknown stat %q", name)
			}
			stats = append(stats, proto.Stat(value))
		}
		return stats, stats[0], nil
	}

	if isCaster(settings.Player.Class) {
		return []proto.Stat{
			proto.Stat_StatIntellect,
			proto.Stat_StatSpirit,
			proto.Stat_StatSpellDamage,
			proto.Stat_StatSpellHitRating,
			proto.Stat_StatSpellCritRating,
			proto.Stat_StatSpellHasteRating,
			proto.Stat_StatMP5,
		}, proto.Stat_StatSpellDamage, nil
	}

	return []proto.Stat{
		proto.Stat_StatStrength,
		proto.Stat_StatAgility,
		proto.Stat_StatAttackPower,
		proto.Stat_StatMeleeHitRating,
		proto.Stat_StatMeleeCritRating,
		proto.Stat_StatMeleeHasteRating,
		proto.Stat_StatExpertiseRating,
		proto.Stat_StatArmorPenetration,
	}, proto.Stat_StatAttackPower, nil
}

func isCaster(class proto.Class) bool {
	switch class {
	case proto.Class_ClassMage, proto.Class_ClassWarlock, proto.Class_ClassPriest:
		return true
	}
	return false
}

func namedWeights(weights *proto.StatWeightValues, stats []proto.Stat) map[string]statWeight {
	named := map[string]statWeight{}
	if weights == nil {
		return named
	}

	for _, stat := range stats {
		index := int(stat)
		if index >= len(weights.Weights.GetStats()) {
			continue
		}
		named[trimEnum(stat.String(), "Stat")] = statWeight{
			Weight: round(weights.Weights.Stats[index]),
			Stdev:  round(weights.WeightsStdev.GetStats()[index]),
			EP:     round(weights.EpValues.GetStats()[index]),
		}
	}
	return named
}
