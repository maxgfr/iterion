package secrets

import (
	"context"
	"testing"
	"time"
)

func mkGenericSecret(t *testing.T, store *MemoryGenericSecretStore, sealer Sealer, team, user, name, value string) GenericSecret {
	t.Helper()
	id := NewGenericSecretID()
	sealed, err := SealGenericSecret(sealer, id, []byte(value))
	if err != nil {
		t.Fatalf("SealGenericSecret: %v", err)
	}
	rec := GenericSecret{
		ID:           id,
		ScopeTeamID:  team,
		ScopeUserID:  user,
		Name:         name,
		Last4:        Last4(value),
		SealedSecret: sealed,
		CreatedAt:    time.Now().UTC(),
		Fingerprint:  FingerprintSHA256(value),
	}
	if err := store.Create(context.Background(), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return rec
}

func TestResolveGeneric_PrioritizesUserOverTeam(t *testing.T) {
	store := NewMemoryGenericSecretStore()
	sealer := newSealer(t)
	mkGenericSecret(t, store, sealer, "team", "", "kubeconfig", "team-secret")
	user := mkGenericSecret(t, store, sealer, "team", "alice", "kubeconfig", "user-secret")

	got, err := ResolveGeneric(context.Background(), store, "team", "alice", []string{"kubeconfig"}, sealer)
	if err != nil {
		t.Fatalf("ResolveGeneric: %v", err)
	}
	r, ok := got["kubeconfig"]
	if !ok {
		t.Fatal("no kubeconfig resolution")
	}
	if r.SecretID != user.ID || string(r.Plaintext) != "user-secret" || r.SourceScope != "user" {
		t.Fatalf("expected user secret, got %+v", r)
	}
}

func TestResolveGeneric_FallsBackToTeam(t *testing.T) {
	store := NewMemoryGenericSecretStore()
	sealer := newSealer(t)
	team := mkGenericSecret(t, store, sealer, "team", "", "deploy_key", "team-secret")

	got, err := ResolveGeneric(context.Background(), store, "team", "bob", []string{"deploy_key"}, sealer)
	if err != nil {
		t.Fatalf("ResolveGeneric: %v", err)
	}
	r := got["deploy_key"]
	if r.SecretID != team.ID || string(r.Plaintext) != "team-secret" || r.SourceScope != "team" {
		t.Fatalf("expected team secret, got %+v", r)
	}
}

func TestGenericSecretAADBound(t *testing.T) {
	sealer := newSealer(t)
	sealed, err := SealGenericSecret(sealer, "a", []byte("payload"))
	if err != nil {
		t.Fatalf("SealGenericSecret: %v", err)
	}
	if _, err := OpenGenericSecret(sealer, "b", sealed); err == nil {
		t.Fatal("expected opening with a different secret id to fail")
	}
}

func TestRunBundleCarriesGenericSecrets(t *testing.T) {
	sealer := newSealer(t)
	sealed, err := SealRunBundle(sealer, "run-1", RunBundle{
		GenericSecrets: map[string]string{"kubeconfig": "payload"},
	})
	if err != nil {
		t.Fatalf("SealRunBundle: %v", err)
	}
	got, err := OpenRunBundle(sealer, "run-1", sealed)
	if err != nil {
		t.Fatalf("OpenRunBundle: %v", err)
	}
	if got.GenericSecrets["kubeconfig"] != "payload" {
		t.Fatalf("GenericSecrets not preserved: %+v", got.GenericSecrets)
	}
}
