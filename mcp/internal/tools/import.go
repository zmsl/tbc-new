package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

// The shape the WowSimsExporter addon writes. Only the fields worth simulating are read; the
// export also carries glyphs, reputation and other things this sim has no use for.
type addonExport struct {
	Class       string `json:"class"`
	Race        string `json:"race"`
	Level       int    `json:"level"`
	Talents     string `json:"talents"`
	Professions []struct {
		Name string `json:"name"`
	} `json:"professions"`
	Gear addonGear `json:"gear"`
}

type addonGear struct {
	Items []*addonItem `json:"items"`
}

type addonItem struct {
	ID           int32   `json:"id"`
	Enchant      int32   `json:"enchant"`
	Gems         []int32 `json:"gems"`
	RandomSuffix int32   `json:"randomSuffix"`
}

type importAddonInput struct {
	Export string `json:"export" jsonschema:"the JSON the WowSimsExporter addon puts on your clipboard"`
	Spec   string `json:"spec,omitempty" jsonschema:"which spec to simulate as. Only needed when the class has more than one, e.g. a priest can be Priest (shadow) or SmitePriest."`
	Bags   string `json:"bags,omitempty" jsonschema:"an optional second export listing items in your bags, as an EquipmentSpec JSON. Returned as a candidate pool to compare against what you are wearing."`
}

type importAddonOutput struct {
	Link    string          `json:"link" jsonschema:"share link for the imported character"`
	Summary settingsSummary `json:"summary" jsonschema:"what was imported"`
	Pool    []poolItem      `json:"pool,omitempty" jsonschema:"items from the bags export, as candidates to try against the equipped set"`
	Notes   []string        `json:"notes,omitempty" jsonschema:"anything that had to be assumed or could not be imported"`
}

func importAddon(config engine.Config) spec.Entry {
	return spec.Tool[importAddonInput, importAddonOutput]{
		Name:    "import_addon",
		Title:   "Import a character",
		Summary: "Turns a WowSimsExporter addon export into a simulatable setup and a share link.",
		Details: "This is how a real character gets into the simulator: gear, gems, enchants, talents, race and\n" +
			"professions exactly as they are in game. Pass the resulting link to sim_run or\n" +
			"sim_compare_batch.\n\n" +
			"A bags export turns into `pool`: the candidates to try against what is worn. Those items are\n" +
			"usually bare, so simulate them with gems and an enchant supplied rather than as they sit in\n" +
			"the bag, or the comparison understates every one of them.\n\n" +
			"Everything the export does not carry -- buffs, consumables, the encounter -- is filled in with\n" +
			"the same raid defaults the website uses, and listed in notes.",
		Examples: []spec.Example{
			{Description: "import an export", Args: `{"export": "{\"class\":\"Priest\",\"race\":\"Undead\",\"talents\":\"...\",\"gear\":{\"items\":[...]}}", "spec": "SmitePriest"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input importAddonInput) (*mcp.CallToolResult, importAddonOutput, error) {
			var output importAddonOutput

			var export addonExport
			if err := json.Unmarshal([]byte(input.Export), &export); err != nil {
				return nil, output, fmt.Errorf("this does not look like an addon export: %w", err)
			}

			class, err := parseEnum[proto.Class](export.Class, "Class", proto.Class_value)
			if err != nil {
				return nil, output, err
			}

			resolved, err := resolveSpec(input.Spec, class)
			if err != nil {
				return nil, output, err
			}

			settings, notes, err := config.BuildSettings(engine.SettingsRequest{Spec: resolved})
			if err != nil {
				return nil, output, err
			}
			output.Notes = notes

			race, err := parseEnum[proto.Race](export.Race, "Race", proto.Race_value)
			if err != nil {
				return nil, output, err
			}
			settings.Player.Race = race

			if export.Talents != "" {
				settings.Player.TalentsString = export.Talents
			} else {
				output.Notes = append(output.Notes, "the export carried no talents, so the spec's default build was kept")
			}

			settings.Player.Profession1, settings.Player.Profession2 = proto.Profession_ProfessionUnknown, proto.Profession_ProfessionUnknown
			for i, profession := range export.Professions {
				parsed, err := parseEnum[proto.Profession](profession.Name, "Profession", proto.Profession_value)
				if err != nil {
					output.Notes = append(output.Notes, "ignored unrecognised profession "+profession.Name)
					continue
				}
				if i == 0 {
					settings.Player.Profession1 = parsed
				} else if i == 1 {
					settings.Player.Profession2 = parsed
				}
			}

			equipment, err := equipmentFromAddon(export.Gear)
			if err != nil {
				return nil, output, err
			}
			settings.Player.Equipment = equipment

			if export.Level != 0 && export.Level < 70 {
				output.Notes = append(output.Notes, fmt.Sprintf("the character is level %d; the simulator only models level 70", export.Level))
			}

			if input.Bags != "" {
				if output.Pool, err = poolFromBags(input.Bags); err != nil {
					return nil, output, err
				}
			}

			output.Summary = summarize(settings)
			if output.Link, err = shareLink(config, settings); err != nil {
				return nil, output, err
			}
			return nil, output, nil
		},
	}
}

// The addon writes display names -- "Night Elf", "Blood Elf", "Death Knight" -- while the enums
// are unspaced. Matching loosely costs nothing and avoids failing an import over a space.
func parseEnum[T ~int32](name, prefix string, values map[string]int32) (T, error) {
	cleaned := strings.NewReplacer(" ", "", "-", "", "'", "").Replace(strings.TrimSpace(name))

	for candidate, value := range values {
		if strings.EqualFold(candidate, prefix+cleaned) || strings.EqualFold(candidate, cleaned) {
			return T(value), nil
		}
	}
	return T(0), fmt.Errorf("unrecognised %s %q", strings.ToLower(prefix), name)
}

// A class may have several specs and the export does not say which is being played, so the
// caller has to choose unless there is only one.
func resolveSpec(requested string, class proto.Class) (proto.Spec, error) {
	if requested != "" {
		resolved, err := engine.ParseSpec(requested)
		if err != nil {
			return proto.Spec_SpecUnknown, err
		}
		if engine.SpecClass(resolved) != class {
			return proto.Spec_SpecUnknown, fmt.Errorf("%s is not a %s spec", requested, trimEnum(class.String(), "Class"))
		}
		return resolved, nil
	}

	var candidates []proto.Spec
	for _, candidate := range core.RegisteredSpecs() {
		if engine.SpecClass(candidate) == class {
			candidates = append(candidates, candidate)
		}
	}

	switch len(candidates) {
	case 0:
		return proto.Spec_SpecUnknown, fmt.Errorf("no simulated spec for %s", trimEnum(class.String(), "Class"))
	case 1:
		return candidates[0], nil
	}

	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, trimEnum(candidate.String(), "Spec"))
	}
	return proto.Spec_SpecUnknown, fmt.Errorf("%s has several simulated specs; pass one of %s", trimEnum(class.String(), "Class"), strings.Join(names, ", "))
}

// Equipment is positional: index is the slot. An empty slot stays as an empty entry rather than
// being dropped, or everything after it shifts up a slot.
func equipmentFromAddon(gear addonGear) (*proto.EquipmentSpec, error) {
	if len(gear.Items) == 0 {
		return nil, fmt.Errorf("the export contains no gear")
	}

	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, len(gear.Items))}
	for i, item := range gear.Items {
		if item == nil {
			equipment.Items[i] = &proto.ItemSpec{}
			continue
		}

		gems := make([]int32, 0, len(item.Gems))
		for _, gem := range item.Gems {
			gems = append(gems, gem)
		}
		equipment.Items[i] = &proto.ItemSpec{
			Id:           item.ID,
			Enchant:      item.Enchant,
			Gems:         gems,
			RandomSuffix: item.RandomSuffix,
		}
	}
	return equipment, nil
}

// A candidate out of the bags, as opposed to something being worn: it has no slot of its own yet,
// and what matters is where it could go and what it would need before it was worth wearing.
type poolItem struct {
	ItemID  int32    `json:"itemId"`
	Name    string   `json:"name,omitempty"`
	Slots   []string `json:"slots,omitempty" jsonschema:"every slot this could be equipped in. Rings and trinkets have two, and both are worth trying."`
	Sockets []string `json:"sockets,omitempty" jsonschema:"empty gem sockets. An item out of the bags is usually unenchanted and ungemmed, so compare it with gems and an enchant supplied or the comparison understates it."`
	Enchant int32    `json:"enchant,omitempty" jsonschema:"the enchant already on it, if any"`
	Gems    []int32  `json:"gems,omitempty" jsonschema:"gems already socketed in it, if any"`
	Phase   int32    `json:"phase,omitempty"`
}

// The bags export is a flat EquipmentSpec used as a candidate pool rather than a worn set, so
// slot positions carry no meaning here -- only the items do.
func poolFromBags(raw string) ([]poolItem, error) {
	var gear addonGear
	if err := json.Unmarshal([]byte(raw), &gear); err != nil {
		return nil, fmt.Errorf("this does not look like a bags export: %w", err)
	}

	var pool []poolItem
	for _, item := range gear.Items {
		if item == nil || item.ID == 0 {
			continue
		}
		entry := poolItem{ItemID: item.ID, Enchant: item.Enchant, Gems: item.Gems}
		if known, ok := engine.Item(item.ID); ok {
			entry.Name = known.Name
			entry.Phase = known.Phase
			for _, slot := range core.EligibleItemSlots(known) {
				entry.Slots = append(entry.Slots, trimEnum(slot.String(), "ItemSlot"))
			}
			for _, socket := range known.GemSockets {
				entry.Sockets = append(entry.Sockets, trimEnum(socket.String(), "GemColor"))
			}
		}
		pool = append(pool, entry)
	}
	return pool, nil
}
