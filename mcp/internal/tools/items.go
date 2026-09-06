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

const (
	defaultItemLimit = 25
	maxItemLimit     = 200
)

type itemSearchInput struct {
	Name       string   `json:"name,omitempty" jsonschema:"match items whose name contains this text, case-insensitively"`
	Slot       string   `json:"slot,omitempty" jsonschema:"restrict to items equippable in this slot, e.g. Head, Trinket1, MainHand"`
	Class      string   `json:"class,omitempty" jsonschema:"restrict to items this class may equip, e.g. Priest"`
	MaxPhase   int32    `json:"maxPhase,omitempty" jsonschema:"only items available by this raid phase (1-5). Use it to answer 'best in phase N' questions."`
	MinQuality string   `json:"minQuality,omitempty" jsonschema:"minimum quality: Common, Uncommon, Rare, Epic or Legendary"`
	HasStats   []string `json:"hasStats,omitempty" jsonschema:"only items carrying all of these stats, e.g. [\"SpellDamage\", \"SpellHitRating\"]"`
	Limit      int      `json:"limit,omitempty" jsonschema:"how many items to return, 1-200. Defaults to 25."`
}

type itemSearchOutput struct {
	Items      []itemSummary `json:"items" jsonschema:"matching items, most recent phase and highest item level first"`
	TotalFound int           `json:"totalFound" jsonschema:"how many items matched before the limit was applied"`
}

type itemSummary struct {
	ID      int32              `json:"id" jsonschema:"the item's id, as used in gear sets"`
	Name    string             `json:"name"`
	Ilvl    int32              `json:"ilvl" jsonschema:"item level"`
	Phase   int32              `json:"phase" jsonschema:"the raid phase the item becomes available in"`
	Quality string             `json:"quality" jsonschema:"Common, Uncommon, Rare, Epic or Legendary"`
	Slots   []string           `json:"slots" jsonschema:"slots this item can be equipped in"`
	Unique  bool               `json:"unique,omitempty" jsonschema:"true when only one may be equipped"`
	Sockets []string           `json:"sockets,omitempty" jsonschema:"gem socket colours"`
	Stats   map[string]float64 `json:"stats,omitempty" jsonschema:"the item's stats; zero-valued stats are omitted"`
	Source  string             `json:"source,omitempty" jsonschema:"where the item comes from, when known"`
}

func itemSearch() spec.Entry {
	return spec.Tool[itemSearchInput, itemSearchOutput]{
		Name:    "db_search_items",
		Title:   "Search the item database",
		Summary: "Finds items by name, slot, class, phase, quality and stats.",
		Details: "This is the item pool to draw candidates from before simulating anything. Filtering by\n" +
			"maxPhase is what makes a 'best in phase N' question answerable: the simulator's own item\n" +
			"records drop phase and quality, so nothing else can filter on them.",
		Examples: []spec.Example{
			{Description: "caster helms available by phase 3", Args: `{"slot": "Head", "class": "Priest", "maxPhase": 3, "hasStats": ["SpellDamage"]}`},
			{Description: "find a trinket by name", Args: `{"name": "Icon of the Silver Crescent"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input itemSearchInput) (*mcp.CallToolResult, itemSearchOutput, error) {
			var output itemSearchOutput

			query, err := input.query()
			if err != nil {
				return nil, output, err
			}

			// Count first, then trim, so the caller can tell a narrow search from a truncated one.
			unlimited := query
			unlimited.Limit = 0
			all := engine.SearchItems(unlimited)
			output.TotalFound = len(all)

			if query.Limit > 0 && len(all) > query.Limit {
				all = all[:query.Limit]
			}
			for _, item := range all {
				output.Items = append(output.Items, summarizeItem(item))
			}
			return nil, output, nil
		},
	}
}

func (i itemSearchInput) query() (engine.ItemQuery, error) {
	query := engine.ItemQuery{
		Name:     i.Name,
		MaxPhase: i.MaxPhase,
		Limit:    defaultItemLimit,
	}

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

func summarizeItem(item *proto.UIItem) itemSummary {
	summary := itemSummary{
		ID:      item.Id,
		Name:    item.Name,
		Ilvl:    item.Ilvl,
		Phase:   item.Phase,
		Quality: trimEnum(item.Quality.String(), "ItemQuality"),
		Unique:  item.Unique,
		Source:  describeSource(item),
	}

	for _, slot := range core.EligibleItemSlots(item) {
		summary.Slots = append(summary.Slots, trimEnum(slot.String(), "ItemSlot"))
	}
	for _, socket := range item.GemSockets {
		summary.Sockets = append(summary.Sockets, trimEnum(socket.String(), "GemColor"))
	}

	summary.Stats = map[string]float64{}
	for _, option := range item.ScalingOptions {
		if option == nil {
			continue
		}
		for index, value := range option.Stats {
			if value == 0 {
				continue
			}
			if name, ok := proto.Stat_name[index]; ok {
				summary.Stats[strings.TrimPrefix(name, "Stat")] = round(value)
			}
		}
		break
	}
	if len(summary.Stats) == 0 {
		summary.Stats = nil
	}

	return summary
}

// One line saying where an item comes from. The database can list several sources; the first is
// enough to tell a raid drop from a crafted item or a reputation reward.
func describeSource(item *proto.UIItem) string {
	for _, source := range item.Sources {
		switch actual := source.Source.(type) {
		case *proto.UIItemSource_Crafted:
			return "crafted (" + trimEnum(actual.Crafted.Profession.String(), "Profession") + ")"
		case *proto.UIItemSource_Drop:
			if actual.Drop.OtherName != "" {
				return "drop: " + actual.Drop.OtherName
			}
			return "drop"
		case *proto.UIItemSource_Quest:
			return "quest: " + actual.Quest.Name
		case *proto.UIItemSource_SoldBy:
			return "vendor: " + actual.SoldBy.NpcName
		case *proto.UIItemSource_Rep:
			return "reputation: " + trimEnum(actual.Rep.RepFactionId.String(), "RepFaction")
		}
	}
	return ""
}
