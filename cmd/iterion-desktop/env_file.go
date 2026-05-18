//go:build desktop

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// dotenvAppliedKeys tracks which keys were set into the process env by a
// previous applyDotenvFile call. Used by reloadIterionEnvFile so that
// commenting out a key in ~/.iterion/env and clicking Refresh actually
// clears the value (default os.Setenv-then-skip-if-present semantics
// would otherwise pin the original value for the life of the process).
//
// Keys passed in by the launching shell are NOT tracked here, so they
// survive a reload — shell wins remains the precedence rule.
var (
	dotenvMu          sync.Mutex
	dotenvAppliedKeys []string
)

// loadIterionEnvFile sources ~/.iterion/env (or ~/.iterion/.env as a
// fallback) into the current process's env BEFORE the editor server
// starts. Operators stash provider credentials there (OPENAI_API_KEY,
// AZURE_OPENAI_API_KEY, etc.) so iterion-desktop launched from a
// .desktop file (no shell to source ~/.bashrc) still has the keys
// available — and ClawBackend.executeViaSandboxRunner can then forward
// them into the sandbox runner.
//
// Existing env vars are NOT overwritten: a value already set on the
// process by the launching shell wins over the file. This matches
// dotenv-style precedence (.env is a default, not an override) and
// keeps `OPENAI_API_KEY=sk-... iterion-desktop` working as a one-shot
// override without editing the file.
//
// Failures are silent — the file is optional. Lookup order:
//
//  1. $ITERION_ENV_FILE (explicit override path)
//  2. $ITERION_HOME/env (when $ITERION_HOME is set)
//  3. ~/.iterion/env
//  4. ~/.iterion/.env (defensive: dotenv-style filename)
func loadIterionEnvFile() {
	for _, path := range candidateEnvFiles() {
		if path == "" {
			continue
		}
		if applyDotenvFile(path) {
			return
		}
	}
}

// ReloadIterionEnvFile is called by the refresh hook in the server: it
// unsets every key that a previous applyDotenvFile call set (so a key
// commented out / deleted in ~/.iterion/env actually disappears) and
// then re-applies the file. Shell-passed env vars are untouched.
func ReloadIterionEnvFile() {
	dotenvMu.Lock()
	previous := dotenvAppliedKeys
	dotenvAppliedKeys = nil
	dotenvMu.Unlock()
	for _, key := range previous {
		_ = os.Unsetenv(key)
	}
	loadIterionEnvFile()
}

func candidateEnvFiles() []string {
	var paths []string
	if explicit := strings.TrimSpace(os.Getenv("ITERION_ENV_FILE")); explicit != "" {
		paths = append(paths, explicit)
	}
	if home := strings.TrimRight(os.Getenv("ITERION_HOME"), string(filepath.Separator)); home != "" {
		paths = append(paths, filepath.Join(home, "env"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths,
			filepath.Join(home, ".iterion", "env"),
			filepath.Join(home, ".iterion", ".env"),
		)
	}
	return paths
}

// applyDotenvFile reads a KEY=value file and sets each key on the
// process env unless it's already set. Returns true when the file
// existed and was processed (even if empty or all-comments) so callers
// know to stop probing fallbacks. Comments and blank lines are
// ignored; values may be optionally double- or single-quoted, in which
// case the matching outer quotes are stripped.
func applyDotenvFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip an optional leading "export " so dotenv files that
		// double as bash-source scripts work too.
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Trim balanced surrounding quotes.
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		// Honour an existing process env value (shell-passed vars
		// override the dotenv default).
		if _, present := os.LookupEnv(key); present {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			continue
		}
		dotenvMu.Lock()
		dotenvAppliedKeys = append(dotenvAppliedKeys, key)
		dotenvMu.Unlock()
	}
	return true
}
