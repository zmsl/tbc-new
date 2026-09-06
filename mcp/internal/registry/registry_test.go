package registry_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/registry"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim"
)

func init() {
	sim.RegisterAll()
}

// Each test gets its own job directory: the store is on disk precisely so it can be shared, and
// sharing it between tests would let one test see another's runs.
func testStore(t *testing.T) jobs.Store {
	t.Helper()
	return jobs.Store{Dir: t.TempDir()}
}

func testConfig() engine.Config {
	return engine.FileConfig(filepath.Join("..", "..", "..", "ui"))
}

// Connects a client to a server carrying the real registry, the way an agent would.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "wowsims-tbc", Version: "test"}, nil)
	if err := spec.Register(server, registry.All(testConfig(), testStore(t))); err != nil {
		t.Fatalf("register: %v", err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx := t.Context()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })

	return clientSession
}

// Nothing registers without documenting itself, and nothing is declared twice.
func TestRegistryValidates(t *testing.T) {
	for _, entry := range registry.All(testConfig(), testStore(t)) {
		if err := entry.Validate(); err != nil {
			t.Errorf("%s %q: %v", entry.Kind(), entry.ID(), err)
		}
	}
}

// What an agent sees when it lists the server's tools: a description with the summary, the
// detail and the worked examples folded in, and a generated input schema.
func TestToolsAreSelfDescribing(t *testing.T) {
	session := connect(t)

	result, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(result.Tools) == 0 {
		t.Fatal("no tools registered")
	}

	for _, tool := range result.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", tool.Name)
		}
		if tool.OutputSchema == nil {
			t.Errorf("tool %q has no output schema", tool.Name)
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q has no annotations", tool.Name)
			continue
		}
		// Everything here answers questions except job_cancel, which stops a run. A tool that
		// changes something has to say so, and to say whether repeating it is safe.
		if !tool.Annotations.ReadOnlyHint && !tool.Annotations.IdempotentHint {
			t.Errorf("tool %q is neither read-only nor idempotent", tool.Name)
		}
	}
}

func TestSpecsListReportsPresets(t *testing.T) {
	session := connect(t)

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "specs_list"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool reported an error: %v", result.Content)
	}

	var output struct {
		Specs []struct {
			Spec        string `json:"spec"`
			Class       string `json:"class"`
			PresetsPath string `json:"presetsPath"`
			Presets     struct {
				GearSets  []string `json:"gearSets"`
				Rotations []string `json:"rotations"`
			} `json:"presets"`
		} `json:"specs"`
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}

	if len(output.Specs) < 18 {
		t.Errorf("got %d specs, expected every registered one", len(output.Specs))
	}

	var found bool
	for _, s := range output.Specs {
		if s.Spec != "SmitePriest" {
			continue
		}
		found = true

		if s.Class != "Priest" || s.PresetsPath != "priest/smite" {
			t.Errorf("smite priest reported as class %q at %q", s.Class, s.PresetsPath)
		}
		if !contains(s.Presets.GearSets, "p3") {
			t.Errorf("smite priest gear sets %v do not include p3", s.Presets.GearSets)
		}
		if !contains(s.Presets.Rotations, "default") {
			t.Errorf("smite priest rotations %v do not include default", s.Presets.Rotations)
		}
	}
	if !found {
		t.Error("smite priest missing from the spec list")
	}
}

// The same arguments must produce the same answer: the server holds nothing between calls.
func TestToolCallsAreDeterministic(t *testing.T) {
	session := connect(t)

	first, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "specs_list"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "specs_list"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	firstJSON, _ := json.Marshal(first.StructuredContent)
	secondJSON, _ := json.Marshal(second.StructuredContent)
	if string(firstJSON) != string(secondJSON) {
		t.Error("two identical calls returned different results")
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	entries := registry.All(testConfig(), testStore(t))
	if len(entries) == 0 {
		t.Fatal("registry is empty")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "dupes", Version: "test"}, nil)
	err := spec.Register(server, append(entries, entries[0]))
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Errorf("err = %v, want a duplicate declaration error", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
