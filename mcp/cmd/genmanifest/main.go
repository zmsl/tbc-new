// Command genmanifest renders mcp/mcpb/manifest.json from the registry.
//
//	make mcp-bundle                   # regenerates it on the way to packing a .mcpb
//	go run ./cmd/genmanifest -check   # fail if it is stale
//
// Packing writes one manifest per platform, so -target and -o name which bundle is being
// described and where it goes; the defaults are the committed Windows manifest.
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
	target := flag.String("target", bundle.Windows.Name, "which platform's bundle to describe: windows, arm64-darwin or amd64-darwin")
	version := flag.String("version", "", "version to stamp on the manifest; defaults to the one in the bundle package")
	out := flag.String("o", bundle.Path, "where to write the manifest")
	flag.Parse()

	sim.RegisterAll()

	// -check is about the committed manifest, which is the default target at the default
	// version, so it deliberately ignores both flags.
	var options bundle.Options
	if !*check {
		selected, ok := bundle.TargetByName(*target)
		if !ok {
			fail(fmt.Errorf("unknown target %q", *target))
		}
		options = bundle.Options{Target: selected, Version: *version}
	}

	// Only declarations are rendered, so neither the preset tree nor the job directory is read.
	rendered, err := bundle.Manifest(registry.All(
		engine.FileConfig(filepath.Join("..", "ui")),
		jobs.Store{Dir: filepath.Join(os.TempDir(), "wowsimmcp-manifest")},
	), options)
	if err != nil {
		fail(err)
	}

	if *check {
		committed, err := os.ReadFile(*out)
		if err != nil {
			fail(err)
		}
		if string(committed) != string(rendered) {
			fail(fmt.Errorf("%s is out of date; run `make mcp-bundle`", bundle.Path))
		}
		return
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*out, rendered, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s\n", *out)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "genmanifest: %v\n", err)
	os.Exit(1)
}
