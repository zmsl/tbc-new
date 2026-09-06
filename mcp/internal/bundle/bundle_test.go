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

func render(t *testing.T) []byte {
	t.Helper()
	rendered, err := bundle.Manifest(entries(t))
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

	if string(committed) != string(render(t)) {
		t.Error("mcp/mcpb/manifest.json is out of date; run `make mcp-bundle` and review the diff")
	}
}

func TestManifestDescribesTheServer(t *testing.T) {
	var parsed struct {
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
	if err := json.Unmarshal(render(t), &parsed); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if parsed.Server.Type != "binary" || parsed.Server.EntryPoint != bundle.EntryPoint {
		t.Errorf("server = %+v", parsed.Server)
	}
	// The binary carries its own database and presets, so a command with arguments would mean
	// something is being looked for outside the bundle.
	if len(parsed.Server.MCPConfig.Args) != 0 {
		t.Errorf("the server takes arguments: %v", parsed.Server.MCPConfig.Args)
	}
	if !strings.HasPrefix(parsed.Server.MCPConfig.Command, "${__dirname}/") {
		t.Errorf("command %q is not relative to the installed bundle", parsed.Server.MCPConfig.Command)
	}
	if len(parsed.Compatibility.Platforms) != 1 || parsed.Compatibility.Platforms[0] != "win32" {
		t.Errorf("platforms = %v; the bundle carries a Windows binary only", parsed.Compatibility.Platforms)
	}

	// Every registered tool and prompt has to appear, or the install dialog under-reports what the
	// server can do.
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
