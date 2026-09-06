// Command genmanifest renders mcp/mcpb/manifest.json from the registry.
//
//	make mcp-bundle                   # regenerates it on the way to packing a .mcpb
//	go run ./cmd/genmanifest -check   # fail if it is stale
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wowsims/tbc/mcp/internal/bundle"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/registry"
	"github.com/wowsims/tbc/sim"
)

func main() {
	check := flag.Bool("check", false, "exit non-zero if the committed manifest is stale instead of rewriting it")
	flag.Parse()

	sim.RegisterAll()

	// Only declarations are rendered, so neither the preset tree nor the job directory is read.
	rendered, err := bundle.Manifest(registry.All(
		engine.FileConfig(filepath.Join("..", "ui")),
		jobs.Store{Dir: filepath.Join(os.TempDir(), "wowsimmcp-manifest")},
	))
	if err != nil {
		fail(err)
	}

	if *check {
		committed, err := os.ReadFile(bundle.Path)
		if err != nil {
			fail(err)
		}
		if string(committed) != string(rendered) {
			fail(fmt.Errorf("%s is out of date; run `make mcp-bundle`", bundle.Path))
		}
		return
	}

	if err := os.MkdirAll(filepath.Dir(bundle.Path), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(bundle.Path, rendered, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s\n", bundle.Path)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "genmanifest: %v\n", err)
	os.Exit(1)
}
