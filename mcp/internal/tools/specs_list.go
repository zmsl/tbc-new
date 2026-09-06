// Package tools holds the callable surface: one file per tool, each declaring its typed input
// and output alongside the handler that serves it.
package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

// Entries lists every tool the server exposes.
func Entries(config engine.Config) []spec.Entry {
	return []spec.Entry{
		specsList(config),
	}
}

type specsListInput struct{}

type specsListOutput struct {
	Specs []specSummary `json:"specs" jsonschema:"every spec this build can simulate"`
}

type specSummary struct {
	Spec        string         `json:"spec" jsonschema:"spec name to pass to other tools, e.g. SmitePriest"`
	Class       string         `json:"class" jsonschema:"the class this spec belongs to, e.g. Priest"`
	PresetsPath string         `json:"presetsPath" jsonschema:"where its presets live under the presets root, e.g. priest/smite"`
	Presets     engine.Presets `json:"presets" jsonschema:"checked-in gear sets, rotations and builds for this spec"`
}

func specsList(config engine.Config) spec.Entry {
	return spec.Tool[specsListInput, specsListOutput]{
		Name:    "specs_list",
		Title:   "List simulable specs",
		Summary: "Lists the class specs this simulator can run, with the gear sets, rotations and builds checked in for each.",
		Details: "Start here when you do not already know what to pass as `spec` elsewhere, or when you want a\n" +
			"named preset to sim rather than assembling gear yourself. Names listed under `presets` are the\n" +
			"names other tools and the wowsims:// resources expect.",
		Examples: []spec.Example{
			{Description: "list everything", Args: "{}"},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input specsListInput) (*mcp.CallToolResult, specsListOutput, error) {
			var output specsListOutput

			for _, registered := range core.RegisteredSpecs() {
				dir, ok := engine.SpecDir(registered)
				if !ok {
					// A spec the engine knows about but this server has no directory for: report
					// it rather than hiding it, since it can still be simmed from a share link.
					output.Specs = append(output.Specs, specSummary{Spec: specName(registered)})
					continue
				}

				presets, err := config.ListPresets(registered)
				if err != nil {
					return nil, output, err
				}

				output.Specs = append(output.Specs, specSummary{
					Spec:        specName(registered),
					Class:       className(registered),
					PresetsPath: dir,
					Presets:     presets,
				})
			}

			return nil, output, nil
		},
	}
}

// The enum spells every value "SpecX"; the prefix is noise once you know they are all specs.
func specName(s proto.Spec) string {
	name := s.String()
	return name[len("Spec"):]
}

func className(s proto.Spec) string {
	if class, ok := specClasses[s]; ok {
		return class.String()[len("Class"):]
	}
	return ""
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
