package feralcat

import (
	"testing"

	"github.com/wowsims/tbc/sim/common"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

func init() {
	RegisterFeralCatDruid()
	common.RegisterAllEffects()
}

func TestFeralCat(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator([]core.CharacterSuiteConfig{
		{
			Class:      proto.Class_ClassDruid,
			Race:       proto.Race_RaceNightElf,
			OtherRaces: []proto.Race{proto.Race_RaceTauren},

			GearSet: core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p1_realistic_6p"),
			OtherGearSets: []core.GearSetCombo{
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "pre_raid"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p1_realistic_9p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p1_bis_6p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p1_bis_9p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p1_alt_6p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p1_alt_9p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p2_6p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p2_9p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p2_alt_6p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p2_alt_9p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p3_6p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p3_9p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p4_6p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p4_9p"),
				core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p5"),
			},

			Talents: DefaultTalents,
			OtherTalentSets: []core.TalentsCombo{
				{Label: "Monocat", Talents: MonocatTalents},
			},

			SpecOptions: core.SpecOptionsCombo{Label: "Standard", SpecOptions: DefaultSpecOptions},

			Rotation: core.GetAplRotation("../../../ui/druid/feralcat/apls", "default"),
			OtherRotations: []core.RotationCombo{
				{
					Label: "Simple",
					Rotation: &proto.APLRotation{
						Type: proto.APLRotation_TypeSimple,
					},
				},
			},

			Consumables: DefaultConsumables,

			Profession1: proto.Profession_Engineering,
			Profession2: proto.Profession_Enchanting,

			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypeLeather,
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeFist,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeStaff,
				},
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeIdol,
				},
			},

			EPReferenceStat: proto.Stat_StatAttackPower,
			StatsToWeigh: []proto.Stat{
				proto.Stat_StatAgility,
				proto.Stat_StatStrength,
				proto.Stat_StatAttackPower,
				proto.Stat_StatFeralAttackPower,
				proto.Stat_StatMeleeHitRating,
				proto.Stat_StatExpertiseRating,
				proto.Stat_StatMeleeCritRating,
				proto.Stat_StatMeleeHasteRating,
				proto.Stat_StatArmorPenetration,
			},
		},
	}))
}

// The rotation is the real APL preset rather than TypeSimple: the APL interpreter is the
// largest single cost in the profile, so a benchmark that bypasses it measures the wrong thing.
func benchmarkRequest() *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Class:         proto.Class_ClassDruid,
				Race:          proto.Race_RaceNightElf,
				TalentsString: DefaultTalents,
				Equipment:     core.GetGearSet("../../../ui/druid/feralcat/gear_sets", "p1_realistic_6p").GearSet,
				Consumables:   DefaultConsumables,
				Spec:          DefaultSpecOptions,
				// The UI defaults to 100ms (Player.applySharedDefaults) and preset builds carry it.
				// Leaving it unset lands on the 10ms floor in character.go, which makes the rotation
				// re-evaluate ten times more often than any real sim does -- and since the rotation is
				// the most expensive thing in the loop, that inflates its share enormously. An
				// iteration at 10ms costs about five times one at 100ms, essentially all of it polling
				// a user never pays for.
				ReactionTimeMs: 100,
				Rotation:       core.GetAplRotation("../../../ui/druid/feralcat/apls", "default").Rotation,
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

const DefaultTalents = "-503032132322105301251-05503301"
const MonocatTalents = "-553002132322105301051-05503301"

var DefaultSpecOptions = &proto.Player_FeralCatDruid{
	FeralCatDruid: &proto.FeralCatDruid{
		Rotation: &proto.FeralCatDruid_Rotation{
			FinishingMove:      proto.FeralCatDruid_Rotation_Rip,
			Biteweave:          true,
			RipMinComboPoints:  5,
			BiteMinComboPoints: 5,
			MangleTrick:        true,
			MaintainFaerieFire: false,
		},
		Options: &proto.FeralCatDruid_Options{},
	},
}

var DefaultConsumables = &proto.ConsumesSpec{
	PotId:            22838, // Haste Potion
	BattleElixirId:   22831, // Elixir of Major Agility
	GuardianElixirId: 32067, // Elixir of Draenic Wisdom
	FoodId:           27664, // Grilled Mudfish
	MhImbueId:        34340, // Adamantite Weightstone
	ConjuredId:       12662, // Demonic Rune
	SuperSapper:      true,
	GoblinSapper:     true,
	ScrollAgi:        true,
	ScrollStr:        true,
}
