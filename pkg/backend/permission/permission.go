// Package permission implements iterion's tool-permission gate — the
// anti-hypnosis / anti-prompt-injection boundary that mirrors Claude
// Code's default "ask" posture for BOTH execution backends
// (claude_code and claw).
//
// The operator declares a Policy (a mode plus allow/ask/deny rule
// lists) in the .bot DSL or via CLI/env. Every tool call an agent
// attempts is evaluated by [Policy.Evaluate] — deterministic code
// OUTSIDE the model's controllable surface — so a prompt-injected or
// "hypnotized" agent that tries an off-policy action (exfiltrate a
// secret, curl an attacker, rm -rf, push to a rogue remote) is denied
// or surfaced to the human instead of silently proceeding.
//
// Rule syntax mirrors Claude Code: a rule is either a bare tool name
// (`Bash`, matches any use) or a scoped pattern (`Bash(go test:*)`,
// `Read(./.env)`, `Edit(pkg/**)`, `WebFetch(domain:example.com)`,
// `mcp__github__get_*`). Because the same Policy drives both backends,
// claude_code (TitleCase tool names) and claw (snake_case) reach
// identical decisions — see [canonicalToolName].
package permission

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Mode is the top-level permission posture, mirroring Claude Code's
// permission modes (the iterion-facing names are off|ask|deny).
type Mode int

const (
	// ModeOff disables the gate entirely — today's bypassPermissions
	// behaviour. The default when nothing is declared.
	ModeOff Mode = iota
	// ModeAsk auto-approves allow-rule matches, hard-blocks deny-rule
	// matches, and PAUSES the run to ask the human for everything else
	// (Claude Code's `default` mode). Resumable.
	ModeAsk
	// ModeDeny auto-approves allow-rule matches and hard-denies
	// everything else with no pause (Claude Code's `dontAsk` mode) —
	// the policy boundary for headless/cloud/cron runs with no human.
	ModeDeny
)

// String returns the DSL/CLI spelling of the mode.
func (m Mode) String() string {
	switch m {
	case ModeAsk:
		return "ask"
	case ModeDeny:
		return "deny"
	default:
		return "off"
	}
}

// ParseMode converts a DSL/CLI/env string to a Mode. The empty string
// and "off" both map to ModeOff. Unknown values are an error so a
// typo'd `permission: askk` is caught at compile time, not silently
// treated as off.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "bypass", "bypasspermissions":
		return ModeOff, nil
	case "ask", "default", "prompt":
		return ModeAsk, nil
	case "deny", "dontask", "dont_ask":
		return ModeDeny, nil
	default:
		return ModeOff, fmt.Errorf("unknown permission mode %q (want off|ask|deny)", s)
	}
}

// Decision is the outcome of evaluating a tool call against a Policy.
type Decision int

const (
	// Allow: execute the tool.
	Allow Decision = iota
	// Ask: pause the run and surface the call to the human.
	Ask
	// Deny: refuse the call (a synthetic isError tool_result is
	// returned to the model so it can adapt).
	Deny
)

// String renders the decision for logs/events.
func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Ask:
		return "ask"
	case Deny:
		return "deny"
	default:
		return "unknown"
	}
}

// Policy is a resolved permission policy: a mode plus three ordered
// rule lists. The zero value (ModeOff, no rules) is a disabled gate.
type Policy struct {
	Mode  Mode
	allow []rule
	ask   []rule
	deny  []rule
}

// NewPolicy parses the rule strings into a Policy. A malformed rule is
// an error (surfaced as a compile diagnostic upstream). Rule order
// within each list is preserved; cross-list precedence is deny → ask →
// allow (see [Policy.Evaluate]).
func NewPolicy(mode Mode, allow, ask, deny []string) (*Policy, error) {
	p := &Policy{Mode: mode}
	for _, group := range []struct {
		name string
		raw  []string
		dst  *[]rule
	}{
		{"allow", allow, &p.allow},
		{"ask", ask, &p.ask},
		{"deny", deny, &p.deny},
	} {
		for _, raw := range group.raw {
			r, err := parseRule(raw)
			if err != nil {
				return nil, fmt.Errorf("%s rule %q: %w", group.name, raw, err)
			}
			*group.dst = append(*group.dst, r)
		}
	}
	return p, nil
}

// Enabled reports whether the gate is active (mode != off).
func (p *Policy) Enabled() bool { return p != nil && p.Mode != ModeOff }

// AllowRuleStrings returns the raw allow-rule strings (used to forward
// the policy to a sandboxed runner / subprocess unchanged).
func (p *Policy) AllowRuleStrings() []string { return ruleStrings(p.allow) }

// AskRuleStrings returns the raw ask-rule strings.
func (p *Policy) AskRuleStrings() []string { return ruleStrings(p.ask) }

// DenyRuleStrings returns the raw deny-rule strings.
func (p *Policy) DenyRuleStrings() []string { return ruleStrings(p.deny) }

func ruleStrings(rs []rule) []string {
	if len(rs) == 0 {
		return nil
	}
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.raw
	}
	return out
}

// AddAllowRule appends an allow rule (used by "allow always" on
// resume — the operator's grant persists for the rest of the run).
// A malformed rule is ignored (it came from a structured decision, not
// user free-text, so this is defensive only).
func (p *Policy) AddAllowRule(raw string) {
	if r, err := parseRule(raw); err == nil {
		p.allow = append(p.allow, r)
	}
}

// Evaluate decides what to do with a tool call. Precedence mirrors
// Claude Code's evaluation order (deny rules → ask rules → allow rules
// → permission mode):
//
//  1. a matching deny rule → Deny (wins in every mode);
//  2. a matching ask rule  → Ask;
//  3. a matching allow rule → Allow;
//  4. otherwise the mode default — ModeAsk → Ask, ModeDeny → Deny,
//     ModeOff → Allow (the gate is disabled).
//
// The returned string is the matched rule (or "" for the mode default)
// — used for the human prompt / deny reason.
func (p *Policy) Evaluate(toolName string, input map[string]any) (Decision, string) {
	if !p.Enabled() {
		return Allow, ""
	}
	// iterion's own interaction/capability plumbing (ask_user, board.*,
	// control, watch) is infrastructure, not an agent action against the
	// environment — never gate it, or `ask` mode would pause on the very
	// tool used to ask the human. Same exemption on both backends.
	if isInfrastructureTool(toolName) {
		return Allow, ""
	}
	summary := summarize(toolName, input)
	if r, ok := matchAny(p.deny, toolName, summary); ok {
		return Deny, r.raw
	}
	if r, ok := matchAny(p.ask, toolName, summary); ok {
		return Ask, r.raw
	}
	if r, ok := matchAny(p.allow, toolName, summary); ok {
		return Allow, r.raw
	}
	switch p.Mode {
	case ModeDeny:
		return Deny, ""
	default: // ModeAsk
		return Ask, ""
	}
}

// rule is a parsed permission entry.
type rule struct {
	raw     string         // original text, for messages + forwarding
	tool    string         // canonical tool key, or "*" / "mcp__srv__*" glob
	toolRe  *regexp.Regexp // non-nil when the tool name itself is a glob
	hasArg  bool           // true when the rule scoped an (arg) pattern
	argKind argMatchKind   // how to match the argument
	argLit  string         // literal for exact/prefix
	argRe   *regexp.Regexp // compiled regex for wildcard matches
}

type argMatchKind int

const (
	argExact  argMatchKind = iota // summary == literal
	argPrefix                     // summary startswith literal (the `:*` idiom)
	argRegex                      // summary matches argRe (contains `*`/`**`)
)

// parseRule parses `Tool` or `Tool(content)` into a rule.
func parseRule(raw string) (rule, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return rule{}, fmt.Errorf("empty rule")
	}
	r := rule{raw: s}

	open := strings.IndexByte(s, '(')
	toolPart := s
	if open >= 0 {
		if !strings.HasSuffix(s, ")") {
			return rule{}, fmt.Errorf("missing closing ')'")
		}
		toolPart = s[:open]
		content := s[open+1 : len(s)-1]
		r.hasArg = true
		r.argKind, r.argLit, r.argRe = compileArg(content)
	}
	toolPart = strings.TrimSpace(toolPart)
	if toolPart == "" {
		return rule{}, fmt.Errorf("missing tool name")
	}

	// Tool-name globs: `*` (any tool) and a trailing `*` after an
	// `mcp__server__` prefix (and, defensively, any literal prefix).
	if strings.Contains(toolPart, "*") {
		re, err := globToRegexp(toolPart)
		if err != nil {
			return rule{}, fmt.Errorf("bad tool glob: %w", err)
		}
		r.tool = toolPart
		r.toolRe = re
		return r, nil
	}
	r.tool = canonicalToolName(toolPart)
	return r, nil
}

// compileArg classifies the `(content)` part of a rule.
func compileArg(content string) (argMatchKind, string, *regexp.Regexp) {
	c := strings.TrimSpace(content)
	// The Bash prefix idiom: `git diff:*` matches `git diff` and any
	// longer command. We also accept a bare trailing `*` as a prefix
	// when it is the only wildcard.
	if strings.HasSuffix(c, ":*") {
		return argPrefix, strings.TrimSuffix(c, ":*"), nil
	}
	if strings.Contains(c, "*") {
		re, err := globToRegexp(c)
		if err == nil {
			return argRegex, c, re
		}
	}
	return argExact, c, nil
}

// matchAny returns the first rule in rs that matches (tool, summary).
func matchAny(rs []rule, toolName, summary string) (rule, bool) {
	canon := canonicalToolName(toolName)
	for _, r := range rs {
		if !r.matchesTool(toolName, canon) {
			continue
		}
		if !r.hasArg {
			return r, true // bare tool rule matches any invocation
		}
		if r.matchesArg(summary) {
			return r, true
		}
	}
	return rule{}, false
}

func (r rule) matchesTool(rawName, canon string) bool {
	if r.toolRe != nil {
		// Glob is matched against the raw name (so `mcp__github__get_*`
		// works on the FQN form) and the canonical key.
		return r.toolRe.MatchString(rawName) || r.toolRe.MatchString(canon)
	}
	if r.tool == "*" {
		return true
	}
	return r.tool == canon
}

func (r rule) matchesArg(summary string) bool {
	// summarize may emit several candidate strings (one per line) — e.g.
	// WebFetch yields "domain:<host>", "<host>" and the full URL — and a
	// scoped rule matches if it matches ANY candidate.
	for _, cand := range strings.Split(summary, "\n") {
		switch r.argKind {
		case argPrefix:
			if strings.HasPrefix(cand, r.argLit) {
				return true
			}
		case argRegex:
			if r.argRe != nil && r.argRe.MatchString(cand) {
				return true
			}
		default: // argExact
			if cand == r.argLit {
				return true
			}
		}
	}
	return false
}

// globToRegexp converts a `*`/`**` glob to an anchored regexp. Both `*`
// and `**` translate to `.*` (greedy) — a faithful, predictable
// superset of Claude Code's segment-aware globbing that covers the
// common rule shapes (`pkg/**`, `*.go`, `mcp__srv__get_*`). Documented
// in docs/permissions.md.
func globToRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for _, part := range strings.Split(glob, "*") {
		b.WriteString(regexp.QuoteMeta(part))
		b.WriteString(".*")
	}
	// The split adds one trailing ".*" too many; trim it.
	pattern := strings.TrimSuffix(b.String(), ".*")
	pattern += "$"
	return regexp.Compile(pattern)
}

// summarize renders a tool's input into the string(s) that scoped rule
// patterns match against, mirroring Claude Code's per-tool summaries
// (Bash → command, Read/Edit/Write → path, WebFetch → domain + url).
// Multiple candidates are joined with '\n' so a rule pattern matches if
// it matches ANY candidate (e.g. WebFetch by domain OR full url).
func summarize(toolName string, input map[string]any) string {
	switch canonicalToolName(toolName) {
	case "bash":
		return str(input, "command")
	case "read", "write", "edit", "notebookedit":
		return firstNonEmpty(str(input, "file_path"), str(input, "path"), str(input, "notebook_path"))
	case "glob", "grep":
		return firstNonEmpty(str(input, "pattern"), str(input, "path"))
	case "webfetch":
		u := str(input, "url")
		host := ""
		if parsed, err := url.Parse(u); err == nil {
			host = parsed.Host
		}
		cands := []string{}
		if host != "" {
			cands = append(cands, "domain:"+host, host)
		}
		if u != "" {
			cands = append(cands, u)
		}
		return strings.Join(cands, "\n")
	case "websearch":
		return str(input, "query")
	default:
		// Generic fallback: a sorted "k=v" join so a pattern can still
		// scope MCP/custom tools by an argument value if needed.
		keys := make([]string, 0, len(input))
		for k := range input {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+fmt.Sprint(input[k]))
		}
		return strings.Join(parts, " ")
	}
}

func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isInfrastructureTool reports whether a tool is iterion's own
// runtime plumbing (the interaction + capability surface) rather than
// an agent action against the environment. These are always allowed by
// the gate. Covers both backends' spellings: ask_user (claw) /
// mcp__iterion__ask_user (claude_code), the board / control / watch MCP
// families, and the iterion-internal __mcp-* servers.
func isInfrastructureTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "ask_user", "askuser", "send_user_message":
		return true
	}
	if strings.HasPrefix(n, "mcp__iterion") || strings.HasPrefix(n, "mcp_iterion") {
		return true
	}
	if strings.HasPrefix(n, "__mcp") {
		return true
	}
	return false
}

// canonicalToolName maps a backend-specific tool name to a stable key
// so a single rule (e.g. `Bash(...)`) gates the matching tool on BOTH
// backends: claude_code's TitleCase names (Bash, Read, Edit, WebFetch)
// and claw's snake_case names (bash, read_file, write_file, web_fetch).
// Unknown names are lower-cased and stripped of separators so a custom
// rule still matches its own spelling.
func canonicalToolName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if alias, ok := toolAliases[n]; ok {
		return alias
	}
	// MCP FQNs (mcp__server__tool / mcp_server_tool) are kept verbatim
	// (lower-cased) so server-scoped globs match.
	if strings.HasPrefix(n, "mcp__") || strings.HasPrefix(n, "mcp_") {
		return n
	}
	return n
}

// toolAliases collapses cross-backend synonyms onto a canonical key.
var toolAliases = map[string]string{
	// shell
	"bash": "bash", "shell": "bash", "sh": "bash",
	// read
	"read": "read", "read_file": "read", "readfile": "read", "cat": "read",
	// write
	"write": "write", "write_file": "write", "writefile": "write",
	// edit
	"edit": "edit", "edit_file": "edit", "file_edit": "edit",
	"multiedit": "edit", "edit_mode": "edit", "str_replace": "edit",
	// notebook
	"notebookedit": "notebookedit", "notebook_edit": "notebookedit",
	// search
	"glob": "glob", "grep": "grep",
	// web
	"webfetch": "webfetch", "web_fetch": "webfetch", "web": "webfetch",
	"websearch": "websearch", "web_search": "websearch",
	// misc
	"ls": "ls", "todowrite": "todowrite", "todo_write": "todowrite",
}
