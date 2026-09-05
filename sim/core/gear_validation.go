package core

import (
	"fmt"

	"github.com/wowsims/tbc/sim/core/proto"
)

// Rules deciding whether a set of gear could actually be worn in game.
//
// These lived only in the TypeScript client (ui/core/proto_utils/{utils,gems,gear}.ts), which
// meant anything that did not go through the UI -- a checked-in gear preset, a test, a script
// writing EquipmentSpec directly -- bypassed them entirely. Presets shipped with a two-handed
// weapon beside an off-hand, weapon enchants on items that take none, nineteen copies of a
// unique-equipped gem, and meta gems whose colour requirements were never met.
//
// The sim cannot enforce all of this at runtime yet: SimItem and SimEnchant, the projections
// the engine actually consumes, drop the fields these rules need (unique, limit_category,
// extra_types, enchant_type, item_id, spell_id). So the rules are written against the full
// UIItem/UIEnchant instead, and TestGearPresetsAreEquippable applies them to every preset.

var itemTypeToSlots = map[proto.ItemType][]proto.ItemSlot{
	proto.ItemType_ItemTypeHead:     {proto.ItemSlot_ItemSlotHead},
	proto.ItemType_ItemTypeNeck:     {proto.ItemSlot_ItemSlotNeck},
	proto.ItemType_ItemTypeShoulder: {proto.ItemSlot_ItemSlotShoulder},
	proto.ItemType_ItemTypeBack:     {proto.ItemSlot_ItemSlotBack},
	proto.ItemType_ItemTypeChest:    {proto.ItemSlot_ItemSlotChest},
	proto.ItemType_ItemTypeWrist:    {proto.ItemSlot_ItemSlotWrist},
	proto.ItemType_ItemTypeHands:    {proto.ItemSlot_ItemSlotHands},
	proto.ItemType_ItemTypeWaist:    {proto.ItemSlot_ItemSlotWaist},
	proto.ItemType_ItemTypeLegs:     {proto.ItemSlot_ItemSlotLegs},
	proto.ItemType_ItemTypeFeet:     {proto.ItemSlot_ItemSlotFeet},
	proto.ItemType_ItemTypeFinger:   {proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2},
	proto.ItemType_ItemTypeTrinket:  {proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2},
	proto.ItemType_ItemTypeRanged:   {proto.ItemSlot_ItemSlotRanged},
}

// EligibleItemSlots returns the slots an item may be equipped in.
func EligibleItemSlots(item *proto.UIItem) []proto.ItemSlot {
	if slots, ok := itemTypeToSlots[item.Type]; ok {
		return slots
	}
	if item.Type == proto.ItemType_ItemTypeWeapon {
		switch item.HandType {
		case proto.HandType_HandTypeMainHand:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand}
		case proto.HandType_HandTypeOffHand:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotOffHand}
		default:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand}
		}
	}
	return nil
}

// EligibleEnchantSlots returns the slots an enchant might be applied in. Armor kits and the
// like carry extra types beyond their primary one.
func EligibleEnchantSlots(enchant *proto.UIEnchant) []proto.ItemSlot {
	var out []proto.ItemSlot
	for _, t := range append([]proto.ItemType{enchant.Type}, enchant.ExtraTypes...) {
		if slots, ok := itemTypeToSlots[t]; ok {
			out = append(out, slots...)
		} else if t == proto.ItemType_ItemTypeWeapon {
			out = append(out, proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand)
		}
	}
	return out
}

// EnchantAppliesToItem mirrors enchantAppliesToItem in ui/core/proto_utils/utils.ts.
func EnchantAppliesToItem(enchant *proto.UIEnchant, item *proto.UIItem) bool {
	shared := false
	for _, es := range EligibleEnchantSlots(enchant) {
		for _, is := range EligibleItemSlots(item) {
			if es == is {
				shared = true
			}
		}
	}
	if !shared {
		return false
	}

	switch enchant.EnchantType {
	case proto.EnchantType_EnchantTypeTwoHand:
		if item.HandType != proto.HandType_HandTypeTwoHand {
			return false
		}
	case proto.EnchantType_EnchantTypeStaff:
		if item.WeaponType != proto.WeaponType_WeaponTypeStaff {
			return false
		}
	case proto.EnchantType_EnchantTypeShield:
		if item.WeaponType != proto.WeaponType_WeaponTypeShield {
			return false
		}
	}

	// Held-in-off-hand items and shields take only off-hand enchants, and nothing else may take
	// one. All off-hand enchants also apply to shields.
	isOffHand := item.WeaponType == proto.WeaponType_WeaponTypeOffHand ||
		(item.WeaponType == proto.WeaponType_WeaponTypeShield && enchant.EnchantType != proto.EnchantType_EnchantTypeShield)
	if (enchant.EnchantType == proto.EnchantType_EnchantTypeOffHand) != isOffHand {
		return false
	}

	if enchant.Type == proto.ItemType_ItemTypeRanged {
		switch item.RangedWeaponType {
		case proto.RangedWeaponType_RangedWeaponTypeBow,
			proto.RangedWeaponType_RangedWeaponTypeCrossbow,
			proto.RangedWeaponType_RangedWeaponTypeGun:
		default:
			return false
		}
	}
	if item.RangedWeaponType != proto.RangedWeaponType_RangedWeaponTypeWand &&
		item.RangedWeaponType > 0 && enchant.Type != proto.ItemType_ItemTypeRanged {
		return false
	}

	return true
}

// ValidWeaponCombo reports whether the two weapons can be worn together.
func ValidWeaponCombo(mainHand, offHand *proto.UIItem) bool {
	if mainHand != nil && mainHand.HandType == proto.HandType_HandTypeTwoHand && offHand != nil {
		return false
	}
	return offHand == nil || offHand.HandType != proto.HandType_HandTypeTwoHand
}

// MetaGemCondition is the colour requirement a meta gem needs met to be active. A condition is
// either a minimum of each colour or a comparison between two colours, never both.
type MetaGemCondition struct {
	MinRed, MinYellow, MinBlue int
	Greater, Lesser            proto.GemColor
}

// MetaGemConditions mirrors the table in ui/core/proto_utils/gems.ts.
var MetaGemConditions = map[int32]MetaGemCondition{
	25890: {MinRed: 2, MinYellow: 2, MinBlue: 2},                                         // Destructive Skyfire Diamond
	25893: {Greater: proto.GemColor_GemColorBlue, Lesser: proto.GemColor_GemColorYellow}, // Mystical Skyfire Diamond
	25894: {MinRed: 1, MinYellow: 2, MinBlue: 0},                                         // Swift Skyfire Diamond
	25895: {Greater: proto.GemColor_GemColorRed, Lesser: proto.GemColor_GemColorYellow},  // Enigmatic Skyfire Diamond
	25896: {MinRed: 0, MinYellow: 0, MinBlue: 3},                                         // Powerful Earthstorm Diamond
	25897: {Greater: proto.GemColor_GemColorRed, Lesser: proto.GemColor_GemColorBlue},    // Bracing Earthstorm Diamond
	25898: {MinRed: 0, MinYellow: 0, MinBlue: 5},                                         // Tenacious Earthstorm Diamond
	25899: {MinRed: 2, MinYellow: 2, MinBlue: 2},                                         // Brutal Earthstorm Diamond
	25901: {MinRed: 2, MinYellow: 2, MinBlue: 2},                                         // Insightful Earthstorm Diamond
	28556: {MinRed: 1, MinYellow: 2, MinBlue: 0},                                         // Swift Windfire Diamond
	28557: {MinRed: 1, MinYellow: 2, MinBlue: 0},                                         // Swift Starfire Diamond
	32409: {MinRed: 2, MinYellow: 2, MinBlue: 2},                                         // Relentless Earthstorm Diamond
	32410: {MinRed: 2, MinYellow: 2, MinBlue: 2},                                         // Thundering Skyfire Diamond
	32640: {Greater: proto.GemColor_GemColorBlue, Lesser: proto.GemColor_GemColorYellow}, // Potent Unstable Diamond
	32641: {MinRed: 0, MinYellow: 3, MinBlue: 0},                                         // Imbued Unstable Diamond
	34220: {MinRed: 0, MinYellow: 0, MinBlue: 2},                                         // Chaotic Skyfire Diamond
	35501: {MinRed: 0, MinYellow: 1, MinBlue: 2},                                         // Eternal Earthstorm Diamond
	35503: {MinRed: 3, MinYellow: 0, MinBlue: 0},                                         // Ember Skyfire Diamond
}

// countsTowards reports whether a gem counts towards a colour requirement, which is the same
// question as whether it would earn that colour's socket bonus: the hybrids count for both of
// their halves.
func countsTowards(gemColor, category proto.GemColor) bool {
	return ColorIntersects(gemColor, category)
}

// IsMetaGemActive reports whether the meta gem's colour requirement is met by the other gems.
// A meta with no known condition is treated as active, matching the client.
func IsMetaGemActive(metaGemID int32, gemColors []proto.GemColor) bool {
	cond, ok := MetaGemConditions[metaGemID]
	if !ok {
		return true
	}
	count := func(category proto.GemColor) int {
		n := 0
		for _, c := range gemColors {
			if c != proto.GemColor_GemColorMeta && countsTowards(c, category) {
				n++
			}
		}
		return n
	}
	if cond.Greater != proto.GemColor_GemColorUnknown {
		return count(cond.Greater) > count(cond.Lesser)
	}
	return count(proto.GemColor_GemColorRed) >= cond.MinRed &&
		count(proto.GemColor_GemColorYellow) >= cond.MinYellow &&
		count(proto.GemColor_GemColorBlue) >= cond.MinBlue
}

// GearLookup supplies the full item, enchant and gem records a gear set refers to.
type GearLookup struct {
	Items    map[int32]*proto.UIItem
	Enchants []*proto.UIEnchant
	Gems     map[int32]*proto.UIGem
}

// ResolveEnchant finds an enchant by any of the three IDs a gear set may name it with. The
// client does the same (Database.getEnchantById); the sim's own lookup is by effect ID only and
// silently drops anything else.
func (l GearLookup) ResolveEnchant(id int32) *proto.UIEnchant {
	for _, e := range l.Enchants {
		if e.EffectId == id || e.ItemId == id || e.SpellId == id {
			return e
		}
	}
	return nil
}

// ValidateEquipment returns one message per reason the gear set could not be worn in game.
func (l GearLookup) ValidateEquipment(spec *proto.EquipmentSpec) []string {
	var problems []string
	items := make([]*proto.UIItem, int(proto.ItemSlot_ItemSlotRanged)+1)
	itemCounts := map[int32]int{}
	var gemColors []proto.GemColor
	gemCounts := map[int32]int{}

	for slot, itemSpec := range spec.Items {
		if itemSpec == nil || itemSpec.Id == 0 || slot >= len(items) {
			continue
		}
		item, ok := l.Items[itemSpec.Id]
		if !ok {
			problems = append(problems, fmt.Sprintf("slot %d: unknown item %d", slot, itemSpec.Id))
			continue
		}
		items[slot] = item
		itemCounts[item.Id]++

		if !slicesContainsSlot(EligibleItemSlots(item), proto.ItemSlot(slot)) {
			problems = append(problems, fmt.Sprintf("%s cannot be equipped in slot %d", item.Name, slot))
		}

		if itemSpec.Enchant != 0 {
			enchant := l.ResolveEnchant(itemSpec.Enchant)
			if enchant == nil {
				problems = append(problems, fmt.Sprintf("%s: no enchant with id %d", item.Name, itemSpec.Enchant))
			} else if !EnchantAppliesToItem(enchant, item) {
				problems = append(problems, fmt.Sprintf("%s cannot take %s", item.Name, enchant.Name))
			}
		}

		for _, gemID := range itemSpec.Gems {
			if gemID == 0 {
				continue
			}
			gem, ok := l.Gems[gemID]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: unknown gem %d", item.Name, gemID))
				continue
			}
			gemColors = append(gemColors, gem.Color)
			gemCounts[gemID]++
		}
	}

	for id, n := range itemCounts {
		if n > 1 && l.Items[id].Unique {
			problems = append(problems, fmt.Sprintf("%s is unique-equipped but appears %d times", l.Items[id].Name, n))
		}
	}
	for id, n := range gemCounts {
		if n > 1 && l.Gems[id].Unique {
			problems = append(problems, fmt.Sprintf("%s is unique-equipped but appears %d times", l.Gems[id].Name, n))
		}
	}

	if !ValidWeaponCombo(items[proto.ItemSlot_ItemSlotMainHand], items[proto.ItemSlot_ItemSlotOffHand]) {
		problems = append(problems, "a two-handed weapon is equipped alongside an off-hand")
	}

	for id := range gemCounts {
		if l.Gems[id].Color == proto.GemColor_GemColorMeta && !IsMetaGemActive(id, gemColors) {
			problems = append(problems, fmt.Sprintf("%s is socketed but its colour requirement is not met", l.Gems[id].Name))
		}
	}

	return problems
}

func slicesContainsSlot(slots []proto.ItemSlot, slot proto.ItemSlot) bool {
	for _, s := range slots {
		if s == slot {
			return true
		}
	}
	return false
}
