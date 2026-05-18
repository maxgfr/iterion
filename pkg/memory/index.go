package memory

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// frontmatterPeekBytes caps how much of each .md file BuildIndex
// reads to extract metadata. Frontmatter + fallback first H1 fit
// in a few hundred bytes; anything past this is body content we
// don't care about for the index.
const frontmatterPeekBytes = 4096

// maxFallbackH1Lines caps how many lines we scan past the
// frontmatter close looking for an H1 when no `title:` was set.
// Files without a heading in the first few lines won't have one.
const maxFallbackH1Lines = 30

// IndexEntry summarises one Markdown file in a scope.
type IndexEntry struct {
	Path        string
	Title       string
	Description string
	Tags        []string
}

// BuildIndex walks the scope recursively, collecting one
// IndexEntry per Markdown file. Lexicographic by path.
func (s *Scope) BuildIndex() ([]IndexEntry, error) {
	if s.root == "" {
		return nil, nil
	}
	if _, err := os.Stat(s.root); os.IsNotExist(err) {
		return nil, nil
	}

	var entries []IndexEntry
	err := filepath.WalkDir(s.root, func(abs string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(abs), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(s.root, abs)
		e, err := readIndexEntry(abs)
		if err != nil {
			return nil
		}
		e.Path = rel
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// readIndexEntry opens abs and reads only the first ~4KB to peek
// at frontmatter + the H1 fallback. Avoids slurping multi-MB
// session files when only the prefix is interesting.
func readIndexEntry(abs string) (IndexEntry, error) {
	f, err := os.Open(abs)
	if err != nil {
		return IndexEntry{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, frontmatterPeekBytes))
	if err != nil {
		return IndexEntry{}, err
	}
	return parseIndexMetadata(data), nil
}

// parseIndexMetadata extracts title / description / tags from a
// Markdown file's YAML-style frontmatter. Title falls back to the
// first body H1 (capped at maxFallbackH1Lines so we don't scan
// past the early body region).
func parseIndexMetadata(data []byte) IndexEntry {
	var e IndexEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4*1024), 64*1024)

	inFrontmatter := false
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			firstLine = false
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
		}
		if inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
				break
			}
			parseFrontmatterLine(line, &e)
			continue
		}
		if e.Title == "" && strings.HasPrefix(strings.TrimSpace(line), "# ") {
			e.Title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		}
		break
	}

	if e.Title == "" {
		for i := 0; i < maxFallbackH1Lines && scanner.Scan(); i++ {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "# ") {
				e.Title = strings.TrimSpace(strings.TrimPrefix(line, "#"))
				break
			}
		}
	}
	return e
}

func parseFrontmatterLine(line string, e *IndexEntry) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return
	}
	key := strings.TrimSpace(line[:idx])
	val := strings.TrimSpace(line[idx+1:])
	switch key {
	case "title":
		e.Title = unquote(val)
	case "description":
		e.Description = unquote(val)
	case "tags":
		e.Tags = parseTagList(val)
	}
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	if len(s) >= 2 && (s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func parseTagList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := unquote(strings.TrimSpace(p))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
