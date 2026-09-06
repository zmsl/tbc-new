package presets_test

import (
	"testing"

	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/presets"
	"github.com/wowsims/tbc/sim"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

func init() {
	sim.RegisterAll()
}

// The whole point of embedding is that an installed binary works with no repository anywhere near
// it, so this checks the embedded tree the way the server would use it -- not that the files
// merely exist.
func TestEmbeddedPresetsAreComplete(t *testing.T) {
	tree := presets.FS()
	if tree == nil {
		t.Skip("this binary was built without embedded presets; run `make mcp-presets`")
	}

	config := engine.Config{Presets: tree, PresetsSource: "embedded"}

	for _, spec := range core.RegisteredSpecs() {
		name := spec.String()[len("Spec"):]

		listed, err := config.ListPresets(spec)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(listed.GearSets) == 0 {
			t.Errorf("%s: no gear sets were embedded", name)
			continue
		}

		if _, err := config.LoadGearSet(spec, listed.GearSets[0]); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		for _, rotation := range listed.Rotations {
			if _, err := config.LoadRotation(spec, rotation); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}

		// Talents are scraped out of presets.ts, so that file has to be embedded too.
		talents, err := config.ListTalents(spec)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if len(talents) == 0 && spec != proto.Spec_SpecHolyPaladin {
			t.Errorf("%s: no talent presets were embedded", name)
		}
	}
}

// A character assembled from the embedded presets has to simulate, which is the only end-to-end
// statement worth making about them.
func TestEmbeddedPresetsSimulate(t *testing.T) {
	tree := presets.FS()
	if tree == nil {
		t.Skip("this binary was built without embedded presets; run `make mcp-presets`")
	}

	config := engine.Config{Presets: tree, PresetsSource: "embedded"}
	settings, _, err := config.BuildSettings(engine.SettingsRequest{Spec: proto.Spec_SpecSmitePriest, GearSet: "p3"})
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}

	request, err := core.BuildRaidSimRequest(settings, core.SimRequestOptions{Iterations: 100})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	result := core.RunRaidSim(request)
	if result.Error != nil {
		t.Fatalf("sim: %s", result.Error.Message)
	}
	if result.RaidMetrics.Dps.Avg <= 0 {
		t.Error("the embedded p3 set did no damage")
	}
}
