package registry_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callTool calls a tool and decodes its structured output, which is how an agent consumes it.
func callTool[T any](t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) T {
	t.Helper()

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s reported an error: %v", name, result.Content)
	}

	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("%s: marshal output: %v", name, err)
	}
	var output T
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("%s: decode output: %v", name, err)
	}
	return output
}

func toolError(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) string {
	t.Helper()

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return err.Error()
	}
	if !result.IsError {
		t.Fatalf("%s unexpectedly succeeded", name)
	}
	var message strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			message.WriteString(text.Text)
		}
	}
	return message.String()
}

type settingsSummary struct {
	Spec      string `json:"spec"`
	Class     string `json:"class"`
	Race      string `json:"race"`
	Talents   string `json:"talents"`
	Buffed    bool   `json:"buffed"`
	Encounter struct {
		DurationSeconds float64 `json:"durationSeconds"`
		Targets         int     `json:"targets"`
	} `json:"encounter"`
	Gear []struct {
		Slot    string  `json:"slot"`
		ItemID  int32   `json:"itemId"`
		Name    string  `json:"name"`
		Enchant int32   `json:"enchant"`
		Gems    []int32 `json:"gems"`
	} `json:"gear"`
}

type settingsCreateOutput struct {
	Link    string          `json:"link"`
	Summary settingsSummary `json:"summary"`
	Notes   []string        `json:"notes"`
}

// The link is the unit of state: a setup assembled by one call has to survive a round trip
// through a link and come back describing the same character.
func TestSettingsRoundTripThroughALink(t *testing.T) {
	session := connect(t)

	created := callTool[settingsCreateOutput](t, session, "settings_create", map[string]any{
		"spec":    "SmitePriest",
		"gearSet": "p3",
	})

	if !strings.HasPrefix(created.Link, "https://wowsims.com/tbc/priest/smite/#") {
		t.Errorf("link does not open the smite priest sim: %s", created.Link)
	}
	if created.Summary.Spec != "SmitePriest" || created.Summary.Class != "Priest" {
		t.Errorf("summary describes %s %s", created.Summary.Race, created.Summary.Spec)
	}
	if len(created.Summary.Gear) < 15 {
		t.Errorf("only %d gear slots filled", len(created.Summary.Gear))
	}
	if created.Summary.Talents == "" {
		t.Error("no talents were applied")
	}
	if !created.Summary.Buffed {
		t.Error("expected raid buffs by default")
	}
	if len(created.Notes) == 0 {
		t.Error("expected notes about the defaults that were applied")
	}

	decoded := callTool[struct {
		Summary settingsSummary `json:"summary"`
		Raw     string          `json:"raw"`
	}](t, session, "link_decode", map[string]any{"link": created.Link})

	if decoded.Summary.Spec != created.Summary.Spec || decoded.Summary.Talents != created.Summary.Talents {
		t.Errorf("round trip changed the setup: %+v vs %+v", decoded.Summary, created.Summary)
	}
	if len(decoded.Summary.Gear) != len(created.Summary.Gear) {
		t.Errorf("round trip changed the gear: %d slots vs %d", len(decoded.Summary.Gear), len(created.Summary.Gear))
	}
	if decoded.Raw != "" {
		t.Error("raw settings returned without being asked for")
	}

	// A link decoded with includeRaw can be edited and re-encoded, which is the escape hatch for
	// anything the summary does not cover.
	withRaw := callTool[struct {
		Raw string `json:"raw"`
	}](t, session, "link_decode", map[string]any{"link": created.Link, "includeRaw": true})
	if !strings.Contains(withRaw.Raw, "smitePriest") {
		t.Errorf("raw settings do not look like a smite priest: %.120s", withRaw.Raw)
	}

	reEncoded := callTool[settingsCreateOutput](t, session, "link_encode", map[string]any{"settings": withRaw.Raw})
	if reEncoded.Link != created.Link {
		t.Error("re-encoding the same settings produced a different link")
	}
}

func TestSettingsCreateReportsBadInput(t *testing.T) {
	session := connect(t)

	for name, arguments := range map[string]map[string]any{
		"no spec or link": {},
		"unknown spec":    {"spec": "NotASpec"},
		"unknown gear":    {"spec": "SmitePriest", "gearSet": "nonexistent"},
		"unknown race":    {"spec": "SmitePriest", "race": "Murloc"},
	} {
		t.Run(name, func(t *testing.T) {
			if message := toolError(t, session, "settings_create", arguments); message == "" {
				t.Error("expected an explanatory error message")
			}
		})
	}
}

func TestCharStats(t *testing.T) {
	session := connect(t)

	output := callTool[struct {
		Stats map[string]float64 `json:"stats"`
		Sets  []string           `json:"sets"`
		Link  string             `json:"link"`
	}](t, session, "char_stats", map[string]any{"spec": "SmitePriest", "gearSet": "p3"})

	if output.Stats["SpellDamage"] <= 0 {
		t.Errorf("no spell damage in %v", output.Stats)
	}
	if output.Stats["SpellHitRating"] <= 0 {
		t.Error("expected some spell hit rating from a raiding gear set")
	}
	if output.Link == "" {
		t.Error("no share link returned")
	}
	// The contract is that zero-valued stats are dropped rather than listed as zero.
	for name, value := range output.Stats {
		if value == 0 {
			t.Errorf("%s was reported as zero rather than omitted", name)
		}
	}
}

func TestGearValidate(t *testing.T) {
	session := connect(t)

	output := callTool[struct {
		Equippable bool     `json:"equippable"`
		Problems   []string `json:"problems"`
	}](t, session, "gear_validate", map[string]any{"spec": "SmitePriest", "gearSet": "p3"})

	if !output.Equippable {
		t.Errorf("the checked-in p3 set should be wearable, but: %v", output.Problems)
	}
}

type itemSearchOutput struct {
	TotalFound int `json:"totalFound"`
	Items      []struct {
		ID      int32              `json:"id"`
		Name    string             `json:"name"`
		Phase   int32              `json:"phase"`
		Quality string             `json:"quality"`
		Slots   []string           `json:"slots"`
		Stats   map[string]float64 `json:"stats"`
	} `json:"items"`
}

func TestItemSearch(t *testing.T) {
	session := connect(t)

	t.Run("by name", func(t *testing.T) {
		output := callTool[itemSearchOutput](t, session, "db_search_items", map[string]any{"name": "Zhar'doom"})
		if len(output.Items) == 0 {
			t.Fatal("no results")
		}
		if !strings.Contains(output.Items[0].Name, "Zhar'doom") {
			t.Errorf("first result is %q", output.Items[0].Name)
		}
	})

	t.Run("filters by slot, class and phase", func(t *testing.T) {
		output := callTool[itemSearchOutput](t, session, "db_search_items", map[string]any{
			"slot":     "Head",
			"class":    "Priest",
			"maxPhase": 3,
			"hasStats": []string{"SpellDamage"},
			"limit":    10,
		})

		if len(output.Items) == 0 {
			t.Fatal("no results")
		}
		if len(output.Items) > 10 {
			t.Errorf("limit ignored: %d results", len(output.Items))
		}
		if output.TotalFound < len(output.Items) {
			t.Errorf("totalFound %d is less than the %d returned", output.TotalFound, len(output.Items))
		}
		for _, item := range output.Items {
			if item.Phase > 3 {
				t.Errorf("%s is phase %d", item.Name, item.Phase)
			}
			if item.Stats["SpellDamage"] <= 0 {
				t.Errorf("%s has no spell damage", item.Name)
			}
			if len(item.Slots) == 0 || item.Slots[0] != "Head" {
				t.Errorf("%s is not a head item: %v", item.Name, item.Slots)
			}
		}
	})

	t.Run("rejects unknown filters", func(t *testing.T) {
		if message := toolError(t, session, "db_search_items", map[string]any{"slot": "Elbow"}); !strings.Contains(message, "Elbow") {
			t.Errorf("error does not name the bad slot: %s", message)
		}
	})
}

func TestResources(t *testing.T) {
	session := connect(t)

	listed, err := session.ListResourceTemplates(t.Context(), nil)
	if err != nil {
		t.Fatalf("list resource templates: %v", err)
	}
	if len(listed.ResourceTemplates) == 0 {
		t.Fatal("no resource templates registered")
	}
	for _, template := range listed.ResourceTemplates {
		if template.Description == "" {
			t.Errorf("resource %q has no description", template.URITemplate)
		}
	}

	read := func(uri string) string {
		t.Helper()
		result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		if len(result.Contents) == 0 {
			t.Fatalf("read %s: no contents", uri)
		}
		return result.Contents[0].Text
	}

	if gear := read("wowsims://spec/priest/smite/gear/p3"); !strings.Contains(gear, "32525") {
		t.Errorf("gear preset does not contain the expected helm: %.120s", gear)
	}
	if apl := read("wowsims://spec/priest/smite/apl/default"); !strings.Contains(apl, "TypeAPL") {
		t.Errorf("rotation does not look like an APL: %.120s", apl)
	}
	if talents := read("wowsims://spec/priest/smite/talents"); !strings.Contains(talents, "5051000130505002501") {
		t.Errorf("talents do not contain the smite build: %.120s", talents)
	}
	if item := read("wowsims://item/32374"); !strings.Contains(item, "Zhar'doom") {
		t.Errorf("item 32374 is not Zhar'doom: %.120s", item)
	}

	if _, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "wowsims://item/999999999"}); err == nil {
		t.Error("expected an error for an unknown item")
	}
}
