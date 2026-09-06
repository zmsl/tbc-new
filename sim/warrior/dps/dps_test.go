package dps

import (
	"github.com/wowsims/tbc/sim/common"
	_ "github.com/wowsims/tbc/sim/common" // imported to get item effects included.
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"

	"testing"
)

// Unhides queue-cancel groups in a fresh copy of the shipped fury preset. Mutating the
// preset here, rather than checking in a near-duplicate .apl.json, keeps the tested
// rotation from drifting away from the one users get. GetAplRotation re-reads the file
// on every call, so each variant starts from a clean copy.
func furyRotationWithGroups(label string, groupNames ...string) core.RotationCombo {
	combo := core.GetAplRotation("../../../ui/warrior/dps/apls", "fury")
	combo.Label = label

	for _, name := range groupNames {
		found := false
		for _, item := range combo.Rotation.PriorityList {
			if group := item.GetAction().GetGroupReference(); group != nil && group.GroupName == name {
				item.Hide = false
				found = true
				break
			}
		}
		if !found {
			panic("group reference " + name + " not found, APL probably changed, fix tests!")
		}
	}

	return combo
}

func init() {
	RegisterDpsWarrior()
	common.RegisterAllEffects()
}

func TestDpsWarrior(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator([]core.CharacterSuiteConfig{
		{
			Class:      proto.Class_ClassWarrior,
			Race:       proto.Race_RaceOrc,
			OtherRaces: []proto.Race{proto.Race_RaceHuman},
			GearSet:    core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p1_fury"),
			OtherGearSets: []core.GearSetCombo{
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p1_arms"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p2_fury"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p2_arms"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p3_fury"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p3_arms"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p4_fury"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p4_arms"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p5_fury"),
				core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p5_arms"),
			},
			Talents: DefaultFuryTalents,
			OtherTalentSets: []core.TalentsCombo{
				{Label: "Arms", Talents: DefaultArmsTalents},
			},
			Consumables:      DefaultConsumables,
			SpecOptions:      core.SpecOptionsCombo{Label: "Fury", SpecOptions: DefaultOptions},
			StartingDistance: 25,
			Profession1:      proto.Profession_Engineering,
			Profession2:      proto.Profession_Blacksmithing,

			Rotation: core.GetAplRotation("../../../ui/warrior/dps/apls", "fury"),
			OtherRotations: []core.RotationCombo{
				core.GetAplRotation("../../../ui/warrior/dps/apls", "arms"),
			},

			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypeLeather,
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeFist,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeSword,
					proto.WeaponType_WeaponTypeAxe,
				},
				HandTypes: []proto.HandType{
					proto.HandType_HandTypeMainHand,
					proto.HandType_HandTypeOffHand,
					proto.HandType_HandTypeOneHand,
					proto.HandType_HandTypeTwoHand,
				},
			},
		},
		// Coverage for queue canceling and the Phase 3 tier set, neither of which any
		// case above exercises: canceling is off in the shipped presets, and p3_fury_t6
		// is a new gear set. Kept as its own config on purpose -- the settings matrix is
		// a full cartesian product, so adding a gear set and a rotation to the config
		// above would multiply all 480 existing cases instead of adding these 24.
		{
			Class:         proto.Class_ClassWarrior,
			Race:          proto.Race_RaceOrc,
			GearSet:       core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p3_fury_t6"),
			OtherGearSets: []core.GearSetCombo{core.GetGearSet("../../../ui/warrior/dps/gear_sets", "p3_fury")},
			Talents:       DefaultFuryTalents,
			Consumables:   DefaultConsumables,
			SpecOptions:   core.SpecOptionsCombo{Label: "Fury", SpecOptions: DefaultOptions},

			Rotation: furyRotationWithGroups("hs-cancel", "HS Queue Cancel"),
			OtherRotations: []core.RotationCombo{
				furyRotationWithGroups("hs-cancel-fallback", "HS Queue Cancel", "HS Queue Cancel: No Cleave Rage"),
			},

			StartingDistance: 25,
			Profession1:      proto.Profession_Engineering,
			Profession2:      proto.Profession_Blacksmithing,
		},
	}))
}

var DefaultOptions = &proto.Player_DpsWarrior{
	DpsWarrior: &proto.DpsWarrior{
		Options: &proto.DpsWarrior_Options{
			ClassOptions: &proto.WarriorOptions{
				DefaultShout:  proto.WarriorShout_WarriorShoutBattle,
				DefaultStance: proto.WarriorStance_WarriorStanceBerserker,
			},
		},
	},
}

var DefaultFuryTalents = "3500501130201-05050005505012050115"
var DefaultArmsTalents = "32005011352010500221-0550000500521203"

var DefaultConsumables = &proto.ConsumesSpec{
	PotId:       22838,
	FlaskId:     22854,
	FoodId:      27658,
	ConjuredId:  22788,
	ExplosiveId: 30217,
	SuperSapper: true,
	OhImbueId:   29453,
	ScrollAgi:   true,
	ScrollStr:   true,
}
