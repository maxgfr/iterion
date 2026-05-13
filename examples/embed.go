// Package examples exposes the recipe files shipped with iterion as an
// embed.FS so the binary can run them by basename from any working
// directory.
//
// Lookup order at launch time (see pkg/server.resolveWorkflowPath):
//  1. resolve the requested path against the server WorkDir;
//  2. on miss, if the path is a bare basename matching an embedded
//     recipe, materialise the embedded content into the run store
//     and use that path.
//
// Only top-level *.iter files and the proven `skill/*.iter` minimal
// recipes are embedded. Companion .md design journals and large
// non-recipe assets (images, mcp test servers, github-actions YAML)
// are intentionally excluded to keep the binary slim.
package examples

import (
	"embed"
	"io/fs"
	"sort"
)

//go:embed *.iter skill/*.iter
var Files embed.FS

// Get returns the contents of the embedded example with the given
// basename (e.g. "secured-renovacy.iter" or "skill/minimal_linear.iter").
// Returns ok=false if no such embedded recipe exists.
func Get(name string) ([]byte, bool) {
	data, err := Files.ReadFile(name)
	if err != nil {
		return nil, false
	}
	return data, true
}

// List returns the relative paths (within the embed FS) of all embedded
// .iter recipes, sorted alphabetically.
func List() []string {
	var out []string
	_ = fs.WalkDir(Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		out = append(out, path)
		return nil
	})
	sort.Strings(out)
	return out
}
