package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/SocialGouv/iterion/pkg/sandbox"
	"github.com/SocialGouv/iterion/pkg/sandbox/registry"
)

// SandboxDoctorOptions tunes `iterion sandbox doctor`. The zero value
// reproduces the historical host-only report.
type SandboxDoctorOptions struct {
	// Strict turns on the pre-run validation battery (driver liveness,
	// image resolvability, k8s compatibility, network syntax, …) and
	// makes the command exit non-zero on any failure.
	Strict bool
	// File is an optional workflow (.bot/.botz/dir) whose sandbox:
	// block is resolved and validated. Empty runs host-level checks only.
	File string
	// Sandbox / SandboxDefaultImage / SandboxHostState mirror the
	// `iterion run` flags so the doctor validates the EXACT spec a run
	// with the same flags would use.
	Sandbox             string
	SandboxDefaultImage string
	SandboxHostState    string
	// Target selects the check battery: "auto" (follow the selected
	// driver, default), "cloud" (force the kubernetes/host-independent
	// battery — validate a cloud workflow from a laptop), or "local"
	// (force docker).
	Target string
}

// RunSandboxDoctor implements `iterion sandbox doctor`. Without
// [SandboxDoctorOptions.Strict] it prints the host-only report
// (unchanged from prior releases). With Strict it resolves the effective
// sandbox spec and runs the full pre-flight battery, returning a non-nil
// error (non-zero exit) when any check fails.
func RunSandboxDoctor(ctx context.Context, p *Printer, opts SandboxDoctorOptions) error {
	if opts.Strict {
		return runSandboxDoctorStrict(ctx, p, opts)
	}
	return runSandboxDoctorBasic(p)
}

// runSandboxDoctorBasic prints diagnostics about the sandbox subsystem.
//
// The output covers:
//
//   - the detected host kind (local / desktop / cloud) and how it was
//     determined (env var vs heuristic);
//   - which container runtime, if any, is on the user's PATH;
//   - the registered sandbox drivers in preference order;
//   - the driver the factory would pick for a run started from this
//     environment, plus its advertised capabilities;
//   - the global sandbox default (ITERION_SANDBOX_DEFAULT) — the lowest
//     precedence layer that all workflows inherit from.
//
// Output is human-readable by default; pass --json on the parent
// command to switch to JSON. Phase 0 prints stable string keys; Phase
// 1+ may extend the JSON shape — keys present today will not change.
func runSandboxDoctorBasic(p *Printer) error {
	factory := sandbox.NewFactory(sandbox.FactoryOptions{
		AvailableDrivers: defaultDriverRegistry(),
	})

	driver, driverErr := factory.Driver()
	caps := sandbox.Capabilities{}
	driverName := "<none>"
	if driver != nil {
		driverName = driver.Name()
		caps = driver.Capabilities()
	}

	containerRuntime := sandbox.HostHasDocker()
	if containerRuntime == "" {
		containerRuntime = "<none>"
	}

	defaultMode := strings.ToLower(os.Getenv("ITERION_SANDBOX_DEFAULT"))
	if defaultMode == "" {
		defaultMode = "<unset>"
	}

	report := map[string]any{
		"host":              string(factory.Host()),
		"os":                runtime.GOOS,
		"arch":              runtime.GOARCH,
		"container_runtime": containerRuntime,
		"sandbox_default":   defaultMode,
		"selected_driver":   driverName,
		"available_drivers": factory.Available(),
		"capabilities": map[string]bool{
			"image":          caps.SupportsImage,
			"build":          caps.SupportsBuild,
			"mounts":         caps.SupportsMounts,
			"network_policy": caps.SupportsNetworkPolicy,
			"post_create":    caps.SupportsPostCreate,
			"remote_user":    caps.SupportsRemoteUser,
		},
	}
	if driverErr != nil {
		report["driver_error"] = driverErr.Error()
	}

	if p.Format == OutputJSON {
		p.JSON(report)
		return nil
	}

	fmt.Fprintln(p.W, "iterion sandbox — doctor report")
	fmt.Fprintln(p.W, "===============================")
	fmt.Fprintf(p.W, "  host                : %s\n", report["host"])
	fmt.Fprintf(p.W, "  platform            : %s/%s\n", report["os"], report["arch"])
	fmt.Fprintf(p.W, "  container runtime   : %s\n", report["container_runtime"])
	fmt.Fprintf(p.W, "  ITERION_SANDBOX_DEFAULT : %s\n", report["sandbox_default"])
	fmt.Fprintf(p.W, "  available drivers   : %s\n", strings.Join(factory.Available(), ", "))
	fmt.Fprintf(p.W, "  selected driver     : %s\n", driverName)
	if driverErr != nil {
		fmt.Fprintf(p.W, "  driver error        : %v\n", driverErr)
	}
	fmt.Fprintln(p.W, "  capabilities:")
	fmt.Fprintf(p.W, "    image          : %v\n", caps.SupportsImage)
	fmt.Fprintf(p.W, "    build          : %v\n", caps.SupportsBuild)
	fmt.Fprintf(p.W, "    mounts         : %v\n", caps.SupportsMounts)
	fmt.Fprintf(p.W, "    network policy : %v\n", caps.SupportsNetworkPolicy)
	fmt.Fprintf(p.W, "    post create    : %v\n", caps.SupportsPostCreate)
	fmt.Fprintf(p.W, "    remote user    : %v\n", caps.SupportsRemoteUser)
	fmt.Fprintln(p.W)
	if driverName == "noop" {
		fmt.Fprintln(p.W, "Note: the noop driver is the safe fallback. Workflows that declare")
		fmt.Fprintln(p.W, "an active sandbox mode will run on the host with a sandbox_skipped event")
		fmt.Fprintln(p.W, "in events.jsonl. Install Docker or Podman locally to enable container")
		fmt.Fprintln(p.W, "isolation, or run iterion in cloud mode for k8s-native isolation.")
	}
	return nil
}

// defaultDriverRegistry forwards to [registry.Default] so the CLI
// and the runtime engine share a single source of truth.
func defaultDriverRegistry() map[string]sandbox.DriverConstructor {
	return registry.Default()
}
