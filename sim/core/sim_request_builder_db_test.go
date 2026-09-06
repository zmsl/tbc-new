//go:build with_db

package core

import (
	"testing"

	"github.com/wowsims/tbc/sim/core/proto"
)

// Meta gem activation needs the item and gem databases to know what colour each socketed gem
// is, so these cases only exist in a with_db build.

func headItem(equipment *proto.EquipmentSpec) *proto.ItemSpec {
	return equipment.Items[proto.ItemSlot_ItemSlotHead]
}

func TestNormalizePlayerGearMetaGem(t *testing.T) {
	// The preset is gemmed to meet its meta requirement -- TestGearPresetsAreEquippable would
	// list it as broken otherwise -- so nothing should be disabled.
	t.Run("leaves a satisfied meta gem alone", func(t *testing.T) {
		player := testSmiteSettings().Player
		if len(headItem(player.Equipment).Gems) == 0 {
			t.Fatal("fixture has no head gems")
		}

		NormalizePlayerGear(player)

		for i, item := range player.Equipment.Items {
			if item.MetaGemDisabled {
				t.Errorf("item %d disabled its meta gem despite the requirement being met", i)
			}
		}
	})

	// Strip every other gem and the colour requirement can no longer be met. The gem stays in
	// its socket -- the helm keeps its socket bonus -- but stops contributing.
	t.Run("disables a meta gem whose colours are missing", func(t *testing.T) {
		player := testSmiteSettings().Player
		for _, item := range player.Equipment.Items {
			for i := range item.Gems {
				if gem, ok := GemsByID[item.Gems[i]]; !ok || gem.Color != proto.GemColor_GemColorMeta {
					item.Gems[i] = 0
				}
			}
		}

		NormalizePlayerGear(player)

		if !headItem(player.Equipment).MetaGemDisabled {
			t.Error("meta gem stayed active with no colours to satisfy it")
		}
		if len(headItem(player.Equipment).Gems) == 0 || headItem(player.Equipment).Gems[0] == 0 {
			t.Error("meta gem was removed rather than disabled")
		}
	})

	// A flag left over from a previous gemming must not survive into a new one.
	t.Run("clears a stale disabled flag", func(t *testing.T) {
		player := testSmiteSettings().Player
		for _, item := range player.Equipment.Items {
			item.MetaGemDisabled = true
		}

		NormalizePlayerGear(player)

		for i, item := range player.Equipment.Items {
			if item.MetaGemDisabled {
				t.Errorf("item %d kept a stale disabled flag", i)
			}
		}
	})
}
