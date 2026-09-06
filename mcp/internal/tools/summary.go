package tools

import (
	"strings"

	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

// A settings blob is a few kilobytes of protobuf covering every buff, consumable and rotation
// step. Handing that back whole would cost far more to read than it is worth, so tools return
// this instead: what a person would check before trusting a sim result.
type settingsSummary struct {
	Spec        string           `json:"spec" jsonschema:"the spec being simulated, e.g. SmitePriest"`
	Class       string           `json:"class,omitempty" jsonschema:"the class, e.g. Priest"`
	Race        string           `json:"race,omitempty" jsonschema:"the character's race"`
	Talents     string           `json:"talents,omitempty" jsonschema:"talent string in wowhead format"`
	Professions []string         `json:"professions,omitempty" jsonschema:"professions, which gate some gear and consumables"`
	Encounter   encounterSummary `json:"encounter" jsonschema:"what is being fought and for how long"`
	Buffed      bool             `json:"buffed" jsonschema:"true when raid buffs and debuffs are applied"`
	Gear        []gearSlot       `json:"gear,omitempty" jsonschema:"equipped items, one entry per filled slot"`
}

type encounterSummary struct {
	DurationSeconds float64 `json:"durationSeconds" jsonschema:"fight length in seconds"`
	Targets         int     `json:"targets" jsonschema:"number of enemies"`
}

type gearSlot struct {
	Slot    string  `json:"slot" jsonschema:"equipment slot, e.g. Head or MainHand"`
	ItemID  int32   `json:"itemId" jsonschema:"the item's id"`
	Name    string  `json:"name,omitempty" jsonschema:"the item's name, when the item database is loaded"`
	Enchant int32   `json:"enchant,omitempty" jsonschema:"enchant effect id, if enchanted"`
	Gems    []int32 `json:"gems,omitempty" jsonschema:"socketed gem ids"`
}

func summarize(settings *proto.IndividualSimSettings) settingsSummary {
	summary := settingsSummary{}
	if settings == nil || settings.Player == nil {
		return summary
	}

	player := settings.Player
	summary.Spec = trimEnum(core.PlayerProtoToSpec(player).String(), "Spec")
	summary.Class = trimEnum(player.Class.String(), "Class")
	summary.Race = trimEnum(player.Race.String(), "Race")
	summary.Talents = player.TalentsString

	for _, profession := range []proto.Profession{player.Profession1, player.Profession2} {
		if profession != proto.Profession_ProfessionUnknown {
			summary.Professions = append(summary.Professions, profession.String())
		}
	}

	if settings.Encounter != nil {
		summary.Encounter = encounterSummary{
			DurationSeconds: settings.Encounter.Duration,
			Targets:         len(settings.Encounter.Targets),
		}
	}

	// A blunt but useful signal: a sim with no raid buffs produces numbers that are not
	// comparable with anything from the website's defaults.
	summary.Buffed = settings.RaidBuffs != nil && len(settings.RaidBuffs.String()) > 0

	summary.Gear = summarizeGear(player.Equipment)
	return summary
}

func summarizeGear(equipment *proto.EquipmentSpec) []gearSlot {
	if equipment == nil {
		return nil
	}

	var slots []gearSlot
	for i, item := range equipment.Items {
		if item == nil || item.Id == 0 {
			continue
		}

		slot := gearSlot{
			Slot:    trimEnum(proto.ItemSlot(i).String(), "ItemSlot"),
			ItemID:  item.Id,
			Enchant: item.Enchant,
		}
		for _, gem := range item.Gems {
			if gem != 0 {
				slot.Gems = append(slot.Gems, gem)
			}
		}
		if known, ok := core.ItemsByID[item.Id]; ok {
			slot.Name = known.Name
		}
		slots = append(slots, slot)
	}
	return slots
}

func trimEnum(value, prefix string) string {
	return strings.TrimPrefix(value, prefix)
}
