// Package bundle implements the `.botz` archive format: a tar.gz that
// packages an iterion workflow (`main.bot`) with adjacent resources
// (skills, prompts, presets, default attachments, manifest). A bundle is loaded
// once per run, extracted into a content-addressed cache directory, and
// then exposed to the engine as a *Bundle so skills/prompts become
// visible to claude_code and the claw tool registry without authoring
// changes.
package bundle

// Kind discriminates how a workflow path was supplied.
type Kind int

const (
	// KindBot is a plain `.bot` source file.
	KindBot Kind = iota
	// KindBundle is a `.botz` tar.gz archive.
	KindBundle
	// KindBundleDir is a directory whose root already contains a
	// recognised bundle layout (`main.bot` at the top).
	// Useful for dev workflows that author bundles in-place.
	KindBundleDir
)

func (k Kind) String() string {
	switch k {
	case KindBot:
		return "bot"
	case KindBundle:
		return "bundle"
	case KindBundleDir:
		return "bundle-dir"
	}
	return "unknown"
}

// Bundle is a resolved, on-disk bundle ready for runtime consumption.
// All path fields are absolute; optional resource directories are the
// empty string when not present in the bundle.
type Bundle struct {
	// Dir is the absolute path of the extracted (or in-place) bundle
	// root. Engine consumers should treat it as read-only.
	Dir string

	// Manifest holds the parsed `manifest.yaml`. Nil when the bundle
	// omits the file (allowed — the field is optional).
	Manifest *Manifest

	// IterPath is the absolute path of the workflow source file
	// inside the bundle (`main.bot`, at the bundle root).
	IterPath string

	// SkillsDir is `<Dir>/skills` when the directory exists, else "".
	SkillsDir string

	// PromptsDir is `<Dir>/prompts` when the directory exists, else "".
	PromptsDir string

	// AttachmentsDir is `<Dir>/attachments` when the directory exists,
	// else "". Holds pre-bundled default values for the workflow's
	// `attachments:` block — runtime uploads (Launch modal) override.
	AttachmentsDir string

	// PresetsDir is `<Dir>/presets` when the directory exists, else "".
	// Holds file-based presets (`<name>.md`, YAML frontmatter + prompt
	// body) — named sous-bots that bias the workflow at launch. Parsed
	// by LoadPresets; merged into the runtime workflow's preset set by
	// the engine at run start.
	PresetsDir string

	// Hash is the SHA-256 of the uncompressed tar stream, used as
	// the cache key. Empty for KindBundleDir bundles (no archive
	// to hash; callers handle directory bundles per-run).
	Hash string

	// SourcePath is the original `.botz` filesystem path for KindBundle,
	// or the source directory for KindBundleDir. Persisted with the run
	// so resume can re-extract from the same archive after a cache GC.
	SourcePath string

	// Kind discriminates how the bundle was supplied.
	Kind Kind
}
