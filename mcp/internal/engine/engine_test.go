package engine_test

import (
	"path/filepath"
	"testing"

	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/sim"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

func init() {
	sim.RegisterAll()
}

func testConfig() engine.Config {
	return engine.FileConfig(filepath.Join("..", "..", "..", "ui"))
}

func specName(spec proto.Spec) string {
	return spec.String()[len("Spec"):]
}

// Every spec the engine can run must be mapped to a preset directory that exists, or the tools
// silently offer nothing for it.
func TestSpecDirsExist(t *testing.T) {
	config := testConfig()

	for _, spec := range core.RegisteredSpecs() {
		dir, ok := engine.SpecDir(spec)
		if !ok {
			t.Errorf("%s has no preset directory", specName(spec))
			continue
		}

		presets, err := config.ListPresets(spec)
		if err != nil {
			t.Errorf("%s (%s): %v", specName(spec), dir, err)
			continue
		}
		if len(presets.GearSets) == 0 {
			t.Errorf("%s (%s) has no gear sets", specName(spec), dir)
		}
		// Several healing specs ship gear but no APL, so a missing rotation is reported rather
		// than failed: the tools have to cope with it either way.
		if len(presets.Rotations) == 0 {
			t.Logf("%s (%s) has no rotations checked in", specName(spec), dir)
		}
		if engine.SpecClass(spec) == proto.Class_ClassUnknown {
			t.Errorf("%s has no class", specName(spec))
		}
	}
}

func TestParseSpec(t *testing.T) {
	for _, name := range []string{"SpecSmitePriest", "SmitePriest", "smitepriest", "priest/smite"} {
		spec, err := engine.ParseSpec(name)
		if err != nil {
			t.Errorf("%q: %v", name, err)
			continue
		}
		if spec != proto.Spec_SpecSmitePriest {
			t.Errorf("%q resolved to %v", name, spec)
		}
	}

	for _, name := range []string{"", "Unknown", "SpecUnknown", "priest/nonexistent"} {
		if spec, err := engine.ParseSpec(name); err == nil {
			t.Errorf("%q resolved to %v, want an error", name, spec)
		}
	}
}

// The talent strings are scraped out of the client's presets.ts, so a format change there has to
// fail here rather than quietly leaving specs untalented.
func TestListTalents(t *testing.T) {
	config := testConfig()
	var withTalents int

	for _, spec := range core.RegisteredSpecs() {
		presets, err := config.ListTalents(spec)
		if err != nil {
			t.Errorf("%s: %v", specName(spec), err)
			continue
		}
		if len(presets) == 0 {
			// A spec whose presets.ts carries an empty talent string, e.g. holy paladin.
			t.Logf("%s: no talent presets found", specName(spec))
			continue
		}
		withTalents++
		for _, preset := range presets {
			if preset.Talents == "" || preset.Name == "" {
				t.Errorf("%s: incomplete talent preset %+v", specName(spec), preset)
			}
		}
	}

	// Individual specs may legitimately have none, but the scrape breaking would take them all
	// out at once, and that has to fail rather than quietly untalent every character.
	if minimum := len(core.RegisteredSpecs()) - 3; withTalents < minimum {
		t.Errorf("only %d specs have talent presets, expected at least %d; the presets.ts scrape has probably broken", withTalents, minimum)
	}
}

// Assembling from presets has to produce something the engine will actually run: nil spec
// options or a missing encounter panic rather than erroring, and both are easy to introduce.
func TestBuildSettingsRunsForEverySpec(t *testing.T) {
	config := testConfig()

	for _, spec := range core.RegisteredSpecs() {
		t.Run(specName(spec), func(t *testing.T) {
			presets, err := config.ListPresets(spec)
			if err != nil {
				t.Fatalf("list presets: %v", err)
			}
			if len(presets.Rotations) == 0 {
				t.Skip("no rotation checked in for this spec")
			}
			if len(mustTalents(t, config, spec)) == 0 {
				t.Skip("no talent preset checked in for this spec")
			}

			settings, notes, err := config.BuildSettings(engine.SettingsRequest{Spec: spec})
			if err != nil {
				t.Fatalf("build settings: %v", err)
			}
			if len(notes) == 0 {
				t.Error("expected notes saying which defaults were applied")
			}

			request, err := core.BuildRaidSimRequest(settings, core.SimRequestOptions{Iterations: 1})
			if err != nil {
				t.Fatalf("build request: %v", err)
			}

			result := core.RunRaidSim(request)
			if result.Error != nil {
				t.Fatalf("sim: %s", result.Error.Message)
			}
			if result.RaidMetrics.Dps.Avg <= 0 && engine.SpecClass(spec) != proto.Class_ClassUnknown {
				t.Logf("dps was %v; healing and tanking specs legitimately do no damage", result.RaidMetrics.Dps.Avg)
			}
		})
	}
}

func TestBuildSettingsRespectsRequest(t *testing.T) {
	config := testConfig()

	settings, _, err := config.BuildSettings(engine.SettingsRequest{
		Spec:      proto.Spec_SpecSmitePriest,
		Race:      proto.Race_RaceUndead,
		GearSet:   "p3",
		Rotation:  "default",
		Talents:   "5051000130505002501-225051000320152-",
		Encounter: "ShortSingleTarget",
		NoBuffs:   true,
	})
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}

	if settings.Player.Race != proto.Race_RaceUndead {
		t.Errorf("race = %v", settings.Player.Race)
	}
	if settings.Encounter.Duration != 60 {
		t.Errorf("encounter duration = %v, want the short encounter", settings.Encounter.Duration)
	}
	if len(settings.RaidBuffs.String()) != 0 {
		t.Errorf("raid buffs = %v, want none", settings.RaidBuffs)
	}
	if settings.Player.Equipment == nil || len(settings.Player.Equipment.Items) == 0 {
		t.Error("no equipment loaded")
	}
}

func TestBuildSettingsReportsMissingPresets(t *testing.T) {
	config := testConfig()

	if _, _, err := config.BuildSettings(engine.SettingsRequest{
		Spec:    proto.Spec_SpecSmitePriest,
		GearSet: "nonexistent",
	}); err == nil {
		t.Error("expected an error for a missing gear set")
	}

	if _, err := config.ReadPreset(proto.Spec_SpecSmitePriest, engine.PresetGear, "../../../etc/passwd"); err == nil {
		t.Error("expected a path separator in a preset name to be rejected")
	}
}

// MustTalents is a test helper: the talent presets for a spec, or a failed test.
func mustTalents(t *testing.T, config engine.Config, spec proto.Spec) []engine.TalentPreset {
	t.Helper()
	presets, err := config.ListTalents(spec)
	if err != nil {
		t.Fatalf("list talents: %v", err)
	}
	return presets
}
