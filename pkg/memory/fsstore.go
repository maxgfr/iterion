package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SocialGouv/iterion/pkg/knowledge"
	"github.com/SocialGouv/iterion/pkg/store"
)

// FSStore is the local-filesystem implementation of
// knowledge.MemoryStore. It hosts spaces under the global iterion data
// dir and reuses the path-clamped Scope primitive for all document IO,
// so the security guarantees (no "../" or absolute escapes) are shared
// with the legacy scope API.
type FSStore struct {
	root string // base data dir; empty = resolve store.GlobalIterionDataDir() lazily
}

var _ knowledge.MemoryStore = (*FSStore)(nil)

// NewFSStore returns an FSStore rooted at the given base data dir.
func NewFSStore(root string) *FSStore { return &FSStore{root: root} }

// DefaultFSStore returns an FSStore rooted at the global iterion data
// dir, resolved lazily on each use so a test's ITERION_HOME override
// (or any runtime change) is honoured.
func DefaultFSStore() *FSStore { return &FSStore{} }

func (s *FSStore) baseDir() string {
	if s.root != "" {
		return s.root
	}
	return store.GlobalIterionDataDir()
}

// LegacyBotRef builds the SpaceRef a legacy `memory: scope:` block
// resolves to: a bot-visibility space keyed by the encoded project key
// of memBase (the run workDir, or the repo root when project_root is
// set). The resolved on-disk path is identical to the pre-knowledge
// WorkspaceMemoryDir(memBase)/<scope> layout, so existing bots see the
// same files.
func LegacyBotRef(memBase, scope string) knowledge.SpaceRef {
	key := ""
	if memBase != "" {
		abs := memBase
		if !filepath.IsAbs(abs) {
			if resolved, err := filepath.Abs(memBase); err == nil {
				abs = resolved
			}
		}
		key = store.EncodeWorkDirKey(abs)
	}
	return knowledge.SpaceRef{
		Visibility: knowledge.VisibilityBot,
		ProjectID:  key,
		Name:       scope,
	}
}

// scopeFor resolves a SpaceRef to a path-clamped Scope on disk. Only
// the bot/project visibilities are hosted by the FS adapter today; the
// tenant-shared visibilities (cross_project/org/user/global) land with
// the shared-tree layout in a later phase and return
// ErrUnsupportedVisibility until then.
func (s *FSStore) scopeFor(ref knowledge.SpaceRef) (*Scope, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	switch ref.Visibility {
	case knowledge.VisibilityBot, knowledge.VisibilityProject:
		if ref.ProjectID == "" {
			return nil, fmt.Errorf("memory: empty project for space %q", ref.Name)
		}
		root := filepath.Join(s.baseDir(), "projects", ref.ProjectID, "memory", ref.Name)
		return &Scope{root: root}, nil
	default:
		return nil, fmt.Errorf("%w: %q", knowledge.ErrUnsupportedVisibility, ref.Visibility)
	}
}

// Root returns the space's absolute on-disk path.
func (s *FSStore) Root(ref knowledge.SpaceRef) (string, error) {
	sc, err := s.scopeFor(ref)
	if err != nil {
		return "", err
	}
	return sc.Root(), nil
}

// BuildIndex returns one IndexEntry per Markdown document in the space.
func (s *FSStore) BuildIndex(_ context.Context, ref knowledge.SpaceRef) ([]knowledge.IndexEntry, error) {
	sc, err := s.scopeFor(ref)
	if err != nil {
		return nil, err
	}
	entries, err := sc.BuildIndex()
	if err != nil {
		return nil, err
	}
	out := make([]knowledge.IndexEntry, len(entries))
	for i, e := range entries {
		out[i] = knowledge.IndexEntry{Path: e.Path, Title: e.Title, Description: e.Description, Tags: e.Tags}
	}
	return out, nil
}

// Autoload returns the full content of documents matching the patterns.
func (s *FSStore) Autoload(_ context.Context, ref knowledge.SpaceRef, patterns []string) ([]knowledge.AutoloadEntry, error) {
	sc, err := s.scopeFor(ref)
	if err != nil {
		return nil, err
	}
	entries, err := sc.Autoload(patterns)
	if err != nil {
		return nil, err
	}
	out := make([]knowledge.AutoloadEntry, len(entries))
	for i, e := range entries {
		out[i] = knowledge.AutoloadEntry{Path: e.Path, Content: e.Content}
	}
	return out, nil
}

// ListDocuments enumerates files directly under the space-relative dir.
func (s *FSStore) ListDocuments(_ context.Context, ref knowledge.SpaceRef, dir string) ([]knowledge.DocumentMeta, error) {
	sc, err := s.scopeFor(ref)
	if err != nil {
		return nil, err
	}
	files, err := sc.List(dir)
	if err != nil {
		return nil, err
	}
	out := make([]knowledge.DocumentMeta, 0, len(files))
	for _, f := range files {
		out = append(out, knowledge.DocumentMeta{Path: f})
	}
	return out, nil
}

// ReadDocument returns a document's metadata + content. A missing
// document returns an error satisfying errors.Is(err, ErrDocNotFound).
func (s *FSStore) ReadDocument(_ context.Context, ref knowledge.SpaceRef, path string) (knowledge.Document, error) {
	sc, err := s.scopeFor(ref)
	if err != nil {
		return knowledge.Document{}, err
	}
	data, err := sc.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return knowledge.Document{}, fmt.Errorf("%w: %q", knowledge.ErrDocNotFound, path)
		}
		return knowledge.Document{}, err
	}
	return knowledge.Document{
		Meta: knowledge.DocumentMeta{
			Path:     path,
			Size:     int64(len(data)),
			Checksum: checksum(data),
		},
		Content: data,
	}, nil
}

// WriteDocument creates or replaces a document and returns its metadata.
func (s *FSStore) WriteDocument(_ context.Context, ref knowledge.SpaceRef, in knowledge.DocumentInput) (knowledge.DocumentMeta, error) {
	sc, err := s.scopeFor(ref)
	if err != nil {
		return knowledge.DocumentMeta{}, err
	}
	if err := sc.Write(in.Path, in.Content); err != nil {
		return knowledge.DocumentMeta{}, err
	}
	return knowledge.DocumentMeta{
		Path:      in.Path,
		Size:      int64(len(in.Content)),
		Checksum:  checksum(in.Content),
		UpdatedBy: in.UpdatedBy,
		UpdatedAt: time.Now(),
	}, nil
}

// DeleteDocument removes a document; deleting an absent doc is a no-op.
func (s *FSStore) DeleteDocument(_ context.Context, ref knowledge.SpaceRef, path string) error {
	sc, err := s.scopeFor(ref)
	if err != nil {
		return err
	}
	abs, err := sc.Resolve(path)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func checksum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
