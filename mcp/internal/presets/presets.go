// Package presets carries the checked-in gear sets, rotations, builds and talent presets inside
// the binary.
//
// The simulator reads them from the repository's ui/ tree, which is fine when the server runs
// beside a checkout and useless when it does not: a copy installed somewhere else, or on another
// operating system, would have to be told where a source tree lives and would break the moment
// that tree moved. Embedding them makes the binary self-contained.
//
// The files are copied in at build time by `make mcp` rather than committed here, so there is
// exactly one copy of each preset in the repository and no chance of the two drifting. A plain
// `go build` therefore produces a binary with no presets, which is not an error: the server falls
// back to reading them from disk.
package presets

import (
	"embed"
	"io/fs"
)

//go:embed all:files
var embedded embed.FS

// FS returns the embedded preset tree, or nil when this binary was built without one.
func FS() fs.FS {
	tree, err := fs.Sub(embedded, "files")
	if err != nil {
		return nil
	}

	entries, err := fs.ReadDir(tree, ".")
	if err != nil {
		return nil
	}
	// A build that skipped the copy still embeds the placeholder that keeps the directory in git,
	// so presence of files is not enough: the tree has to have class directories in it.
	for _, entry := range entries {
		if entry.IsDir() {
			return tree
		}
	}
	return nil
}
