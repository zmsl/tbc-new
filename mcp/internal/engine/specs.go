// Package engine wraps the simulator for the MCP server: where its presets live, which specs it
// can run, and (later) how requests are executed.
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wowsims/tbc/sim/core/proto"
)

// Config is the server's runtime configuration. It is passed in rather than read from globals so
// handlers can be exercised against a fixture tree in tests.
type Config struct {
	// PresetsRoot is the ui/ directory holding gear sets, rotations and builds.
	PresetsRoot string
}

// specDirs maps each spec to its directory under the presets root.
//
// There is no rule to derive these from the enum -- "SpecFeralCatDruid" lives in
// "druid/feralcat" while "SpecPriest" (shadow) lives in "priest/dps" -- so the mapping is
// written out, the same way ui/core/proto_utils/utils.ts writes out its own. A missing or wrong
// entry surfaces immediately: TestSpecDirsExist walks all of them.
var specDirs = map[proto.Spec]string{
	proto.Spec_SpecBalanceDruid:       "druid/balance",
	proto.Spec_SpecFeralCatDruid:      "druid/feralcat",
	proto.Spec_SpecFeralBearDruid:     "druid/feralbear",
	proto.Spec_SpecRestorationDruid:   "druid/restoration",
	proto.Spec_SpecHunter:             "hunter/dps",
	proto.Spec_SpecMage:               "mage/dps",
	proto.Spec_SpecHolyPaladin:        "paladin/holy",
	proto.Spec_SpecProtectionPaladin:  "paladin/protection",
	proto.Spec_SpecRetributionPaladin: "paladin/retribution",
	proto.Spec_SpecPriest:             "priest/dps",
	proto.Spec_SpecSmitePriest:        "priest/smite",
	proto.Spec_SpecRogue:              "rogue/dps",
	proto.Spec_SpecElementalShaman:    "shaman/elemental",
	proto.Spec_SpecEnhancementShaman:  "shaman/enhancement",
	proto.Spec_SpecRestorationShaman:  "shaman/restoration",
	proto.Spec_SpecWarlock:            "warlock/dps",
	proto.Spec_SpecDpsWarrior:         "warrior/dps",
	proto.Spec_SpecProtectionWarrior:  "warrior/protection",
}

// SpecDir returns a spec's directory under the presets root, e.g. "priest/smite".
func SpecDir(spec proto.Spec) (string, bool) {
	dir, ok := specDirs[spec]
	return dir, ok
}

// ParseSpec resolves a spec named either by its enum ("SpecSmitePriest"), without the prefix
// ("SmitePriest"), or by its preset path ("priest/smite"). Agents reach for all three.
func ParseSpec(name string) (proto.Spec, error) {
	trimmed := strings.TrimSpace(name)

	for candidate, dir := range specDirs {
		if strings.EqualFold(trimmed, dir) {
			return candidate, nil
		}
	}

	for value, enumName := range proto.Spec_name {
		if strings.EqualFold(trimmed, enumName) || strings.EqualFold("Spec"+trimmed, enumName) {
			spec := proto.Spec(value)
			if spec == proto.Spec_SpecUnknown {
				break
			}
			return spec, nil
		}
	}

	return proto.Spec_SpecUnknown, fmt.Errorf("unknown spec %q", name)
}

// Presets are the checked-in setups for one spec.
type Presets struct {
	GearSets  []string `json:"gearSets,omitempty" jsonschema:"names of checked-in gear sets"`
	Rotations []string `json:"rotations,omitempty" jsonschema:"names of checked-in APL rotations"`
	Builds    []string `json:"builds,omitempty" jsonschema:"names of checked-in full builds (gear, talents, rotation and encounter together)"`
}

// The three preset flavours, as directory name and file suffix.
var presetKinds = []struct {
	dir    string
	suffix string
}{
	{dir: "gear_sets", suffix: ".gear.json"},
	{dir: "apls", suffix: ".apl.json"},
	{dir: "builds", suffix: ".build.json"},
}

// ListPresets reports what is checked in for a spec. A spec with no directory at all is not an
// error: several specs ship rotations but no builds.
func (c Config) ListPresets(spec proto.Spec) (Presets, error) {
	dir, ok := SpecDir(spec)
	if !ok {
		return Presets{}, fmt.Errorf("no preset directory known for %v", spec)
	}

	var presets Presets
	targets := []*[]string{&presets.GearSets, &presets.Rotations, &presets.Builds}

	for i, kind := range presetKinds {
		names, err := listPresetNames(filepath.Join(c.PresetsRoot, dir, kind.dir), kind.suffix)
		if err != nil {
			return Presets{}, err
		}
		*targets[i] = names
	}

	return presets, nil
}

func listPresetNames(dir, suffix string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var names []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), suffix) {
			names = append(names, strings.TrimSuffix(file.Name(), suffix))
		}
	}
	slices.Sort(names)
	return names, nil
}
