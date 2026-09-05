package smite

import (
	"testing"

	"github.com/wowsims/tbc/sim/common"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

func init() {
	RegisterSmitePriest()
	common.RegisterAllEffects()
}

// 33/28/0 -- Discipline through Power Infusion, Holy through Surge of Light.
const defaultTalents = "5051000130505002501-225051000320152-"

func TestSmitePriest(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator([]core.CharacterSuiteConfig{
		{
			Class: proto.Class_ClassPriest,
			Race:  proto.Race_RaceHuman,
			// Undead adds Devouring Plague and Night Elf adds Starshards, both of which the
			// default rotation casts when the race knows them.
			OtherRaces: []proto.Race{proto.Race_RaceUndead, proto.Race_RaceNightElf},

			SpecOptions: core.SpecOptionsCombo{
				Label: "Smite",
				SpecOptions: &proto.Player_SmitePriest{
					SmitePriest: &proto.SmitePriest{
						Options: &proto.SmitePriest_Options{
							ClassOptions: &proto.PriestOptions{},
						},
					},
				},
			},

			GearSet: core.GetGearSet("../../../ui/priest/smite/gear_sets", "pre_raid"),
			OtherGearSets: []core.GearSetCombo{
				core.GetGearSet("../../../ui/priest/smite/gear_sets", "p3"),
			},

			Talents: defaultTalents,

			// Primary rotation
			Rotation: core.GetAplRotation("../../../ui/priest/smite/apls", "default"),

			// Secondary rotation: casts every implemented spell
			OtherRotations: []core.RotationCombo{
				core.GetAplRotation("../../../ui/priest/smite/apls", "test"),
			},

			ItemFilter: core.ItemFilter{
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeStaff,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeOffHand,
				},
				ArmorType: proto.ArmorType_ArmorTypeCloth,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeWand,
				},
				// Blacklist melee enchants that appear on cloth-relevant slots but
				// are never used by casters.
				EnchantBlacklist: []int32{2673, 3225, 3273},
			},
		},
	}))
}
