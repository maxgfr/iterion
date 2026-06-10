package knowledge

import (
	"bufio"
	"bytes"
	"strings"
)

const frontmatterPeekBytes = 4096

// ParseMarkdownMeta extracts title / description / tags from a Markdown
// document's YAML-style frontmatter, falling back to the first body H1
// for the title. Only the first ~4KB is scanned. Shared by both the FS
// and cloud MemoryStore adapters so the auto-index renders identically.
func ParseMarkdownMeta(data []byte) (title, description string, tags []string) {
	if len(data) > frontmatterPeekBytes {
		data = data[:frontmatterPeekBytes]
	}
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
				break
			}
			k, v := splitFrontmatterLine(line)
			switch k {
			case "title":
				title = unquoteMeta(v)
			case "description":
				description = unquoteMeta(v)
			case "tags":
				tags = parseTagList(v)
			}
			continue
		}
		if title == "" && strings.HasPrefix(strings.TrimSpace(line), "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		}
		break
	}
	if title == "" {
		for i := 0; i < 30 && scanner.Scan(); i++ {
			l := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(l, "# ") {
				title = strings.TrimSpace(strings.TrimPrefix(l, "#"))
				break
			}
		}
	}
	return title, description, tags
}

func splitFrontmatterLine(line string) (key, val string) {
	i := strings.Index(line, ":")
	if i <= 0 {
		return "", ""
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
}

func unquoteMeta(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
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
		if t := unquoteMeta(strings.TrimSpace(p)); t != "" {
			out = append(out, t)
		}
	}
	return out
}
