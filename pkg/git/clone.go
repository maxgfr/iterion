package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ShallowClone clones url into dest with --depth 1 --single-branch. When ref
// is non-empty it is passed as --branch (a branch name or tag). dest must not
// already exist (git clone requires an absent target). The clone is shallow —
// enough to read a bot bundle, not its history. Network access and
// authentication are git's responsibility: the host's configured credential
// helpers and SSH keys apply, exactly as for a manual `git clone`.
//
// url is gated by ValidateCloneSource: only https:// and ssh git URLs are
// accepted, so dangerous transports (ext::, file://, …) are rejected before
// git runs. The `--` sentinel below is kept as additional flag-injection
// defense in depth — it is NOT a transport check.
func ShallowClone(ctx context.Context, url, ref, dest string) error {
	if err := ValidateCloneSource(url); err != nil {
		return err
	}
	args := []string{"clone", "--depth", "1", "--single-branch"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "--", url, dest)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitEnv()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w (stderr: %s)", url, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
