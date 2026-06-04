package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SocialGouv/iterion/pkg/backend/tool"
	"github.com/SocialGouv/iterion/pkg/backend/tool/privacy"
	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	"github.com/SocialGouv/iterion/pkg/sandbox"
)

// ---------------------------------------------------------------------------
// Tool node execution
//
// Extracted from executor.go to keep that file focused on Execute /
// resolveBackend / executeBackend flow. Same package, so no API change.
// ---------------------------------------------------------------------------

// executeToolNode runs a tool node (direct command, no LLM).
// The tool policy is checked before execution; denied tools produce an
// explicit error with the tool_called hook fired (Error != nil).
func (e *ClawExecutor) executeToolNode(ctx context.Context, node *ir.ToolNode, input map[string]interface{}) (map[string]interface{}, error) {
	// `script:` body takes precedence over `command:` (IR validation
	// ensures they're mutually exclusive at compile time, so this is
	// just a clean dispatch).
	if node.Script != "" {
		return e.executeToolNodeScript(ctx, node, input)
	}
	// When the command contains template refs ({{input.X}}) or looks like a
	// shell command (contains spaces or shell operators), execute as a direct
	// shell command. Otherwise, use the tool registry.
	if len(node.CommandRefs) > 0 || looksLikeShellCommand(node.Command) {
		return e.executeToolNodeShell(ctx, node, input)
	}

	toolName := node.Command

	// Policy check before resolution — fail fast on denied tools.
	if err := e.checkToolNodePolicy(ctx, node, toolName); err != nil {
		return nil, err
	}

	resolved, ok, err := e.resolveSingleToolForNode(ctx, node, toolName)
	if err != nil {
		return nil, fmt.Errorf("model: tool node %q: %w", node.ID, err)
	}
	if !ok {
		return nil, fmt.Errorf("model: tool node %q references unregistered tool %q", node.ID, toolName)
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("model: tool node %q: marshal input: %w", node.ID, err)
	}

	// Persistence-aware redaction: privacy_filter carries raw PII as
	// input; privacy_unfilter produces raw PII as output. The hooks
	// below feed the persisted event log, so apply redaction up-front
	// (input) and after Execute (output). The in-memory values handed
	// to Execute and downstream nodes are untouched.
	inputForEvent := inputJSON
	if toolName == privacy.FilterToolName {
		inputForEvent = redactJSONTextField(inputJSON)
	}

	if e.hooks.OnToolStarted != nil {
		e.hooks.OnToolStarted(node.ID, LLMToolStartedInfo{
			ToolName:  toolName,
			InputSize: len(inputJSON),
			Input:     json.RawMessage(inputForEvent),
		})
	}

	start := time.Now()
	outputStr, err := resolved.Execute(ctx, inputJSON)
	duration := time.Since(start)
	if e.hooks.OnToolCall != nil {
		e.hooks.OnToolCall(node.ID, LLMToolCallInfo{
			ToolName:  toolName,
			InputSize: len(inputJSON),
			Duration:  duration,
			Error:     err,
		})
	}
	outputForEvent := outputStr
	if toolName == privacy.UnfilterToolName {
		outputForEvent = string(redactJSONTextField([]byte(outputStr)))
	}
	// Emit detailed tool I/O via the prompt hook (reused for tool node logging).
	if e.hooks.OnToolNodeResult != nil {
		e.hooks.OnToolNodeResult(node.ID, toolName, inputForEvent, outputForEvent, duration, err)
	}
	if err != nil {
		return nil, fmt.Errorf("model: tool node %q: execute: %w", node.ID, err)
	}

	// Try to parse tool output as JSON map, otherwise wrap as text.
	var output map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(outputStr), &output); jsonErr != nil {
		output = map[string]interface{}{"result": outputStr}
	}

	return output, nil
}

// executeToolNodeShell handles tool nodes whose command contains {{...}}
// template references. Templates are resolved from the node's input map,
// and the resulting string is executed as a shell command via sh -c.
func (e *ClawExecutor) executeToolNodeShell(ctx context.Context, node *ir.ToolNode, input map[string]interface{}) (map[string]interface{}, error) {
	toolName := shellToolNodeToolName(node)
	if err := e.checkToolNodePolicy(ctx, node, toolName); err != nil {
		return nil, err
	}

	// Expand environment variables FIRST, on the author-controlled command
	// template only. Doing this AFTER resolveCommandTemplate would re-introduce
	// shell metacharacters into substituted values that shellEscape thought
	// were inert single-quoted strings — e.g. an upstream-LLM-controlled input
	// of `$INJECT` would survive shellEscape as `'$INJECT'`, then become
	// `''; rm -rf ~; ''` if the env had INJECT=`'; rm -rf ~; '`. By expanding
	// before substitution, only the .iter author's own `$VAR` references in
	// the static command template are expanded; substituted values stay safely
	// quoted.
	//
	// Only the BRACED form `${NAME}` is treated as an env var reference —
	// matching the SKILL.md documentation ("Environment variables are
	// supported with ${ENV_VAR} syntax"). Bare `$NAME`, `$ec`, `$?`, `$1`
	// etc. are passed through verbatim so shell-level variables (positional
	// args, exit-status, captured stdout) survive into the resolved command
	// for sh -c to interpret. The previous os.ExpandEnv call ate those,
	// silently turning `$ec` into `""` and breaking any tool that wanted to
	// capture exit codes or compose intermediate shell values.
	expandedCommand := expandBracedEnv(node.Command)

	// Resolve template references in the (env-expanded) command.
	resolved := resolveCommandTemplate(expandedCommand, node.CommandRefs, input, e.vars)

	if e.hooks.OnToolStarted != nil {
		e.hooks.OnToolStarted(node.ID, LLMToolStartedInfo{
			ToolName:  toolName,
			InputSize: len(resolved),
		})
	}

	start := time.Now()
	cmd := e.toolNodeCommand(ctx, resolved)
	// Separate stdout (for structured JSON parsing) from stderr (for
	// diagnostic logging). Tools that emit a JSON result on stdout MUST
	// be able to use stderr for prose (yarn's resolution output, git's
	// `[detached HEAD ...]` line, etc.) without that prose breaking the
	// JSON parse downstream. CombinedOutput() conflated the two and
	// poisoned the parse.
	stdoutBytes, runErr, stderrStr := runWithSeparateStreams(cmd)
	outputStr := string(stdoutBytes)
	duration := time.Since(start)

	if e.hooks.OnToolCall != nil {
		e.hooks.OnToolCall(node.ID, LLMToolCallInfo{
			ToolName: toolName,
			Duration: duration,
			Error:    runErr,
		})
	}
	if e.hooks.OnToolNodeResult != nil {
		// Log both streams concatenated so run.log still surfaces what
		// the operator would see in an interactive shell. Stdout first
		// so the structured payload is visible at the top of long
		// stderr dumps from yarn/npm/git.
		logged := combineStreamsForLog(outputStr, stderrStr)
		e.hooks.OnToolNodeResult(node.ID, toolName, []byte(resolved), logged, duration, runErr)
	}
	if runErr != nil {
		return nil, fmt.Errorf("model: tool node %q: shell command failed: %w\nstdout: %s\nstderr: %s", node.ID, runErr, outputStr, stderrStr)
	}

	// Try to parse stdout as JSON, otherwise wrap as text.
	var output map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(outputStr), &output); jsonErr != nil {
		output = map[string]interface{}{"result": strings.TrimSpace(outputStr)}
	}

	return output, nil
}

// shellToolNodeToolName returns the canonical virtual tool name used for
// policy checks and hooks for direct shell-command tool nodes. Operators can
// allow a specific shell node with "shell:<nodeID>" or all shell nodes with
// "shell:*".
func shellToolNodeToolName(node *ir.ToolNode) string {
	return "shell:" + node.ID
}

// scriptToolNodeToolName returns the canonical virtual tool name used for
// policy checks and hooks for direct script tool nodes. Empty language
// defaults to "sh", matching scriptInterpreter. Operators can allow a
// specific script node with "script:<language>:<nodeID>" or scripts in a
// language with "script:<language>:*".
func scriptToolNodeToolName(node *ir.ToolNode) string {
	language := node.Language
	if language == "" {
		language = "sh"
	}
	return "script:" + language + ":" + node.ID
}

// checkToolNodePolicy applies the executor tool policy to all tool-node
// execution modes: registry tools and direct virtual shell/script tools. On
// denial it emits OnToolCall with the policy error, matching failed executed
// tool calls, and returns an error wrapped with node and tool context.
func (e *ClawExecutor) checkToolNodePolicy(ctx context.Context, node *ir.ToolNode, toolName string) error {
	if e.toolPolicy == nil {
		return nil
	}
	pctx := tool.PolicyContext{
		Ctx:      ctx,
		NodeID:   node.ID,
		NodeKind: ir.NodeTool.String(),
		ToolName: toolName,
		Vars:     e.vars,
	}
	if err := e.toolPolicy.CheckContext(pctx); err != nil {
		if e.hooks.OnToolCall != nil {
			e.hooks.OnToolCall(node.ID, LLMToolCallInfo{
				ToolName: toolName,
				Error:    err,
			})
		}
		return fmt.Errorf("model: tool node %q: tool %q denied: %w", node.ID, toolName, err)
	}
	return nil
}

// runWithSeparateStreams runs cmd with stdout and stderr captured into
// distinct buffers. Returns (stdout bytes, run error, stderr string).
// Use this for tool nodes where downstream needs to parse stdout as a
// structured payload while leaving stderr free for diagnostic chatter.
func runWithSeparateStreams(cmd *exec.Cmd) ([]byte, error, string) {
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	stdoutBytes, runErr := cmd.Output()
	// exec.ExitError carries stderr it captured before we set ours; the
	// buffer we provided is still the source of truth in our path.
	return stdoutBytes, runErr, stderrBuf.String()
}

// combineStreamsForLog formats stdout + stderr into a single string for
// run.log display. Empty streams are elided so the common
// "JSON on stdout, nothing on stderr" case stays compact.
func combineStreamsForLog(stdout, stderr string) string {
	stdout = strings.TrimRight(stdout, "\n")
	stderr = strings.TrimRight(stderr, "\n")
	switch {
	case stdout == "" && stderr == "":
		return ""
	case stderr == "":
		return stdout
	case stdout == "":
		return stderr
	default:
		return stdout + "\n--- stderr ---\n" + stderr
	}
}

// executeToolNodeScript handles tool nodes declared with `script:` +
// optional `language:`. The script body is resolved (template refs
// substituted), written to a temp file inside the workspace, and
// executed via the interpreter named by Language (defaulting to sh
// when empty). The temp file lives in the workspace so it is visible
// from inside the sandbox bind-mount, and is removed on success or
// failure.
func (e *ClawExecutor) executeToolNodeScript(ctx context.Context, node *ir.ToolNode, input map[string]interface{}) (map[string]interface{}, error) {
	toolName := scriptToolNodeToolName(node)
	if err := e.checkToolNodePolicy(ctx, node, toolName); err != nil {
		return nil, err
	}

	// Same env-then-substitution ordering as executeToolNodeShell: only
	// the author-controlled `${NAME}` braces are env-expanded so injected
	// values from inputs/vars stay inert. But where the shell path uses
	// shell-escape, script: tools use resolveScriptTemplate (JSON literal
	// rendering) so values land as valid JS/Python/Ruby literals —
	// shell-escape's single-quote wrapping breaks script-language string
	// parsers when the value contains embedded apostrophes (e.g. an
	// agent output blob with `yarn workspaces foreach ... '\''…'\''`).
	expanded := expandBracedEnv(node.Script)
	resolved := resolveScriptTemplate(expanded, node.ScriptRefs, input, e.vars)

	interp, ext := scriptInterpreter(node.Language)
	if interp == "" {
		return nil, fmt.Errorf("model: tool node %q: unsupported language %q", node.ID, node.Language)
	}

	// Determine the workspace directory the temp file should live in.
	// Inside a sandbox, e.workDir is the host-side bind source which the
	// container also sees, so a basename written there is reachable from
	// both sides via the same relative path.
	wd := e.workDir
	if wd == "" {
		wd = "."
	}
	tmpFile, err := os.CreateTemp(wd, ".iterion-script-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("model: tool node %q: create temp script: %w", node.ID, err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, werr := tmpFile.WriteString(resolved); werr != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("model: tool node %q: write temp script: %w", node.ID, werr)
	}
	if cerr := tmpFile.Close(); cerr != nil {
		return nil, fmt.Errorf("model: tool node %q: close temp script: %w", node.ID, cerr)
	}

	// Compute the relative basename for the in-sandbox view (the bind
	// mount uses the same path; passing just the basename keeps it
	// portable whether we run via sandbox or host).
	scriptBasename := filepath.Base(tmpPath)

	if e.hooks.OnToolStarted != nil {
		e.hooks.OnToolStarted(node.ID, LLMToolStartedInfo{
			ToolName:  toolName,
			InputSize: len(resolved),
		})
	}

	start := time.Now()
	cmd := e.toolNodeScriptCommand(ctx, interp, scriptBasename)
	// Same stdout/stderr separation as executeToolNodeShell — a
	// `script: js` body can console.error() freely without breaking
	// the JSON.parse on stdout.
	stdoutBytes, runErr, stderrStr := runWithSeparateStreams(cmd)
	outputStr := string(stdoutBytes)
	duration := time.Since(start)

	if e.hooks.OnToolCall != nil {
		e.hooks.OnToolCall(node.ID, LLMToolCallInfo{
			ToolName: toolName,
			Duration: duration,
			Error:    runErr,
		})
	}
	if e.hooks.OnToolNodeResult != nil {
		logged := combineStreamsForLog(outputStr, stderrStr)
		e.hooks.OnToolNodeResult(node.ID, toolName, []byte(resolved), logged, duration, runErr)
	}
	if runErr != nil {
		return nil, fmt.Errorf("model: tool node %q: script failed: %w\nstdout: %s\nstderr: %s", node.ID, runErr, outputStr, stderrStr)
	}

	var output map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(outputStr), &output); jsonErr != nil {
		output = map[string]interface{}{"result": strings.TrimSpace(outputStr)}
	}
	return output, nil
}

// scriptInterpreter maps a `language:` token to the executable name on
// PATH and a file extension hint (extension is informational, not
// required by any interpreter). An empty language defaults to sh.
func scriptInterpreter(language string) (cmd string, ext string) {
	switch language {
	case "", "sh":
		return "sh", ".sh"
	case "bash":
		return "bash", ".sh"
	case "js", "node":
		return "node", ".js"
	case "py", "python", "python3":
		return "python3", ".py"
	default:
		return "", ""
	}
}

// toolNodeScriptCommand returns a configured *exec.Cmd that invokes the
// interpreter on the basename of the script temp file. Mirrors
// toolNodeCommand for the script-mode path: sandbox-routed if a sandbox
// is active and the node has not opted out.
func (e *ClawExecutor) toolNodeScriptCommand(ctx context.Context, interpreter, scriptBasename string) *exec.Cmd {
	if e.sandbox != nil && !e.nodeOptsOutOfSandbox(toolNodeOptOut) {
		return e.sandbox.Command(ctx, []string{interpreter, scriptBasename}, sandbox.ExecOpts{})
	}
	cmd := exec.CommandContext(ctx, interpreter, scriptBasename)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	return cmd
}

// toolNodeCommand returns a configured *exec.Cmd for a tool node's
// shell snippet. When the run is sandboxed and the node has not opted
// out (`sandbox: none` at node scope), the command is routed through
// the sandbox via [sandbox.Run.Command]; otherwise it is the
// pre-sandbox host invocation.
//
// Per-node opt-out lets a workflow run mostly sandboxed but cherry-pick
// a tool node that needs host access (e.g. `gh` configured against
// the host's keychain).
//
// Shell selection: we invoke `bash -c` (not `sh -c`) so recipe authors
// can use modern bashisms (`set -o pipefail`, arrays `BLOCKERS=()`,
// `[[ ... ]]`, process substitution, etc.) without surprises. On
// Debian/Ubuntu-derived containers and hosts, `/bin/sh` is dash —
// strict POSIX, no arrays, no pipefail, errors with
// `Syntax error: "(" unexpected` on `X=()`. The iterion-sandbox-slim
// image ships bash 5+; modern Linux desktops always have bash. If
// bash is genuinely absent (extremely minimal containers), the recipe
// author should either install bash via post_create or rewrite the
// tool body in POSIX shell.
func (e *ClawExecutor) toolNodeCommand(ctx context.Context, resolved string) *exec.Cmd {
	if e.sandbox != nil && !e.nodeOptsOutOfSandbox(toolNodeOptOut) {
		return e.sandbox.Command(ctx, []string{"bash", "-c", resolved}, sandbox.ExecOpts{})
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", resolved)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	return cmd
}

// nodeOptOut classifies the kind of node being inspected for sandbox
// opt-out purposes. The current callers all examine the tool-node
// path, but the broadcast-style API leaves room for agent/judge
// routing later.
type nodeOptOut int

const (
	toolNodeOptOut nodeOptOut = iota
)

// nodeOptsOutOfSandbox reports whether the node currently being
// executed declared `sandbox: none` and therefore wants to run on
// the host even though the workflow has an active sandbox.
//
// Phase 1 keeps this simple: there is no per-call node context, so
// the executor cannot consult per-node overrides here. The hook is
// in place for Phase 2 where engine + executor pass the in-flight
// node identifier through. Returning false today preserves the
// "sandbox active = everything sandboxed" guarantee.
func (e *ClawExecutor) nodeOptsOutOfSandbox(_ nodeOptOut) bool {
	return false
}

// looksLikeShellCommand returns true if the command string looks like a shell
// command rather than a bare tool name. Tool names are simple identifiers
// (e.g. "read_file", "bash"), while shell commands contain spaces, operators,
// or path separators.
func looksLikeShellCommand(cmd string) bool {
	return strings.ContainsAny(cmd, " \t|&;><$`(){}\"'/")
}

// resolveCommandTemplate substitutes {{input.X}} and {{vars.X}} references in
// a command string with values from the input map and workflow variables.
// Values are shell-escaped to prevent command injection when the resolved
// string is passed to sh -c.
//
// Refs flagged as `Raw` (authored as `{{!input.X}}` / `{{!vars.X}}`) bypass
// shellEscape and are inserted verbatim. Use only for trusted values that
// are intentionally shell snippets (e.g. an upstream node returns a
// command line that the wrapping tool needs to RE-INTERPRET as shell, not
// pass as a single quoted token). Untrusted external inputs MUST keep the
// default escaping.
func resolveCommandTemplate(command string, refs []*ir.Ref, input map[string]interface{}, vars map[string]interface{}) string {
	// Shell commands preserve `{{input.X}}` literal text for missing
	// values — sh -c sees the placeholder and either fails informatively
	// or the operator notices. Substituting silently would lose that
	// signal and could mask wiring bugs.
	return resolveTemplateWith(command, refs, input, vars, shellEscapeValue, false)
}

// resolveScriptTemplate substitutes refs in a tool node's `script:` body.
// SCRIPT contexts (JS / Python / Ruby / any JSON-superset language) want
// JSON-encoded values, not shell-escaped ones — shell-escape wraps
// strings in single quotes (and renders embedded apostrophes as the
// shell-only `'\”` escape sequence) which then breaks the script
// language's string-literal parser. JSON encoding produces valid
// literals in all major scripting languages: `"foo"` is a JS / Python /
// Ruby string, `{"k":"v"}` is an object/dict literal, `[1,2,3]` is an
// array/list literal.
//
// The bang form `{{!input.X}}` keeps the legacy raw-passthrough
// behaviour (strings inserted unquoted) for authors who need to drop
// a snippet of source directly into the script body.
func resolveScriptTemplate(script string, refs []*ir.Ref, input map[string]interface{}, vars map[string]interface{}) string {
	// Script bodies (JS/Python/Ruby/…) must always parse — a nil-valued
	// ref left as raw `{{input.X}}` text crashes the interpreter at
	// parse time before any user logic can react. Substitute with the
	// language's null literal (rendered as JSON null = "null") so the
	// script can still run and handle the missing input itself.
	return resolveTemplateWith(script, refs, input, vars, jsonLiteralValue, true)
}

// resolveTemplateWith is the shared core: walk refs, look up each value,
// dispatch to rawTemplateValue (bang form) or the renderer the caller
// provided (default form). Keeps shell- and script-mode template logic
// in one place.
//
// Substitution is single-pass over the original template: we build a
// map[ref.Raw]rendered and then scan the template once, replacing
// each {{...}} occurrence by its rendered value. The previous
// strings.ReplaceAll loop fed each substitution's output back into
// subsequent passes, so an input value that happened to contain a
// {{...}} literal matching a later ref would be silently rewritten
// (the "cascade" bug). The single-pass walk only touches positions
// that were in the source template.
func resolveTemplateWith(template string, refs []*ir.Ref, input map[string]interface{}, vars map[string]interface{}, defaultRender func(interface{}) string, substituteNil bool) string {
	if len(refs) == 0 {
		return template
	}
	subs := make(map[string]string, len(refs))
	for _, ref := range refs {
		var val interface{}
		var handled bool
		switch {
		case ref.Kind == ir.RefInput && len(ref.Path) > 0:
			val = input[ref.Path[0]]
			handled = true
		case ref.Kind == ir.RefVars && len(ref.Path) > 0:
			val = vars[ref.Path[0]]
			handled = true
		}
		if !handled {
			continue
		}
		// substituteNil controls whether a recognised-but-nil ref gets
		// rendered or left as raw template text. Shell contexts keep
		// the raw `{{input.X}}` placeholder so a missing wiring is
		// visible; script contexts MUST render (renderer turns nil into
		// "null") because a JS/Python/Ruby parser otherwise crashes on
		// the literal braces before any script logic runs.
		if val == nil && !substituteNil {
			continue
		}
		if ref.Unquoted {
			subs[ref.Raw] = rawTemplateValue(val)
		} else {
			subs[ref.Raw] = defaultRender(val)
		}
	}
	if len(subs) == 0 {
		return template
	}
	var b strings.Builder
	b.Grow(len(template))
	i := 0
	for i < len(template) {
		if i+1 < len(template) && template[i] == '{' && template[i+1] == '{' {
			end := strings.Index(template[i:], "}}")
			if end == -1 {
				b.WriteString(template[i:])
				return b.String()
			}
			raw := template[i : i+end+2]
			if rendered, ok := subs[raw]; ok {
				b.WriteString(rendered)
			} else {
				b.WriteString(raw)
			}
			i += end + 2
			continue
		}
		b.WriteByte(template[i])
		i++
	}
	return b.String()
}

// jsonLiteralValue renders val as a valid JSON literal suitable for
// pasting into a script-language source file. Strings get JSON-quoted
// (`"foo"`, with embedded quotes escaped as `\"`); maps and slices get
// JSON-encoded as object/array literals. Numbers, bools, and nil are
// rendered as JSON's natural form. The result is a valid expression
// in JavaScript, Python, Ruby, and any modern language that accepts
// JSON-superset literal syntax — no further wrapping needed.
func jsonLiteralValue(val interface{}) string {
	b, err := json.Marshal(val)
	if err != nil {
		// json.Marshal effectively never fails on values we accept
		// (interface{} of map/slice/string/number/bool/nil), but if
		// it ever did, emit a JSON null so the script parses rather
		// than aborting at a bare unquoted identifier.
		return "null"
	}
	return string(b)
}

// rawTemplateValue renders a value verbatim (without shell-escaping) for
// the {{!ref}} raw substitution mode. Strings pass through; complex types
// are JSON-encoded (matches formatValue's prompt-rendering convention so
// authors can reason about both contexts uniformly).
func rawTemplateValue(val interface{}) string {
	if val == nil {
		return "null"
	}
	if s, ok := val.(string); ok {
		return s
	}
	return formatValue(val)
}

// expandBracedEnv expands ${NAME} and ${NAME:-default} env references in
// the author-controlled command template. Bare $NAME (and shell-special
// $1, $?, $@, $ec, $OUT, ...) are passed through verbatim so shell-level
// constructs survive into the resolved command for sh -c to interpret.
//
// The previous implementation called os.ExpandEnv which treats both forms
// the same — silently eating any bare $NAME and breaking tool authors who
// wanted to capture exit codes (`ec=$?`), compose intermediate shell
// values (`OUT=$(...) ; echo "$OUT"`), or use positional args from
// `bash -c '...' _ "$1" "$2"`. The braced form is also the only form
// SKILL.md documents, so tightening to it is BC for documented usage.
func expandBracedEnv(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			// Find matching '}'.
			end := strings.IndexByte(s[i+2:], '}')
			if end == -1 {
				// Unterminated — pass through verbatim.
				out.WriteByte(s[i])
				i++
				continue
			}
			body := s[i+2 : i+2+end]
			// Only treat the body as an env var reference when it
			// LOOKS like one (`NAME` or `NAME:-default` where NAME is
			// a valid C-style identifier). Without this guard, a
			// script: js body's `${batchPackages.length}` or
			// `${sha.slice(0, 7)}` would be eaten — the body matches
			// no env var, no default, and the function used to return
			// "" which silently erased the JS template literal. Same
			// hazard for `${p.name}`, `${process.env.X}`, etc. inside
			// any script-language source we substitute into.
			// Expand only when:
			//   - the body lexically looks like a shell env ref, AND
			//   - either the ref has an explicit `:-default` (which is
			//     unambiguously a shell-style env ref — JS template
			//     literals don't have that syntax), OR the named env
			//     var is actually set on the process.
			// Otherwise (e.g. `${fname}` in a JS template literal where
			// no FNAME env var is set, or `${sha.slice(0, 7)}` whose
			// body doesn't even look like an env name), we pass the
			// `${body}` through verbatim so the downstream language
			// parser sees what the author wrote. The old behaviour was
			// to erase unset-no-default refs which silently broke JS
			// template literal interpolations.
			if looksLikeEnvRef(body) && bracedEnvWouldExpand(body) {
				out.WriteString(resolveBracedEnvBody(body))
				i += 2 + end + 1
				continue
			}
			// Pass the literal `${body}` through unchanged.
			out.WriteString(s[i : i+2+end+1])
			i += 2 + end + 1
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// bracedEnvWouldExpand reports whether expandBracedEnv would produce a
// non-empty, non-passthrough result for this body. Used to decide
// whether `${body}` should be substituted at all: a body that
// lexically resembles an env ref (looksLikeEnvRef) is only treated as
// one when either the author wrote an explicit `:-default` (a syntax
// that script languages don't use, so this is unambiguously shell) OR
// the named env var is actually set on the process. Otherwise the
// `${body}` is preserved verbatim so a script-language interpolation
// like `${fname}` doesn't get silently erased.
func bracedEnvWouldExpand(body string) bool {
	if strings.Contains(body, ":-") {
		return true
	}
	name := body
	if idx := strings.Index(body, ":-"); idx != -1 {
		name = body[:idx]
	}
	_, ok := os.LookupEnv(name)
	return ok
}

// looksLikeEnvRef reports whether body matches the shell convention
// for a `${NAME}` or `${NAME:-default}` reference: NAME is a valid
// C-style identifier (`[A-Za-z_][A-Za-z0-9_]*`), nothing else allowed
// before the optional `:-default` suffix. This separates "real env
// var lookups" from script-language template literals (`.`, `(`,
// `[`, spaces, etc.) which we must NOT eat.
func looksLikeEnvRef(body string) bool {
	name := body
	if idx := strings.Index(body, ":-"); idx != -1 {
		name = body[:idx]
	}
	if name == "" {
		return false
	}
	for i, r := range name {
		isLetter := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_'
		isDigit := r >= '0' && r <= '9'
		if i == 0 {
			if !isLetter {
				return false
			}
		} else {
			if !isLetter && !isDigit {
				return false
			}
		}
	}
	return true
}

// resolveBracedEnvBody handles the body of a ${...} reference. Supports
// the bash-style default form ${NAME:-fallback} so authors can write
// recipes that work both with and without the env var set.
func resolveBracedEnvBody(body string) string {
	name := body
	defaultVal := ""
	hasDefault := false
	if idx := strings.Index(body, ":-"); idx != -1 {
		name = body[:idx]
		defaultVal = body[idx+2:]
		hasDefault = true
	}
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	if hasDefault {
		return defaultVal
	}
	return ""
}

// shellEscapeValue formats val for safe interpolation into a sh -c command.
//
// Homogeneous scalar slices ([]string, or []interface{} of strings /
// numbers / bools) become a space-separated list of individually-shell-
// quoted tokens, so each element survives sh's re-tokenization as its
// own argument — letting workflow authors pass file lists or argument
// arrays via a single {{input.x}} reference. An empty slice substitutes
// as empty string (the surrounding command will fail naturally if it
// required at least one argument).
//
// Complex slices (containing maps / slices) and bare maps are JSON-
// encoded and the resulting string is shell-escaped as a single token.
// This matches what the {{!input.x}} raw-substitution form does via
// formatValue, so authors get the same JSON wire shape across both
// forms — the default form just adds shell-escaping so the JSON can be
// safely captured into a shell variable (e.g. `X={{input.x}}` then
// `printf '%s' "$X" | jq ...`). Without this, `fmt.Sprint(map)` would
// render Go's native `map[k:v ...]` representation into the command,
// breaking the shell parse with `command not found` (exit 127) on the
// `[`/`map[...]` fragments.
//
// Scalars fall back to fmt.Sprint + shellEscape, preserving the prior
// single-value behaviour for strings, numbers, and booleans.
func shellEscapeValue(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case []string:
		if len(v) == 0 {
			return ""
		}
		parts := make([]string, len(v))
		for i, s := range v {
			parts[i] = shellEscape(s)
		}
		return strings.Join(parts, " ")
	case []interface{}:
		if len(v) == 0 {
			return ""
		}
		if sliceHasComplexElement(v) {
			// Mixed or complex slice → JSON-encode as a single shell token.
			b, err := json.Marshal(v)
			if err == nil {
				return shellEscape(string(b))
			}
			// json.Marshal essentially never fails on []interface{} built
			// from JSON input; fall through to the per-element path so the
			// caller at least sees something rather than empty string.
		}
		parts := make([]string, len(v))
		for i, e := range v {
			parts[i] = shellEscape(fmt.Sprint(e))
		}
		return strings.Join(parts, " ")
	case map[string]interface{}:
		// Maps don't have a sensible space-separated representation;
		// JSON is the only round-trippable shape.
		b, err := json.Marshal(v)
		if err == nil {
			return shellEscape(string(b))
		}
		return shellEscape(fmt.Sprint(v))
	default:
		return shellEscape(fmt.Sprint(v))
	}
}

// sliceHasComplexElement reports whether s contains at least one
// non-scalar element (map or nested slice). Used to decide between
// space-separated shell tokens (scalar-only slices) and a single
// JSON-encoded token (anything else).
func sliceHasComplexElement(s []interface{}) bool {
	for _, e := range s {
		switch e.(type) {
		case map[string]interface{}, []interface{}, []string, []map[string]interface{}:
			return true
		}
	}
	return false
}

// shellEscape wraps a value in single quotes, escaping any embedded single
// quotes. This produces a string safe for interpolation into sh -c commands.
//
// SECURITY: the returned string MUST NOT be passed through any further
// expansion that interprets shell metacharacters (notably os.ExpandEnv,
// which expands $VAR even inside single quotes from sh's perspective).
// Any post-escape expansion can re-introduce metacharacters that defeat
// the quoting and re-open command-injection paths. Apply such expansions
// to the raw command template BEFORE substitution, never after.
func shellEscape(s string) string {
	// Replace each ' with '\'': end current quote, insert escaped quote, reopen quote.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
