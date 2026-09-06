// Package prompts holds the multi-step workflows.
//
// A search that runs over several tool calls lives here rather than inside a tool, and that is a
// deliberate division: the server stays a set of pure, bounded operations, while the strategy --
// which candidates to try, when to stop, when to spend more iterations -- runs in the agent,
// where it is visible and can be steered mid-way.
package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
)

// Entries lists every prompt the server exposes.
func Entries(config engine.Config) []spec.Entry {
	return []spec.Entry{
		findBiS(),
		bestInBags(),
		compareRotations(),
	}
}

func findBiS() spec.Entry {
	return spec.Prompt{
		Name:    "find_bis",
		Title:   "Find the best gear for a phase",
		Summary: "A procedure for finding the strongest gear set a spec can assemble by a given raid phase.",
		Details: "Searching every combination is hopeless -- a handful of candidates in each of sixteen slots is\n" +
			"already more sets than could be simulated in a lifetime -- so the work is narrowing, not\n" +
			"enumerating.",
		Arguments: []spec.Argument{
			{Name: "spec", Description: "the spec to gear up, e.g. SmitePriest", Required: true},
			{Name: "phase", Description: "the raid phase whose items are available, 1 to 5", Required: true},
			{Name: "encounter", Description: "which fight to optimise for: ShortSingleTarget, LongSingleTarget or LongMultiTarget"},
		},
		Handler: func(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			arguments := request.Params.Arguments
			specName := argument(arguments, "spec", "the spec")
			phase := argument(arguments, "phase", "the target phase")
			encounter := argument(arguments, "encounter", "LongSingleTarget")

			return message("Find the best gear set for %s available by phase %s, fighting %s.\n\n"+
				"Work like this:\n\n"+
				"1. Start from the best checked-in gear set as a baseline: `settings_create` with that spec, then `sim_run` on the link. That is the number to beat, and if nothing beats it, say so.\n"+
				"2. `sim_stat_weights` on the baseline. The weights say what a point of each stat is worth, and they are how candidates get ranked without simulating every one.\n"+
				"3. For each slot worth changing, `db_search_items` with `maxPhase: %s`, the spec's class, and the slot. Score the results with the stat weights and keep the best three to five per slot.\n"+
				"4. Narrow with `sim_compare_batch`, one slot at a time, starting from the slots where the candidates differ most. Each call compares the current best set against the candidates for one slot; keep the winner and move on. This converges in a few calls per slot rather than combinatorially.\n"+
				"5. Watch `significant` on every row. When a difference is not significant, the two options are indistinguishable at that iteration count -- either say so and pick on another basis, or re-run the close pair with more iterations.\n"+
				"6. Before trusting the final set, `gear_validate` it. Unique-equipped items, meta gem colour requirements and enchant restrictions are not enforced by the simulator, and a set that breaks them is not a real answer.\n"+
				"7. Re-run the finalists at 20000 iterations or more to separate the last few DPS, then report the set, its DPS with the error, the gain over the baseline, and the share link.\n\n"+
				"State the assumptions you optimised under -- encounter, buffs, talents, rotation -- because a 'best' set is only best for those.",
				specName, phase, encounter, phase)
		},
	}
}

func bestInBags() spec.Entry {
	return spec.Prompt{
		Name:    "best_in_bags",
		Title:   "Find the best set from what you own",
		Summary: "A procedure for finding the strongest set a character can assemble from gear they already have.",
		Details: "Unlike find_bis this is bounded: the pool is what the player owns, which is usually a few dozen items.",
		Arguments: []spec.Argument{
			{Name: "spec", Description: "which spec to play, if the class has more than one"},
		},
		Handler: func(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			specName := argument(request.Params.Arguments, "spec", "")
			specHint := ""
			if specName != "" {
				specHint = fmt.Sprintf(" Simulate them as %s.", specName)
			}

			return message("Work out the best gear the player can put together from what they already own.%s\n\n"+
				"1. Ask for their WowSimsExporter export if you do not have it, and for the bags export too if they want items out of their bags considered. Import both with `import_addon`.\n"+
				"2. `sim_run` the imported set as it stands. That is the baseline, and the number they will compare everything against.\n"+
				"3. `sim_stat_weights` on the baseline, then score every item in the pool by those weights to decide what is worth simulating. Ignore items that are clearly worse than what is already equipped in that slot.\n"+
				"4. `sim_compare_batch` the promising swaps, one slot at a time, using the `items` field so only that slot changes. Keep whatever wins and carry it into the next comparison.\n"+
				"5. `gear_validate` the result: two unique trinkets or a dead meta gem would make the answer wrong in a way the simulation itself will not catch.\n"+
				"6. Report the upgrade path in order of value -- what to change, what it gains, and what it costs -- and give them the share link so they can open it in the sim.\n\n"+
				"If nothing in the pool beats what they are wearing, say that plainly.",
				specHint)
		},
	}
}

func compareRotations() spec.Entry {
	return spec.Prompt{
		Name:    "compare_rotations",
		Title:   "Test a rotation change",
		Summary: "A procedure for deciding whether a change to the rotation is actually an improvement.",
		Details: "Rotation differences are usually small, so this is mostly about not being fooled by noise.",
		Arguments: []spec.Argument{
			{Name: "spec", Description: "the spec whose rotation is in question", Required: true},
			{Name: "change", Description: "what to try, e.g. 'add Mind Blast to the priority list'"},
		},
		Handler: func(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			arguments := request.Params.Arguments
			specName := argument(arguments, "spec", "the spec")
			change := argument(arguments, "change", "the proposed rotation change")

			return message("Decide whether this change to the %s rotation is worth making: %s\n\n"+
				"1. Read the current rotation from `wowsims://spec/{class}/{spec}/apl/default` and the list of alternatives from `specs_list`.\n"+
				"2. `sim_compare_batch` the current rotation against the variants, at the same seed. Rotation changes are usually worth a percent or two at most, so the paired seeds matter more here than anywhere else.\n"+
				"3. Test across conditions, not just one: short and long fights, single and multiple targets, and with buffs and without. A change that wins one scenario and loses another has not earned a default.\n"+
				"4. Look at the per-spell breakdown in the results, not only the totals. It shows what actually changed -- casts displaced, mana spent, a spell missing more than expected -- which is the difference between knowing a change works and knowing why.\n"+
				"5. Report the deltas per scenario with their significance, and recommend only if the gain holds across them. Say explicitly when the answer is 'no measurable difference'.",
				specName, change)
		},
	}
}

func argument(arguments map[string]string, name, fallback string) string {
	if value, ok := arguments[name]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func message(format string, args ...any) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Messages: []*mcp.PromptMessage{{
			Role:    "user",
			Content: &mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		}},
	}, nil
}
