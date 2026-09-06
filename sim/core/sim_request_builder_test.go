package core

import (
	"testing"

	"github.com/wowsims/tbc/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// 33/28/0 smite priest, matching sim/priest/smite/smite_test.go.
const testSmiteTalents = "5051000130505002501-225051000320152-"

func testSmiteSpecOptions() *proto.Player_SmitePriest {
	return &proto.Player_SmitePriest{
		SmitePriest: &proto.SmitePriest{
			Options: &proto.SmitePriest_Options{ClassOptions: &proto.PriestOptions{}},
		},
	}
}

func testSmiteGearSet() *proto.EquipmentSpec {
	return GetGearSet("../../ui/priest/smite/gear_sets", "p3").GearSet
}

func testSmiteRotation() *proto.APLRotation {
	return GetAplRotation("../../ui/priest/smite/apls", "default").Rotation
}

// The settings a user would have saved for the setup the test generator builds by hand.
func testSmiteSettings() *proto.IndividualSimSettings {
	return &proto.IndividualSimSettings{
		Settings: &proto.SimSettings{Iterations: 2000},
		Player: &proto.Player{
			Race:               proto.Race_RaceHuman,
			Class:              proto.Class_ClassPriest,
			Equipment:          testSmiteGearSet(),
			TalentsString:      testSmiteTalents,
			Buffs:              FullIndividualBuffs,
			Profession1:        proto.Profession_Engineering,
			Rotation:           testSmiteRotation(),
			DistanceFromTarget: 0,
			ReactionTimeMs:     100,
			ChannelClipDelayMs: 50,
			Spec:               testSmiteSpecOptions(),
		},
		RaidBuffs:  FullRaidBuffs,
		PartyBuffs: FullPartyBuffs,
		Debuffs:    FullDebuffs,
		Encounter:  MakeDefaultEncounterCombos()[1].Encounter, // LongSingleTarget
	}
}

func TestBuildRaidSimRequestSimOptions(t *testing.T) {
	settings := testSmiteSettings()

	t.Run("falls back to the saved iteration count and a fixed seed", func(t *testing.T) {
		request, err := BuildRaidSimRequest(settings, SimRequestOptions{})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if request.SimOptions.Iterations != settings.Settings.Iterations {
			t.Errorf("iterations = %d, want %d", request.SimOptions.Iterations, settings.Settings.Iterations)
		}
		// A zero seed would make the engine reach for the clock, and two identical requests would
		// stop producing identical results.
		if request.SimOptions.RandomSeed != DefaultSimSeed {
			t.Errorf("seed = %d, want %d", request.SimOptions.RandomSeed, DefaultSimSeed)
		}
	})

	t.Run("options win over saved settings", func(t *testing.T) {
		request, err := BuildRaidSimRequest(settings, SimRequestOptions{
			Iterations:      37,
			RandomSeed:      99,
			UseLabeledRands: true,
			Encounter:       MakeDefaultEncounterCombos()[0].Encounter, // ShortSingleTarget
		})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if request.SimOptions.Iterations != 37 || request.SimOptions.RandomSeed != 99 {
			t.Errorf("overrides ignored: %v", request.SimOptions)
		}
		if !request.SimOptions.UseLabeledRands {
			t.Error("UseLabeledRands not set")
		}
		if request.Encounter.Duration != ShortDuration {
			t.Errorf("encounter override ignored: duration %v", request.Encounter.Duration)
		}
	})

	t.Run("leaves the settings alone", func(t *testing.T) {
		before := googleProto.Clone(settings)
		if _, err := BuildRaidSimRequest(settings, SimRequestOptions{Iterations: 10}); err != nil {
			t.Fatalf("build: %v", err)
		}
		if !googleProto.Equal(before, settings) {
			t.Error("BuildRaidSimRequest mutated the settings it was given")
		}
	})
}

func TestBuildRaidSimRequestTargetDummies(t *testing.T) {
	for _, tc := range []struct {
		dummies     int32
		wantParties int
		wantActive  int32
	}{
		{dummies: 0, wantParties: 1, wantActive: 0},
		{dummies: 4, wantParties: 1, wantActive: 1},
		{dummies: 12, wantParties: 2, wantActive: 2},
		{dummies: 25, wantParties: 5, wantActive: 5},
	} {
		settings := testSmiteSettings()
		settings.TargetDummies = tc.dummies

		request, err := BuildRaidSimRequest(settings, SimRequestOptions{})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if got := len(request.Raid.Parties); got != tc.wantParties {
			t.Errorf("%d dummies: %d parties, want %d", tc.dummies, got, tc.wantParties)
		}
		if got := request.Raid.NumActiveParties; got != tc.wantActive {
			t.Errorf("%d dummies: NumActiveParties %d, want %d", tc.dummies, got, tc.wantActive)
		}
		if request.Raid.TargetDummies != tc.dummies {
			t.Errorf("%d dummies: TargetDummies %d", tc.dummies, request.Raid.TargetDummies)
		}
	}
}

func TestBuildRaidSimRequestRejectsIncompleteSettings(t *testing.T) {
	settings := testSmiteSettings()
	settings.Encounter = nil
	if _, err := BuildRaidSimRequest(settings, SimRequestOptions{}); err != ErrNoEncounter {
		t.Errorf("missing encounter: err = %v, want %v", err, ErrNoEncounter)
	}

	settings = testSmiteSettings()
	settings.Player = nil
	if _, err := BuildRaidSimRequest(settings, SimRequestOptions{}); err != ErrNoPlayer {
		t.Errorf("missing player: err = %v, want %v", err, ErrNoPlayer)
	}

	if _, err := BuildRaidSimRequest(nil, SimRequestOptions{}); err != ErrNoPlayer {
		t.Errorf("nil settings: err = %v, want %v", err, ErrNoPlayer)
	}
}

// An enchanter keeps their ring enchants.
func TestNormalizePlayerGearKeepsEnchanterRings(t *testing.T) {
	player := testSmiteSettings().Player
	player.Profession2 = proto.Profession_Enchanting

	before := player.Equipment.Items[proto.ItemSlot_ItemSlotFinger1].Enchant
	NormalizePlayerGear(player)

	if got := player.Equipment.Items[proto.ItemSlot_ItemSlotFinger1].Enchant; got != before {
		t.Errorf("ring enchant = %d, want %d kept for an enchanter", got, before)
	}
}
