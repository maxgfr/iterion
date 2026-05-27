package ir

import "testing"

// providerSrc builds a minimal one-agent workflow with the given backend
// and provider field so the provider validator can be exercised in
// isolation.
func providerSrc(backend, provider string) string {
	return `
schema empty:
  ok: bool

prompt sys:
  body
  hello

agent writer:
  model: "gpt-4"
  backend: "` + backend + `"
  provider: "` + provider + `"
  system: sys
  output: empty

workflow w:
  entry: writer
  writer -> done
`
}

func TestProvider_UnknownTokenWarns(t *testing.T) {
	r := compileFile(t, providerSrc("claude_code", "anthropic,zaai"))
	expectDiag(t, r, DiagUnknownProvider)
}

func TestProvider_KnownChainNoWarning(t *testing.T) {
	r := compileFile(t, providerSrc("claude_code", "anthropic,zai,openai,auto"))
	expectNoDiag(t, r, DiagUnknownProvider)
}

// Env-ref forms resolve at run time, so the validator must not try to
// validate their literal text (and not misfire on a typo it can't yet see).
func TestProvider_EnvRefSkipsValidation(t *testing.T) {
	r := compileFile(t, providerSrc("claude_code", "${RESCUE_PROVIDER:-zai},anthropic"))
	expectNoDiag(t, r, DiagUnknownProvider)
}

func TestProvider_ChainIgnoredOnClawWarns(t *testing.T) {
	r := compileFile(t, providerSrc("claw", "anthropic,zai"))
	expectDiag(t, r, DiagProviderChainIgnored)
}

func TestProvider_ChainIgnoredOnCodexWarns(t *testing.T) {
	r := compileFile(t, providerSrc("codex", "anthropic,zai"))
	expectDiag(t, r, DiagProviderChainIgnored)
}

// claude_code consumes the hint, so a chain there is meaningful — no C088.
func TestProvider_ChainOnClaudeCodeNoWarning(t *testing.T) {
	r := compileFile(t, providerSrc("claude_code", "anthropic,zai"))
	expectNoDiag(t, r, DiagProviderChainIgnored)
}

// A single-value provider on claw is not a chain, so C088 must not fire.
func TestProvider_SingleValueOnClawNoWarning(t *testing.T) {
	r := compileFile(t, providerSrc("claw", "zai"))
	expectNoDiag(t, r, DiagProviderChainIgnored)
}
