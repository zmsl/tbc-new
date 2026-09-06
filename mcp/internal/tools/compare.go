package tools

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

const (
	maxVariants = 40
	// Variants times iterations. 40 variants at 5000 iterations is a few seconds of wall clock;
	// beyond that a client is waiting long enough that the run belongs in several calls.
	maxComparisonIterations = 200_000
)

type comparisonVariant struct {
	Label string `json:"label" jsonschema:"a short name for this variant, used in the results table"`

	Link      string `json:"link,omitempty" jsonschema:"an entirely different setup to compare, as a share link. Everything else in this variant is ignored."`
	GearSet   string `json:"gearSet,omitempty" jsonschema:"swap the whole gear set for a checked-in one"`
	Rotation  string `json:"rotation,omitempty" jsonschema:"swap the rotation for a checked-in one"`
	Talents   string `json:"talents,omitempty" jsonschema:"swap the talent string"`
	Encounter string `json:"encounter,omitempty" jsonschema:"fight this encounter instead"`

	Items []itemChange `json:"items,omitempty" jsonschema:"swap individual items, leaving the rest of the gear alone"`
}

type itemChange struct {
	Slot    string  `json:"slot" jsonschema:"the slot to change, e.g. Trinket1 or MainHand"`
	ItemID  int32   `json:"itemId" jsonschema:"the item to equip. Zero empties the slot."`
	Enchant int32   `json:"enchant,omitempty" jsonschema:"enchant effect id to apply. Changing the item clears whatever was in the slot, so an item worth wearing needs its enchant supplied here or it is simulated bare."`
	Gems    []int32 `json:"gems,omitempty" jsonschema:"gem ids to socket. Changing the item clears the old gems, since they cannot be assumed to fit; supply the new ones or the item is simulated with empty sockets."`
}

type compareInput struct {
	setupInput

	Variants   []comparisonVariant `json:"variants" jsonschema:"the changes to compare against the base setup, at most 40"`
	Iterations int32               `json:"iterations,omitempty" jsonschema:"iterations per variant. Defaults to 2000. Variants times iterations may not exceed 200000."`
	Seed       int64               `json:"seed,omitempty" jsonschema:"RNG seed, shared by every variant. Defaults to a fixed value."`
}

type compareOutput struct {
	Base     comparisonRow   `json:"base" jsonschema:"the unchanged setup, which every variant is measured against"`
	Results  []comparisonRow `json:"results" jsonschema:"one row per variant, best first"`
	Combined *combinedResult `json:"combined,omitempty" jsonschema:"the improvements applied together, when more than one of them can be"`
	Notes    []string        `json:"notes,omitempty" jsonschema:"which defaults were applied to the base setup"`
}

// Variants are measured one at a time, so reading the winners as an upgrade path assumes they
// add up. Usually they do. They stop adding up wherever the game has a threshold -- the spell hit
// cap, a set bonus, a meta gem whose colour requirement a swap breaks -- and nothing in the
// individual measurements would show it. So the improvements are also run together, and the
// interaction is reported rather than left to be assumed.
type combinedResult struct {
	Applied  []string          `json:"applied" jsonschema:"the variants that were applied together"`
	Excluded map[string]string `json:"excluded,omitempty" jsonschema:"variants left out, and why"`

	Dps      float64 `json:"dps"`
	StdErr   float64 `json:"stderr"`
	Delta    float64 `json:"delta" jsonschema:"DPS difference from the base setup with all of them applied"`
	DeltaPct float64 `json:"deltaPercent"`

	Equippable bool     `json:"equippable" jsonschema:"whether the combination could be worn. Two changes that are each legal alone can be illegal together."`
	Problems   []string `json:"problems,omitempty" jsonschema:"why the combination could not be worn"`

	SumOfDeltas float64 `json:"sumOfDeltas" jsonschema:"what the individual measurements add up to, which is what you would have assumed"`
	Interaction float64 `json:"interaction" jsonschema:"measured minus assumed. Negative means the changes overlap -- a stat cap reached twice, say; positive means they reinforce, like a set bonus completed."`
	Significant bool    `json:"interactionSignificant" jsonschema:"true when the interaction is larger than the measurement error. When false the changes add up, and picking them one slot at a time was safe."`

	Link string `json:"link" jsonschema:"share link for the combined setup"`
}

type comparisonRow struct {
	Label       string  `json:"label"`
	Dps         float64 `json:"dps"`
	StdErr      float64 `json:"stderr" jsonschema:"standard error of this variant's DPS"`
	Delta       float64 `json:"delta,omitempty" jsonschema:"DPS difference from the base setup"`
	DeltaPct    float64 `json:"deltaPercent,omitempty" jsonschema:"difference from the base as a percentage"`
	Significant bool    `json:"significant" jsonschema:"true when the difference is larger than the combined error of both runs. A false here means the variants are indistinguishable at this iteration count, not that they are equal."`
	Link        string  `json:"link" jsonschema:"share link for this variant"`
	Error       string  `json:"error,omitempty" jsonschema:"why this variant could not be simulated"`

	// The simulator will happily run gear nobody could wear -- a two-handed weapon beside an
	// off-hand, two of the same unique trinket -- and report a large gain for it. Checking is
	// cheap, and an unwearable row presented as the best upgrade is worse than no row at all.
	Equippable bool     `json:"equippable" jsonschema:"whether this variant's gear could actually be worn in game. A false here means the DPS is real but unreachable."`
	Problems   []string `json:"problems,omitempty" jsonschema:"why the gear could not be worn"`
}

func simCompare(config engine.Config) spec.Entry {
	return spec.Tool[compareInput, compareOutput]{
		Name:    "sim_compare_batch",
		Title:   "Compare setups",
		Summary: "Simulates a base setup and a list of variations at one seed, and ranks them by DPS with the error on each difference.",
		Details: "The tool to reach for whenever the question is 'which is better'. Every variant runs against the\n" +
			"same seed and the same random streams as the base, so the difference between them is far less\n" +
			"noisy than two separate sim_run calls would be.\n\n" +
			"When more than one variant is an improvement and they change different slots, they are also run\n" +
			"together and reported as `combined`. Read its `interaction`: measuring changes one at a time\n" +
			"assumes they add up, which stops being true at a stat cap, a set bonus, or a gem swap that\n" +
			"deactivates a meta gem. An insignificant interaction means picking slot by slot was safe.\n\n" +
			"Every variant's gear is checked against the rules of the game, because the simulator will run\n" +
			"gear nobody could wear -- a two-handed weapon beside an off-hand, two of the same unique\n" +
			"trinket -- and report a large gain for it. Such rows carry `equippable: false` with the reason,\n" +
			"and rank below anything wearable.\n\n" +
			"Read `significant` before believing a ranking: a difference smaller than the combined error is\n" +
			"noise, and the answer is either 'no measurable difference' or 'run it again with more\n" +
			"iterations'. Searching a large space is done by calling this repeatedly on a narrowing set of\n" +
			"candidates, not by one enormous call -- see the find_bis prompt.",
		Examples: []spec.Example{
			{
				Description: "compare two trinkets",
				Args:        `{"spec": "SmitePriest", "gearSet": "p3", "variants": [{"label": "Icon", "items": [{"slot": "Trinket1", "itemId": 29370}]}, {"label": "Skull", "items": [{"slot": "Trinket1", "itemId": 32483}]}]}`,
			},
			{
				Description: "compare gear sets across phases",
				Args:        `{"spec": "SmitePriest", "variants": [{"label": "p3", "gearSet": "p3"}, {"label": "p5", "gearSet": "p5"}]}`,
			},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input compareInput) (*mcp.CallToolResult, compareOutput, error) {
			var output compareOutput

			if len(input.Variants) == 0 {
				return nil, output, fmt.Errorf("supply at least one variant to compare against the base")
			}
			if len(input.Variants) > maxVariants {
				return nil, output, fmt.Errorf("%d variants is above the limit of %d; narrow the candidates and call again", len(input.Variants), maxVariants)
			}

			iterations := input.Iterations
			if iterations <= 0 {
				iterations = defaultIterations
			}
			// Variants, plus the base, plus the combined run.
			if budget := int(iterations) * (len(input.Variants) + 2); budget > maxComparisonIterations {
				return nil, output, fmt.Errorf("%d variants at %d iterations is %d simulated fights, above the limit of %d; use fewer variants or fewer iterations",
					len(input.Variants), iterations, budget, maxComparisonIterations)
			}

			base, notes, err := input.resolve(config)
			if err != nil {
				return nil, output, err
			}
			output.Notes = notes

			// Every variant is a change to the same base, so they are built up front and run
			// together: one failing to build should not waste the simulations of the others.
			settings := make([]*proto.IndividualSimSettings, len(input.Variants))
			buildErrors := make([]error, len(input.Variants))
			for i, variant := range input.Variants {
				settings[i], buildErrors[i] = applyVariant(config, base, variant)
			}

			options := core.SimRequestOptions{
				Iterations: iterations,
				RandomSeed: input.Seed,
				// Ties each variant's randomness to the base's, so a difference reflects the change
				// rather than the dice. The same trick the stat weight code uses.
				UseLabeledRands: true,
			}

			baseRow, err := runComparison(config, "base", base, options)
			if err != nil {
				return nil, output, err
			}
			output.Base = baseRow

			rows := make([]comparisonRow, len(input.Variants))
			var wait sync.WaitGroup
			guard := make(chan struct{}, runtime.NumCPU())

			for i, variant := range input.Variants {
				label := variant.Label
				if label == "" {
					label = fmt.Sprintf("variant %d", i+1)
				}
				if buildErrors[i] != nil {
					rows[i] = comparisonRow{Label: label, Error: buildErrors[i].Error()}
					continue
				}

				wait.Add(1)
				go func(i int, label string, variantSettings *proto.IndividualSimSettings) {
					defer wait.Done()
					guard <- struct{}{}
					defer func() { <-guard }()

					row, err := runComparison(config, label, variantSettings, options)
					if err != nil {
						rows[i] = comparisonRow{Label: label, Error: err.Error()}
						return
					}
					rows[i] = row
				}(i, label, settings[i])
			}
			wait.Wait()

			for i := range rows {
				if rows[i].Error != "" {
					continue
				}
				rows[i].Delta = round(rows[i].Dps - baseRow.Dps)
				if baseRow.Dps != 0 {
					rows[i].DeltaPct = round(100 * rows[i].Delta / baseRow.Dps)
				}
				// Conservative: the paired seeds make the real uncertainty on a difference smaller
				// than this, so anything flagged significant is comfortably so.
				combined := 2 * math.Sqrt(rows[i].StdErr*rows[i].StdErr+baseRow.StdErr*baseRow.StdErr)
				rows[i].Significant = math.Abs(rows[i].Delta) > combined
			}

			slices.SortFunc(rows, func(a, b comparisonRow) int {
				switch {
				case a.Error != "" && b.Error == "":
					return 1
				case a.Error == "" && b.Error != "":
					return -1
				// Gear nobody could wear ranks below gear they could, whatever it sims for.
				case !a.Equippable && b.Equippable:
					return 1
				case a.Equippable && !b.Equippable:
					return -1
				case a.Dps > b.Dps:
					return -1
				case a.Dps < b.Dps:
					return 1
				}
				return 0
			})

			output.Results = rows
			output.Combined = combine(config, base, input.Variants, rows, baseRow, options)
			return nil, output, nil
		},
	}
}

// combine runs the improvements together, when more than one of them can be. Returns nil when
// there is nothing to say: fewer than two improvements, or none that can be applied side by side.
func combine(config engine.Config, base *proto.IndividualSimSettings, variants []comparisonVariant, rows []comparisonRow, baseRow comparisonRow, options core.SimRequestOptions) *combinedResult {
	byLabel := map[string]comparisonRow{}
	for _, row := range rows {
		byLabel[row.Label] = row
	}

	result := &combinedResult{Excluded: map[string]string{}}
	combined := googleProto.Clone(base).(*proto.IndividualSimSettings)
	claimed := map[string]string{}

	for i, variant := range variants {
		label := variant.Label
		if label == "" {
			label = fmt.Sprintf("variant %d", i+1)
		}
		row, measured := byLabel[label]

		switch {
		case !measured || row.Error != "":
			result.Excluded[label] = "it did not run"
		case !row.Equippable:
			result.Excluded[label] = "the gear could not be worn: " + strings.Join(row.Problems, "; ")
		case row.Delta <= 0:
			result.Excluded[label] = "it was not an improvement"
		case len(variant.Items) == 0:
			// A whole different gear set, rotation or encounter is an alternative to the base
			// rather than a change that could sit alongside another one.
			result.Excluded[label] = "it replaces the setup rather than changing part of it"
		default:
			if conflict := firstClaimedSlot(variant.Items, claimed); conflict != "" {
				result.Excluded[label] = "it changes the same slot as " + conflict
				continue
			}
			for _, change := range variant.Items {
				if err := applyItemChange(combined.Player.Equipment, change); err != nil {
					result.Excluded[label] = err.Error()
					continue
				}
				claimed[change.Slot] = label
			}
			result.Applied = append(result.Applied, label)
			result.SumOfDeltas += row.Delta
		}
	}

	if len(result.Applied) < 2 {
		return nil
	}

	row, err := runComparison(config, "combined", combined, options)
	if err != nil {
		return nil
	}

	result.Dps, result.StdErr, result.Link = row.Dps, row.StdErr, row.Link
	// Two changes that are each legal can still be illegal together -- two unique trinkets, or a
	// meta gem whose colours the pair of swaps breaks -- so the combination is checked too.
	result.Equippable, result.Problems = row.Equippable, row.Problems
	result.Delta = round(row.Dps - baseRow.Dps)
	if baseRow.Dps != 0 {
		result.DeltaPct = round(100 * result.Delta / baseRow.Dps)
	}
	result.SumOfDeltas = round(result.SumOfDeltas)
	result.Interaction = round(result.Delta - result.SumOfDeltas)

	// The interaction is a difference of differences, so it carries the error of the combined run
	// and of every individual one it is compared against.
	variance := row.StdErr*row.StdErr + baseRow.StdErr*baseRow.StdErr
	for _, label := range result.Applied {
		applied := byLabel[label]
		variance += applied.StdErr*applied.StdErr + baseRow.StdErr*baseRow.StdErr
	}
	result.Significant = math.Abs(result.Interaction) > 2*math.Sqrt(variance)

	if len(result.Excluded) == 0 {
		result.Excluded = nil
	}
	return result
}

func firstClaimedSlot(changes []itemChange, claimed map[string]string) string {
	for _, change := range changes {
		if owner, taken := claimed[change.Slot]; taken {
			return owner
		}
	}
	return ""
}

func runComparison(config engine.Config, label string, settings *proto.IndividualSimSettings, options core.SimRequestOptions) (comparisonRow, error) {
	request, err := core.BuildRaidSimRequest(settings, options)
	if err != nil {
		return comparisonRow{}, err
	}

	// Each variant runs single-threaded and the variants run in parallel: less coordination than
	// splitting every run across every core, and it matches the shape of the work.
	result := core.RunRaidSim(request)
	if result.Error != nil {
		return comparisonRow{}, fmt.Errorf("%s", result.Error.Message)
	}

	summary := engine.SummarizeResult(request, result, 1)
	link, err := shareLink(config, settings)
	if err != nil {
		return comparisonRow{}, err
	}

	row := comparisonRow{
		Label:  label,
		Dps:    summary.Dps.Avg,
		StdErr: summary.Dps.StdErr,
		Link:   link,
	}
	if settings.Player != nil && settings.Player.Equipment != nil {
		row.Problems = engine.GearLookup().ValidateEquipment(settings.Player.Equipment)
	}
	row.Equippable = len(row.Problems) == 0
	return row, nil
}

// applyVariant returns a copy of base with the variant's changes applied. The base is untouched,
// so variants cannot leak into each other.
func applyVariant(config engine.Config, base *proto.IndividualSimSettings, variant comparisonVariant) (*proto.IndividualSimSettings, error) {
	if variant.Link != "" {
		return core.DecodeIndividualShareLink(variant.Link)
	}

	settings := googleProto.Clone(base).(*proto.IndividualSimSettings)
	simSpec := core.PlayerProtoToSpec(settings.Player)

	if variant.GearSet != "" {
		gear, err := config.LoadGearSet(simSpec, variant.GearSet)
		if err != nil {
			return nil, err
		}
		settings.Player.Equipment = gear
	}

	if variant.Rotation != "" {
		rotation, err := config.LoadRotation(simSpec, variant.Rotation)
		if err != nil {
			return nil, err
		}
		settings.Player.Rotation = rotation
	}

	if variant.Talents != "" {
		settings.Player.TalentsString = variant.Talents
	}

	if variant.Encounter != "" {
		encounter, err := engine.Encounter(variant.Encounter)
		if err != nil {
			return nil, err
		}
		settings.Encounter = encounter
	}

	for _, change := range variant.Items {
		if err := applyItemChange(settings.Player.Equipment, change); err != nil {
			return nil, err
		}
	}

	return settings, nil
}

func applyItemChange(equipment *proto.EquipmentSpec, change itemChange) error {
	if equipment == nil {
		return fmt.Errorf("the base setup has no equipment to change")
	}

	value, ok := proto.ItemSlot_value["ItemSlot"+change.Slot]
	if !ok {
		return fmt.Errorf("unknown slot %q", change.Slot)
	}
	slot := int(value)

	for len(equipment.Items) <= slot {
		equipment.Items = append(equipment.Items, &proto.ItemSpec{})
	}
	if equipment.Items[slot] == nil {
		equipment.Items[slot] = &proto.ItemSpec{}
	}

	item := equipment.Items[slot]
	if change.ItemID != item.Id {
		// A different item does not keep the old one's enchant and gems, which would silently
		// carry stats onto something that cannot hold them.
		*item = proto.ItemSpec{Id: change.ItemID}
	}
	if change.Enchant != 0 {
		item.Enchant = change.Enchant
	}
	if len(change.Gems) > 0 {
		item.Gems = change.Gems
	}
	return nil
}
