package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowsims/tbc/mcp/internal/docs"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/registry"
	"github.com/wowsims/tbc/sim"
)

func init() {
	sim.RegisterAll()
}

func render(t *testing.T) string {
	t.Helper()

	rendered, err := docs.Render(registry.All(engine.FileConfig(filepath.Join("..", "..", "..", "ui")), jobs.Store{Dir: t.TempDir()}))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return rendered
}

// The committed reference carries every tool's generated JSON Schema, so this is the schema
// snapshot: a change to any field's name, type or description fails here until it is regenerated
// and shows up in the diff.
func TestReferenceIsUpToDate(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", docs.Path))
	if err != nil {
		t.Fatalf("read committed reference: %v (run `make mcp-docs`)", err)
	}

	if string(committed) != render(t) {
		t.Error("mcp/docs/TOOLS.md is out of date; run `make mcp-docs` and review the diff")
	}
}

// Rendering must not depend on anything outside the declarations, or the reference would differ
// between machines and the snapshot would be worthless.
func TestRenderIsDeterministic(t *testing.T) {
	if render(t) != render(t) {
		t.Error("two renders of the same registry differ")
	}
}

func TestRenderIncludesDescriptionsAndSchemas(t *testing.T) {
	rendered := render(t)

	for _, want := range []string{
		"### `specs_list`",
		"Lists the class specs this simulator can run",
		"Examples:",
		"Input schema",
		"Output schema",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("reference is missing %q", want)
		}
	}
}
