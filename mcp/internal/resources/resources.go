// Package resources exposes the checked-in presets and the item database as readable URIs.
//
// These are resources rather than tools because they are addressable things with no arguments to
// get wrong: an agent (or a person) can point at wowsims://spec/priest/smite/gear/p3 and get
// exactly that file, and clients can list and cache them.
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim/core/proto"
)

const scheme = "wowsims://"

// Entries lists every resource the server exposes.
func Entries(config engine.Config) []spec.Entry {
	return []spec.Entry{
		presetResource(config, engine.PresetGear, "gear", "Gear set",
			"A checked-in gear set: item ids with their enchants and gems, in slot order.",
			"wowsims://spec/priest/smite/gear/p3"),
		presetResource(config, engine.PresetRotation, "apl", "Rotation",
			"A checked-in APL rotation: the priority list the simulated character follows.",
			"wowsims://spec/priest/smite/apl/default"),
		presetResource(config, engine.PresetBuild, "build", "Build",
			"A checked-in build: gear, talents, rotation and encounter together, in the same shape a share link carries.",
			"wowsims://spec/warrior/protection/build/default_encounter_only"),
		talentsResource(config),
		itemResource(),
	}
}

// Presets are files, so they are served verbatim: the same bytes the simulator reads, and the
// same bytes a person would see in the repository.
func presetResource(config engine.Config, kind engine.PresetKind, segment, title, summary, example string) spec.Entry {
	return spec.Resource{
		URI:      scheme + "spec/{class}/{spec}/" + segment + "/{name}",
		Name:     segment,
		Title:    title,
		Summary:  summary,
		Details:  "Names come from specs_list. The class and spec segments are the spec's preset path, e.g. priest/smite.",
		MIMEType: "application/json",
		Examples: []spec.Example{{Description: "read one", Args: example}},
		Handler: func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			resolved, name, err := parseSpecURI(request.Params.URI, segment)
			if err != nil {
				return nil, err
			}

			data, err := config.ReadPreset(resolved, kind, name)
			if err != nil {
				return nil, err
			}
			return jsonResult(request.Params.URI, data), nil
		},
	}
}

func talentsResource(config engine.Config) spec.Entry {
	return spec.Resource{
		URI:      scheme + "spec/{class}/{spec}/talents",
		Name:     "talents",
		Title:    "Talent builds",
		Summary:  "The talent builds checked in for a spec, as wowhead-format strings.",
		Details:  "These are what settings_create uses when no talents are given. Pass one as `talents` to sim a different build.",
		MIMEType: "application/json",
		Examples: []spec.Example{{Description: "the smite priest's builds", Args: "wowsims://spec/priest/smite/talents"}},
		Handler: func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			resolved, _, err := parseSpecURI(request.Params.URI, "talents")
			if err != nil {
				return nil, err
			}

			presets, err := config.ListTalents(resolved)
			if err != nil {
				return nil, err
			}
			encoded, err := json.MarshalIndent(presets, "", "  ")
			if err != nil {
				return nil, err
			}
			return jsonResult(request.Params.URI, encoded), nil
		},
	}
}

func itemResource() spec.Entry {
	return spec.Resource{
		URI:      scheme + "item/{id}",
		Name:     "item",
		Title:    "Item",
		Summary:  "One item from the database, by id: stats, sockets, phase, quality and where it drops.",
		Details:  "Use db_search_items to find ids; this is for reading one you already have, such as an id out of a gear set.",
		MIMEType: "application/json",
		Examples: []spec.Example{{Description: "Zhar'doom, Greatstaff of the Devourer", Args: "wowsims://item/32374"}},
		Handler: func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			raw := strings.TrimPrefix(request.Params.URI, scheme+"item/")
			id, err := strconv.ParseInt(raw, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("item id %q is not a number", raw)
			}

			item, ok := engine.Item(int32(id))
			if !ok {
				return nil, fmt.Errorf("no item with id %d", id)
			}
			encoded, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				return nil, err
			}
			return jsonResult(request.Params.URI, encoded), nil
		},
	}
}

// Templates are matched by the SDK before the handler runs, but the handler still receives the
// concrete URI and has to pull its own parameters back out of it.
func parseSpecURI(uri, segment string) (proto.Spec, string, error) {
	rest, ok := strings.CutPrefix(uri, scheme+"spec/")
	if !ok {
		return proto.Spec_SpecUnknown, "", fmt.Errorf("not a spec resource: %s", uri)
	}

	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[2] != segment {
		return proto.Spec_SpecUnknown, "", fmt.Errorf("expected %sspec/{class}/{spec}/%s/{name}, got %s", scheme, segment, uri)
	}

	resolved, err := engine.ParseSpec(parts[0] + "/" + parts[1])
	if err != nil {
		return proto.Spec_SpecUnknown, "", err
	}

	// Preset names may themselves contain a slash, e.g. the hunter's phase_3/bm.
	return resolved, strings.Join(parts[3:], "/"), nil
}

func jsonResult(uri string, data []byte) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}
}
