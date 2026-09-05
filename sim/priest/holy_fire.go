package priest

import (
	"fmt"
	"time"

	"github.com/wowsims/tbc/sim/common/shared"
	"github.com/wowsims/tbc/sim/core"
)

// Holy Fire is a direct hit plus a 10 second damage-over-time effect, and unlike its later
// incarnations it has no cooldown in TBC -- a rotation gates it on the DoT, not on a timer.
//
// Coefficient is the direct portion. Every rank shares it: rank 1 is already a level 20 spell
// with the full 3.5 second cast, so neither the sub-level-20 penalty nor the cast time scaling
// that ramps Smite from 0.123 to 0.714 applies to any of them.
//
// DotTickDamage is the tooltip's "additional N damage over 10 sec" divided by the five ticks.
var HolyFireRankMap = shared.SpellRankMap{
	{Rank: 1, SpellID: 14914, Cost: 85, MinDamage: 84, MaxDamage: 104, DotTickDamage: 6, Coefficient: 0.8571},
	{Rank: 2, SpellID: 15262, Cost: 95, MinDamage: 106, MaxDamage: 131, DotTickDamage: 8, Coefficient: 0.8571},
	{Rank: 3, SpellID: 15263, Cost: 125, MinDamage: 144, MaxDamage: 178, DotTickDamage: 11, Coefficient: 0.8571},
	{Rank: 4, SpellID: 15264, Cost: 145, MinDamage: 178, MaxDamage: 223, DotTickDamage: 13, Coefficient: 0.8571},
	{Rank: 5, SpellID: 15265, Cost: 170, MinDamage: 219, MaxDamage: 273, DotTickDamage: 17, Coefficient: 0.8571},
	{Rank: 6, SpellID: 15266, Cost: 200, MinDamage: 271, MaxDamage: 340, DotTickDamage: 20, Coefficient: 0.8571},
	{Rank: 7, SpellID: 15267, Cost: 230, MinDamage: 323, MaxDamage: 406, DotTickDamage: 25, Coefficient: 0.8571},
	{Rank: 8, SpellID: 15261, Cost: 255, MinDamage: 375, MaxDamage: 470, DotTickDamage: 29, Coefficient: 0.8571},
	{Rank: 9, SpellID: 25384, Cost: 290, MinDamage: 426, MaxDamage: 537, DotTickDamage: 33, Coefficient: 0.8571},
}

// Shared by every rank, same as the direct coefficient and for the same reason.
const holyFireDotCoefficient = 0.0333

func (priest *Priest) registerHolyFireSpell(rankConfig shared.SpellRankConfig) {
	spell := priest.RegisterSpell(core.SpellConfig{
		ActionID:       core.ActionID{SpellID: rankConfig.SpellID},
		SpellSchool:    core.SpellSchoolHoly,
		ProcMask:       core.ProcMaskSpellDamage,
		Flags:          core.SpellFlagAPL,
		ClassSpellMask: PriestSpellHolyFire,
		Rank:           rankConfig.Rank,

		ManaCost: core.ManaCostOptions{
			FlatCost: rankConfig.Cost,
		},

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: 3500 * time.Millisecond,
			},
		},

		DamageMultiplier:         1,
		DamageMultiplierAdditive: 1,
		CritMultiplier:           priest.DefaultSpellCritMultiplier(),
		BonusCoefficient:         rankConfig.Coefficient,
		ThreatMultiplier:         1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: fmt.Sprintf("HolyFire-%d", rankConfig.Rank),
			},
			NumberOfTicks:       5,
			TickLength:          2 * time.Second,
			AffectedByCastSpeed: false, // DoT ticks not haste-affected in TBC
			BonusCoefficient:    holyFireDotCoefficient,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.Snapshot(target, rankConfig.DotTickDamage)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := priest.CalcAndRollDamageRange(sim, rankConfig.MinDamage, rankConfig.MaxDamage)
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
			}
		},

		ExpectedTickDamage: func(sim *core.Simulation, target *core.Unit, spell *core.Spell, useSnapshot bool) *core.SpellResult {
			if useSnapshot {
				dot := spell.Dot(target)
				return dot.CalcSnapshotDamage(sim, target, spell.OutcomeExpectedMagicHit)
			}
			return spell.CalcPeriodicDamage(sim, target, rankConfig.DotTickDamage, spell.OutcomeExpectedMagicHit)
		},
	})

	priest.HolyFire = append(priest.HolyFire, spell)
}

// Holy Fire is trainable by any priest, but only the smite spec ever casts it, so a spec opts
// in rather than the shared Initialize registering ten spells nothing will use.
func (priest *Priest) RegisterHolyFireSpells() {
	HolyFireRankMap.RegisterAll(priest.registerHolyFireSpell)
}
