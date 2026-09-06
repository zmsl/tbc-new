//go:build with_db

package core

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wowsims/tbc/assets/database"
	"github.com/wowsims/tbc/sim/core/proto"
)

// Gear presets that are known to be unwearable. Each entry is a bug in the preset, not in the
// rules; they are listed so the guard is live now and no new breakage can be introduced while
// they are sorted out one spec at a time. Fixing one means deleting its line here.
var knownUnequippableGearSets = map[string]string{
	"druid/balance/gear_sets/p1_a.gear.json":         "boots name 35297, the Formula: recipe item rather than the enchant (2940)",
	"druid/balance/gear_sets/p2_a.gear.json":         "boots name 35297, the Formula: recipe item rather than the enchant (2940)",
	"druid/balance/gear_sets/p3.gear.json":           "boots name 35297, the Formula: recipe item rather than the enchant (2940)",
	"druid/balance/gear_sets/p4.gear.json":           "boots name 35297, the Formula: recipe item rather than the enchant (2940)",
	"druid/balance/gear_sets/p5.gear.json":           "boots name 35297, the Formula: recipe item rather than the enchant (2940)",
	"druid/feralbear/gear_sets/p2_warden.gear.json":  "a cloak enchant on the bracers",
	"paladin/protection/gear_sets/p5.gear.json":      "a bracer enchant on the legs",
	"shaman/enhancement/gear_sets/p1.gear.json":      "belt and gloves are in each other's slots",
	"shaman/enhancement/gear_sets/preraid.gear.json": "belt and gloves are in each other's slots",
	"warrior/dps/gear_sets/p3_fury.gear.json":        "Relentless Earthstorm Diamond never activates with these gems",
	"warrior/dps/gear_sets/p3_fury_t6.gear.json":     "Relentless Earthstorm Diamond never activates with these gems",
}

// Every checked-in gear preset should describe gear a character could actually wear. The rules
// used to live only in the TypeScript client, so presets written by hand or by a script never
// met them; see gear_validation.go.
func TestGearPresetsAreEquippable(t *testing.T) {
	db := database.Load()
	lookup := GearLookup{
		Items:    make(map[int32]*proto.UIItem, len(db.Items)),
		Enchants: db.Enchants,
		Gems:     make(map[int32]*proto.UIGem, len(db.Gems)),
	}
	for _, item := range db.Items {
		lookup.Items[item.Id] = item
	}
	for _, gem := range db.Gems {
		lookup.Gems[gem.Id] = gem
	}

	paths, err := filepath.Glob("../../ui/*/*/gear_sets/*.gear.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("found no gear presets to check: %v", err)
	}
	sort.Strings(paths)

	stillBroken := map[string]bool{}
	for _, path := range paths {
		name := filepath.ToSlash(strings.TrimPrefix(path, "../../ui/"))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %s", name, err)
			continue
		}

		problems := lookup.ValidateEquipment(EquipmentSpecFromJsonString(string(data)))
		if reason, known := knownUnequippableGearSets[name]; known {
			if len(problems) == 0 {
				t.Errorf("%s is in knownUnequippableGearSets (%s) but now validates; remove its entry", name, reason)
			}
			stillBroken[name] = true
			continue
		}
		for _, problem := range problems {
			t.Errorf("%s: %s", name, problem)
		}
	}

	for name := range knownUnequippableGearSets {
		if !stillBroken[name] {
			t.Errorf("knownUnequippableGearSets lists %s, which no longer exists", name)
		}
	}
}
