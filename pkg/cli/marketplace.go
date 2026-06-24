package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/SocialGouv/iterion/pkg/botinstall"
	"github.com/SocialGouv/iterion/pkg/botregistry"
	"github.com/SocialGouv/iterion/pkg/marketplace"
)

// MarketplaceListOptions configures `iterion marketplace list`.
type MarketplaceListOptions struct {
	StoreDir string
	Text     string
	Tag      string
}

// MarketplaceSubmitOptions configures `iterion marketplace submit`.
type MarketplaceSubmitOptions struct {
	StoreDir string
	Source   string // git URL (optionally url#ref) or local path
	Ref      string
	Path     string
	Tags     []string
}

// MarketplaceInstallOptions configures `iterion marketplace install`.
type MarketplaceInstallOptions struct {
	StoreDir string
	Slug     string
	Workdir  string
	Force    bool
}

// marketplaceStoreDir resolves the on-disk marketplace store directory
// from a --store-dir value (default ".iterion"), mirroring the studio's
// <store-dir>/marketplace layout so the CLI and the studio share one
// registry.
func marketplaceStoreDir(storeDir string) string {
	if storeDir == "" {
		storeDir = ".iterion"
	}
	return filepath.Join(storeDir, "marketplace")
}

func openMarketplaceStore(storeDir string) (*marketplace.JSONStore, error) {
	return marketplace.NewJSONStore(marketplaceStoreDir(storeDir))
}

// MarketplaceList returns the registry entries matching the filters.
func MarketplaceList(ctx context.Context, opts MarketplaceListOptions) ([]marketplace.Entry, error) {
	store, err := openMarketplaceStore(opts.StoreDir)
	if err != nil {
		return nil, err
	}
	return store.List(ctx, marketplace.Query{Text: opts.Text, Tag: opts.Tag})
}

// MarketplaceSubmit validates the source bundle (no install) and indexes
// it in the local registry, returning the persisted entry. Mirrors the
// server's POST /api/v1/marketplace/submit business logic.
func MarketplaceSubmit(ctx context.Context, opts MarketplaceSubmitOptions) (*marketplace.Entry, error) {
	if strings.TrimSpace(opts.Source) == "" {
		return nil, fmt.Errorf("a git URL or local path is required")
	}
	store, err := openMarketplaceStore(opts.StoreDir)
	if err != nil {
		return nil, err
	}
	md, err := botinstall.Inspect(ctx, botinstall.Options{Source: opts.Source, Ref: opts.Ref, Path: opts.Path})
	if err != nil {
		return nil, fmt.Errorf("inspect: %w", err)
	}
	slug := botregistry.NormalizeName(md.Name)
	if slug == "" {
		return nil, fmt.Errorf("bundle has no name")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	entry := marketplace.Entry{
		Slug:        slug,
		Name:        md.Name,
		DisplayName: md.DisplayName,
		Description: md.Description,
		Author:      md.Author,
		Tags:        normalizeMarketplaceTags(opts.Tags),
		RepoURL:     opts.Source,
		Ref:         opts.Ref,
		Subpath:     opts.Path,
		Version:     md.Version,
		README:      md.README,
		Presets:     toMarketplacePresets(md.Presets),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.Upsert(ctx, entry); err != nil {
		return nil, fmt.Errorf("upsert: %w", err)
	}
	stored, ok, err := store.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &entry, nil
	}
	return stored, nil
}

// MarketplaceInstall resolves the slug and installs the entry's bundle
// into the workspace, bumping the install counter. Returns the install
// result and the refreshed entry.
func MarketplaceInstall(ctx context.Context, opts MarketplaceInstallOptions) (*botinstall.Result, *marketplace.Entry, error) {
	store, err := openMarketplaceStore(opts.StoreDir)
	if err != nil {
		return nil, nil, err
	}
	entry, ok, err := store.Get(ctx, opts.Slug)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, fmt.Errorf("marketplace entry %q not found", opts.Slug)
	}
	res, err := botinstall.Install(ctx, botinstall.Options{
		Source:  entry.RepoURL,
		Ref:     entry.Ref,
		Path:    entry.Subpath,
		Force:   opts.Force,
		Workdir: opts.Workdir,
	})
	if err != nil {
		return nil, nil, err
	}
	// Best-effort counter bump — the install already succeeded.
	_ = store.IncrementInstalls(ctx, opts.Slug)
	refreshed, _, _ := store.Get(ctx, opts.Slug)
	if refreshed == nil {
		refreshed = entry
	}
	return res, refreshed, nil
}

// normalizeMarketplaceTags strips blanks and de-dups (mirrors the
// server's normalizeTags).
func normalizeMarketplaceTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toMarketplacePresets converts botinstall.PresetMeta into the registry
// EntryPreset shape (mirrors the server's toEntryPresets).
func toMarketplacePresets(in []botinstall.PresetMeta) []marketplace.EntryPreset {
	if len(in) == 0 {
		return nil
	}
	out := make([]marketplace.EntryPreset, len(in))
	for i, p := range in {
		out[i] = marketplace.EntryPreset{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Description: p.Description,
			Skills:      append([]string(nil), p.Skills...),
		}
	}
	return out
}
