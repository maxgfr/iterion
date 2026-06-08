package secretguard

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestMaterializeForHost_Scoping(t *testing.T) {
	const real = "ghp_REAL0123456789abcdefABCDEF0123456789"
	g := New([]Secret{
		{Name: "gh", Value: real, Placeholder: "__ITERION_SECRET_gh__", Hosts: []string{"api.github.com"}},
	}, DefaultConfig())
	ph := "__ITERION_SECRET_gh__"

	// Approved host (and parent-domain): substitute.
	if got := g.MaterializeForHost("Bearer "+ph, "api.github.com"); got != "Bearer "+real {
		t.Errorf("approved host not materialised: %q", got)
	}
	// Unapproved host: placeholder left intact.
	if got := g.MaterializeForHost("Bearer "+ph, "evil.com"); got != "Bearer "+ph {
		t.Errorf("unapproved host should not materialise: %q", got)
	}
}

func TestExfiltratesTo_Gate(t *testing.T) {
	const real = "ghp_REAL0123456789abcdefABCDEF0123456789"
	g := New([]Secret{
		{Name: "gh", Value: real, Placeholder: "__ITERION_SECRET_gh__", Hosts: []string{"github.com"}},
	}, DefaultConfig())

	// Real value to an unapproved host → exfiltration.
	if !g.ExfiltratesTo("Authorization: Bearer "+real, "evil.com") {
		t.Error("real value to unapproved host should be flagged")
	}
	// Parent-domain match: api.github.com is permitted by github.com.
	if g.ExfiltratesTo("Bearer "+real, "api.github.com") {
		t.Error("approved (parent-domain) host must not be flagged")
	}
	// base64-encoded value to an unapproved host → still caught.
	enc := base64.StdEncoding.EncodeToString([]byte(real))
	if !g.ExfiltratesTo("blob="+enc, "evil.com") {
		t.Error("base64-encoded value should be caught by DLP")
	}
}

func TestHostScoping_UnrestrictedSecret(t *testing.T) {
	const real = "sk-MODELKEY-0123456789abcdefABCDEF"
	// Empty Hosts = unrestricted (e.g. the model API key).
	g := New([]Secret{
		{Name: "model", Value: real, Placeholder: "__ITERION_SECRET_model__"},
	}, DefaultConfig())

	// Materialises anywhere; never flagged as exfiltration.
	if got := g.MaterializeForHost("__ITERION_SECRET_model__", "anything.example"); !strings.Contains(got, real) {
		t.Errorf("unrestricted secret should materialise anywhere: %q", got)
	}
	if g.ExfiltratesTo(real, "anything.example") {
		t.Error("unrestricted secret must never be flagged as exfiltration")
	}
}
