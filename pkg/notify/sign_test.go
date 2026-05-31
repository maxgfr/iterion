package notify

import "testing"

func TestSignDeterministicAndPrefixed(t *testing.T) {
	body := []byte(`{"run_id":"r1"}`)
	a := Sign("s3cr3t", body)
	b := Sign("s3cr3t", body)
	if a != b {
		t.Fatalf("Sign not deterministic: %q vs %q", a, b)
	}
	if a == "" || a[:7] != "sha256=" {
		t.Fatalf("missing sha256= prefix: %q", a)
	}
}

func TestSignEmptySecretDisables(t *testing.T) {
	if got := Sign("", []byte("x")); got != "" {
		t.Errorf("empty secret should yield empty signature, got %q", got)
	}
}

func TestVerifyRoundTrip(t *testing.T) {
	body := []byte(`{"v":1,"status":"finished"}`)
	sig := Sign("topsecret", body)
	if !Verify("topsecret", body, sig) {
		t.Error("Verify rejected a valid signature")
	}
}

func TestVerifyRejects(t *testing.T) {
	body := []byte("payload")
	good := Sign("k", body)

	cases := []struct {
		name   string
		secret string
		body   []byte
		header string
	}{
		{"wrong secret", "other", body, good},
		{"tampered body", "k", []byte("payload!"), good},
		{"empty header", "k", body, ""},
		{"garbage header", "k", body, "sha256=zzzz"},
		{"no prefix", "k", body, good[7:]},
		{"empty secret rejects even matching", "", body, Sign("", body)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if Verify(tc.secret, tc.body, tc.header) {
				t.Errorf("Verify accepted an invalid input")
			}
		})
	}
}

func TestVerifyToleratesWhitespace(t *testing.T) {
	body := []byte("b")
	sig := Sign("k", body)
	if !Verify("k", body, "  "+sig+"  ") {
		t.Error("Verify should trim surrounding whitespace in the header")
	}
}
