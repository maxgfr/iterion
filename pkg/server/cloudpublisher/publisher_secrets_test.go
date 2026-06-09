package cloudpublisher

import (
	"context"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	"github.com/SocialGouv/iterion/pkg/secrets"
	"github.com/SocialGouv/iterion/pkg/store"
)

func TestResolveAndSealCredentials_GenericWorkflowSecrets(t *testing.T) {
	sealer, err := secrets.NewAESGCMSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	genericStore := secrets.NewMemoryGenericSecretStore()
	secretID := secrets.NewGenericSecretID()
	sealed, err := secrets.SealGenericSecret(sealer, secretID, []byte("apiVersion: v1"))
	if err != nil {
		t.Fatalf("SealGenericSecret: %v", err)
	}
	if err := genericStore.Create(context.Background(), secrets.GenericSecret{
		ID:           secretID,
		ScopeTeamID:  "team",
		ScopeUserID:  "alice",
		Name:         "kubeconfig",
		SealedSecret: sealed,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	runSecrets := secrets.NewMemoryRunSecretsStore()
	p := &Publisher{
		genericSecrets: genericStore,
		runSecrets:     runSecrets,
		sealer:         sealer,
	}
	wf := &ir.Workflow{Secrets: map[string]*ir.Secret{
		"kubeconfig": {As: "file"},
	}}
	ctx := store.WithTenant(context.Background(), "team")
	ref, err := p.resolveAndSealCredentials(ctx, "run-1", "team", "alice", wf)
	if err != nil {
		t.Fatalf("resolveAndSealCredentials: %v", err)
	}
	if ref == "" {
		t.Fatal("expected secrets ref")
	}
	rec, err := runSecrets.Get(ctx, ref)
	if err != nil {
		t.Fatalf("RunSecrets.Get: %v", err)
	}
	bundle, err := secrets.OpenRunBundle(sealer, "run-1", rec.SealedBundle)
	if err != nil {
		t.Fatalf("OpenRunBundle: %v", err)
	}
	if bundle.GenericSecrets["kubeconfig"] != "apiVersion: v1" {
		t.Fatalf("GenericSecrets not sealed into bundle: %+v", bundle.GenericSecrets)
	}
}
