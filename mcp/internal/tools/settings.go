package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// setupInput is how every tool that needs a character takes one. Either paste a share link, or
// name a spec and let the checked-in presets fill in the rest.
//
// It is embedded rather than repeated so the vocabulary is identical everywhere, and so a share
// link produced by one tool can be fed straight into the next -- which is what keeps the server
// stateless: the link is the state.
type setupInput struct {
	Link      string `json:"link,omitempty" jsonschema:"a wowsims share link. Takes precedence over every other field here."`
	Spec      string `json:"spec,omitempty" jsonschema:"spec to build from presets, e.g. SmitePriest or priest/smite. Required unless link is given."`
	Build     string `json:"build,omitempty" jsonschema:"name of a checked-in build preset, which supplies gear, talents, rotation and encounter at once"`
	GearSet   string `json:"gearSet,omitempty" jsonschema:"name of a checked-in gear set. Defaults to the highest-numbered phase set."`
	Rotation  string `json:"rotation,omitempty" jsonschema:"name of a checked-in APL rotation. Defaults to 'default' where it exists, otherwise the first one."`
	Talents   string `json:"talents,omitempty" jsonschema:"talent string in wowhead format. Defaults to the spec's first checked-in talent preset."`
	Race      string `json:"race,omitempty" jsonschema:"race name, e.g. Undead. Defaults to a race the class can be."`
	Encounter string `json:"encounter,omitempty" jsonschema:"ShortSingleTarget (60s), LongSingleTarget (180s, the default) or LongMultiTarget (180s, 20 enemies)"`
	NoBuffs   bool   `json:"noBuffs,omitempty" jsonschema:"strip raid buffs, party buffs and debuffs. Use to measure a spec alone rather than in a raid."`
}

// resolve turns the input into settings, along with notes saying which defaults were applied.
func (i setupInput) resolve(config engine.Config) (*proto.IndividualSimSettings, []string, error) {
	if i.Link != "" {
		settings, err := core.DecodeIndividualShareLink(i.Link)
		if err != nil {
			return nil, nil, err
		}
		return settings, []string{"from the supplied share link"}, nil
	}

	if i.Spec == "" {
		return nil, nil, fmt.Errorf("supply either a share link or a spec")
	}
	resolved, err := engine.ParseSpec(i.Spec)
	if err != nil {
		return nil, nil, err
	}

	race := proto.Race_RaceUnknown
	if i.Race != "" {
		value, ok := proto.Race_value["Race"+i.Race]
		if !ok {
			if value, ok = proto.Race_value[i.Race]; !ok {
				return nil, nil, fmt.Errorf("unknown race %q", i.Race)
			}
		}
		race = proto.Race(value)
	}

	return config.BuildSettings(engine.SettingsRequest{
		Spec:      resolved,
		Race:      race,
		Build:     i.Build,
		GearSet:   i.GearSet,
		Rotation:  i.Rotation,
		Talents:   i.Talents,
		Encounter: i.Encounter,
		NoBuffs:   i.NoBuffs,
	})
}

// link builds a share URL for settings, which is how a result is handed back to a person.
func shareLink(config engine.Config, settings *proto.IndividualSimSettings) (string, error) {
	page, err := config.SpecPageURL(core.PlayerProtoToSpec(settings.Player))
	if err != nil {
		return "", err
	}
	return core.EncodeShareLink(page, settings)
}

type settingsCreateInput struct {
	setupInput
}

type settingsCreateOutput struct {
	Link    string          `json:"link" jsonschema:"share link for this setup; open it in a browser or pass it to another tool"`
	Summary settingsSummary `json:"summary" jsonschema:"what the setup actually contains"`
	Notes   []string        `json:"notes,omitempty" jsonschema:"which defaults were applied while assembling it"`
}

func settingsCreate(config engine.Config) spec.Entry {
	return spec.Tool[settingsCreateInput, settingsCreateOutput]{
		Name:    "settings_create",
		Title:   "Assemble a setup",
		Summary: "Builds a character setup from checked-in presets and returns a share link for it.",
		Details: "The link is the unit of state here: it carries the whole setup, so pass it to the other\n" +
			"tools instead of repeating the same arguments, and hand it to a person to open in the sim.\n" +
			"Every field except the spec has a default, and `notes` says which defaults were used.",
		Examples: []spec.Example{
			{Description: "the smite priest's phase 3 setup", Args: `{"spec": "SmitePriest", "gearSet": "p3"}`},
			{Description: "unbuffed, on a short fight", Args: `{"spec": "SmitePriest", "encounter": "ShortSingleTarget", "noBuffs": true}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input settingsCreateInput) (*mcp.CallToolResult, settingsCreateOutput, error) {
			var output settingsCreateOutput

			settings, notes, err := input.resolve(config)
			if err != nil {
				return nil, output, err
			}
			link, err := shareLink(config, settings)
			if err != nil {
				return nil, output, err
			}

			output.Link = link
			output.Summary = summarize(settings)
			output.Notes = notes
			return nil, output, nil
		},
	}
}

type linkDecodeInput struct {
	Link       string `json:"link" jsonschema:"a wowsims share link, as copied from the sim's export dialog or the browser's address bar"`
	IncludeRaw bool   `json:"includeRaw,omitempty" jsonschema:"also return the full settings as JSON. Large; only ask for it when you need to edit fields the summary does not cover."`
}

type linkDecodeOutput struct {
	Summary settingsSummary `json:"summary" jsonschema:"what the link contains"`
	Raw     string          `json:"raw,omitempty" jsonschema:"the complete IndividualSimSettings as JSON, when includeRaw was set"`
}

func linkDecode() spec.Entry {
	return spec.Tool[linkDecodeInput, linkDecodeOutput]{
		Name:    "link_decode",
		Title:   "Read a share link",
		Summary: "Decodes a wowsims share link into a readable summary of the setup it carries.",
		Details: "Use this when a person hands you a link and you need to know what is in it -- gear, talents,\n" +
			"race, encounter -- before simulating or changing anything.",
		Examples: []spec.Example{
			{Description: "summarise a link", Args: `{"link": "https://wowsims.com/tbc/priest/smite/#eJys..."}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input linkDecodeInput) (*mcp.CallToolResult, linkDecodeOutput, error) {
			var output linkDecodeOutput

			settings, err := core.DecodeIndividualShareLink(input.Link)
			if err != nil {
				return nil, output, err
			}

			output.Summary = summarize(settings)
			if input.IncludeRaw {
				encoded, err := protojson.Marshal(settings)
				if err != nil {
					return nil, output, err
				}
				output.Raw = string(encoded)
			}
			return nil, output, nil
		},
	}
}

type linkEncodeInput struct {
	Settings string `json:"settings" jsonschema:"a complete IndividualSimSettings as JSON, e.g. from link_decode with includeRaw set"`
	Spec     string `json:"spec,omitempty" jsonschema:"which sim page the link should open. Inferred from the settings when omitted."`
}

type linkEncodeOutput struct {
	Link    string          `json:"link" jsonschema:"the share link"`
	Summary settingsSummary `json:"summary" jsonschema:"what the link contains"`
}

func linkEncode(config engine.Config) spec.Entry {
	return spec.Tool[linkEncodeInput, linkEncodeOutput]{
		Name:    "link_encode",
		Title:   "Write a share link",
		Summary: "Packs edited settings back into a share link.",
		Details: "The counterpart to link_decode: decode a link, change the fields you need, encode it again.\n" +
			"For setups built from presets, settings_create is easier.",
		Examples: []spec.Example{
			{Description: "re-link edited settings", Args: `{"settings": "{\"player\":{...},\"encounter\":{...}}"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input linkEncodeInput) (*mcp.CallToolResult, linkEncodeOutput, error) {
			var output linkEncodeOutput

			settings := &proto.IndividualSimSettings{}
			if err := protojson.Unmarshal([]byte(input.Settings), settings); err != nil {
				return nil, output, fmt.Errorf("settings: %w", err)
			}
			if settings.Player == nil {
				return nil, output, fmt.Errorf("settings contain no player")
			}

			page := ""
			if input.Spec != "" {
				resolved, err := engine.ParseSpec(input.Spec)
				if err != nil {
					return nil, output, err
				}
				if page, err = config.SpecPageURL(resolved); err != nil {
					return nil, output, err
				}
			}

			var err error
			if page == "" {
				if output.Link, err = shareLink(config, settings); err != nil {
					return nil, output, err
				}
			} else if output.Link, err = core.EncodeShareLink(page, settings); err != nil {
				return nil, output, err
			}

			output.Summary = summarize(settings)
			return nil, output, nil
		},
	}
}
