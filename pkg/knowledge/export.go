package knowledge

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// ExportFormat identifies the memory archive format.
const ExportFormat = "iterion.memory.v1"

// ExportManifest is the archive's manifest.json.
type ExportManifest struct {
	Format    string         `json:"format"`
	Space     SpaceRef       `json:"space"`
	Documents []DocumentMeta `json:"documents"`
	DocCount  int            `json:"doc_count"`
}

// ImportStrategy controls how an import treats a doc that already exists.
type ImportStrategy string

const (
	ImportSkip      ImportStrategy = "skip"      // default: never overwrite
	ImportOverwrite ImportStrategy = "overwrite" // replace (new revision)
	ImportRename    ImportStrategy = "rename"    // write under "<base>.import.<ext>"
)

// ImportSummary reports what an import did.
type ImportSummary struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Renamed  int `json:"renamed"`
}

// ErrSecretInExport is returned by ExportSpace when a document body
// contains a literal credential shape. Exports must never leak secret
// plaintext — the operator cleans the space and re-exports.
type ErrSecretInExport struct {
	Path   string
	Reason string
}

func (e *ErrSecretInExport) Error() string {
	return fmt.Sprintf("knowledge: refusing to export %q: %s", e.Path, e.Reason)
}

// ExportSpace writes a gzip+tar archive of every markdown document in a
// space (manifest.json first, then docs/<path>, then checksums.sha256).
// It aborts with *ErrSecretInExport if any body contains a literal
// credential shape; symbolic __ITERION_SECRET_*__ placeholders pass.
func ExportSpace(ctx context.Context, store MemoryStore, ref SpaceRef, w io.Writer) (ExportManifest, error) {
	index, err := store.BuildIndex(ctx, ref)
	if err != nil {
		return ExportManifest{}, err
	}
	sort.Slice(index, func(i, j int) bool { return index[i].Path < index[j].Path })

	type entry struct {
		meta DocumentMeta
		body []byte
	}
	var docs []entry
	manifest := ExportManifest{Format: ExportFormat, Space: ref}
	for _, e := range index {
		doc, err := store.ReadDocument(ctx, ref, e.Path)
		if err != nil {
			return ExportManifest{}, fmt.Errorf("export read %q: %w", e.Path, err)
		}
		if reason := scanForSecret(doc.Content); reason != "" {
			return ExportManifest{}, &ErrSecretInExport{Path: e.Path, Reason: reason}
		}
		docs = append(docs, entry{meta: doc.Meta, body: doc.Content})
		manifest.Documents = append(manifest.Documents, doc.Meta)
	}
	manifest.DocCount = len(docs)

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	if err := writeTarFile(tw, "manifest.json", manifestBytes); err != nil {
		return ExportManifest{}, err
	}
	var checksums strings.Builder
	for _, d := range docs {
		if err := writeTarFile(tw, "docs/"+d.meta.Path, d.body); err != nil {
			return ExportManifest{}, err
		}
		fmt.Fprintf(&checksums, "%s  docs/%s\n", ChecksumHex(d.body), d.meta.Path)
	}
	if err := writeTarFile(tw, "checksums.sha256", []byte(checksums.String())); err != nil {
		return ExportManifest{}, err
	}
	if err := tw.Close(); err != nil {
		return ExportManifest{}, err
	}
	if err := gz.Close(); err != nil {
		return ExportManifest{}, err
	}
	return manifest, nil
}

// ImportSpace reads an archive produced by ExportSpace and writes its
// documents into ref. Document paths are written through the store's
// path-clamped WriteDocument, so a malicious "../" entry is rejected by
// the adapter. Bodies containing literal secret shapes are rejected.
func ImportSpace(ctx context.Context, store MemoryStore, ref SpaceRef, r io.Reader, strategy ImportStrategy) (ImportSummary, error) {
	if strategy == "" {
		strategy = ImportSkip
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return ImportSummary{}, fmt.Errorf("import: gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var sum ImportSummary
	// Match the per-document write cap so an oversized entry fails with a
	// clear size error here rather than a confusing late QuotaError from
	// WriteDocument deeper in the import.
	const maxEntry = DefaultMaxDocumentSize
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return sum, fmt.Errorf("import: tar: %w", err)
		}
		name := path.Clean(hdr.Name)
		if !strings.HasPrefix(name, "docs/") {
			continue // manifest.json + checksums are advisory
		}
		rel := strings.TrimPrefix(name, "docs/")
		if rel == "" || strings.HasPrefix(rel, "..") || path.IsAbs(rel) {
			return sum, fmt.Errorf("import: unsafe path %q", hdr.Name)
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxEntry+1))
		if err != nil {
			return sum, fmt.Errorf("import read %q: %w", rel, err)
		}
		if int64(len(body)) > maxEntry {
			return sum, fmt.Errorf("import: %q exceeds %d bytes", rel, maxEntry)
		}
		if reason := scanForSecret(body); reason != "" {
			return sum, &ErrSecretInExport{Path: rel, Reason: "import source " + reason}
		}

		dst := rel
		if _, err := store.ReadDocument(ctx, ref, rel); err == nil {
			switch strategy {
			case ImportSkip:
				sum.Skipped++
				continue
			case ImportRename:
				ext := path.Ext(rel)
				dst = strings.TrimSuffix(rel, ext) + ".import" + ext
				sum.Renamed++
			}
		}
		if _, err := store.WriteDocument(ctx, ref, DocumentInput{Path: dst, Content: body, UpdatedBy: "import"}); err != nil {
			return sum, fmt.Errorf("import write %q: %w", dst, err)
		}
		sum.Imported++
	}
	return sum, nil
}

func writeTarFile(tw *tar.Writer, name string, body []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err := tw.Write(body)
	return err
}

// secretPrefixes are literal credential markers we refuse to export.
// Symbolic iterion placeholders are deliberately NOT here (safe to
// round-trip).
var secretPrefixes = []string{"sk-", "xoxb-", "xoxp-", "ghp_", "gho_", "github_pat_", "glpat-", "AKIA", "ASIA"}

// scanForSecret returns a non-empty reason when content contains a
// literal credential shape, else "".
func scanForSecret(content []byte) string {
	s := string(content)
	for _, p := range secretPrefixes {
		if strings.Contains(s, p) {
			return "contains a literal credential token (" + p + "…)"
		}
	}
	return ""
}
