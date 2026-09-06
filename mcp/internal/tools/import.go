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

	// Named the way the game names it -- "fury", "beast mastery" -- rather than the way the
	// simulator does. Newer addon versions carry it, and it saves asking which spec to simulate.
	Spec string `json:"spec"`

	// Newer addon versions put the bags and bank in the same export as the character, so a
	// separate paste is no longer needed.
	BagItems addonGear `json:"bagItems"`
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
	Bags   string `json:"bags,omitempty" jsonschema:"a separate bags export, as an EquipmentSpec JSON. Only needed for older addon versions: a current export carries the bags and bank inline and they are read automatically."`
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

			resolved, specNote, err := resolveSpec(input.Spec, class, export.Spec)
			if err != nil {
				return nil, output, err
			}

			// The export carries gear and talents, so those are not defaulted -- but the rotation is
			// not in an export, and several specs ship one per talent tree. A fury warrior handed the
			// arms rotation would sim badly and silently, so the export's spec picks the rotation too
			// when a preset is named after it.
			rotation := rotationForSpec(config, resolved, export.Spec)

			settings, notes, err := config.BuildSettings(engine.SettingsRequest{
				Spec:     resolved,
				Talents:  export.Talents,
				Rotation: rotation,
			})
			if err != nil {
				return nil, output, err
			}
			// The gear set is replaced wholesale below, so saying which one was defaulted would be
			// describing something that is not in the result.
			for _, note := range notes {
				if !strings.HasPrefix(note, "gear set defaulted") {
					output.Notes = append(output.Notes, note)
				}
			}
			if specNote != "" {
				output.Notes = append(output.Notes, specNote)
			}
			if rotation != "" {
				output.Notes = append(output.Notes, fmt.Sprintf("using the %q rotation", rotation))
			}

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

			// A separate bags export still works, but a current one carries them inline.
			switch {
			case input.Bags != "":
				if output.Pool, err = poolFromBags(input.Bags); err != nil {
					return nil, output, err
				}
			case len(export.BagItems.Items) > 0:
				output.Pool = poolFromItems(export.BagItems)
				output.Notes = append(output.Notes, describePool(len(output.Pool), countItems(export.BagItems)))
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

// Which spec to simulate comes from the caller, then from the export, then from the class having
// only one. The returned note says which of those it was, since a talent tree and a simulated
// spec are not the same thing and the choice should not be silent.
func resolveSpec(requested string, class proto.Class, exported string) (proto.Spec, string, error) {
	if requested != "" {
		resolved, err := engine.ParseSpec(requested)
		if err != nil {
			return proto.Spec_SpecUnknown, "", err
		}
		if engine.SpecClass(resolved) != class {
			return proto.Spec_SpecUnknown, "", fmt.Errorf("%s is not a %s spec", requested, trimEnum(class.String(), "Class"))
		}
		return resolved, "", nil
	}

	if exported != "" {
		if resolved, ok := wseSpecs[class][normalizeSpecName(exported)]; ok {
			return resolved, fmt.Sprintf("the export says %q, simulated as %s", exported, trimEnum(resolved.String(), "Spec")), nil
		}
	}

	var candidates []proto.Spec
	for _, candidate := range core.RegisteredSpecs() {
		if engine.SpecClass(candidate) == class {
			candidates = append(candidates, candidate)
		}
	}

	switch len(candidates) {
	case 0:
		return proto.Spec_SpecUnknown, "", fmt.Errorf("no simulated spec for %s", trimEnum(class.String(), "Class"))
	case 1:
		return candidates[0], "", nil
	}

	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, trimEnum(candidate.String(), "Spec"))
	}
	if exported != "" {
		return proto.Spec_SpecUnknown, "", fmt.Errorf("%s %q could be simulated as any of %s; pass one as spec",
			trimEnum(class.String(), "Class"), exported, strings.Join(names, ", "))
	}
	return proto.Spec_SpecUnknown, "", fmt.Errorf("%s has several simulated specs; pass one of %s", trimEnum(class.String(), "Class"), strings.Join(names, ", "))
}

// The export names the talent tree the game's way. Several trees map onto one simulated spec -- a
// mage is a mage whichever tree it took -- and a feral druid maps onto two, cat and bear, so that
// one is deliberately absent and still has to be asked.
var wseSpecs = map[proto.Class]map[string]proto.Spec{
	proto.Class_ClassWarrior: {
		"arms": proto.Spec_SpecDpsWarrior, "fury": proto.Spec_SpecDpsWarrior,
		"protection": proto.Spec_SpecProtectionWarrior,
	},
	proto.Class_ClassPaladin: {
		"holy": proto.Spec_SpecHolyPaladin, "protection": proto.Spec_SpecProtectionPaladin,
		"retribution": proto.Spec_SpecRetributionPaladin,
	},
	proto.Class_ClassShaman: {
		"elemental": proto.Spec_SpecElementalShaman, "enhancement": proto.Spec_SpecEnhancementShaman,
		"restoration": proto.Spec_SpecRestorationShaman,
	},
	proto.Class_ClassDruid: {
		"balance": proto.Spec_SpecBalanceDruid, "restoration": proto.Spec_SpecRestorationDruid,
	},
	proto.Class_ClassPriest: {
		"shadow": proto.Spec_SpecPriest,
		// The only non-shadow priest this simulator models is the smite build.
		"discipline": proto.Spec_SpecSmitePriest, "holy": proto.Spec_SpecSmitePriest,
	},
	proto.Class_ClassHunter: {
		"beastmastery": proto.Spec_SpecHunter, "marksmanship": proto.Spec_SpecHunter,
		"survival": proto.Spec_SpecHunter,
	},
	proto.Class_ClassRogue: {
		"assassination": proto.Spec_SpecRogue, "combat": proto.Spec_SpecRogue, "subtlety": proto.Spec_SpecRogue,
	},
	proto.Class_ClassMage: {
		"arcane": proto.Spec_SpecMage, "fire": proto.Spec_SpecMage, "frost": proto.Spec_SpecMage,
	},
	proto.Class_ClassWarlock: {
		"affliction": proto.Spec_SpecWarlock, "demonology": proto.Spec_SpecWarlock,
		"destruction": proto.Spec_SpecWarlock,
	},
}

func describePool(pool, carried int) string {
	note := fmt.Sprintf("read %d equippable items from the export's bags and bank", pool)
	if skipped := carried - pool; skipped > 0 {
		note += fmt.Sprintf("; %d others are not gear the simulator models and were skipped", skipped)
	}
	return note
}

// A rotation preset named after the talent tree the character actually plays, if there is one.
func rotationForSpec(config engine.Config, spec proto.Spec, exported string) string {
	if exported == "" {
		return ""
	}
	presets, err := config.ListPresets(spec)
	if err != nil {
		return ""
	}

	wanted := normalizeSpecName(exported)
	for _, name := range presets.Rotations {
		if normalizeSpecName(name) == wanted {
			return name
		}
	}
	return ""
}

func normalizeSpecName(name string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.TrimSpace(name)))
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
	return poolFromItems(gear), nil
}

func poolFromItems(gear addonGear) []poolItem {
	var pool []poolItem
	for _, item := range gear.Items {
		if item == nil || item.ID == 0 {
			continue
		}

		// Bags hold plenty the simulator has no record of -- crafting tools, quest items,
		// consumables -- and none of it is a gear candidate. They are counted rather than listed,
		// by the caller, so nothing disappears without being mentioned.
		known, ok := engine.Item(item.ID)
		if !ok {
			continue
		}
		entry := poolItem{
			ItemID:  item.ID,
			Name:    known.Name,
			Phase:   known.Phase,
			Enchant: item.Enchant,
			Gems:    item.Gems,
		}
		for _, slot := range core.EligibleItemSlots(known) {
			entry.Slots = append(entry.Slots, trimEnum(slot.String(), "ItemSlot"))
		}
		for _, socket := range known.GemSockets {
			entry.Sockets = append(entry.Sockets, trimEnum(socket.String(), "GemColor"))
		}
		if len(entry.Slots) == 0 {
			continue
		}
		pool = append(pool, entry)
	}
	return pool
}

// countItems is how many entries a bag export actually carried, so the difference between it and
// the pool can be reported.
func countItems(gear addonGear) int {
	var count int
	for _, item := range gear.Items {
		if item != nil && item.ID != 0 {
			count++
		}
	}
	return count
}
