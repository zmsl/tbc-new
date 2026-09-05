package warrior

import (
	"time"

	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	"github.com/wowsims/tbc/sim/core/stats"
)

var TalentTreeSizes = [3]int{23, 21, 22}

type WarriorInputs struct {
	DefaultShout  proto.WarriorShout
	DefaultStance proto.WarriorStance

	StartingRage          float64
	QueueDelay            int32
	HsRageThreshold       float64
	CleaveRageThreshold   float64
	StanceSnapshot        bool
	HasBsSolarianSapphire bool
	HasBsT2               bool
}

const (
	HeroicStrikeRageCost = 15.0
	CleaveRageCost       = 20.0
)

// Applied when the Heroic Strike threshold is unset, so settings saved before the option
// existed keep the previous behaviour. Cleave has no constant of its own: it is derived
// from Cleave's live rage cost in cleaveRageThreshold().
const DefaultHSRageThreshold = 40.0

const (
	SpellFlagBleed = core.SpellFlagAgentReserved1
)

const (
	SpellMaskNone int64 = 0
	// Abilities that don't cost rage and aren't attacks
	SpellMaskBattleShout int64 = 1 << iota
	SpellMaskCommandingShout
	SpellMaskBerserkerRage
	SpellMaskRecklessness
	SpellMaskDeathWish
	SpellMaskRetaliation
	SpellMaskRetaliationHit
	SpellMaskRampage
	SpellMaskShieldWall
	SpellMaskLastStand
	SpellMaskCharge
	SpellMaskIntercept
	SpellMaskDemoralizingShout

	// Stances
	SpellMaskBattleStance
	SpellMaskBerserkerStance
	SpellMaskDefensiveStance

	// Special attacks
	SpellMaskRend
	SpellMaskDeepWounds
	SpellMaskSweepingStrikes
	SpellMaskSweepingStrikesHit
	SpellMaskSweepingStrikesNormalizedHit
	SpellMaskHeroicStrike
	SpellMaskCleave
	SpellMaskDevastate
	SpellMaskExecute
	SpellMaskOverpower
	SpellMaskRevenge
	SpellMaskSlam
	SpellMaskSunderArmor
	SpellMaskThunderClap
	SpellMaskWhirlwind
	SpellMaskWhirlwindOh
	SpellMaskShieldSlam
	SpellMaskConcussionBlow
	SpellMaskShieldBash
	SpellMaskBloodthirst
	SpellMaskMortalStrike
	SpellMaskShieldBlock
	SpellMaskHamstring
	SpellMaskPummel

	WarriorSpellLast
	WarriorSpellsAll = WarriorSpellLast<<1 - 1

	SpellMaskShouts             = SpellMaskCommandingShout | SpellMaskBattleShout | SpellMaskDemoralizingShout
	SpellMaskDirectDamageSpells = SpellMaskSweepingStrikesHit | SpellMaskSweepingStrikesNormalizedHit |
		SpellMaskCleave | SpellMaskExecute | SpellMaskHeroicStrike | SpellMaskOverpower |
		SpellMaskRevenge | SpellMaskSlam | SpellMaskShieldBash | SpellMaskSunderArmor |
		SpellMaskThunderClap | SpellMaskWhirlwind | SpellMaskWhirlwindOh | SpellMaskShieldSlam |
		SpellMaskBloodthirst | SpellMaskMortalStrike | SpellMaskIntercept | SpellMaskDevastate | SpellMaskRetaliationHit

	SpellMaskDamageSpells = SpellMaskDirectDamageSpells | SpellMaskDeepWounds | SpellMaskRend
)

const EnrageTag = "EnrageEffect"

type Warrior struct {
	core.Character

	ClassSpellScaling float64

	Talents *proto.WarriorTalents

	WarriorInputs

	// Current state
	Stance                Stance
	ChargeRageGain        float64
	BerserkerRageRageGain float64

	BattleShout       *core.Spell
	CommandingShout   *core.Spell
	DemoralizingShout *core.Spell
	BattleStance      *core.Spell
	DefensiveStance   *core.Spell
	BerserkerStance   *core.Spell

	Rend                            *core.Spell
	DeepWounds                      *core.Spell
	MortalStrike                    *core.Spell
	DevastateSunder                 *core.Spell
	SweepingStrikesNormalizedAttack *core.Spell
	SunderArmorDevastate            *core.Spell

	HeroicStrike       *core.Spell
	Cleave             *core.Spell
	curQueueAura       *core.Aura
	curQueuedAutoSpell *core.Spell
	// Set when the active queue was made by a "queue and cancel" spell, which drops
	// the queue right before the main hand swing instead of spending it.
	curQueueWillCancel bool
	// Rage threshold of whichever ability is currently queued, captured at queue time
	// because Heroic Strike and Cleave use different ones.
	curQueueThreshold float64
	// A queue has been requested but its aura has not activated yet. Shared across
	// Heroic Strike and Cleave: only one on-next-swing ability can be queued, so
	// without this both can be queued in a single rotation pass whenever the queue
	// delay is short enough that the aura has not landed yet.
	queueIsPending bool

	sharedMCD        *core.Timer // Recklessness, Shield Wall & Retaliation
	sharedShoutsCD   *core.Timer
	queuedRealismICD *core.Cooldown

	EnrageAura *core.Aura

	SweepingStrikesAura *core.Aura

	DemoralizingShoutAuras core.AuraArray
	SunderArmorAuras       core.AuraArray

	// Set bonuses
	T6Tank2P *core.Aura
}

func (warrior *Warrior) GetCharacter() *core.Character {
	return &warrior.Character
}

func (warrior *Warrior) AddRaidBuffs(raidBuffs *proto.RaidBuffs) {
}

func (warrior *Warrior) AddPartyBuffs(_ *proto.PartyBuffs) {
}

func (warrior *Warrior) Initialize() {
	warrior.registerRecklessness()
	warrior.registerShieldWall()
	warrior.registerRetaliation()

	warrior.registerBerserkerRage()
	warrior.registerBloodrage()
	warrior.registerCharge()
	warrior.registerIntercept()
	warrior.registerPummel()
	warrior.registerHamstring()

	warrior.registerRend()
	warrior.registerSunderArmor()
	warrior.registerHeroicStrike()
	warrior.registerCleave()
	warrior.registerOverpower()
	warrior.registerSlam()
	warrior.registerWhirlwind()
	warrior.registerExecute()
	warrior.registerThunderClap()
	warrior.registerRevenge()
	warrior.registerShieldBlock()
	warrior.registerShieldBash()

	warrior.registerStances()
	warrior.registerShouts()

	warrior.addPvpGloves()
}

func (warrior *Warrior) Reset(_ *core.Simulation) {
	warrior.curQueueAura = nil
	warrior.curQueuedAutoSpell = nil
	warrior.curQueueWillCancel = false
	warrior.curQueueThreshold = 0
	warrior.queueIsPending = false

	warrior.ChargeRageGain = 15
	warrior.BerserkerRageRageGain = 0

	switch warrior.DefaultStance {
	case proto.WarriorStance_WarriorStanceBattle:
		warrior.Stance = BattleStance
	case proto.WarriorStance_WarriorStanceDefensive:
		warrior.Stance = DefensiveStance
	case proto.WarriorStance_WarriorStanceBerserker:
		warrior.Stance = BerserkerStance
	}
}

func (warrior *Warrior) OnEncounterStart(sim *core.Simulation) {}

func (war *Warrior) GetMainHandType() proto.HandType {
	mh := war.GetMHWeapon()

	if mh != nil && (mh.HandType == proto.HandType_HandTypeTwoHand) {
		return proto.HandType_HandTypeTwoHand
	}

	return proto.HandType_HandTypeOneHand
}

func NewWarrior(character *core.Character, options *proto.WarriorOptions, talents string, inputs WarriorInputs) *Warrior {
	warrior := &Warrior{
		Character:     *character,
		Talents:       &proto.WarriorTalents{},
		WarriorInputs: inputs,
	}
	if warrior.WarriorInputs.HsRageThreshold <= 0 {
		warrior.WarriorInputs.HsRageThreshold = DefaultHSRageThreshold
	}
	core.FillTalentsProto(warrior.Talents.ProtoReflect(), talents, TalentTreeSizes)

	warrior.EnableRageBar(core.RageBarOptions{
		MaxRage:            100,
		BaseRageMultiplier: 1,
		StartingRage:       inputs.StartingRage,
	})

	warrior.EnableAutoAttacks(warrior, core.AutoAttackOptions{
		MainHand:       warrior.WeaponFromMainHand(warrior.DefaultMeleeCritMultiplier()),
		OffHand:        warrior.WeaponFromOffHand(warrior.DefaultMeleeCritMultiplier()),
		AutoSwingMelee: true,
		ReplaceMHSwing: warrior.TryHSOrCleave,
	})

	warrior.PseudoStats.CanParry = true
	warrior.PseudoStats.BaseDodgeChance += 0.0075
	warrior.PseudoStats.BaseParryChance += 0.05
	warrior.PseudoStats.BaseBlockChance += 0.05

	warrior.AddStatDependency(stats.Strength, stats.AttackPower, 2)
	warrior.AddStatDependency(stats.Strength, stats.BlockValue, 1/20.0)
	warrior.AddStatDependency(stats.Agility, stats.PhysicalCritPercent, core.CritPerAgiMaxLevel[character.Class])
	warrior.AddStatDependency(stats.Agility, stats.DodgeRating, 1/30.0*core.DodgeRatingPerDodgePercent)
	warrior.AddStatDependency(stats.BonusArmor, stats.Armor, 1)

	warrior.sharedShoutsCD = warrior.NewTimer()
	warrior.sharedMCD = warrior.NewTimer()
	warrior.ChargeRageGain = 15
	warrior.BerserkerRageRageGain = 0
	// The sim often re-enables heroic strike in an unrealistic amount of time.
	// This can cause an unrealistic immediate double-hit around wild strikes procs
	warrior.queuedRealismICD = &core.Cooldown{
		Timer:    warrior.NewTimer(),
		Duration: time.Millisecond * time.Duration(warrior.WarriorInputs.QueueDelay),
	}

	return warrior
}

func (warrior *Warrior) CastNormalizedSweepingStrikesAttack(results core.SpellResultSlice, sim *core.Simulation) {
	if warrior.SweepingStrikesAura != nil && warrior.SweepingStrikesAura.IsActive() {
		for _, result := range results {
			if result.Landed() {
				warrior.SweepingStrikesNormalizedAttack.Cast(sim, warrior.Env.NextActiveTargetUnit(result.Target))
				warrior.SweepingStrikesAura.RemoveStack(sim)
				break
			}
		}
	}
}

// Agent is a generic way to access underlying warrior on any of the agents.
type WarriorAgent interface {
	GetWarrior() *Warrior
}
