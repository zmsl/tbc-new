// Package engine wraps the simulator for the MCP server: where its presets live, which specs it
// can run, and (later) how requests are executed.
package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/wowsims/tbc/sim/core/proto"
)

// Config is the server's runtime configuration. It is passed in rather than read from globals so
// handlers can be exercised against a fixture tree in tests.
type Config struct {
	// Presets holds the checked-in gear sets, rotations, builds and presets.ts files, laid out as
	// they are under the repository's ui/. It is an fs.FS rather than a path so the same code
	// serves a directory on disk and the tree compiled into the binary.
	Presets fs.FS

	// PresetsSource says where Presets came from, for diagnostics.
	PresetsSource string

	// SiteURL is the base the share links point at. Defaults to DefaultSiteURL.
	SiteURL string
}

// FileConfig reads presets from a directory, which is what a server running beside a checkout
// does.
func FileConfig(root string) Config {
	return Config{Presets: os.DirFS(root), PresetsSource: root}
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

	if c.Presets == nil {
		return Presets{}, ErrNoPresets
	}

	var presets Presets
	targets := []*[]string{&presets.GearSets, &presets.Rotations, &presets.Builds}

	for i, kind := range presetKinds {
		names, err := listPresetNames(c.Presets, path.Join(dir, kind.dir), kind.suffix)
		if err != nil {
			return Presets{}, err
		}
		*targets[i] = names
	}

	return presets, nil
}

// Walks rather than lists: the hunter keeps its gear sets in per-phase subdirectories, so a
// preset name can be "phase_3/bm" as well as "p3".
func listPresetNames(tree fs.FS, dir, suffix string) ([]string, error) {
	if _, err := fs.Stat(tree, dir); err != nil {
		// A spec with no directory of this kind is normal: several ship no builds at all.
		return nil, nil
	}

	var names []string
	err := fs.WalkDir(tree, dir, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			return nil
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(entryPath, dir), "/")
		names = append(names, strings.TrimSuffix(relative, suffix))
		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.Sort(names)
	return names, nil
}

// SpecClass returns the class a spec belongs to, or ClassUnknown.
func SpecClass(spec proto.Spec) proto.Class {
	return specClasses[spec]
}

var specClasses = map[proto.Spec]proto.Class{
	proto.Spec_SpecBalanceDruid:       proto.Class_ClassDruid,
	proto.Spec_SpecFeralCatDruid:      proto.Class_ClassDruid,
	proto.Spec_SpecFeralBearDruid:     proto.Class_ClassDruid,
	proto.Spec_SpecRestorationDruid:   proto.Class_ClassDruid,
	proto.Spec_SpecHunter:             proto.Class_ClassHunter,
	proto.Spec_SpecMage:               proto.Class_ClassMage,
	proto.Spec_SpecHolyPaladin:        proto.Class_ClassPaladin,
	proto.Spec_SpecProtectionPaladin:  proto.Class_ClassPaladin,
	proto.Spec_SpecRetributionPaladin: proto.Class_ClassPaladin,
	proto.Spec_SpecPriest:             proto.Class_ClassPriest,
	proto.Spec_SpecSmitePriest:        proto.Class_ClassPriest,
	proto.Spec_SpecRogue:              proto.Class_ClassRogue,
	proto.Spec_SpecElementalShaman:    proto.Class_ClassShaman,
	proto.Spec_SpecEnhancementShaman:  proto.Class_ClassShaman,
	proto.Spec_SpecRestorationShaman:  proto.Class_ClassShaman,
	proto.Spec_SpecWarlock:            proto.Class_ClassWarlock,
	proto.Spec_SpecDpsWarrior:         proto.Class_ClassWarrior,
	proto.Spec_SpecProtectionWarrior:  proto.Class_ClassWarrior,
}

// DefaultSiteURL is where a share link should open. The desktop app serves the same pages from
// its own scheme, but a link handed to a user has to work in a browser.
const DefaultSiteURL = "https://wowsims.com/tbc/"

// SpecPageURL is the sim page for a spec, which is what a share link is hung off.
func (c Config) SpecPageURL(spec proto.Spec) (string, error) {
	dir, ok := SpecDir(spec)
	if !ok {
		return "", fmt.Errorf("no sim page known for %v", spec)
	}
	site := c.SiteURL
	if site == "" {
		site = DefaultSiteURL
	}
	if !strings.HasSuffix(site, "/") {
		site += "/"
	}
	return site + dir + "/", nil
}
