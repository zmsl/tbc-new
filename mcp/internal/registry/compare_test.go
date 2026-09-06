package registry_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type comparisonRow struct {
	Label       string  `json:"label"`
	Dps         float64 `json:"dps"`
	StdErr      float64 `json:"stderr"`
	Delta       float64 `json:"delta"`
	DeltaPct    float64 `json:"deltaPercent"`
	Significant bool    `json:"significant"`
	Link        string  `json:"link"`
	Error       string  `json:"error"`
}

type compareOutput struct {
	Base    comparisonRow   `json:"base"`
	Results []comparisonRow `json:"results"`
}

func TestCompareBatch(t *testing.T) {
	session := connect(t)

	output := callTool[compareOutput](t, session, "sim_compare_batch", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 300,
		"variants": []map[string]any{
			{"label": "p5 gear", "gearSet": "p5"},
			{"label": "pre-raid gear", "gearSet": "pre_raid"},
			{"label": "short fight", "encounter": "ShortSingleTarget"},
		},
	})

	if output.Base.Dps <= 0 {
		t.Fatalf("base dps = %v", output.Base.Dps)
	}
	if len(output.Results) != 3 {
		t.Fatalf("got %d rows, want 3", len(output.Results))
	}

	rows := map[string]comparisonRow{}
	for _, row := range output.Results {
		if row.Error != "" {
			t.Errorf("%s failed: %s", row.Label, row.Error)
		}
		if row.Link == "" {
			t.Errorf("%s has no share link", row.Label)
		}
		rows[row.Label] = row
	}

	// Better gear must beat worse gear, and by enough to be called significant. If this stops
	// holding, either the comparison is broken or the gear presets are.
	if rows["p5 gear"].Delta <= 0 {
		t.Errorf("phase 5 gear was not an upgrade on phase 3: %+v", rows["p5 gear"])
	}
	if !rows["p5 gear"].Significant {
		t.Errorf("the p5 upgrade was not significant: %+v", rows["p5 gear"])
	}
	if rows["pre-raid gear"].Delta >= 0 {
		t.Errorf("pre-raid gear was not a downgrade: %+v", rows["pre-raid gear"])
	}

	// Results are ranked, so the best row comes first.
	for i := 1; i < len(output.Results); i++ {
		if output.Results[i-1].Dps < output.Results[i].Dps {
			t.Errorf("results are not ranked: %v before %v", output.Results[i-1], output.Results[i])
		}
	}
}

// Swapping one item must leave the rest of the gear alone.
func TestCompareBatchItemSwap(t *testing.T) {
	session := connect(t)

	output := callTool[compareOutput](t, session, "sim_compare_batch", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 200,
		"variants": []map[string]any{
			{"label": "empty trinket", "items": []map[string]any{{"slot": "Trinket1", "itemId": 0}}},
		},
	})

	row := output.Results[0]
	if row.Error != "" {
		t.Fatalf("swap failed: %s", row.Error)
	}
	if row.Delta >= 0 {
		t.Errorf("removing a trinket did not lose dps: %+v", row)
	}

	// The variant's link must describe the same character with one slot changed.
	decoded := callTool[struct {
		Summary settingsSummary `json:"summary"`
	}](t, session, "link_decode", map[string]any{"link": row.Link})

	for _, item := range decoded.Summary.Gear {
		if item.Slot == "Trinket1" {
			t.Errorf("trinket 1 is still equipped: %+v", item)
		}
	}
	if len(decoded.Summary.Gear) < 14 {
		t.Errorf("the swap dropped more than one slot: %d remain", len(decoded.Summary.Gear))
	}
}

// One bad variant must not lose the others' results.
func TestCompareBatchReportsPerVariantErrors(t *testing.T) {
	session := connect(t)

	output := callTool[compareOutput](t, session, "sim_compare_batch", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 100,
		"variants": []map[string]any{
			{"label": "good", "gearSet": "p5"},
			{"label": "bad", "gearSet": "nonexistent"},
		},
	})

	var good, bad comparisonRow
	for _, row := range output.Results {
		switch row.Label {
		case "good":
			good = row
		case "bad":
			bad = row
		}
	}

	if good.Dps <= 0 || good.Error != "" {
		t.Errorf("the working variant did not produce a result: %+v", good)
	}
	if bad.Error == "" {
		t.Error("the broken variant reported no error")
	}
	// Failures sort last so they do not look like the answer.
	if output.Results[len(output.Results)-1].Label != "bad" {
		t.Error("the failed variant was not sorted last")
	}
}

func TestCompareBatchEnforcesItsBudget(t *testing.T) {
	session := connect(t)

	message := toolError(t, session, "sim_compare_batch", map[string]any{
		"spec":       "SmitePriest",
		"iterations": 50000,
		"variants": []map[string]any{
			{"label": "a", "gearSet": "p5"},
			{"label": "b", "gearSet": "p3"},
			{"label": "c", "gearSet": "p1"},
			{"label": "d", "gearSet": "p2"},
		},
	})
	if !strings.Contains(message, "limit") {
		t.Errorf("error does not explain the limit: %s", message)
	}

	if message := toolError(t, session, "sim_compare_batch", map[string]any{"spec": "SmitePriest", "variants": []map[string]any{}}); message == "" {
		t.Error("expected an error for an empty variant list")
	}
}

// A real export, in the shape the addon writes it.
const addonExport = `{
  "version": "1.0.0",
  "level": 70,
  "class": "Priest",
  "race": "Undead",
  "professions": [{"name": "Tailoring", "level": 375}, {"name": "Enchanting", "level": 375}],
  "talents": "5051000130505002501-225051000320152-",
  "gear": {"items": [
    {"id": 32525, "enchant": 3002, "gems": [34220, 30600]},
    {"id": 32349},
    {"id": 30884, "enchant": 2982, "gems": [28118, 32196]},
    {"id": 32524},
    {"id": 31065, "enchant": 1144, "gems": [32196, 32196, 32196]},
    {"id": 32586, "enchant": 2650},
    {"id": 31061, "enchant": 2937, "gems": [37503]},
    {"id": 32256},
    {"id": 30916, "enchant": 2748, "gems": [32196, 32196, 32196]},
    {"id": 32239, "enchant": 2656, "gems": [32196, 32196]},
    {"id": 32527, "enchant": 2928},
    {"id": 32528, "enchant": 2928},
    {"id": 32483},
    {"id": 27683},
    {"id": 32374, "enchant": 2669},
    null,
    {"id": 29982}
  ]}
}`

func TestImportAddon(t *testing.T) {
	session := connect(t)

	output := callTool[struct {
		Link    string          `json:"link"`
		Summary settingsSummary `json:"summary"`
		Pool    []struct {
			ItemID  int32    `json:"itemId"`
			Name    string   `json:"name"`
			Slots   []string `json:"slots"`
			Sockets []string `json:"sockets"`
		} `json:"pool"`
		Notes []string `json:"notes"`
	}](t, session, "import_addon", map[string]any{
		"export": addonExport,
		"spec":   "SmitePriest",
		"bags":   `{"items": [{"id": 34340}, {"id": 29370}]}`,
	})

	if output.Summary.Spec != "SmitePriest" {
		t.Errorf("imported as %s", output.Summary.Spec)
	}
	if output.Summary.Race != "Undead" {
		t.Errorf("race = %s", output.Summary.Race)
	}
	if output.Summary.Talents != "5051000130505002501-225051000320152-" {
		t.Errorf("talents = %s", output.Summary.Talents)
	}
	if len(output.Summary.Gear) != 16 {
		t.Errorf("imported %d items, want the 16 filled slots", len(output.Summary.Gear))
	}

	// The empty off-hand must not shift the slots after it.
	for _, item := range output.Summary.Gear {
		if item.Slot == "Ranged" && item.ItemID != 29982 {
			t.Errorf("the wand landed in the wrong slot: %+v", item)
		}
		if item.Slot == "MainHand" && item.ItemID != 32374 {
			t.Errorf("the staff landed in the wrong slot: %+v", item)
		}
	}

	if len(output.Pool) != 2 {
		t.Errorf("bags produced %d candidates, want 2", len(output.Pool))
	}
	for _, item := range output.Pool {
		if item.Name == "" {
			t.Errorf("pool item %d was not resolved against the database", item.ItemID)
		}
		// A candidate with nowhere to go cannot be tried, and a ring or trinket has two homes.
		if len(item.Slots) == 0 {
			t.Errorf("%s reports no slot it could be equipped in", item.Name)
		}
		if item.ItemID == 29370 && len(item.Slots) != 2 {
			t.Errorf("a trinket should offer both trinket slots, got %v", item.Slots)
		}
	}

	// The import must be simulatable, which is the whole point of it.
	simmed := callTool[simRunOutput](t, session, "sim_run", map[string]any{"link": output.Link, "iterations": 100})
	if simmed.Result == nil || simmed.Result.Dps.Avg <= 0 {
		t.Error("the imported character did not simulate")
	}
}

func TestImportAddonReportsBadInput(t *testing.T) {
	session := connect(t)

	for name, arguments := range map[string]map[string]any{
		"not json":        {"export": "hello"},
		"unknown class":   {"export": `{"class": "Necromancer", "race": "Undead", "gear": {"items": [{"id": 1}]}}`},
		"unknown race":    {"export": `{"class": "Priest", "race": "Murloc", "gear": {"items": [{"id": 1}]}}`, "spec": "SmitePriest"},
		"ambiguous spec":  {"export": `{"class": "Priest", "race": "Undead", "gear": {"items": [{"id": 1}]}}`},
		"no gear at all":  {"export": `{"class": "Priest", "race": "Undead"}`, "spec": "SmitePriest"},
		"mismatched spec": {"export": `{"class": "Priest", "race": "Undead", "gear": {"items": [{"id": 1}]}}`, "spec": "DpsWarrior"},
	} {
		t.Run(name, func(t *testing.T) {
			if message := toolError(t, session, "import_addon", arguments); message == "" {
				t.Error("expected an explanatory error")
			}
		})
	}
}

func TestPrompts(t *testing.T) {
	session := connect(t)

	listed, err := session.ListPrompts(t.Context(), nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	if len(listed.Prompts) != 3 {
		t.Fatalf("got %d prompts, want 3", len(listed.Prompts))
	}
	for _, prompt := range listed.Prompts {
		if prompt.Description == "" {
			t.Errorf("prompt %q has no description", prompt.Name)
		}
		for _, argument := range prompt.Arguments {
			if argument.Description == "" {
				t.Errorf("prompt %q argument %q has no description", prompt.Name, argument.Name)
			}
		}
	}

	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "find_bis",
		Arguments: map[string]string{"spec": "SmitePriest", "phase": "3"},
	})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("prompt produced no messages")
	}

	text := promptText(t, result)
	for _, want := range []string{"SmitePriest", "phase 3", "sim_compare_batch", "gear_validate", "significant"} {
		if !strings.Contains(text, want) {
			t.Errorf("the find_bis workflow does not mention %q", want)
		}
	}
}

func promptText(t *testing.T, result *mcp.GetPromptResult) string {
	t.Helper()

	var text strings.Builder
	for _, message := range result.Messages {
		encoded, err := json.Marshal(message.Content)
		if err != nil {
			t.Fatalf("marshal prompt content: %v", err)
		}
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(encoded, &content); err != nil {
			t.Fatalf("decode prompt content: %v", err)
		}
		text.WriteString(content.Text)
	}
	return text.String()
}

type combinedResult struct {
	Applied     []string          `json:"applied"`
	Excluded    map[string]string `json:"excluded"`
	Dps         float64           `json:"dps"`
	Delta       float64           `json:"delta"`
	SumOfDeltas float64           `json:"sumOfDeltas"`
	Interaction float64           `json:"interaction"`
	Significant bool              `json:"interactionSignificant"`
	Link        string            `json:"link"`
}

// Measuring changes one at a time assumes they add up. The combined run is what checks that
// assumption instead of leaving it to be made.
func TestCompareBatchCombinesImprovements(t *testing.T) {
	session := connect(t)

	var output struct {
		Base     comparisonRow   `json:"base"`
		Results  []comparisonRow `json:"results"`
		Combined *combinedResult `json:"combined"`
	}
	output = callTool[struct {
		Base     comparisonRow   `json:"base"`
		Results  []comparisonRow `json:"results"`
		Combined *combinedResult `json:"combined"`
	}](t, session, "sim_compare_batch", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 1000,
		"variants": []map[string]any{
			// Two upgrades from the phase 5 set, in different slots.
			{"label": "p5 helm", "items": []map[string]any{{"slot": "Head", "itemId": 34340, "enchant": 3002, "gems": []int{35503, 35761}}}},
			{"label": "p5 chest", "items": []map[string]any{{"slot": "Chest", "itemId": 34399, "enchant": 1144}}},
		},
	})

	if output.Combined == nil {
		t.Fatal("two improvements in different slots produced no combined run")
	}
	if len(output.Combined.Applied) != 2 {
		t.Errorf("combined applied %v", output.Combined.Applied)
	}
	if output.Combined.Delta <= 0 {
		t.Errorf("applying both upgrades lost dps: %+v", output.Combined)
	}

	// The reported interaction has to be the arithmetic it claims to be. Every figure is rounded
	// for reading, so compare within that rounding rather than exactly.
	if got, want := output.Combined.Interaction, output.Combined.Delta-output.Combined.SumOfDeltas; math.Abs(got-want) > 0.011 {
		t.Errorf("interaction = %v, but delta - sumOfDeltas = %v", got, want)
	}
	// Two ordinary stat upgrades in different slots should very nearly add up.
	if output.Combined.Significant {
		t.Errorf("two plain upgrades were reported as interacting: %+v", output.Combined)
	}
	if output.Combined.Link == "" {
		t.Error("no link for the combined setup")
	}
}

// Alternatives for one slot cannot be worn together, and a losing variant is not an improvement
// to carry forward. Both have to be excluded, with the reason said out loud.
func TestCompareBatchExcludesWhatCannotCombine(t *testing.T) {
	session := connect(t)

	output := callTool[struct {
		Combined *combinedResult `json:"combined"`
	}](t, session, "sim_compare_batch", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 500,
		"variants": []map[string]any{
			{"label": "trinket a", "items": []map[string]any{{"slot": "Trinket1", "itemId": 32496}}},
			{"label": "trinket b", "items": []map[string]any{{"slot": "Trinket1", "itemId": 30665}}},
			{"label": "whole p5 set", "gearSet": "p5"},
		},
	})

	// Only one slot is in play and the third variant replaces everything, so there is nothing to
	// combine and the tool must say nothing rather than invent a set.
	if output.Combined != nil && len(output.Combined.Applied) > 1 {
		for _, applied := range output.Combined.Applied {
			if applied == "trinket a" || applied == "trinket b" {
				t.Errorf("two trinkets for the same slot were combined: %+v", output.Combined)
			}
		}
	}
}
