package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

type gemSearchInput struct {
	Name       string   `json:"name,omitempty" jsonschema:"match gems whose name contains this text"`
	Color      string   `json:"color,omitempty" jsonschema:"socket colour to fill: Red, Yellow, Blue or Meta. Hybrid gems (Orange, Green, Purple) are returned for either of their halves, because that is how socket bonuses and meta requirements count them."`
	MaxPhase   int32    `json:"maxPhase,omitempty" jsonschema:"only gems available by this raid phase (1-5)"`
	MinQuality string   `json:"minQuality,omitempty" jsonschema:"minimum quality: Common, Uncommon, Rare, Epic or Legendary"`
	HasStats   []string `json:"hasStats,omitempty" jsonschema:"only gems carrying all of these stats, e.g. [\"SpellDamage\"]"`
	Limit      int      `json:"limit,omitempty" jsonschema:"how many gems to return, 1-200. Defaults to 25."`
}

type gemSearchOutput struct {
	Gems       []gemSummary `json:"gems" jsonschema:"matching gems, most recent phase and highest quality first"`
	TotalFound int          `json:"totalFound" jsonschema:"how many gems matched before the limit was applied"`
}

type gemSummary struct {
	ID              int32              `json:"id" jsonschema:"the gem's id, as used in a gear set's gems array"`
	Name            string             `json:"name"`
	Color           string             `json:"color" jsonschema:"Red, Yellow, Blue, Meta, or a hybrid colour that counts for two"`
	Phase           int32              `json:"phase,omitempty"`
	Quality         string             `json:"quality,omitempty"`
	Unique          bool               `json:"unique,omitempty" jsonschema:"true when only one may be equipped"`
	Stats           map[string]float64 `json:"stats,omitempty"`
	MetaRequirement string             `json:"metaRequirement,omitempty" jsonschema:"for meta gems, the colours needed elsewhere in the gear for it to be active at all"`
}

func gemSearch() spec.Entry {
	return spec.Tool[gemSearchInput, gemSearchOutput]{
		Name:    "db_search_gems",
		Title:   "Search gems",
		Summary: "Finds gems by colour, stats, phase and quality, including what each meta gem requires to be active.",
		Details: "Gems are worth several hundred DPS across a full set, and the simulator's own records do not\n" +
			"carry their colour, so this is the only way to choose one.\n\n" +
			"Meta gems matter more than their stats suggest: a meta whose colour requirement is unmet\n" +
			"contributes nothing at all. `metaRequirement` says what has to be socketed elsewhere, and\n" +
			"gear_validate reports when a set fails it.",
		Examples: []spec.Example{
			{Description: "spell damage gems for a blue socket", Args: `{"color": "Blue", "hasStats": ["SpellDamage"], "minQuality": "Rare"}`},
			{Description: "what does Chaotic Skyfire Diamond need", Args: `{"name": "Chaotic Skyfire"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input gemSearchInput) (*mcp.CallToolResult, gemSearchOutput, error) {
			var output gemSearchOutput

			query, err := input.query()
			if err != nil {
				return nil, output, err
			}

			unlimited := query
			unlimited.Limit = 0
			all := engine.SearchGems(unlimited)
			output.TotalFound = len(all)

			if query.Limit > 0 && len(all) > query.Limit {
				all = all[:query.Limit]
			}
			for _, gem := range all {
				output.Gems = append(output.Gems, summarizeGem(gem))
			}
			return nil, output, nil
		},
	}
}

func (i gemSearchInput) query() (engine.GemQuery, error) {
	query := engine.GemQuery{Name: i.Name, MaxPhase: i.MaxPhase, Limit: defaultItemLimit}

	if i.Limit > 0 {
		query.Limit = min(i.Limit, maxItemLimit)
	}
	if i.Color != "" {
		value, ok := proto.GemColor_value["GemColor"+i.Color]
		if !ok {
			return query, fmt.Errorf("unknown gem colour %q", i.Color)
		}
		query.Color, query.HasColor = proto.GemColor(value), true
	}
	if i.MinQuality != "" {
		value, ok := proto.ItemQuality_value["ItemQuality"+i.MinQuality]
		if !ok {
			return query, fmt.Errorf("unknown quality %q", i.MinQuality)
		}
		query.MinQuality = proto.ItemQuality(value)
	}
	for _, name := range i.HasStats {
		value, ok := proto.Stat_value["Stat"+name]
		if !ok {
			return query, fmt.Errorf("unknown stat %q", name)
		}
		query.HasStats = append(query.HasStats, proto.Stat(value))
	}
	return query, nil
}

func summarizeGem(gem *proto.UIGem) gemSummary {
	return gemSummary{
		ID:              gem.Id,
		Name:            gem.Name,
		Color:           trimEnum(gem.Color.String(), "GemColor"),
		Phase:           gem.Phase,
		Quality:         trimEnum(gem.Quality.String(), "ItemQuality"),
		Unique:          gem.Unique,
		Stats:           statsFromArray(gem.Stats),
		MetaRequirement: engine.MetaRequirement(gem.Id),
	}
}

type enchantSearchInput struct {
	Name     string   `json:"name,omitempty" jsonschema:"match enchants whose name contains this text"`
	Slot     string   `json:"slot,omitempty" jsonschema:"the slot to enchant, e.g. Head, Chest, MainHand"`
	Class    string   `json:"class,omitempty" jsonschema:"restrict to enchants this class may use"`
	MaxPhase int32    `json:"maxPhase,omitempty" jsonschema:"only enchants available by this raid phase (1-5)"`
	HasStats []string `json:"hasStats,omitempty" jsonschema:"only enchants carrying all of these stats"`
	Limit    int      `json:"limit,omitempty" jsonschema:"how many enchants to return, 1-200. Defaults to 25."`
}

type enchantSearchOutput struct {
	Enchants   []enchantSummary `json:"enchants" jsonschema:"matching enchants, most recent phase first"`
	TotalFound int              `json:"totalFound"`
}

type enchantSummary struct {
	EffectID           int32              `json:"effectId" jsonschema:"the id a gear set names an enchant by"`
	Name               string             `json:"name"`
	Slots              []string           `json:"slots" jsonschema:"the slots this enchant may be applied to"`
	Phase              int32              `json:"phase,omitempty"`
	RequiredProfession string             `json:"requiredProfession,omitempty" jsonschema:"a profession the character must have to apply it, e.g. Enchanting for ring enchants"`
	Stats              map[string]float64 `json:"stats,omitempty"`
}

func enchantSearch() spec.Entry {
	return spec.Tool[enchantSearchInput, enchantSearchOutput]{
		Name:    "db_search_enchants",
		Title:   "Search enchants",
		Summary: "Finds enchants by slot, class, stats and phase, including which profession an enchant needs.",
		Details: "Enchants are named in a gear set by their effect id, which is what this returns.\n\n" +
			"Watch `requiredProfession`: ring enchants need Enchanting, and a character without it simply\n" +
			"does not get them -- the simulator strips them rather than crediting stats the character\n" +
			"could not have.",
		Examples: []spec.Example{
			{Description: "caster head enchants", Args: `{"slot": "Head", "class": "Priest", "hasStats": ["SpellDamage"]}`},
			{Description: "what a ring enchant needs", Args: `{"slot": "Finger1"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input enchantSearchInput) (*mcp.CallToolResult, enchantSearchOutput, error) {
			var output enchantSearchOutput

			query, err := input.query()
			if err != nil {
				return nil, output, err
			}

			unlimited := query
			unlimited.Limit = 0
			all := engine.SearchEnchants(unlimited)
			output.TotalFound = len(all)

			if query.Limit > 0 && len(all) > query.Limit {
				all = all[:query.Limit]
			}
			for _, enchant := range all {
				output.Enchants = append(output.Enchants, summarizeEnchant(enchant))
			}
			return nil, output, nil
		},
	}
}

func (i enchantSearchInput) query() (engine.EnchantQuery, error) {
	query := engine.EnchantQuery{Name: i.Name, MaxPhase: i.MaxPhase, Limit: defaultItemLimit}

	if i.Limit > 0 {
		query.Limit = min(i.Limit, maxItemLimit)
	}
	if i.Slot != "" {
		value, ok := proto.ItemSlot_value["ItemSlot"+i.Slot]
		if !ok {
			return query, fmt.Errorf("unknown slot %q", i.Slot)
		}
		query.Slot, query.HasSlot = proto.ItemSlot(value), true
	}
	if i.Class != "" {
		value, ok := proto.Class_value["Class"+i.Class]
		if !ok {
			return query, fmt.Errorf("unknown class %q", i.Class)
		}
		query.Class = proto.Class(value)
	}
	for _, name := range i.HasStats {
		value, ok := proto.Stat_value["Stat"+name]
		if !ok {
			return query, fmt.Errorf("unknown stat %q", name)
		}
		query.HasStats = append(query.HasStats, proto.Stat(value))
	}
	return query, nil
}

func summarizeEnchant(enchant *proto.UIEnchant) enchantSummary {
	summary := enchantSummary{
		EffectID: enchant.EffectId,
		Name:     enchant.Name,
		Phase:    enchant.Phase,
		Stats:    statsFromArray(enchant.Stats),
	}
	if enchant.RequiredProfession != proto.Profession_ProfessionUnknown {
		summary.RequiredProfession = trimEnum(enchant.RequiredProfession.String(), "Profession")
	}
	for _, slot := range core.EligibleEnchantSlots(enchant) {
		summary.Slots = append(summary.Slots, trimEnum(slot.String(), "ItemSlot"))
	}
	return summary
}

// Gems and enchants carry their stats as a bare array indexed by the Stat enum, the same as items.
func statsFromArray(values []float64) map[string]float64 {
	stats := map[string]float64{}
	for index, value := range values {
		if value == 0 {
			continue
		}
		if name, ok := proto.Stat_name[int32(index)]; ok {
			stats[strings.TrimPrefix(name, "Stat")] = round(value)
		}
	}
	if len(stats) == 0 {
		return nil
	}
	return stats
}
