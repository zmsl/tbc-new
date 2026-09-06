package rogue

import (
	"testing"

	"github.com/wowsims/tbc/sim/common"
	_ "github.com/wowsims/tbc/sim/common" // imported to get item effects included.
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

func init() {
	RegisterRogue()
	common.RegisterAllEffects()
}

func TestRogue(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator([]core.CharacterSuiteConfig{
		{
			Class:      proto.Class_ClassRogue,
			Race:       proto.Race_RaceHuman,
			OtherRaces: []proto.Race{proto.Race_RaceOrc},
			GearSet:    core.GetGearSet("../../ui/rogue/dps/gear_sets", "preraid"),
			OtherGearSets: []core.GearSetCombo{
				core.GetGearSet("../../ui/rogue/dps/gear_sets", "p1"),
				//core.GetGearSet("../../../ui/rogue/combat/gear_sets", "p4_combat"),
			},
			Talents:     DefaultTalents,
			Consumables: DefaultConsumables,
			SpecOptions: core.SpecOptionsCombo{Label: "Rogue", SpecOptions: DefaultOptions},

			Rotation:       core.GetAplRotation("../../ui/rogue/dps/apls", "swords"),
			OtherRotations: []core.RotationCombo{},
			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypeLeather,
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeFist,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeSword,
				},
				HandTypes: []proto.HandType{
					proto.HandType_HandTypeMainHand,
					proto.HandType_HandTypeOffHand,
					proto.HandType_HandTypeOneHand,
				},
			},
		},
	}))
}

var DefaultOptions = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: &proto.Rogue_Options{
			ClassOptions: &proto.RogueOptions{},
		},
	},
}

var DefaultTalents = "00532012502-023305200005015002321151"

var DefaultConsumables = &proto.ConsumesSpec{
	FlaskId:    22854,
	FoodId:     33872,
	PotId:      22838,
	ConjuredId: 7676,
	OhImbueId:  27186,
}

// Energy/combo-point coverage for the perf harness, and the longest priority list of the five
// benchmarked specs. Mirrors TestRogue above.
func benchmarkRequest() *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Class:         proto.Class_ClassRogue,
				Race:          proto.Race_RaceHuman,
				TalentsString: DefaultTalents,
				Equipment:     core.GetGearSet("../../ui/rogue/dps/gear_sets", "p1").GearSet,
				Consumables:   DefaultConsumables,
				Spec:          DefaultOptions,
				// The UI defaults to 100ms (Player.applySharedDefaults) and preset builds carry it.
				// Leaving it unset lands on the 10ms floor in character.go, which makes the rotation
				// re-evaluate ten times more often than any real sim does -- and since the rotation is
				// the most expensive thing in the loop, that inflates its share enormously. An
				// iteration at 10ms costs about five times one at 100ms, essentially all of it polling
				// a user never pays for.
				ReactionTimeMs: 100,
				Rotation:       core.GetAplRotation("../../ui/rogue/dps/apls", "swords").Rotation,
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
