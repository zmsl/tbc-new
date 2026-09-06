// Command wowsimmcp serves the TBC simulator over the Model Context Protocol, on stdio.
//
// The server is stateless: every tool is a function of its arguments, and the only thing held
// between calls is the item database, which is loaded once and never changes. Long simulations
// are addressed by a content hash of the request rather than by a session, so an ID means the
// same thing to any instance and survives a restart.
//
// Build it with the item database or the item lookups come back empty:
//
//	go build --tags=with_db ./...
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/registry"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/sim"
	"github.com/wowsims/tbc/sim/core"
)

// Version is set by the makefile at build time.
var Version = "development"

func main() {
	presetsRoot := flag.String("presets-root", "", "directory holding the checked-in presets (the repo's ui/). Defaults to $WOWSIMS_PRESETS_ROOT, then ./ui, then the ui/ beside the binary.")
	flag.Parse()

	root, err := resolvePresetsRoot(*presetsRoot)
	if err != nil {
		log.Fatalf("wowsimmcp: %v", err)
	}

	// Registers every spec's agent factory. Without it the engine cannot build a player for any
	// spec, and nothing simulates.
	sim.RegisterAll()

	if !core.WITH_DB {
		// Not fatal: share links carry their own settings, so simulating still works. Item
		// lookups and gear searches do not.
		log.Print("wowsimmcp: built without the item database; item lookups will be empty (build with --tags=with_db)")
	}

	config := engine.Config{PresetsRoot: root}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "wowsims-tbc",
		Title:   "WoW TBC Classic simulator",
		Version: Version,
	}, nil)

	if err := spec.Register(server, registry.All(config)); err != nil {
		log.Fatalf("wowsimmcp: %v", err)
	}

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("wowsimmcp: %v", err)
	}
}

// The presets are files in the repo rather than data compiled into the binary, so the server has
// to be told where they are. Checking the obvious places keeps the common cases -- running from
// the repo, or from a build directory beside it -- working with no flag at all.
func resolvePresetsRoot(flagValue string) (string, error) {
	candidates := []string{flagValue, os.Getenv("WOWSIMS_PRESETS_ROOT")}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "ui"), filepath.Join(cwd, "..", "ui"))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "ui"), filepath.Join(exeDir, "..", "ui"))
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Clean(candidate), nil
		}
	}

	return "", fmt.Errorf("cannot find the presets directory; pass --presets-root pointing at the repo's ui/")
}
