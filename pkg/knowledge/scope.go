package knowledge

import (
	"fmt"
	"strings"
)

// Visibility is the discriminator that says how a memory space is
// scoped and shared. It is the primary sharing axis; the remaining
// SpaceRef fields (TenantID / ProjectID / BotID / UserID) qualify it.
type Visibility string

const (
	// VisibilityPrivate — a single run's scratch space (ephemeral).
	VisibilityPrivate Visibility = "private"
	// VisibilityBot — shared across runs of one bot in one project.
	// This is what a legacy `memory: scope:` block resolves to.
	VisibilityBot Visibility = "bot"
	// VisibilityProject — shared across all bots in one project (the
	// cross-bot "findings/" inbox).
	VisibilityProject Visibility = "project"
	// VisibilityCrossProject — shared across projects in one org.
	VisibilityCrossProject Visibility = "cross_project"
	// VisibilityUser — one user's private notes across all projects.
	VisibilityUser Visibility = "user"
	// VisibilityOrg — org-wide, shared across all bots/runs/projects.
	VisibilityOrg Visibility = "org"
	// VisibilityGlobal — instance-wide catalogue, read-only to orgs.
	VisibilityGlobal Visibility = "global"
)

// knownVisibilities is the closed enum used for validation.
var knownVisibilities = map[Visibility]bool{
	VisibilityPrivate:      true,
	VisibilityBot:          true,
	VisibilityProject:      true,
	VisibilityCrossProject: true,
	VisibilityUser:         true,
	VisibilityOrg:          true,
	VisibilityGlobal:       true,
}

// SpaceRef is the resolver-friendly handle the DSL/runtime produces
// and hands to a MemoryStore. Every axis is optional except
// Visibility and Name; which qualifiers are required depends on the
// visibility (see Validate). ProjectID is the encoded project key
// (store.EncodeWorkDirKey of the repo root), never a raw host path,
// so it is stable and host-agnostic in the cloud document store.
type SpaceRef struct {
	Visibility Visibility
	TenantID   string // org tenancy; required in cloud, empty for local single-tenant
	UserID     string // required when Visibility == VisibilityUser
	ProjectID  string // encoded project key; required for bot/project
	BotID      string // bot name (Workflow.Name); required for bot
	Name       string // single-segment slug ("session-continuity", "findings", ...)
}

// Validate checks the ref is well-formed: a known visibility, a valid
// single-segment Name, and the qualifiers the visibility requires.
// Tenant presence is an adapter concern (the cloud adapter fail-closes
// on an empty tenant), so it is not enforced here.
func (r SpaceRef) Validate() error {
	if !knownVisibilities[r.Visibility] {
		return fmt.Errorf("knowledge: unknown visibility %q", r.Visibility)
	}
	if err := ValidateName(r.Name); err != nil {
		return err
	}
	// Every qualifier becomes a path segment (FS adapter scopeFor) and an
	// ID component (cloud _id), so a present value must be a single safe
	// segment — otherwise an untrusted REST `?project=`/`?user=` could
	// reach scopeFor() as `../../etc` and escape the store tree. Empty is
	// allowed; the switch below enforces presence where required.
	for _, c := range []struct{ field, val string }{
		{"tenant", r.TenantID}, {"user", r.UserID},
		{"project", r.ProjectID}, {"bot", r.BotID},
	} {
		if err := validateSegment(c.field, c.val); err != nil {
			return err
		}
	}
	switch r.Visibility {
	case VisibilityBot:
		if r.ProjectID == "" {
			return fmt.Errorf("knowledge: bot space %q requires a project", r.Name)
		}
		if r.BotID == "" {
			return fmt.Errorf("knowledge: bot space %q requires a bot", r.Name)
		}
	case VisibilityProject:
		if r.ProjectID == "" {
			return fmt.Errorf("knowledge: project space %q requires a project", r.Name)
		}
	case VisibilityUser:
		if r.UserID == "" {
			return fmt.Errorf("knowledge: user space %q requires a user", r.Name)
		}
	}
	return nil
}

// ID returns a deterministic, filesystem- and document-store-safe
// identifier for the space. The cloud adapter uses it as the
// memory_spaces _id; equal refs always produce equal ids.
func (r SpaceRef) ID() string {
	return strings.Join([]string{
		"v1", string(r.Visibility), r.TenantID, r.ProjectID, r.BotID, r.UserID, r.Name,
	}, ":")
}

// validateSegment guards a SpaceRef qualifier (tenant/user/project/bot)
// that becomes a path segment + ID component. Empty is allowed (presence
// is the visibility switch's job); a present value must not contain a
// path separator or be a traversal token.
func validateSegment(field, v string) error {
	if v == "" {
		return nil
	}
	if strings.ContainsAny(v, `/\`) || v == "." || v == ".." {
		return fmt.Errorf("knowledge: %s %q must not contain a path separator or traversal", field, v)
	}
	return nil
}

// ValidateName rejects names that are empty, contain path separators,
// or attempt traversal. A space Name is a single folder segment; the
// sharing spread lives in the SpaceRef fields, not in slashed names —
// this preserves the per-segment path-clamp guarantee.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("knowledge: space name is required")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("knowledge: space name %q must be a single folder segment", name)
	}
	return nil
}

// ErrInvalidDocPath is returned by ValidateDocPath (and the stores that
// call it) for a document path that is absolute, contains a ".."
// segment, or is otherwise unsafe. Callers can map it to a 400.
var ErrInvalidDocPath = fmt.Errorf("knowledge: invalid document path")

// ValidateDocPath clamps a document path to its space. Unlike a space
// Name, a doc path MAY contain "/" (subdirectories, e.g.
// "findings/2026.md"), but it must stay inside the space: no absolute
// paths, no NUL byte, and no ".." segment. The FS adapter clamps via its
// Scope; this is the shared check so the cloud adapter and the REST
// boundary enforce the SAME rule (a "../" path must be rejected
// everywhere, not silently stored as a weird Mongo key).
func ValidateDocPath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: path is required", ErrInvalidDocPath)
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("%w: %q contains a NUL byte", ErrInvalidDocPath, path)
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) ||
		(len(path) >= 2 && path[1] == ':') { // drive-letter / Windows abs
		return fmt.Errorf("%w: %q must be relative", ErrInvalidDocPath, path)
	}
	for _, seg := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
		if seg == ".." {
			return fmt.Errorf("%w: %q escapes the space root", ErrInvalidDocPath, path)
		}
	}
	return nil
}
