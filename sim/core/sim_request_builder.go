package core

import (
	"errors"
	"math"

	"github.com/wowsims/tbc/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// Turning saved UI settings into a runnable sim request used to be possible only in the
// TypeScript client (Sim.makeRaidSimRequest and getModifiedRaidProto, ui/core/sim.ts). Anything
// headless -- the CLI, a script, an agent holding a share link -- had to assemble a
// RaidSimRequest by hand and hope it matched what the website would have sent.
//
// The conversion is mostly mechanical, but two gear fixups the client applies on the way are
// not, and skipping either changes DPS without erroring: an inactive meta gem has to be flagged
// so its stats stop counting, and ring enchants have to come off a non-enchanter.

// DefaultSimSeed is the seed used when a caller does not choose one. It is deliberately a
// constant rather than the clock: newSimWithEnv falls back to time.Now() for a zero seed, which
// would make otherwise identical requests return different answers.
const DefaultSimSeed int64 = 101

// Fallback only, for settings that carry no iteration count of their own.
const defaultSimIterations int32 = 1000

var (
	ErrNoPlayer    = errors.New("sim settings contain no player")
	ErrNoEncounter = errors.New("sim settings contain no encounter")
)

// SimRequestOptions overrides parts of the saved settings for one run. A zero value means "use
// whatever the settings say".
type SimRequestOptions struct {
	// Iterations to run. Falls back to the saved settings, then to defaultSimIterations.
	Iterations int32

	// RandomSeed for the run. Falls back to DefaultSimSeed. Two runs with the same seed,
	// iteration count and inputs produce the same result.
	RandomSeed int64

	// Encounter replaces the saved encounter entirely when set.
	Encounter *proto.Encounter

	// UseLabeledRands draws each effect's randomness from its own stream, so two runs that
	// differ in one variable stay aligned everywhere else. Stat weights use it for exactly this
	// reason (see buildStatWeightRequests); it makes paired comparisons far less noisy.
	UseLabeledRands bool

	// Debug turns on combat logging for the first iteration.
	Debug bool
}

// BuildRaidSimRequest converts saved individual sim settings into a request the engine can run.
// The settings are not modified.
func BuildRaidSimRequest(settings *proto.IndividualSimSettings, opts SimRequestOptions) (*proto.RaidSimRequest, error) {
	if settings == nil || settings.Player == nil {
		return nil, ErrNoPlayer
	}

	encounter := opts.Encounter
	if encounter == nil {
		encounter = settings.Encounter
	}
	if encounter == nil {
		return nil, ErrNoEncounter
	}

	player := googleProto.Clone(settings.Player).(*proto.Player)
	NormalizePlayerGear(player)

	raid := SinglePlayerRaidProto(player, settings.PartyBuffs, settings.RaidBuffs, settings.Debuffs)
	raid.Tanks = settings.Tanks
	raid.TargetDummies = settings.TargetDummies

	// Dummies fill out the player's own party first and spill into further ones. Same shape the
	// test generators build (see FullCharacterTestSuiteGenerator), so a request built from
	// settings and one built from a test config agree.
	raid.NumActiveParties = min(5, int32(math.Round(float64(raid.TargetDummies)/5)))
	for range raid.NumActiveParties - 1 {
		raid.Parties = append(raid.Parties, &proto.Party{})
	}

	iterations := opts.Iterations
	if iterations <= 0 && settings.Settings != nil {
		iterations = settings.Settings.Iterations
	}
	if iterations <= 0 {
		iterations = defaultSimIterations
	}

	seed := opts.RandomSeed
	if seed == 0 {
		seed = DefaultSimSeed
	}

	return &proto.RaidSimRequest{
		Raid:      raid,
		Encounter: googleProto.Clone(encounter).(*proto.Encounter),
		SimOptions: &proto.SimOptions{
			Iterations:          iterations,
			RandomSeed:          seed,
			Debug:               opts.Debug,
			DebugFirstIteration: opts.Debug,
			UseLabeledRands:     opts.UseLabeledRands,
		},
	}, nil
}

// NormalizePlayerGear applies the fixups the client makes before simming, in place. Both depend
// on the item database, so on a build without it they are no-ops -- the same way the rest of the
// engine degrades without `with_db`.
func NormalizePlayerGear(player *proto.Player) {
	if player == nil || player.Equipment == nil {
		return
	}

	if !hasProfession(player, proto.Profession_Enchanting) {
		stripRingEnchants(player.Equipment)
	}
	applyMetaGemActivation(player.Equipment)
}

func hasProfession(player *proto.Player, profession proto.Profession) bool {
	return player.Profession1 == profession || player.Profession2 == profession
}

// Ring enchants are enchanter-only in TBC, and the client silently drops them for everyone else
// rather than handing the sim stats the character could not have (Gear.withoutEnchanting).
func stripRingEnchants(equipment *proto.EquipmentSpec) {
	for _, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2} {
		if item := itemInSlot(equipment, slot); item != nil {
			item.Enchant = 0
		}
	}
}

// An inactive meta gem stays in its socket -- the item keeps its socket bonus -- but contributes
// no stats and no effect. The engine reads that from ItemSpec.MetaGemDisabled; deciding it is
// the caller's job, because the engine has no colour requirements table.
func applyMetaGemActivation(equipment *proto.EquipmentSpec) {
	metaIdx, metaGemID := findMetaGem(equipment)
	if metaIdx < 0 {
		return
	}

	active := IsMetaGemActive(metaGemID, equippedGemColors(equipment))
	for i, item := range equipment.Items {
		if item == nil {
			continue
		}
		// Clear any stale flag elsewhere, so a set that was fixed up for one gemming does not
		// carry the old verdict into another.
		item.MetaGemDisabled = i == metaIdx && !active
	}
}

// Returns the index of the item holding the meta gem, or -1. In TBC only helms have meta
// sockets, but nothing here depends on that.
func findMetaGem(equipment *proto.EquipmentSpec) (int, int32) {
	for i, item := range equipment.Items {
		if item == nil {
			continue
		}
		for _, gemID := range item.Gems {
			if gem, ok := GemsByID[gemID]; ok && gem.Color == proto.GemColor_GemColorMeta {
				return i, gemID
			}
		}
	}
	return -1, 0
}

// Colours of every gem actually sitting in a socket. Gems past the item's socket count are
// ignored: they cannot be socketed, so they cannot count towards a meta requirement.
func equippedGemColors(equipment *proto.EquipmentSpec) []proto.GemColor {
	var colors []proto.GemColor
	for _, itemSpec := range equipment.Items {
		if itemSpec == nil {
			continue
		}
		item, ok := ItemsByID[itemSpec.Id]
		if !ok {
			continue
		}
		for gemIdx, gemID := range itemSpec.Gems {
			if gemIdx >= len(item.GemSockets) {
				break
			}
			if gem, ok := GemsByID[gemID]; ok {
				colors = append(colors, gem.Color)
			}
		}
	}
	return colors
}

func itemInSlot(equipment *proto.EquipmentSpec, slot proto.ItemSlot) *proto.ItemSpec {
	if int(slot) >= len(equipment.Items) {
		return nil
	}
	return equipment.Items[slot]
}
