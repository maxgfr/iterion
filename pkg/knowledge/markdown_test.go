package knowledge

import (
	"reflect"
	"testing"
)

func TestParseMarkdownMeta(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		title, desc string
		tags        []string
	}{
		{
			"frontmatter",
			"---\ntitle: Brief\ndescription: a note\ntags: [a, b]\n---\nbody",
			"Brief", "a note", []string{"a", "b"},
		},
		{"h1 fallback", "# Heading\n\nbody", "Heading", "", nil},
		{"quoted title", "---\ntitle: \"Quoted X\"\n---\n", "Quoted X", "", nil},
		{"empty", "", "", "", nil},
		{"plain body no title", "just text\nmore", "", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, desc, tags := ParseMarkdownMeta([]byte(c.in))
			if title != c.title || desc != c.desc || !reflect.DeepEqual(tags, c.tags) {
				t.Fatalf("got (%q,%q,%v) want (%q,%q,%v)", title, desc, tags, c.title, c.desc, c.tags)
			}
		})
	}
}

func TestChecksumHex(t *testing.T) {
	a := ChecksumHex([]byte("hello"))
	if len(a) != 64 || a != ChecksumHex([]byte("hello")) {
		t.Fatalf("checksum not stable/hex: %q", a)
	}
	if a == ChecksumHex([]byte("world")) {
		t.Fatal("distinct content must differ")
	}
}
