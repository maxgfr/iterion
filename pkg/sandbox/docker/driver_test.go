package docker

import (
	"context"
	"testing"

	"github.com/SocialGouv/iterion/pkg/sandbox"
)

func TestCapabilitiesAdvertisedFeatures(t *testing.T) {
	d := &Driver{rt: RuntimeDocker}
	caps := d.Capabilities()

	if !caps.SupportsImage {
		t.Error("docker driver must advertise SupportsImage")
	}
	if !caps.SupportsMounts {
		t.Error("docker driver must advertise SupportsMounts")
	}
	if !caps.SupportsPostCreate {
		t.Error("docker driver must advertise SupportsPostCreate")
	}
	if !caps.SupportsRemoteUser {
		t.Error("docker driver must advertise SupportsRemoteUser")
	}
	if !caps.SupportsBuild {
		t.Error("V2-6 docker driver must advertise SupportsBuild (docker buildx)")
	}
	// Phase 1 explicitly defers these.
	if caps.SupportsNetworkPolicy {
		t.Error("Phase 1 docker driver must NOT advertise SupportsNetworkPolicy")
	}
}

func TestPrepareAcceptsBuild(t *testing.T) {
	d := &Driver{rt: RuntimeDocker}
	prepared, err := d.Prepare(context.Background(), sandbox.Spec{
		Mode:  sandbox.ModeInline,
		Build: &sandbox.Build{Dockerfile: "Dockerfile"},
		User:  "1000:1000",
	})
	if err != nil {
		t.Fatalf("Prepare must accept sandbox.Build (V2-6): %v", err)
	}
	if prepared == nil {
		t.Fatal("expected non-nil PreparedSpec")
	}
}

func TestPrepareRejectsMissingImage(t *testing.T) {
	d := &Driver{rt: RuntimeDocker}
	_, err := d.Prepare(context.Background(), sandbox.Spec{Mode: sandbox.ModeInline})
	if err == nil {
		t.Fatal("expected rejection of inline spec without image")
	}
}

func TestPrepareValidatesSpec(t *testing.T) {
	d := &Driver{rt: RuntimeDocker}
	_, err := d.Prepare(context.Background(), sandbox.Spec{Mode: sandbox.Mode("nonsense")})
	if err == nil {
		t.Fatal("expected validation error for invalid mode")
	}
}

func TestContainerNameDeterministic(t *testing.T) {
	a := containerNameFor("run_123")
	b := containerNameFor("run_123")
	if a != b {
		t.Errorf("containerNameFor not deterministic: %q vs %q", a, b)
	}
	if a != "iterion-run_123" {
		t.Errorf("name = %q, want iterion-run_123", a)
	}
}

func TestContainerNameTruncatesLong(t *testing.T) {
	long := "run_" + string(make([]byte, 200))
	for i := range long[4:] {
		long = long[:4+i] + "x" + long[4+i+1:]
	}
	got := containerNameFor(long)
	if len(got) > 64 {
		t.Errorf("name %q exceeds 64 chars", got)
	}
}

func TestContainerShortIDTruncates(t *testing.T) {
	full := "abcdef0123456789abcdef"
	got := containerShortID(full)
	if got != "abcdef012345" {
		t.Errorf("shortID = %q, want abcdef012345", got)
	}
}

func TestContainerShortIDPassesThroughShort(t *testing.T) {
	got := containerShortID("abc")
	if got != "abc" {
		t.Errorf("shortID = %q, want abc", got)
	}
}

func TestStartRejectsForeignPreparedSpec(t *testing.T) {
	d := &Driver{rt: RuntimeDocker}
	type otherPrepared struct{ name string }
	_ = otherPrepared{}
	// Use a noop-shaped PreparedSpec — DriverName != docker, must reject.
	_, err := d.Start(context.Background(), foreignPrepared{}, sandbox.RunInfo{})
	if err == nil {
		t.Fatal("expected rejection of foreign PreparedSpec")
	}
}

type foreignPrepared struct{}

func (foreignPrepared) DriverName() string { return "not-docker" }

// The docker driver must bind the network proxy on 0.0.0.0, not the
// engine-default 127.0.0.1, because the container reaches the proxy via
// host.docker.internal (resolved to host-gateway, e.g. 172.17.0.1) —
// not the host's loopback. A 127.0.0.1 bind silently lands every
// outbound HTTPS_PROXY connection in ECONNREFUSED, which surfaces in
// claude as "API Error: Unable to connect to API (ConnectionRefused)".
func TestProxyConfigBindsAllInterfaces(t *testing.T) {
	d := &Driver{rt: RuntimeDocker}
	bind, advertise, err := d.ProxyConfig()
	if err != nil {
		t.Fatalf("ProxyConfig: %v", err)
	}
	if bind != "0.0.0.0:0" {
		t.Errorf("bind = %q, want 0.0.0.0:0 (loopback bind is unreachable from container via host.docker.internal)", bind)
	}
	if advertise != "host.docker.internal" {
		t.Errorf("advertise = %q, want host.docker.internal", advertise)
	}
}

// Run.Command must add `docker exec --interactive` whenever Stdin is
// attached OR KeepStdinOpen is set. The latter exists for callers that
// build the *exec.Cmd here and then attach stdin via cmd.StdinPipe()
// after the fact (claudesdk Session via CommandBuilder); without the
// flag, docker closes the child's stdin before the pipe is wired and
// the in-container CLI exits silently with no result.
func TestRunCommandKeepStdinOpenAddsInteractiveFlag(t *testing.T) {
	d := &Driver{rt: RuntimeDocker}
	r := &Run{
		driver:      d,
		containerID: "deadbeef",
		prepared: &Prepared{
			workspace: "/workspace",
			spec:      sandbox.Spec{},
			runtime:   RuntimeDocker,
		},
	}

	cases := []struct {
		name string
		opts sandbox.ExecOpts
		want bool
	}{
		{"empty opts", sandbox.ExecOpts{}, false},
		{"keep stdin open", sandbox.ExecOpts{KeepStdinOpen: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := r.Command(context.Background(), []string{"true"}, tc.opts)
			got := false
			for _, a := range cmd.Args {
				if a == "--interactive" {
					got = true
					break
				}
			}
			if got != tc.want {
				t.Errorf("--interactive present=%v, want %v; args=%v", got, tc.want, cmd.Args)
			}
		})
	}
}
