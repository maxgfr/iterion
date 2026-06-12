// Package marketplace is the hosted bot registry that sits on top of
// pkg/botinstall. A repository "publishes" a bot by holding a bundle
// directory (main.bot + manifest.yaml) and optionally an
// `iterion-bots.yaml` index — see pkg/botinstall. The marketplace adds
// a curated index over those repos: an author submits a repo URL, the
// server validates it via botinstall.Inspect, persists an Entry, and
// users browse + install by slug. Install resolves the entry's repo URL
// and reuses botinstall.Install — the marketplace is metadata only; the
// actual install path is unchanged from Phase A.
package marketplace

import "context"

// Entry is one bot listing in the marketplace registry.
//
// Timestamps (CreatedAt, UpdatedAt) are RFC3339 strings supplied by the
// caller (Server handlers) rather than minted inside the store. This
// keeps the store deterministic (essential for testing) and is the same
// convention used elsewhere in the codebase that needs reproducible
// time stamps.
type Entry struct {
	// Slug is the registry-unique id, derived from the manifest name
	// via botregistry.NormalizeName. Used as the URL segment in
	// `/api/v1/marketplace/bots/{slug}` and as the Mongo _id.
	Slug string `json:"slug" bson:"_id"`

	// Name is the bundle's technical id (manifest.name).
	Name string `json:"name" bson:"name"`

	// DisplayName is the operator-facing label (manifest.display_name).
	DisplayName string `json:"display_name,omitempty" bson:"display_name,omitempty"`

	// Description is the one-line summary shown on the card.
	Description string `json:"description,omitempty" bson:"description,omitempty"`

	// Author is the free-form attribution (manifest.author).
	Author string `json:"author,omitempty" bson:"author,omitempty"`

	// Tags are operator-supplied at submit time (e.g. "review",
	// "kanban") — distinct from manifest.triggers, which the engine
	// uses to match a bot to a ticket. Tags drive marketplace browse
	// filtering and have no engine semantics.
	Tags []string `json:"tags,omitempty" bson:"tags,omitempty"`

	// RepoURL, Ref and Subpath are the install coordinates persisted
	// from the submit form. The Install endpoint forwards them to
	// botinstall.Install verbatim.
	RepoURL string `json:"repo_url" bson:"repo_url"`
	Ref     string `json:"ref,omitempty" bson:"ref,omitempty"`
	Subpath string `json:"subpath,omitempty" bson:"subpath,omitempty"`

	// Version mirrors manifest.version at index time. The registry is
	// a snapshot — operators re-submit to refresh.
	Version string `json:"version,omitempty" bson:"version,omitempty"`

	// README is the bundle's README.md content (capped — see
	// botinstall.Inspect). Shown in the detail panel.
	README string `json:"readme,omitempty" bson:"readme,omitempty"`

	// Presets mirrors the bundle's file-based presets metadata. Only
	// the registry-relevant slice is kept (no prompt body / vars map);
	// callers that need the full preset bias install the bot.
	Presets []EntryPreset `json:"presets,omitempty" bson:"presets,omitempty"`

	// Installs is the rough install count, bumped each time the
	// install endpoint succeeds. Approximate (no per-user dedup) —
	// good enough to surface "popular" bots.
	Installs int `json:"installs" bson:"installs"`

	// CreatedAt / UpdatedAt are RFC3339 timestamps stamped by the
	// caller.
	CreatedAt string `json:"created_at,omitempty" bson:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty" bson:"updated_at,omitempty"`
}

// EntryPreset is the slim, registry-facing slice of bundle.PresetSpec.
// Vars + prompt body are intentionally omitted — they're only useful at
// run time after install.
type EntryPreset struct {
	Name        string   `json:"name" bson:"name"`
	DisplayName string   `json:"display_name,omitempty" bson:"display_name,omitempty"`
	Description string   `json:"description,omitempty" bson:"description,omitempty"`
	Skills      []string `json:"skills,omitempty" bson:"skills,omitempty"`
}

// Query filters a List call. Zero-value fields don't filter.
type Query struct {
	// Text is a case-insensitive substring matched against Slug,
	// Name, DisplayName, Description, Author and Tags.
	Text string
	// Tag, when set, requires an exact (case-insensitive) match in
	// the entry's Tags.
	Tag string
}

// Store is the marketplace persistence interface. Two implementations
// land alongside this file: a JSON-file store for self-host / local
// mode (jsonstore.go) and a Mongo store for cloud mode (mongostore.go).
type Store interface {
	List(ctx context.Context, q Query) ([]Entry, error)
	Get(ctx context.Context, slug string) (*Entry, bool, error)
	Upsert(ctx context.Context, e Entry) error
	IncrementInstalls(ctx context.Context, slug string) error
}
