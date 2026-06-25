package devcontainer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// expandLocalVars resolves the host-side devcontainer.json variable
// substitutions per https://containers.dev/implementors/json_reference/.
// We expand ONLY the variables whose values are knowable on the host
// before docker run (`${localEnv:NAME}`, `${localEnv:NAME:default}`,
// `${localWorkspaceFolder}`, `${localWorkspaceFolderBasename}`).
// Container-side variables (`${containerEnv:...}`,
// `${containerWorkspaceFolder*}`) are left untouched — those are
// resolved at runtime by lifecycle commands inside the container.
//
// The runtime invokes this on every host-bound string before passing
// it to docker (mounts, image, runArgs, postCreate, ...). Without it,
// any devcontainer.json that mounts `${localEnv:HOME}/.claude` (which
// is the canonical Claude Code dev pattern) blows up at `docker run`
// with "invalid mount path: '${localEnv:HOME}/...' must be absolute".
// ExpandLocalVars resolves `${localEnv:VAR}`, `${localEnv:VAR:default}`,
// `${localWorkspaceFolder}`, and `${localWorkspaceFolderBasename}` in
// the input string. Same semantics as ExpandLocalVarsInFile but on a
// raw string — used by the runtime to expand inline `sandbox:` blocks
// in .bot files (Mounts, PostCreate, etc.) just like devcontainer.json.
//
// Container-side variables (`${containerEnv:...}`,
// `${containerWorkspaceFolder*}`) are left untouched; they are
// resolved at runtime inside the container by lifecycle commands.
func ExpandLocalVars(s, repoRoot string) string {
	return expandLocalVars(s, repoRoot)
}

func expandLocalVars(s, repoRoot string) string {
	if s == "" {
		return s
	}
	// ${localWorkspaceFolder*} are positional, no inner colon.
	if repoRoot != "" {
		s = strings.ReplaceAll(s, "${localWorkspaceFolder}", repoRoot)
		s = strings.ReplaceAll(s, "${localWorkspaceFolderBasename}", filepath.Base(repoRoot))
	}
	return localEnvRegex.ReplaceAllStringFunc(s, func(match string) string {
		// match looks like `${localEnv:NAME}` or `${localEnv:NAME:default}`.
		inner := strings.TrimSuffix(strings.TrimPrefix(match, "${localEnv:"), "}")
		name := inner
		def := ""
		if i := strings.Index(inner, ":"); i >= 0 {
			name, def = inner[:i], inner[i+1:]
		}
		if !localEnvAllowed(name) {
			// An attacker-controlled devcontainer.json could otherwise
			// reference credentials (AWS_SECRET_ACCESS_KEY,
			// ANTHROPIC_API_KEY, SSH_AUTH_SOCK, ...) and exfiltrate the
			// value via Mounts / ContainerEnv / RunArgs. Resolve to the
			// default (or empty) instead, mirroring the "name not set"
			// branch — the substitution mechanism stays functional for
			// the common HOME / XDG_* / locale cases.
			return def
		}
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return def
	})
}

// localEnvAllowed reports whether a host environment variable name can
// be substituted via ${localEnv:NAME}. The list is small and explicit:
// names that legitimately drive devcontainer paths (HOME / XDG_* /
// locale / display) plus a handful of cache-locating vars commonly used
// in published devcontainer.json templates. Credentials, tokens,
// SSH-agent sockets, and provider-specific keys are deliberately not
// here — see F-SB-1 in docs/reviews/codebase-2026-05-17.md for the
// threat model.
//
// Operators who need a different allowlist must extend this in source;
// the explicit list is intentional, not a config knob.
func localEnvAllowed(name string) bool {
	switch name {
	case "HOME",
		"USER", "USERNAME", "LOGNAME",
		"LANG", "LANGUAGE", "TERM", "TZ",
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
		"XDG_STATE_HOME", "XDG_RUNTIME_DIR",
		"PWD", "OLDPWD",
		"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY",
		"PUID", "PGID",
		// Devcontainer-spec implementers commonly let template authors
		// pick a variant via a benign env var; keep this allowed so
		// upstream templates keep working.
		"VARIANT":
		return true
	}
	// LC_* locale variables are a family; allow them all.
	if strings.HasPrefix(name, "LC_") {
		return true
	}
	return false
}

// Variable names per the spec are word characters (letters, digits,
// underscore). Default value (after the second colon) accepts anything
// up to the closing brace, so it can include slashes, dots, etc.
var localEnvRegex = regexp.MustCompile(`\$\{localEnv:[A-Za-z_][A-Za-z0-9_]*(?::[^}]*)?\}`)

// ExpandLocalVarsInFile resolves all host-side devcontainer.json
// variables in fields the host needs to read before `docker run`:
// Mounts, RunArgs, ContainerEnv values, RemoteEnv values, Image,
// WorkspaceFolder, WorkspaceMount, and the PostCreateCommand /
// PostStartCommand snippets.
//
// Container-only fields (`containerEnv:` keys, container-side
// commands referencing `${containerEnv:...}`) are left as-is — the
// shell inside the container resolves them with its own environment.
//
// Mutates f in place. Safe to call repeatedly.
func ExpandLocalVarsInFile(f *File, repoRoot string) {
	if f == nil {
		return
	}
	for i, m := range f.Mounts {
		f.Mounts[i] = expandLocalVars(m, repoRoot)
	}
	for i, a := range f.RunArgs {
		f.RunArgs[i] = expandLocalVars(a, repoRoot)
	}
	for k, v := range f.ContainerEnv {
		f.ContainerEnv[k] = expandLocalVars(v, repoRoot)
	}
	for k, v := range f.RemoteEnv {
		f.RemoteEnv[k] = expandLocalVars(v, repoRoot)
	}
	f.Image = expandLocalVars(f.Image, repoRoot)
	f.WorkspaceFolder = expandLocalVars(f.WorkspaceFolder, repoRoot)
	f.WorkspaceMount = expandLocalVars(f.WorkspaceMount, repoRoot)
	if f.PostCreateCommand.Shell != "" {
		f.PostCreateCommand.Shell = expandLocalVars(f.PostCreateCommand.Shell, repoRoot)
	}
	for i, a := range f.PostCreateCommand.Argv {
		f.PostCreateCommand.Argv[i] = expandLocalVars(a, repoRoot)
	}
	if f.PostStartCommand.Shell != "" {
		f.PostStartCommand.Shell = expandLocalVars(f.PostStartCommand.Shell, repoRoot)
	}
	for i, a := range f.PostStartCommand.Argv {
		f.PostStartCommand.Argv[i] = expandLocalVars(a, repoRoot)
	}
	if f.Build != nil {
		f.Build.Dockerfile = expandLocalVars(f.Build.Dockerfile, repoRoot)
		f.Build.Context = expandLocalVars(f.Build.Context, repoRoot)
		for k, v := range f.Build.Args {
			f.Build.Args[k] = expandLocalVars(v, repoRoot)
		}
	}
}
