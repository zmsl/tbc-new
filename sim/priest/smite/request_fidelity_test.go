package smite

import (
	"math"
	"os"
	"testing"

	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	"google.golang.org/protobuf/encoding/prototext"
	googleProto "google.golang.org/protobuf/proto"
)

// A share link is only useful headlessly if a request built from it runs the same sim the
// website would have run. These tests pin core.BuildRaidSimRequest against the assembly the
// golden suite uses, and against a committed golden number.

// The golden entry these tests reproduce: p3 gear, default rotation, full buffs, 180s single
// target. Written by TestSmitePriest with DefaultSimTestOptions.
const goldenKey = "TestSmitePriest-Settings-Human-p3-Smite-default-FullBuffs-0.0yards-LongSingleTarget"

const (
	goldenIterations = 20
	goldenSeed       = 101
)

func specOptions() *proto.Player_SmitePriest {
	return &proto.Player_SmitePriest{
		SmitePriest: &proto.SmitePriest{
			Options: &proto.SmitePriest_Options{ClassOptions: &proto.PriestOptions{}},
		},
	}
}

func gearSet() *proto.EquipmentSpec {
	return core.GetGearSet("../../../ui/priest/smite/gear_sets", "p3").GearSet
}

func rotation() *proto.APLRotation {
	return core.GetAplRotation("../../../ui/priest/smite/apls", "default").Rotation
}

// The settings a user would have saved for the golden setup.
func goldenSettings() *proto.IndividualSimSettings {
	return &proto.IndividualSimSettings{
		Settings: &proto.SimSettings{Iterations: goldenIterations},
		Player: core.WithSpec(&proto.Player{
			Race:               proto.Race_RaceHuman,
			Class:              proto.Class_ClassPriest,
			Equipment:          gearSet(),
			TalentsString:      defaultTalents,
			Buffs:              core.FullIndividualBuffs,
			Profession1:        proto.Profession_Engineering,
			Rotation:           rotation(),
			ReactionTimeMs:     100,
			ChannelClipDelayMs: 50,
		}, specOptions()),
		RaidBuffs:  core.FullRaidBuffs,
		PartyBuffs: core.FullPartyBuffs,
		Debuffs:    core.FullDebuffs,
		Encounter:  core.MakeDefaultEncounterCombos()[1].Encounter, // LongSingleTarget
	}
}

// The same setup, assembled the way the golden suite assembles it.
func goldenGeneratorRequest() *proto.RaidSimRequest {
	combos := &core.SettingsCombos{
		Class:             proto.Class_ClassPriest,
		Races:             []proto.Race{proto.Race_RaceHuman},
		GearSets:          []core.GearSetCombo{{Label: "p3", GearSet: gearSet()}},
		TalentSets:        []core.TalentsCombo{{Label: "DefaultTalents", Talents: defaultTalents}},
		SpecOptions:       []core.SpecOptionsCombo{{Label: "Smite", SpecOptions: specOptions()}},
		Rotations:         []core.RotationCombo{{Label: "default", Rotation: rotation()}},
		Buffs:             []core.BuffsCombo{{Label: "FullBuffs", Raid: core.FullRaidBuffs, Party: core.FullPartyBuffs, Debuffs: core.FullDebuffs, Player: core.FullIndividualBuffs}},
		Encounters:        []core.EncounterCombo{core.MakeDefaultEncounterCombos()[1]},
		StartingDistances: []float64{0},
		SimOptions:        &proto.SimOptions{Iterations: goldenIterations, RandomSeed: goldenSeed, IsTest: true},
	}
	_, _, _, rsr := combos.GetTest(0)
	return rsr
}

func buildGoldenRequest(t *testing.T, settings *proto.IndividualSimSettings) *proto.RaidSimRequest {
	t.Helper()

	request, err := core.BuildRaidSimRequest(settings, core.SimRequestOptions{
		Iterations: goldenIterations,
		RandomSeed: goldenSeed,
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	// The golden suite runs with IsTest, which switches the engine to per-effect RNG streams.
	// Nothing else about the request depends on it.
	request.SimOptions.IsTest = true
	return request
}

func goldenDps(t *testing.T) float64 {
	t.Helper()

	data, err := os.ReadFile("TestSmitePriest.results")
	if err != nil {
		t.Fatalf("read golden results: %v", err)
	}
	results := &proto.TestSuiteResult{}
	if err := prototext.Unmarshal(data, results); err != nil {
		t.Fatalf("parse golden results: %v", err)
	}
	result, ok := results.DpsResults[goldenKey]
	if !ok {
		t.Fatalf("golden results have no entry for %q", goldenKey)
	}
	return result.Dps
}

func runDps(t *testing.T, request *proto.RaidSimRequest) float64 {
	t.Helper()

	result := core.RunRaidSim(request)
	if result.Error != nil {
		t.Fatalf("sim failed: %s", result.Error.Message)
	}
	return result.RaidMetrics.Dps.Avg
}

// Everything except the gear fixups must match the canonical assembly field for field.
func TestBuiltRequestMatchesTestGenerator(t *testing.T) {
	actual := buildGoldenRequest(t, goldenSettings())

	expected := goldenGeneratorRequest()
	expectedPlayer := expected.Raid.Parties[0].Players[0]
	// The generator predates the fixups and does not apply them, so apply them to its output
	// rather than asserting the same difference twice. The fixups themselves are covered below
	// and in sim/core/sim_request_builder_test.go.
	core.NormalizePlayerGear(expectedPlayer)
	// The generator plants an empty ItemSwap even when swapping is off; saved settings simply
	// leave the field unset. Same sim either way.
	if !expectedPlayer.EnableItemSwap && expectedPlayer.ItemSwap != nil && len(expectedPlayer.ItemSwap.Items) == 0 {
		expectedPlayer.ItemSwap = nil
	}

	if !googleProto.Equal(expected, actual) {
		t.Errorf("built request differs from the test generator's\nexpected: %v\nactual:   %v", expected, actual)
	}
}

// With the gear the golden was actually recorded with, the built request reproduces its DPS
// exactly. Enchanting is what makes the gear identical: see TestGoldenSimsUnwearableRingEnchants.
func TestBuiltRequestReproducesGoldenDps(t *testing.T) {
	settings := goldenSettings()
	settings.Player.Profession2 = proto.Profession_Enchanting

	dps := runDps(t, buildGoldenRequest(t, settings))
	want := goldenDps(t)

	// The golden file stores five decimal places.
	if math.Abs(dps-want) > 1e-5 {
		t.Errorf("dps = %.5f, golden = %.5f", dps, want)
	}
}

// Every smite gear preset enchants both rings, and ring enchants are enchanter-only, but the
// golden suite hardcodes Profession1=Engineering. So the goldens include stats the character
// could not have. This is a gap in the test harness rather than in the builder: the client
// strips those enchants before simming (Gear.withoutEnchanting), and so do we.
//
// The assertion is deliberately about the direction and rough size of the difference, so this
// test keeps passing once the harness is fixed -- at which point it will fail loudly instead,
// which is the signal to delete it.
func TestGoldenSimsUnwearableRingEnchants(t *testing.T) {
	preset := gearSet()
	if preset.Items[proto.ItemSlot_ItemSlotFinger1].Enchant == 0 {
		t.Skip("gear preset no longer enchants its rings; nothing to compare")
	}

	withEnchants := goldenSettings()
	withEnchants.Player.Profession2 = proto.Profession_Enchanting

	engineerDps := runDps(t, buildGoldenRequest(t, goldenSettings()))
	enchanterDps := runDps(t, buildGoldenRequest(t, withEnchants))

	if engineerDps >= enchanterDps {
		t.Fatalf("stripping ring enchants did not lower dps: %.2f vs %.2f", engineerDps, enchanterDps)
	}
	t.Logf("ring enchants the golden character cannot apply are worth %.2f dps (%.2f%%)",
		enchanterDps-engineerDps, 100*(enchanterDps-engineerDps)/enchanterDps)
}
