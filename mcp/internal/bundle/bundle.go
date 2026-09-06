// Package bundle renders the manifest that ships inside a .mcpb.
//
// An MCP bundle is a zip carrying the server and a manifest.json describing it. Claude Desktop
// reads that manifest to show an install dialog, so its tool and prompt lists are what a person
// sees before deciding to trust this thing. They are generated from the registry for the same
// reason the reference documentation is: a hand-written list would describe a server that does
// not exist the first time a tool is added.
package bundle

import (
	"encoding/json"

	"github.com/wowsims/tbc/mcp/internal/spec"
)

// Path is where the generated manifest lives, relative to the module root.
const Path = "mcpb/manifest.json"

// Version is the bundle's version, which must be semver and is what Claude Desktop shows and
// compares when offering an update. Bump it by hand when shipping a bundle.
const Version = "0.1.0"

// ManifestVersion is the spec revision this manifest is written against.
const ManifestVersion = "0.3"

// The manifest schema, narrowed to the parts this bundle uses.
type manifest struct {
	ManifestVersion string        `json:"manifest_version"`
	Name            string        `json:"name"`
	DisplayName     string        `json:"display_name"`
	Version         string        `json:"version"`
	Description     string        `json:"description"`
	LongDescription string        `json:"long_description"`
	Author          author        `json:"author"`
	Repository      link          `json:"repository"`
	License         string        `json:"license"`
	Keywords        []string      `json:"keywords"`
	Icon            string        `json:"icon"`
	Server          server        `json:"server"`
	Tools           []namedThing  `json:"tools"`
	Prompts         []promptEntry `json:"prompts"`
	Compatibility   compatibility `json:"compatibility"`
}

type author struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type link struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type server struct {
	Type       string    `json:"type"`
	EntryPoint string    `json:"entry_point"`
	MCPConfig  mcpConfig `json:"mcp_config"`
}

type mcpConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type namedThing struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type promptEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Arguments   []string `json:"arguments,omitempty"`
	Text        string   `json:"text"`
}

type compatibility struct {
	Platforms []string `json:"platforms"`
}

// A Target is one platform a bundle is built for. Claude Desktop reads compatibility.platforms
// to decide whether a bundle can be installed at all, and the manifest names a single entry
// point, so a bundle serves exactly one of these.
//
// One bundle for everything is not available: mcp_config.platform_overrides is keyed by platform
// rather than architecture, so a single bundle cannot serve both Apple Silicon and Intel without
// a lipo-fused universal binary, which would mean building on macOS purely to fuse two files.
type Target struct {
	// Name is what the makefile and the release artifact call this target.
	Name string
	// Platform is the manifest's compatibility value: darwin, win32 or linux.
	Platform string
	// EntryPoint is where the packed bundle puts the server, relative to the bundle root.
	EntryPoint string
}

var (
	Windows     = Target{Name: "windows", Platform: "win32", EntryPoint: "server/wowsimmcp.exe"}
	DarwinArm64 = Target{Name: "arm64-darwin", Platform: "darwin", EntryPoint: "server/wowsimmcp"}
	DarwinAmd64 = Target{Name: "amd64-darwin", Platform: "darwin", EntryPoint: "server/wowsimmcp"}
)

// Targets is every platform a bundle is published for. Windows is first because it is the
// default, and the one the committed manifest describes.
var Targets = []Target{Windows, DarwinArm64, DarwinAmd64}

// TargetByName looks a target up by the name the makefile passes.
func TargetByName(name string) (Target, bool) {
	for _, target := range Targets {
		if target.Name == name {
			return target, true
		}
	}
	return Target{}, false
}

// Options selects what a rendered manifest describes. The zero value renders the Windows
// bundle at the version in this file, which is what the committed manifest holds.
type Options struct {
	Target Target
	// Version overrides the constant above. A release stamps its tag here, because Claude
	// Desktop compares this field to decide whether an installed bundle has an update; a
	// version that never moved would mean nobody is ever offered one.
	Version string
}

func (o Options) resolve() Options {
	if o.Target.Platform == "" {
		o.Target = Windows
	}
	if o.Version == "" {
		o.Version = Version
	}
	return o
}

// Manifest renders the manifest for the given registry entries.
func Manifest(entries []spec.Entry, opts Options) ([]byte, error) {
	opts = opts.resolve()

	documents, err := spec.Docs(entries)
	if err != nil {
		return nil, err
	}

	rendered := manifest{
		ManifestVersion: ManifestVersion,
		Name:            "wowsims-tbc",
		DisplayName:     "WoW TBC Classic simulator",
		Version:         opts.Version,
		Description:     "Simulate WoW: The Burning Crusade Classic characters: gear, talents, rotations and encounters.",
		LongDescription: longDescription,
		Author:          author{Name: "wowsims", URL: "https://github.com/zmsl/tbc-new"},
		Repository:      link{Type: "git", URL: "https://github.com/zmsl/tbc-new"},
		License:         "MIT",
		Keywords:        []string{"wow", "tbc", "simulator", "dps", "theorycrafting"},
		Icon:            "icon.png",
		Server: server{
			Type:       "binary",
			EntryPoint: opts.Target.EntryPoint,
			MCPConfig: mcpConfig{
				// Everything the server needs -- the item database and the gear, rotation and talent
				// presets -- is compiled into the binary, so it takes no arguments and needs no
				// paths pointing anywhere.
				Command: "${__dirname}/" + opts.Target.EntryPoint,
				Args:    []string{},
			},
		},
		Compatibility: compatibility{Platforms: []string{opts.Target.Platform}},
	}

	for _, doc := range documents {
		switch doc.Kind {
		case spec.KindTool:
			rendered.Tools = append(rendered.Tools, namedThing{Name: doc.ID, Description: summaryOf(doc)})
		case spec.KindPrompt:
			entry := promptEntry{Name: doc.ID, Description: summaryOf(doc), Text: doc.Text}
			for _, argument := range doc.Arguments {
				entry.Arguments = append(entry.Arguments, argument.Name)
			}
			rendered.Prompts = append(rendered.Prompts, entry)
		}
	}

	encoded, err := json.MarshalIndent(rendered, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// The install dialog has room for a line, not an essay: the first paragraph of a declaration's
// description is its summary, which is exactly what belongs there.
func summaryOf(doc spec.Doc) string {
	for i := 0; i+1 < len(doc.Description); i++ {
		if doc.Description[i] == '\n' && doc.Description[i+1] == '\n' {
			return doc.Description[:i]
		}
	}
	return doc.Description
}

const longDescription = `Runs the wowsims TBC Classic simulator locally, with no network access and no
external services: the item database and every checked-in gear set, rotation and talent build are
compiled into the binary.

Ask it to simulate a character, compare gear or rotations, work out stat weights, or import a
character straight out of the game with the WowSimsExporter addon. Results come back with error
bars, and every setup has a share link you can open on wowsims.com.

Simulations are deterministic: the same question gives the same answer, and long runs happen in
the background with a job you can poll or cancel.`
