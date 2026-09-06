package spec

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register turns declarations into a live server. This is the only place that calls the SDK's
// registration functions, so adding a tool never means touching protocol code.
//
// Every entry is validated first, and a failure stops the whole registration rather than
// starting a server that is missing something: a half-registered server is much harder to
// diagnose from the client side than one that refuses to start.
func Register(server *mcp.Server, entries []Entry) error {
	seen := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return err
		}

		key := string(entry.Kind()) + ":" + entry.ID()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s %q is declared twice", entry.Kind(), entry.ID())
		}
		seen[key] = struct{}{}

		if err := entry.register(server); err != nil {
			return fmt.Errorf("%s %q: %w", entry.Kind(), entry.ID(), err)
		}
	}

	return nil
}

// Docs renders every entry, in declaration order. The docs generator and the schema snapshot
// test both read this, so what ships as documentation is what the server actually registered.
func Docs(entries []Entry) ([]Doc, error) {
	docs := make([]Doc, 0, len(entries))
	for _, entry := range entries {
		doc, err := entry.Doc()
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
