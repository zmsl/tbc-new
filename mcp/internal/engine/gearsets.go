package engine

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/wowsims/tbc/sim/core/proto"
)

// GearSetInfo describes a checked-in gear set well enough to choose between them without opening
// each one.
type GearSetInfo struct {
	Name    string `json:"name" jsonschema:"the name to pass as gearSet, e.g. p3 or phase_3/bm/2h_6p"`
	Phase   int    `json:"phase,omitempty" jsonschema:"the raid phase this set is for, when the name says; absent when it could not be worked out"`
	PreRaid bool   `json:"preRaid,omitempty" jsonschema:"true for sets meant for before the first raid tier"`
	Variant string `json:"variant,omitempty" jsonschema:"what distinguishes this set from others in the same phase, e.g. 'fury t6' or 'bm 2h 6p'"`

	Items      int      `json:"items" jsonschema:"how many slots the set fills; a set with very few is a placeholder rather than a real answer"`
	Equippable bool     `json:"equippable" jsonschema:"whether the set could actually be worn in game"`
	Problems   []string `json:"problems,omitempty" jsonschema:"why it could not be worn, when it could not"`
}

// PhaseHeuristic explains where the phase numbers come from, and is returned alongside them: the
// presets carry no phase field, so this is inference from their names and can be wrong.
const PhaseHeuristic = "Phases are read from preset names (p3, phase_3, tier names t4/t5/t6, raid names " +
	"za and swp, pre-raid), because the gear presets themselves carry no phase field. A set with no phase is one " +
	"whose name did not say. Treat these as candidates to simulate, not as verified best-in-slot: " +
	"the names are the authors' claims, and `equippable` is the only checked fact here."

// DescribeGearSets lists a spec's gear sets with what can be worked out about each.
func (c Config) DescribeGearSets(spec proto.Spec) ([]GearSetInfo, error) {
	presets, err := c.ListPresets(spec)
	if err != nil {
		return nil, err
	}

	lookup := GearLookup()
	infos := make([]GearSetInfo, 0, len(presets.GearSets))

	for _, name := range presets.GearSets {
		info := GearSetInfo{Name: name}
		info.Phase, info.PreRaid, info.Variant = describeName(name)

		equipment, err := c.LoadGearSet(spec, name)
		if err != nil {
			info.Problems = []string{err.Error()}
			infos = append(infos, info)
			continue
		}

		for _, item := range equipment.Items {
			if item != nil && item.Id != 0 {
				info.Items++
			}
		}

		// The equippability rules live in the engine and are the one thing here that is checked
		// rather than inferred: eleven checked-in presets fail them today.
		info.Problems = lookup.ValidateEquipment(equipment)
		info.Equippable = len(info.Problems) == 0

		infos = append(infos, info)
	}

	return infos, nil
}

var (
	// "p3", "p3_fury", "p3ArcaneStaff", "phase_1/bm/2h_6p"
	phasePrefix = regexp.MustCompile(`(?i)^p(?:hase)?_?(\d)`)
	// "t6", "destro_fire_t4" -- tier names, which map onto phases.
	tierAnywhere = regexp.MustCompile(`(?i)(?:^|[_\-/])t(\d)(?:$|[_\-/])`)
	// Raids named rather than numbered.
	raidAnywhere = regexp.MustCompile(`(?i)(?:^|[_\-/])(swp|za)(?:$|[_\-/])`)
	// A capital starting a word inside a name like "ArcaneStaff".
	camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

// TBC's tiers and phases do not share numbering: Tier 4 is phase 1, and Sunwell is phase 5.
var tierPhases = map[int]int{4: 1, 5: 2, 6: 3}

// Some sets are named for their raid instead: Zul'Aman is phase 4, Sunwell Plateau phase 5.
var raidPhases = map[string]int{"za": 4, "swp": 5}

func describeName(name string) (phase int, preRaid bool, variant string) {
	remainder := name

	if match := phasePrefix.FindStringSubmatch(name); match != nil {
		phase, _ = strconv.Atoi(match[1])
		remainder = name[len(match[0]):]
	} else if match := tierAnywhere.FindStringSubmatch(name); match != nil {
		tier, _ := strconv.Atoi(match[1])
		phase = tierPhases[tier]
		remainder = strings.Replace(name, match[0], "_", 1)
	} else if match := raidAnywhere.FindStringSubmatch(name); match != nil {
		phase = raidPhases[strings.ToLower(match[1])]
		remainder = strings.Replace(name, match[1], "", 1)
	}

	// A pre-raid set can still live in a phase directory -- the hunter keeps one under phase_1 --
	// and pre-raid is the more useful answer, so it wins. Stripping works on what is left rather
	// than the original name, so the phase does not come back as part of the variant.
	if flat := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(name)); strings.Contains(flat, "preraid") || strings.Contains(flat, "prebis") {
		preRaid, phase = true, 0
		remainder = stripAny(remainder, "pre_raid", "preraid", "pre-raid", "preBis", "prebis")
	}

	return phase, preRaid, tidyVariant(remainder)
}

func stripAny(name string, needles ...string) string {
	lowered := strings.ToLower(name)
	for _, needle := range needles {
		if index := strings.Index(lowered, strings.ToLower(needle)); index >= 0 {
			return name[:index] + name[index+len(needle):]
		}
	}
	return name
}

// Turns what is left of a name into something readable: "phase_1/bm/2h_6p" leaves "/bm/2h_6p",
// which is more useful to a reader as "bm 2h 6p".
func tidyVariant(remainder string) string {
	spaced := camelBoundary.ReplaceAllString(remainder, "$1 $2")
	spaced = strings.NewReplacer("_", " ", "-", " ", "/", " ").Replace(spaced)
	return strings.Join(strings.Fields(strings.ToLower(spaced)), " ")
}
