package engine

import (
	"math"
	"slices"
	"strings"

	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

// A RaidSimResult carries every cast, aura and resource event of every iteration, which runs to
// megabytes. Almost none of it answers the question that was asked, so results are reduced to
// this before they leave the server: the number, how certain it is, and enough of a breakdown to
// explain where it came from.
type ResultSummary struct {
	Dps             Metric           `json:"dps" jsonschema:"damage per second across all iterations"`
	Tps             *Metric          `json:"tps,omitempty" jsonschema:"threat per second, when non-zero"`
	Hps             *Metric          `json:"hps,omitempty" jsonschema:"healing per second, when non-zero"`
	Iterations      int32            `json:"iterations" jsonschema:"how many fights were simulated"`
	DurationSeconds float64          `json:"durationSeconds" jsonschema:"the fight length that was simulated"`
	SecondsOOM      float64          `json:"secondsOutOfMana,omitempty" jsonschema:"average seconds spent with no mana; anything above zero means the rotation is mana-capped"`
	Spells          []SpellBreakdown `json:"spells,omitempty" jsonschema:"per-spell contribution, largest share first"`
}

// Metric is an averaged number with its spread. The standard error is what says whether two
// results actually differ: a gap smaller than the errors of both is noise.
type Metric struct {
	Avg    float64 `json:"avg"`
	Stdev  float64 `json:"stdev" jsonschema:"standard deviation across iterations"`
	StdErr float64 `json:"stderr" jsonschema:"standard error of the mean: stdev divided by the square root of the iteration count"`
}

type SpellBreakdown struct {
	Name       string  `json:"name" jsonschema:"the spell or ability"`
	SpellID    int32   `json:"spellId,omitempty"`
	Casts      float64 `json:"casts" jsonschema:"casts per iteration"`
	DamageShar float64 `json:"damageSharePercent" jsonschema:"percentage of total damage this accounted for"`
	Dps        float64 `json:"dps" jsonschema:"damage per second from this spell alone"`
	CritPct    float64 `json:"critPercent,omitempty" jsonschema:"percentage of hits that critted"`
	MissPct    float64 `json:"missPercent,omitempty" jsonschema:"percentage of casts that missed or were resisted"`
}

// SummarizeResult reduces a finished simulation. spellLimit caps the breakdown; pass zero for
// the default.
func SummarizeResult(request *proto.RaidSimRequest, result *proto.RaidSimResult, spellLimit int) ResultSummary {
	summary := ResultSummary{}
	if result == nil || result.RaidMetrics == nil {
		return summary
	}

	if request != nil {
		if request.SimOptions != nil {
			summary.Iterations = request.SimOptions.Iterations
		}
		if request.Encounter != nil {
			summary.DurationSeconds = request.Encounter.Duration
		}
	}

	summary.Dps = metric(result.RaidMetrics.Dps, summary.Iterations)

	player := firstPlayer(result.RaidMetrics)
	if player == nil {
		return summary
	}

	if player.Threat != nil && player.Threat.Avg > 0 {
		threat := metric(player.Threat, summary.Iterations)
		summary.Tps = &threat
	}
	if player.Hps != nil && player.Hps.Avg > 0 {
		healing := metric(player.Hps, summary.Iterations)
		summary.Hps = &healing
	}
	summary.SecondsOOM = roundTo(player.SecondsOomAvg, 2)
	summary.Spells = breakdown(player, summary.Iterations, summary.DurationSeconds, spellLimit)

	return summary
}

func firstPlayer(metrics *proto.RaidMetrics) *proto.UnitMetrics {
	for _, party := range metrics.Parties {
		for _, player := range party.Players {
			if player != nil {
				return player
			}
		}
	}
	return nil
}

func metric(distribution *proto.DistributionMetrics, iterations int32) Metric {
	if distribution == nil {
		return Metric{}
	}
	stderr := 0.0
	if iterations > 0 {
		stderr = distribution.Stdev / math.Sqrt(float64(iterations))
	}
	return Metric{
		Avg:    roundTo(distribution.Avg, 2),
		Stdev:  roundTo(distribution.Stdev, 2),
		StdErr: roundTo(stderr, 3),
	}
}

const defaultSpellLimit = 12

func breakdown(player *proto.UnitMetrics, iterations int32, duration float64, limit int) []SpellBreakdown {
	if limit <= 0 {
		limit = defaultSpellLimit
	}
	if iterations <= 0 {
		iterations = 1
	}

	// Pets do their damage under their own metrics, but from the caller's point of view it is
	// still part of what the character did, so they are folded into the same list.
	units := append([]*proto.UnitMetrics{player}, player.Pets...)

	var total float64
	var spells []SpellBreakdown

	for _, unit := range units {
		for _, action := range unit.Actions {
			var casts, hits, crits, misses, damage float64
			for _, target := range action.Targets {
				casts += float64(target.Casts)
				hits += float64(target.Hits + target.Ticks)
				crits += float64(target.Crits + target.CritTicks)
				misses += float64(target.Misses + target.ResistedHits)
				damage += target.Damage
			}
			if damage == 0 && casts == 0 {
				continue
			}
			total += damage

			entry := SpellBreakdown{
				Name:    actionName(action, unit, player),
				SpellID: action.Id.GetSpellId(),
				Casts:   roundTo(casts/float64(iterations), 2),
			}
			if duration > 0 {
				entry.Dps = roundTo(damage/float64(iterations)/duration, 2)
			}
			if hits > 0 {
				entry.CritPct = roundTo(100*crits/hits, 1)
			}
			if casts > 0 && misses > 0 {
				entry.MissPct = roundTo(100*misses/casts, 1)
			}
			spells = append(spells, entry)
		}
	}

	if total > 0 {
		for i := range spells {
			spells[i].DamageShar = roundTo(100*spells[i].Dps*duration*float64(iterations)/total, 1)
		}
	}

	slices.SortFunc(spells, func(a, b SpellBreakdown) int {
		switch {
		case a.Dps > b.Dps:
			return -1
		case a.Dps < b.Dps:
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	if len(spells) > limit {
		spells = spells[:limit]
	}
	return spells
}

// Actions are identified by id, not by name, so a readable label has to come from the database.
// A pet's actions are prefixed with the pet's name, since "Melee" from the character and from
// its pet are otherwise indistinguishable.
func actionName(action *proto.ActionMetrics, unit, player *proto.UnitMetrics) string {
	name := ""
	if spellID := action.Id.GetSpellId(); spellID != 0 {
		name = SpellName(spellID)
	}
	if name == "" {
		if itemID := action.Id.GetItemId(); itemID != 0 {
			if item, ok := core.ItemsByID[itemID]; ok {
				name = item.Name
			}
		}
	}
	if name == "" {
		if other := action.Id.GetOtherId(); other != proto.OtherAction_OtherActionNone {
			name = strings.TrimPrefix(other.String(), "OtherAction")
		}
	}
	if name == "" {
		name = "Unknown"
	}

	if unit != player && unit.Name != "" {
		return unit.Name + ": " + name
	}
	return name
}

func roundTo(value float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(value*scale) / scale
}
