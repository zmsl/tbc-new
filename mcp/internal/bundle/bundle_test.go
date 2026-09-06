package bundle_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowsims/tbc/mcp/internal/bundle"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/registry"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim"
)

func init() {
	sim.RegisterAll()
}

func entries(t *testing.T) []spec.Entry {
	t.Helper()
	return registry.All(
		engine.FileConfig(filepath.Join("..", "..", "..", "ui")),
		jobs.Store{Dir: t.TempDir()},
	)
}

func render(t *testing.T, options bundle.Options) []byte {
	t.Helper()
	rendered, err := bundle.Manifest(entries(t), options)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return rendered
}

// The manifest is what Claude Desktop shows before someone installs this, so it has to describe
// the server that actually ships rather than the one that shipped last time.
func TestManifestIsUpToDate(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", bundle.Path))
	if err != nil {
		t.Fatalf("read committed manifest: %v (run `make mcp-bundle`)", err)
	}

	if string(committed) != string(render(t, bundle.Options{})) {
		t.Error("mcp/mcpb/manifest.json is out of date; run `make mcp-bundle` and review the diff")
	}
}

// manifestShape is the part of a rendered manifest these tests read.
type manifestShape struct {
	ManifestVersion string `json:"manifest_version"`
	Version         string `json:"version"`
	Server          struct {
		Type       string `json:"type"`
		EntryPoint string `json:"entry_point"`
		MCPConfig  struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcp_config"`
	} `json:"server"`
	Tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"tools"`
	Prompts []struct {
		Name string `json:"name"`
		Text string `json:"text"`
	} `json:"prompts"`
	Compatibility struct {
		Platforms []string `json:"platforms"`
	} `json:"compatibility"`
}

func decode(t *testing.T, options bundle.Options) manifestShape {
	t.Helper()

	var parsed manifestShape
	if err := json.Unmarshal(render(t, options), &parsed); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return parsed
}

// A bundle carries one platform's binary, and Claude Desktop reads compatibility.platforms to
// decide whether it can be installed at all, so a target that misdescribes itself either fails
// to install or installs a binary the machine cannot run.
func TestManifestDescribesEachTarget(t *testing.T) {
	for _, target := range bundle.Targets {
		t.Run(target.Name, func(t *testing.T) {
			parsed := decode(t, bundle.Options{Target: target})

			if parsed.Server.Type != "binary" || parsed.Server.EntryPoint != target.EntryPoint {
				t.Errorf("server = %+v, want entry point %q", parsed.Server, target.EntryPoint)
			}
			if len(parsed.Compatibility.Platforms) != 1 || parsed.Compatibility.Platforms[0] != target.Platform {
				t.Errorf("platforms = %v, want [%s]", parsed.Compatibility.Platforms, target.Platform)
			}
			// The binary carries its own database and presets, so a command with arguments would
			// mean something is being looked for outside the bundle.
			if len(parsed.Server.MCPConfig.Args) != 0 {
				t.Errorf("the server takes arguments: %v", parsed.Server.MCPConfig.Args)
			}
			if parsed.Server.MCPConfig.Command != "${__dirname}/"+target.EntryPoint {
				t.Errorf("command %q does not point at this target's entry point", parsed.Server.MCPConfig.Command)
			}
			if !strings.HasPrefix(parsed.Server.MCPConfig.Command, "${__dirname}/") {
				t.Errorf("command %q is not relative to the installed bundle", parsed.Server.MCPConfig.Command)
			}
		})
	}
}

// Claude Desktop compares this field against what is installed to decide whether an update
// exists, so a release has to be able to stamp its tag on it.
func TestManifestVersionCanBeStamped(t *testing.T) {
	if got := decode(t, bundle.Options{Version: "9.9.9"}).Version; got != "9.9.9" {
		t.Errorf("stamped version = %q, want 9.9.9", got)
	}
	if got := decode(t, bundle.Options{}).Version; got != bundle.Version {
		t.Errorf("default version = %q, want %q", got, bundle.Version)
	}
}

// Every registered tool and prompt has to appear, or the install dialog under-reports what the
// server can do.
func TestManifestListsEveryDeclaration(t *testing.T) {
	parsed := decode(t, bundle.Options{})

	listed := map[string]bool{}
	for _, tool := range parsed.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q has no description in the manifest", tool.Name)
		}
		listed[tool.Name] = true
	}
	for _, prompt := range parsed.Prompts {
		// The schema requires the workflow text, and an empty one would install a prompt that says
		// nothing.
		if strings.TrimSpace(prompt.Text) == "" {
			t.Errorf("prompt %q has no text", prompt.Name)
		}
		listed[prompt.Name] = true
	}

	for _, entry := range entries(t) {
		if entry.Kind() == spec.KindResource {
			continue
		}
		if !listed[entry.ID()] {
			t.Errorf("%s %q is registered but missing from the manifest", entry.Kind(), entry.ID())
		}
	}
}

// An unknown name has to fail rather than silently pack the default target's manifest next to
// another platform's binary.
func TestTargetByNameRejectsUnknown(t *testing.T) {
	if _, ok := bundle.TargetByName("solaris"); ok {
		t.Error("TargetByName accepted a target that is not published")
	}
	for _, target := range bundle.Targets {
		if found, ok := bundle.TargetByName(target.Name); !ok || found != target {
			t.Errorf("TargetByName(%q) = %+v, %v", target.Name, found, ok)
		}
	}
}
