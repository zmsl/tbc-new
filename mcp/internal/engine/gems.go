package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

// Gems and enchants are the other half of gearing, and the simulator's own records drop almost
// everything needed to choose one: which colour a gem is, what a meta gem requires to activate,
// which slots an enchant may go in. All of it is in the client database, so it is read from there
// the same way items are.

// GemQuery filters the gem database. A zero field means "no restriction".
type GemQuery struct {
	Name       string
	Color      proto.GemColor
	HasColor   bool
	MaxPhase   int32
	MinQuality proto.ItemQuality
	HasStats   []proto.Stat
	Limit      int
}

// SearchGems returns the gems matching a query, most recent phase and highest quality first.
func SearchGems(query GemQuery) []*proto.UIGem {
	loadDatabase()

	name := strings.ToLower(strings.TrimSpace(query.Name))
	var matches []*proto.UIGem

	for _, gem := range uiDatabase.Gems {
		if name != "" && !strings.Contains(strings.ToLower(gem.Name), name) {
			continue
		}
		if query.MaxPhase > 0 && gem.Phase > query.MaxPhase {
			continue
		}
		if query.MinQuality != proto.ItemQuality_ItemQualityJunk && gem.Quality < query.MinQuality {
			continue
		}
		// Hybrids count for both of their colours, which is how socket bonuses and meta gem
		// requirements read them, so asking for blue should return the blue-purple gems too.
		if query.HasColor && !core.ColorIntersects(gem.Color, query.Color) {
			continue
		}
		if !gemHasStats(gem, query.HasStats) {
			continue
		}
		matches = append(matches, gem)
	}

	slices.SortFunc(matches, func(a, b *proto.UIGem) int {
		if a.Phase != b.Phase {
			return int(b.Phase - a.Phase)
		}
		if a.Quality != b.Quality {
			return int(b.Quality - a.Quality)
		}
		return strings.Compare(a.Name, b.Name)
	})

	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches
}

func gemHasStats(gem *proto.UIGem, wanted []proto.Stat) bool {
	for _, stat := range wanted {
		index := int(stat)
		if index >= len(gem.Stats) || gem.Stats[index] == 0 {
			return false
		}
	}
	return true
}

// Gem returns one gem by id.
func Gem(id int32) (*proto.UIGem, bool) {
	loadDatabase()
	gem, ok := gearLookup.Gems[id]
	return gem, ok
}

// MetaRequirement describes in words what a meta gem needs to activate, or "" for a gem that is
// not a meta or has no known requirement. Without this an agent can be told by gear_validate that
// a meta gem is inactive and have no way to work out what would fix it.
func MetaRequirement(gemID int32) string {
	condition, ok := core.MetaGemConditions[gemID]
	if !ok {
		return ""
	}

	if condition.Greater != proto.GemColor_GemColorUnknown {
		return fmt.Sprintf("more %s gems than %s gems", colorName(condition.Greater), colorName(condition.Lesser))
	}

	var parts []string
	for _, requirement := range []struct {
		count int
		color proto.GemColor
	}{
		{condition.MinRed, proto.GemColor_GemColorRed},
		{condition.MinYellow, proto.GemColor_GemColorYellow},
		{condition.MinBlue, proto.GemColor_GemColorBlue},
	} {
		if requirement.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", requirement.count, colorName(requirement.color)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "at least " + strings.Join(parts, ", ") + " (hybrid gems count for both of their colours)"
}

func colorName(color proto.GemColor) string {
	return strings.ToLower(strings.TrimPrefix(color.String(), "GemColor"))
}

// EnchantQuery filters the enchant database.
type EnchantQuery struct {
	Name     string
	Slot     proto.ItemSlot
	HasSlot  bool
	Class    proto.Class
	MaxPhase int32
	HasStats []proto.Stat
	Limit    int
}

// SearchEnchants returns the enchants matching a query, most recent phase first.
func SearchEnchants(query EnchantQuery) []*proto.UIEnchant {
	loadDatabase()

	name := strings.ToLower(strings.TrimSpace(query.Name))
	var matches []*proto.UIEnchant

	for _, enchant := range uiDatabase.Enchants {
		if name != "" && !strings.Contains(strings.ToLower(enchant.Name), name) {
			continue
		}
		if query.MaxPhase > 0 && enchant.Phase > query.MaxPhase {
			continue
		}
		if query.Class != proto.Class_ClassUnknown && !enchantAllowsClass(enchant, query.Class) {
			continue
		}
		// An armor kit goes in several slots, which EligibleEnchantSlots already accounts for.
		if query.HasSlot && !slices.Contains(core.EligibleEnchantSlots(enchant), query.Slot) {
			continue
		}
		if !enchantHasStats(enchant, query.HasStats) {
			continue
		}
		matches = append(matches, enchant)
	}

	slices.SortFunc(matches, func(a, b *proto.UIEnchant) int {
		if a.Phase != b.Phase {
			return int(b.Phase - a.Phase)
		}
		return strings.Compare(a.Name, b.Name)
	})

	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches
}

func enchantAllowsClass(enchant *proto.UIEnchant, class proto.Class) bool {
	if len(enchant.ClassAllowlist) == 0 {
		return true
	}
	return slices.Contains(enchant.ClassAllowlist, class)
}

func enchantHasStats(enchant *proto.UIEnchant, wanted []proto.Stat) bool {
	for _, stat := range wanted {
		index := int(stat)
		if index >= len(enchant.Stats) || enchant.Stats[index] == 0 {
			return false
		}
	}
	return true
}
