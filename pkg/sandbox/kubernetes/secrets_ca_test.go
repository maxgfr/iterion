package kubernetes

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/sandbox"
)

func TestBuildPodManifest_CAInjection(t *testing.T) {
	out, err := BuildPodManifest(PodManifestInput{
		Namespace:    "ns",
		Name:         "iterion-run-x",
		RunID:        "x",
		Spec:         sandbox.Spec{Image: "img"},
		CASecretName: "iterion-run-x-ca",
	})
	if err != nil {
		t.Fatalf("BuildPodManifest: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "CURL_CA_BUNDLE",
		"GIT_SSL_CAINFO", "REQUESTS_CA_BUNDLE",
		caContainerPath, caMountDir, "iterion-egress-ca", "iterion-run-x-ca",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("CA-injected manifest missing %q:\n%s", want, s)
		}
	}

	// No CA wiring when no secret is configured.
	bare, err := BuildPodManifest(PodManifestInput{
		Namespace: "ns", Name: "iterion-run-y", Spec: sandbox.Spec{Image: "img"},
	})
	if err != nil {
		t.Fatalf("BuildPodManifest (bare): %v", err)
	}
	if strings.Contains(string(bare), "NODE_EXTRA_CA_CERTS") || strings.Contains(string(bare), "iterion-egress-ca") {
		t.Errorf("CA wiring leaked into a manifest with no CA secret:\n%s", bare)
	}
}

func TestBuildCASecret(t *testing.T) {
	pem := []byte("-----BEGIN CERTIFICATE-----\nMIIBdummy\n-----END CERTIFICATE-----\n")
	out, err := BuildCASecret("ns", "iterion-run-x-ca", "x", "friendly", pem)
	if err != nil {
		t.Fatalf("BuildCASecret: %v", err)
	}
	var sec map[string]any
	if err := json.Unmarshal(out, &sec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sec["kind"] != "Secret" || sec["type"] != "Opaque" {
		t.Errorf("unexpected kind/type: %v / %v", sec["kind"], sec["type"])
	}
	data, _ := sec["data"].(map[string]any)
	b64, _ := data[caSecretKey].(string)
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("data not base64: %v", err)
	}
	if string(dec) != string(pem) {
		t.Errorf("decoded CA data mismatch:\n got %q\nwant %q", dec, pem)
	}
}
