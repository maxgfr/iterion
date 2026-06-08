package ir

import (
	"testing"

	"github.com/SocialGouv/iterion/pkg/dsl/parser"
)

func TestSecretsBlockCompiles(t *testing.T) {
	src := `
secrets:
  github_token: "${GITHUB_TOKEN_TEST}"
  deploy_key:
    value: "${DEPLOY_KEY_TEST}"
    hosts: ["api.github.com", "github.com"]

agent x:
  model: "anthropic/c"
  system: p

prompt p:
  Use {{secrets.github_token}} to authenticate.

workflow w:
  entry: x
  x -> done
`
	pr := parser.Parse("t.iter", src)
	if len(pr.Diagnostics) > 0 {
		t.Fatalf("parser diagnostics: %+v", pr.Diagnostics)
	}
	cr := Compile(pr.File)
	if cr.HasErrors() {
		t.Fatalf("compile errors: %+v", cr.Diagnostics)
	}
	secrets := cr.Workflow.Secrets
	if len(secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d: %+v", len(secrets), secrets)
	}
	if got := secrets["github_token"].Value; got != "${GITHUB_TOKEN_TEST}" {
		t.Errorf("github_token value = %q", got)
	}
	dk := secrets["deploy_key"]
	if dk == nil || dk.Value != "${DEPLOY_KEY_TEST}" {
		t.Fatalf("deploy_key not compiled: %+v", dk)
	}
	if len(dk.Hosts) != 2 || dk.Hosts[0] != "api.github.com" {
		t.Errorf("deploy_key hosts = %+v", dk.Hosts)
	}
}

func TestSecretsUnknownRefDiagnostic(t *testing.T) {
	src := `
secrets:
  known: "${X_TEST}"

agent a:
  model: "anthropic/c"
  system: p

prompt p:
  {{secrets.unknown}} should fail

workflow w:
  entry: a
  a -> done
`
	pr := parser.Parse("t.iter", src)
	if len(pr.Diagnostics) > 0 {
		t.Fatalf("parser diagnostics: %+v", pr.Diagnostics)
	}
	cr := Compile(pr.File)
	found := false
	for _, d := range cr.Diagnostics {
		if d.Code == DiagUnknownSecret {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DiagUnknownSecret (C093), got %+v", cr.Diagnostics)
	}
}

func TestSecretsDuplicateDiagnostic(t *testing.T) {
	src := `
secrets:
  dup: "${A_TEST}"
  dup: "${B_TEST}"

agent a:
  model: "anthropic/c"
  system: p

prompt p:
  hi

workflow w:
  entry: a
  a -> done
`
	pr := parser.Parse("t.iter", src)
	cr := Compile(pr.File)
	found := false
	for _, d := range cr.Diagnostics {
		if d.Code == DiagDuplicateSecret {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DiagDuplicateSecret (C090), got %+v", cr.Diagnostics)
	}
}
