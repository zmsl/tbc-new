// Command gendocs renders mcp/docs/TOOLS.md from the registry.
//
//	make mcp-docs            # rewrite the reference
//	go run ./cmd/gendocs -check   # fail if it is stale
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wowsims/tbc/mcp/internal/docs"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/registry"
	"github.com/wowsims/tbc/sim"
)

func main() {
	check := flag.Bool("check", false, "exit non-zero if the committed reference is stale instead of rewriting it")
	flag.Parse()

	sim.RegisterAll()

	// Entries are constructed with a config, but only their declarations are rendered, so the
	// output does not depend on what is in the presets tree.
	// The jobs directory is never touched while rendering: only declarations are read.
	rendered, err := docs.Render(registry.All(
		engine.Config{PresetsRoot: filepath.Join("..", "ui")},
		jobs.Store{Dir: filepath.Join(os.TempDir(), "wowsimmcp-docs")},
	))
	if err != nil {
		fail(err)
	}

	if *check {
		committed, err := os.ReadFile(docs.Path)
		if err != nil {
			fail(err)
		}
		if string(committed) != rendered {
			fail(fmt.Errorf("%s is out of date; run `make mcp-docs`", docs.Path))
		}
		return
	}

	if err := os.MkdirAll(filepath.Dir(docs.Path), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(docs.Path, []byte(rendered), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s\n", docs.Path)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "gendocs: %v\n", err)
	os.Exit(1)
}
