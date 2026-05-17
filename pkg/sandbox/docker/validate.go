package docker

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateMount enforces a minimum-trust contract on the strings passed
// to `docker run --mount`. Values arrive from the workflow's own inline
// `sandbox.mounts:` or from a `.devcontainer/devcontainer.json` parsed
// in `sandbox: auto` mode; the latter is NOT necessarily author-trusted
// (the working tree at run time may be a checked-out PR or a third-
// party repo).
//
// Three classes of rejection:
//
//  1. Control characters in the spec, which can split the spec into
//     multiple `--mount` args or break argv on some docker variants.
//  2. Specs that begin with `-` — a flag-injection defence-in-depth
//     (docker won't reinterpret a `--mount` value as a flag today, but
//     a future code path that concatenates differently would).
//  3. Bind mounts of well-known sensitive host paths (docker socket,
//     /proc, /sys, host config dirs like ~/.aws, ~/.ssh, ~/.gnupg) and
//     of /etc files that reveal credentials (/etc/shadow, /etc/sudoers).
//     A sandbox that bind-mounts /var/run/docker.sock is no sandbox.
//
// The function does NOT attempt to fully parse docker's mount syntax —
// that surface is too rich (CSV with quoting, multiple types, volume
// drivers). Containment is enforced by spotting the dangerous source=
// values in the spec; legitimate workflows pass mounts that only ever
// touch the workspace tree or well-known cache dirs.
func validateMount(spec string) error {
	if spec == "" {
		return fmt.Errorf("empty mount spec")
	}
	if strings.ContainsAny(spec, "\n\r\x00") {
		return fmt.Errorf("mount spec contains control character: %q", spec)
	}
	if strings.HasPrefix(spec, "-") {
		return fmt.Errorf("mount spec must not begin with '-' (flag injection guard): %q", spec)
	}
	src, mtype := parseMountSource(spec)
	if mtype == "bind" && src != "" {
		clean, err := filepath.Abs(src)
		if err != nil {
			return fmt.Errorf("mount source %q is not a valid path: %w", src, err)
		}
		clean = filepath.Clean(clean)
		if isSensitiveHostPath(clean) {
			return fmt.Errorf("mount source %q is on the blocked list (docker.sock, /proc, /sys, host credentials)", clean)
		}
	}
	return nil
}

// parseMountSource splits a comma-separated `--mount` value into its
// `source=` / `src=` and `type=` components. Returns ("", "") when the
// fields aren't present (e.g. legacy `-v` style which we don't accept
// here, or `type=tmpfs` with no source). The parser is intentionally
// lenient — docker rejects malformed specs at runtime; the goal here is
// only to spot the dangerous bind-source forms.
func parseMountSource(spec string) (source, mtype string) {
	for _, kv := range strings.Split(spec, ",") {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(kv[:eq])
		v := strings.TrimSpace(kv[eq+1:])
		switch k {
		case "source", "src":
			source = v
		case "type":
			mtype = v
		}
	}
	return source, mtype
}

// sensitiveHostPathPrefixes lists host paths we never accept as a bind
// source. Exact-match or prefix-match: a request for `/proc/self` is as
// bad as `/proc`. Suffix matters for ~/.aws (we want to block ~/.aws
// AND ~/.aws/credentials).
var sensitiveHostPathPrefixes = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/var/run/podman/podman.sock",
	"/run/podman/podman.sock",
	"/proc",
	"/sys",
	"/etc/shadow",
	"/etc/gshadow",
	"/etc/sudoers",
	"/etc/sudoers.d",
	"/root",
	"/boot",
}

// sensitiveUserDotDirs are dot-directories under any user's home that
// commonly hold credentials. Matched against the trailing component of
// the cleaned absolute path (anything ending in `/.aws`, `/.ssh`,
// etc.). This catches both `/home/jo/.aws` and `/Users/jo/.aws` without
// hard-coding the home root.
var sensitiveUserDotDirs = []string{
	".aws",
	".ssh",
	".gnupg",
	".docker",
	".kube",
	".gcloud",
	".config/gcloud",
	".azure",
	".npmrc",
	".pypirc",
}

func isSensitiveHostPath(clean string) bool {
	for _, p := range sensitiveHostPathPrefixes {
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	for _, d := range sensitiveUserDotDirs {
		// Match `/<anything>/<dot-dir>` or `/<anything>/<dot-dir>/...`.
		if strings.HasSuffix(clean, "/"+d) || strings.Contains(clean, "/"+d+"/") {
			return true
		}
	}
	return false
}

// validateContainsWorkspace checks that `target` resolves under
// `workspace` after cleaning. Used by Build to refuse a Dockerfile or
// build-context path that escapes the run's working tree — without
// this guard, a workflow (or an attacker-controlled devcontainer.json)
// can set `build.context: "../../.."` and BuildKit will COPY/ADD host
// files into the resulting image, where they persist in `docker
// history` and the local layer cache.
func validateContainsWorkspace(workspace, target, label string) error {
	absWS, err := filepath.Abs(workspace)
	if err != nil {
		return fmt.Errorf("workspace path: %w", err)
	}
	absT, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("%s path: %w", label, err)
	}
	absWS = filepath.Clean(absWS)
	absT = filepath.Clean(absT)
	rel, err := filepath.Rel(absWS, absT)
	if err != nil {
		return fmt.Errorf("%s path %q is not relative to workspace %q: %w", label, absT, absWS, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s path %q escapes workspace %q", label, absT, absWS)
	}
	return nil
}

// validatePlainArg rejects argv-style values that would corrupt
// `docker run` parsing. Used for User, Image, WorkspaceFolder — fields
// that arrive as plain values (not key=value pairs) and are appended
// directly after a flag like `--user`. docker treats them as values so
// flag injection is mostly theoretical, but newlines/NULs still split
// argv on some runtimes; and a leading `-` would be misread by a future
// helper that concatenates differently.
func validatePlainArg(label, v string) error {
	if v == "" {
		return nil
	}
	if strings.ContainsAny(v, "\n\r\x00") {
		return fmt.Errorf("%s contains control character: %q", label, v)
	}
	if strings.HasPrefix(v, "-") {
		return fmt.Errorf("%s must not begin with '-' (flag injection guard): %q", label, v)
	}
	return nil
}
