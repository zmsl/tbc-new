package engine

import (
	"slices"
	"strings"
	"sync"

	"github.com/wowsims/tbc/assets/database"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

// The engine keeps a trimmed projection of the item database -- SimItem drops phase, quality,
// sources, unique-equipped and class restrictions, because the simulation does not need them.
// Searching for gear does need them, so the full UIDatabase is loaded here as well. It is
// embedded in the binary either way, and never mutated, so loading it once at first use costs a
// few megabytes and nothing else.
var (
	databaseOnce sync.Once
	uiDatabase   *proto.UIDatabase
	gearLookup   core.GearLookup
)

func loadDatabase() {
	databaseOnce.Do(func() {
		uiDatabase = database.Load()

		gearLookup = core.GearLookup{
			Items:    make(map[int32]*proto.UIItem, len(uiDatabase.Items)),
			Enchants: uiDatabase.Enchants,
			Gems:     make(map[int32]*proto.UIGem, len(uiDatabase.Gems)),
		}
		for _, item := range uiDatabase.Items {
			gearLookup.Items[item.Id] = item
		}
		for _, gem := range uiDatabase.Gems {
			gearLookup.Gems[gem.Id] = gem
		}
	})
}

// Database returns the full item database.
func Database() *proto.UIDatabase {
	loadDatabase()
	return uiDatabase
}

// GearLookup returns the item, enchant and gem records the equippability rules need.
func GearLookup() core.GearLookup {
	loadDatabase()
	return gearLookup
}

// Item returns one item by id.
func Item(id int32) (*proto.UIItem, bool) {
	loadDatabase()
	item, ok := gearLookup.Items[id]
	return item, ok
}

// ItemQuery filters the item database. A zero field means "no restriction", so an empty query
// matches everything.
type ItemQuery struct {
	Name       string
	Slot       proto.ItemSlot
	HasSlot    bool
	Class      proto.Class
	MaxPhase   int32
	MinQuality proto.ItemQuality
	HasStats   []proto.Stat
	Limit      int
}

// SearchItems returns the items matching a query, best first: higher phase, then higher item
// level, so the most relevant results survive the limit.
func SearchItems(query ItemQuery) []*proto.UIItem {
	loadDatabase()

	name := strings.ToLower(strings.TrimSpace(query.Name))
	var matches []*proto.UIItem

	for _, item := range uiDatabase.Items {
		if name != "" && !strings.Contains(strings.ToLower(item.Name), name) {
			continue
		}
		if query.MaxPhase > 0 && item.Phase > query.MaxPhase {
			continue
		}
		if query.MinQuality != proto.ItemQuality_ItemQualityJunk && item.Quality < query.MinQuality {
			continue
		}
		if query.Class != proto.Class_ClassUnknown && !itemAllowsClass(item, query.Class) {
			continue
		}
		if query.HasSlot && !slices.Contains(core.EligibleItemSlots(item), query.Slot) {
			continue
		}
		if !hasStats(item, query.HasStats) {
			continue
		}
		matches = append(matches, item)
	}

	sortItems(matches)

	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches
}

func itemAllowsClass(item *proto.UIItem, class proto.Class) bool {
	if len(item.ClassAllowlist) == 0 {
		return true
	}
	for _, allowed := range item.ClassAllowlist {
		if allowed == class {
			return true
		}
	}
	return false
}

// An item "has" a stat when any of its scaling options carries a non-zero value for it. Items
// carry several scaling options for upgrade levels; TBC items have one.
func hasStats(item *proto.UIItem, wanted []proto.Stat) bool {
	for _, stat := range wanted {
		index := int(stat)
		found := false
		for _, option := range item.ScalingOptions {
			if option == nil {
				continue
			}
			if value, ok := option.Stats[int32(index)]; ok && value != 0 {
				found = true
				break
			}
		}
		if !found && index < len(item.Stats) && item.Stats[index] != 0 {
			found = true
		}
		if !found {
			return false
		}
	}
	return true
}

// Sorted by relevance: phase first, because a phase 5 item is not an answer to a phase 3
// question, then item level within a phase, then name for a stable order.
func sortItems(items []*proto.UIItem) {
	slices.SortFunc(items, func(a, b *proto.UIItem) int {
		if a.Phase != b.Phase {
			return int(b.Phase - a.Phase)
		}
		if a.Ilvl != b.Ilvl {
			return int(b.Ilvl - a.Ilvl)
		}
		return strings.Compare(a.Name, b.Name)
	})
}
