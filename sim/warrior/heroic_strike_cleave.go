package warrior

import (
	"github.com/wowsims/tbc/sim/core"
)

func (war *Warrior) registerHeroicStrike() {
	spell := war.RegisterSpell(core.SpellConfig{
		ActionID:       core.ActionID{SpellID: 29707},
		SpellSchool:    core.SpellSchoolPhysical,
		ProcMask:       core.ProcMaskMeleeMH,
		Flags:          core.SpellFlagMeleeMetrics,
		ClassSpellMask: SpellMaskHeroicStrike,
		MaxRange:       core.MaxMeleeRange,

		RageCost: core.RageCostOptions{
			Cost:   HeroicStrikeRageCost,
			Refund: 0.8,
		},

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				NonEmpty: true,
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		CritMultiplier:   war.DefaultMeleeCritMultiplier(),
		FlatThreatBonus:  194,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := 176 + war.MHWeaponDamage(sim, spell.MeleeAttackPower(target))
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)

			if !result.Landed() {
				spell.IssueRefund(sim)
			}

			if war.curQueueAura != nil {
				war.curQueueAura.Deactivate(sim)
			}
		},
	})
	war.HeroicStrike = spell
	war.makeQueueSpellsAndAura(spell, war.heroicStrikeRageThreshold)
}

func (war *Warrior) registerCleave() {
	const maxTargets int32 = 2
	flatDamage := 70 * (1 + 0.4*float64(war.Talents.ImprovedCleave))

	spell := war.RegisterSpell(core.SpellConfig{
		ActionID:       core.ActionID{SpellID: 25231},
		SpellSchool:    core.SpellSchoolPhysical,
		ProcMask:       core.ProcMaskMeleeMH,
		Flags:          core.SpellFlagMeleeMetrics,
		ClassSpellMask: SpellMaskCleave,
		MaxRange:       core.MaxMeleeRange,

		RageCost: core.RageCostOptions{
			Cost: CleaveRageCost,
		},

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				NonEmpty: true,
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		CritMultiplier:   war.DefaultMeleeCritMultiplier(),
		FlatThreatBonus:  125,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := flatDamage + war.MHWeaponDamage(sim, spell.MeleeAttackPower(target))
			spell.CalcCleaveDamage(sim, target, maxTargets, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
			spell.DealBatchedAoeDamage(sim)

			if war.curQueueAura != nil {
				war.curQueueAura.Deactivate(sim)
			}
		},
	})
	war.Cleave = spell
	war.makeQueueSpellsAndAura(spell, war.cleaveRageThreshold)
}

// A rage threshold is really "this ability's own cost, plus the rage to keep back for
// Bloodthirst and Whirlwind". Costs are read live rather than captured, so the talents
// that discount them -- Improved Heroic Strike on Heroic Strike, Focused Rage on both --
// are accounted for.
func (war *Warrior) rageReserve() float64 {
	return war.HsRageThreshold - war.HeroicStrike.Cost.GetCurrentCost()
}

func (war *Warrior) heroicStrikeRageThreshold() float64 {
	return war.HsRageThreshold
}

// Derived from Cleave's own cost rather than from the Heroic Strike threshold, so both
// abilities keep the same rage in reserve. An explicit setting overrides it.
func (war *Warrior) cleaveRageThreshold() float64 {
	if war.CleaveRageThreshold > 0 {
		return war.CleaveRageThreshold
	}
	return war.Cleave.Cost.GetCurrentCost() + war.rageReserve()
}

func (war *Warrior) makeQueueSpellsAndAura(srcSpell *core.Spell, rageThreshold func() float64) {
	queueAura := war.RegisterAura(core.Aura{
		Label:    "HS/Cleave Queue Aura-" + srcSpell.ActionID.String(),
		ActionID: srcSpell.ActionID.WithTag(1),
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			war.queueIsPending = false
		},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if war.curQueueAura != nil {
				war.curQueueAura.Deactivate(sim)
			}
			war.PseudoStats.DisableDWMissPenalty = true
			war.curQueueAura = aura
			war.curQueuedAutoSpell = srcSpell
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			war.PseudoStats.DisableDWMissPenalty = false
			war.curQueueAura = nil
			war.curQueuedAutoSpell = nil
			war.curQueueWillCancel = false
			war.curQueueThreshold = 0
		},
	})

	// Two ways to queue the same ability. Tag 1 queues normally and lets the next main
	// hand swing be spent on it. Tag 2 queues only to suppress the dual wield miss
	// penalty on off hand swings, then drops the queue again just before the main hand
	// swing lands, so the swing stays white and the rage is never spent.
	makeQueueSpell := func(tag int32, willCancel bool) {
		war.RegisterSpell(core.SpellConfig{
			ActionID:    srcSpell.ActionID.WithTag(tag),
			SpellSchool: core.SpellSchoolPhysical,
			ProcMask:    core.ProcMaskMeleeMHSpecial,
			Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | core.SpellFlagNoMetrics,

			Cast: core.CastConfig{
				DefaultCast: core.Cast{
					NonEmpty: true,
				},
			},

			ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
				return war.curQueueAura == nil &&
					!war.queueIsPending &&
					war.CurrentRage() >= srcSpell.Cost.GetCurrentCost() &&
					war.queuedRealismICD.IsReady(sim)
			},
			ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				if war.queuedRealismICD.IsReady(sim) {
					war.queueIsPending = true
					war.queuedRealismICD.Use(sim)
					sim.AddPendingAction(&core.PendingAction{
						NextActionAt: sim.CurrentTime + war.queuedRealismICD.Duration,
						OnAction: func(sim *core.Simulation) {
							queueAura.Activate(sim)
							// Must come after Activate: OnGain drops any previous queue
							// aura, and that OnExpire clears this flag.
							war.curQueueWillCancel = willCancel
							war.curQueueThreshold = rageThreshold()
							war.queueIsPending = false
						},
					})
				}
			},
		})
	}

	makeQueueSpell(1, false)
	makeQueueSpell(2, true)
}

// Returns true if the regular melee swing should be used, false otherwise.
func (war *Warrior) TryHSOrCleave(sim *core.Simulation, mhSwingSpell *core.Spell) *core.Spell {
	if !war.curQueueAura.IsActive() || (mhSwingSpell.ActionID.Tag != 1 && mhSwingSpell.ActionID.Tag != 12281) {
		war.PseudoStats.DisableDWMissPenalty = false
		return mhSwingSpell
	}

	// Deliberate queue cancel. Off hand swings already collected the dual wield benefit
	// while the aura was up, so drop it now and let the main hand swing land white,
	// keeping both the rage it generates and the rage the ability would have cost.
	//
	// Decided here rather than when the queue was armed, so a queue armed to cancel
	// still fires if rage climbed back over the threshold before the swing landed.
	// Two exclusions: extra attacks, which land too fast for a player to react to and
	// are what makes canceling risky in game, and two handers, which have no off hand
	// swings and so gain nothing from holding the queue.
	// A Heroic Strike queue held on multiple targets is only ever a stand-in for a Cleave
	// queue that could not be afforded, so it must never upgrade into a real cast --
	// spending the swing on a single-target Heroic Strike is worse than taking the white
	// hit and putting the rage into the next Cleave.
	standInForCleave := war.curQueuedAutoSpell.Matches(SpellMaskHeroicStrike) && sim.ActiveTargetCount() > 1

	if war.curQueueWillCancel &&
		mhSwingSpell.ActionID.Tag == 1 &&
		war.AutoAttacks.IsDualWielding &&
		(standInForCleave || war.CurrentRage() < war.curQueueThreshold) {
		war.curQueueAura.Deactivate(sim)
		return mhSwingSpell
	}

	if !war.curQueuedAutoSpell.CanCast(sim, war.CurrentTarget) {
		war.curQueueAura.Deactivate(sim)
		return mhSwingSpell
	}

	return war.curQueuedAutoSpell
}
