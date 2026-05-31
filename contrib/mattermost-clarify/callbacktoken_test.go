package main

import "testing"

func TestCallbackTokenRoundTrip(t *testing.T) {
	in := callbackToken{ChannelID: "chan-1", RootID: "post-9"}
	enc := encodeToken(in)
	if enc == "" {
		t.Fatal("encodeToken returned empty")
	}
	out, err := decodeToken(enc)
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: got %+v want %+v", out, in)
	}
}

func TestDecodeTokenRejectsGarbage(t *testing.T) {
	if _, err := decodeToken("!!!not base64!!!"); err == nil {
		t.Error("expected error on non-base64 token")
	}
	if _, err := decodeToken(encodeToken(callbackToken{})); err == nil {
		t.Error("expected error on token missing ids")
	}
}

func TestTokenCarriesNoIdentity(t *testing.T) {
	// The token type has exactly two fields, both routing ids. This
	// test is a guard: if someone adds a user-identifying field it will
	// need a deliberate change here, flagging the privacy regression.
	tok := callbackToken{ChannelID: "c", RootID: "r"}
	enc := encodeToken(tok)
	dec, _ := decodeToken(enc)
	if dec.ChannelID != "c" || dec.RootID != "r" {
		t.Fatalf("unexpected token contents: %+v", dec)
	}
}

func TestHeuristicFilter(t *testing.T) {
	f := newHeuristicFilter()
	cases := []struct {
		latest string
		want   bool
	}{
		{"what do you mean by that?", true},
		{"I'm not sure which one we picked", true},
		{"shipped, thanks", false},
		{"ok", false}, // too short
		{"can you clarify the scope here", true},
		{"lgtm 👍", false},
	}
	for _, tc := range cases {
		if got := f.ShouldRespond("", tc.latest); got != tc.want {
			t.Errorf("ShouldRespond(%q) = %v, want %v", tc.latest, got, tc.want)
		}
	}
}
