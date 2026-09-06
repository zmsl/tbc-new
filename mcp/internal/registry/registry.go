// Package registry is the one list of everything the server exposes.
//
// It exists so that the server, the documentation generator and the schema snapshot test all
// read the same list: anything registered is documented, and anything documented is registered.
package registry

import (
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/prompts"
	"github.com/wowsims/tbc/mcp/internal/resources"
	"github.com/wowsims/tbc/mcp/internal/spec"
	"github.com/wowsims/tbc/mcp/internal/tools"
)

// All returns every entry, in the order they should be presented.
func All(config engine.Config, store jobs.Store) []spec.Entry {
	var entries []spec.Entry
	entries = append(entries, tools.Entries(config, store)...)
	entries = append(entries, resources.Entries(config)...)
	entries = append(entries, prompts.Entries(config)...)
	return entries
}
