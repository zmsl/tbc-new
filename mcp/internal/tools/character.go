package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

type charStatsInput struct {
	setupInput
}

type charStatsOutput struct {
	Summary settingsSummary    `json:"summary" jsonschema:"the setup these stats belong to"`
	Stats   map[string]float64 `json:"stats" jsonschema:"final stats with buffs, talents and gear applied; zero-valued stats are omitted"`
	Sets    []string           `json:"sets,omitempty" jsonschema:"item set bonuses currently active"`
	Link    string             `json:"link" jsonschema:"share link for this setup"`
	Notes   []string           `json:"notes,omitempty" jsonschema:"which defaults were applied while assembling the setup"`
}

func charStats(config engine.Config) spec.Entry {
	return spec.Tool[charStatsInput, charStatsOutput]{
		Name:    "char_stats",
		Title:   "Compute character stats",
		Summary: "Reports a character's final stats -- with gear, talents, buffs and consumables applied -- without running a simulation.",
		Details: "Much cheaper than a sim and enough to answer most gear questions: whether a set reaches the\n" +
			"spell hit cap, how much mana it has, which set bonuses are active. Reach for a sim only when\n" +
			"the question is about throughput rather than stats.",
		Examples: []spec.Example{
			{Description: "stats for the smite priest's phase 5 set", Args: `{"spec": "SmitePriest", "gearSet": "p5"}`},
			{Description: "stats for a setup someone shared", Args: `{"link": "https://wowsims.com/tbc/priest/smite/#eJys..."}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input charStatsInput) (*mcp.CallToolResult, charStatsOutput, error) {
			var output charStatsOutput

			settings, notes, err := input.resolve(config)
			if err != nil {
				return nil, output, err
			}

			simRequest, err := core.BuildRaidSimRequest(settings, core.SimRequestOptions{Iterations: 1})
			if err != nil {
				return nil, output, err
			}

			result := core.ComputeStats(&proto.ComputeStatsRequest{
				Raid:      simRequest.Raid,
				Encounter: simRequest.Encounter,
			})
			if result.ErrorResult != "" {
				return nil, output, fmt.Errorf("%s", result.ErrorResult)
			}

			player, err := firstPlayerStats(result)
			if err != nil {
				return nil, output, err
			}

			output.Stats = namedStats(player.FinalStats)
			output.Sets = player.Sets
			output.Summary = summarize(settings)
			output.Notes = notes
			if output.Link, err = shareLink(config, settings); err != nil {
				return nil, output, err
			}
			return nil, output, nil
		},
	}
}

func firstPlayerStats(result *proto.ComputeStatsResult) (*proto.PlayerStats, error) {
	if result.RaidStats == nil {
		return nil, fmt.Errorf("no raid stats were computed")
	}
	for _, party := range result.RaidStats.Parties {
		for _, player := range party.Players {
			if player != nil {
				return player, nil
			}
		}
	}
	return nil, fmt.Errorf("no player stats were computed")
}

// The stats arrive as a bare array indexed by the Stat enum. Naming them costs nothing and makes
// the difference between a readable answer and a list of numbers.
func namedStats(unitStats *proto.UnitStats) map[string]float64 {
	named := map[string]float64{}
	if unitStats == nil {
		return named
	}

	for i, value := range unitStats.Stats {
		if value == 0 {
			continue
		}
		name, ok := proto.Stat_name[int32(i)]
		if !ok {
			continue
		}
		named[strings.TrimPrefix(name, "Stat")] = round(value)
	}
	return named
}

func round(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}

type gearValidateInput struct {
	setupInput
}

type gearValidateOutput struct {
	Equippable bool            `json:"equippable" jsonschema:"true when the gear could actually be worn in game"`
	Problems   []string        `json:"problems,omitempty" jsonschema:"one entry per reason the gear could not be worn"`
	Summary    settingsSummary `json:"summary" jsonschema:"the setup that was checked"`
}

func gearValidate(config engine.Config) spec.Entry {
	return spec.Tool[gearValidateInput, gearValidateOutput]{
		Name:    "gear_validate",
		Title:   "Check gear is wearable",
		Summary: "Checks that a gear set could actually be worn: slots, unique-equipped items and gems, meta gem colour requirements, enchant applicability and weapon combinations.",
		Details: "The simulator itself does not enforce any of this -- it will happily sim two of the same\n" +
			"unique trinket, or a meta gem whose requirement is unmet -- so check gear you assembled\n" +
			"yourself before trusting its numbers.",
		Examples: []spec.Example{
			{Description: "check a checked-in preset", Args: `{"spec": "SmitePriest", "gearSet": "p3"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input gearValidateInput) (*mcp.CallToolResult, gearValidateOutput, error) {
			var output gearValidateOutput

			settings, _, err := input.resolve(config)
			if err != nil {
				return nil, output, err
			}
			if settings.Player == nil || settings.Player.Equipment == nil {
				return nil, output, fmt.Errorf("the setup has no equipment")
			}

			output.Problems = engine.GearLookup().ValidateEquipment(settings.Player.Equipment)
			output.Equippable = len(output.Problems) == 0
			output.Summary = summarize(settings)
			return nil, output, nil
		},
	}
}
