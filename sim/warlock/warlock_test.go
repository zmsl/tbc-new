package warlock

import (
	"testing"

	"github.com/wowsims/tbc/sim/common"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

func init() {
	RegisterWarlock()
	common.RegisterAllEffects()
}

func TestAffliction(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator([]core.CharacterSuiteConfig{
		{
			Class:      proto.Class_ClassWarlock,
			Race:       proto.Race_RaceOrc,
			OtherRaces: []proto.Race{proto.Race_RaceHuman},
			SpecOptions: core.SpecOptionsCombo{Label: "Affliction", SpecOptions: &proto.Player_Warlock{
				Warlock: &proto.Warlock{
					Options: &proto.Warlock_Options{
						ClassOptions: &proto.WarlockOptions{
							Summon:          proto.WarlockOptions_Imp,
							SacrificeSummon: false,
							Armor:           proto.WarlockOptions_FelArmor,
							CurseOptions:    proto.WarlockOptions_Elements,
						},
					},
				},
			}},
			GearSet:  core.GetGearSet("../../ui/warlock/dps/gear_sets", "preraid"),
			Talents:  "05022221112351055003--50500051220001",
			Rotation: core.GetAplRotation("../../ui/warlock/dps/apls", "affliction"),
			ItemFilter: core.ItemFilter{
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeStaff,
					proto.WeaponType_WeaponTypeSword,
				},
				ArmorType: proto.ArmorType_ArmorTypeCloth,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeWand,
				},
				EnchantBlacklist: []int32{2673, 3225, 3273},
				IDBlacklist:      []int32{28556},
			},
		},
	}))
}

func TestDestruction(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator([]core.CharacterSuiteConfig{
		{
			Class:      proto.Class_ClassWarlock,
			Race:       proto.Race_RaceOrc,
			OtherRaces: []proto.Race{proto.Race_RaceGnome},
			SpecOptions: core.SpecOptionsCombo{Label: "Destruction", SpecOptions: &proto.Player_Warlock{
				Warlock: &proto.Warlock{
					Options: &proto.Warlock_Options{
						ClassOptions: &proto.WarlockOptions{
							Summon:          proto.WarlockOptions_Succubus,
							SacrificeSummon: true,
							Armor:           proto.WarlockOptions_FelArmor,
							CurseOptions:    proto.WarlockOptions_Agony,
						},
					},
				},
			}},
			GearSet:  core.GetGearSet("../../ui/warlock/dps/gear_sets", "preraid"),
			Talents:  "-20500301332101-50500051220051053105",
			Rotation: core.GetAplRotation("../../ui/warlock/dps/apls", "destruction"),
			ItemFilter: core.ItemFilter{
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeStaff,
					proto.WeaponType_WeaponTypeSword,
				},
				ArmorType: proto.ArmorType_ArmorTypeCloth,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeWand,
				},
				EnchantBlacklist: []int32{2673, 3225, 3273},
				IDBlacklist:      []int32{28556},
			},
		},
	}))
}

// Pet + dot coverage for the perf harness: the warlock drives a second unit through the same
// event loop, which no other benchmarked spec does. Mirrors TestAffliction above.
func benchmarkRequest() *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Class:         proto.Class_ClassWarlock,
				Race:          proto.Race_RaceOrc,
				TalentsString: "05022221112351055003--50500051220001",
				Equipment:     core.GetGearSet("../../ui/warlock/dps/gear_sets", "preraid").GearSet,
				Spec: &proto.Player_Warlock{
					Warlock: &proto.Warlock{
						Options: &proto.Warlock_Options{
							ClassOptions: &proto.WarlockOptions{
								Summon:          proto.WarlockOptions_Imp,
								SacrificeSummon: false,
								Armor:           proto.WarlockOptions_FelArmor,
								CurseOptions:    proto.WarlockOptions_Elements,
							},
						},
					},
				},
				Rotation: core.GetAplRotation("../../ui/warlock/dps/apls", "affliction").Rotation,
			},
			nil, nil, nil,
		),
		Encounter: &proto.Encounter{
			Duration: 300,
			Targets:  []*proto.Target{core.NewDefaultTarget()},
		},
		SimOptions: core.AverageDefaultSimTestOptions,
	}
}

// Setup plus one iteration. Dominated by NewEnvironment; tracks the cost paid once per RunSim.
func BenchmarkSimulate(b *testing.B) {
	core.RaidBenchmark(b, benchmarkRequest())
}

// The event loop alone, with setup amortized away.
func BenchmarkIteration(b *testing.B) {
	core.RaidIterationBenchmark(b, benchmarkRequest())
}
