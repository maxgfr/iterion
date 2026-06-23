package bundle

import (
	"os"
	"strings"

	"go.yaml.in/yaml/v2"
)

// Frontmatter is the optional `## ---` … `## ---` YAML block at the top of a
// main.bot. It lets a loose .bot file or a bundle carry catalog metadata
// (name / description / triggers / capabilities) inline. For a bundle the
// manifest is authoritative; a non-empty frontmatter value OVERRIDES the
// manifest's triggers/capabilities at discovery time
// (botregistry.parseBundle). bundlelint flags that silent override (C221).
type Frontmatter struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Triggers     []string `yaml:"triggers"`
	Capabilities []string `yaml:"capabilities"`
}

// ReadFrontmatter reads the file at path and returns its parsed frontmatter,
// or nil when the file is unreadable or carries no `## ---` block.
func ReadFrontmatter(path string) *Frontmatter {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return ParseFrontmatter(raw)
}

// ParseFrontmatter pulls a `## ---` … `## ---` block from the top of the file
// and YAML-decodes the inner content. The block is allowed only at the very
// top of the file, optionally after blank lines. Returns nil when the block
// is absent or malformed.
func ParseFrontmatter(raw []byte) *Frontmatter {
	lines := strings.Split(string(raw), "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "## ---" {
		return nil
	}
	start := i + 1
	end := -1
	for j := start; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "## ---" {
			end = j
			break
		}
	}
	if end < 0 {
		return nil
	}
	var yamlLines []string
	for _, ln := range lines[start:end] {
		stripped := strings.TrimPrefix(ln, "## ")
		stripped = strings.TrimPrefix(stripped, "##")
		yamlLines = append(yamlLines, stripped)
	}
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &fm); err != nil {
		return nil
	}
	return &fm
}
