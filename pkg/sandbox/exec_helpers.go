package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// ExecCmd runs a prepared [*exec.Cmd], wiring stdout/stderr per opts and
// returning a normalised [ExecResult].
//
// Driver implementations of [Run.Exec] typically construct the [*exec.Cmd]
// via their own [Run.Command] (which knows how to wrap docker/kubectl)
// and then defer to this helper to share the buffer-vs-stream plumbing,
// exit-code extraction, and the convention that a non-zero exit code
// surfaces via [ExecResult.ExitCode] (not via a returned error).
//
// Empty stdout/stderr in opts → bytes are buffered into the result.
// Non-nil opts.Stdout / opts.Stderr → bytes are streamed to the writer
// and the result's Stdout/Stderr remain nil.
//
// Errors other than [*exec.ExitError] are returned verbatim. ExitError
// is normalised to (result with ExitCode set, nil error) so callers
// can branch on res.ExitCode without a type assertion.
func ExecCmd(c *exec.Cmd, opts ExecOpts) (ExecResult, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	if opts.Stdout != nil {
		c.Stdout = opts.Stdout
	} else {
		c.Stdout = &stdoutBuf
	}
	if opts.Stderr != nil {
		c.Stderr = opts.Stderr
	} else {
		c.Stderr = &stderrBuf
	}

	err := c.Run()
	res := ExecResult{}
	if c.ProcessState != nil {
		res.ExitCode = c.ProcessState.ExitCode()
	}
	if opts.Stdout == nil {
		res.Stdout = stdoutBuf.Bytes()
	}
	if opts.Stderr == nil {
		res.Stderr = stderrBuf.Bytes()
	}
	if _, isExit := err.(*exec.ExitError); isExit {
		err = nil
	}
	return res, err
}

// RunPostCreate executes a `sh -c <snippet>` inside the given [Run] and
// returns a friendly error on non-zero exit (with stdout/stderr
// embedded for diagnostics). Used by drivers that honour
// [Spec.PostCreate] — currently docker and kubernetes.
//
// When logger is non-nil the snippet's stdout and stderr are also
// streamed to the logger as Info / Warn lines so operators see
// install progress live in run.log instead of waiting for a terminal
// failure to dump a buffered blob. Pass nil to suppress streaming.
func RunPostCreate(ctx context.Context, run Run, snippet string, logger *iterlog.Logger) error {
	opts := ExecOpts{}
	var bufOut, bufErr bytes.Buffer
	if logger != nil {
		// Stream + buffer in parallel so the on-error message keeps the
		// embedded captures it always had.
		opts.Stdout = io.MultiWriter(&bufOut, &lineLogger{l: logger, level: "info"})
		opts.Stderr = io.MultiWriter(&bufErr, &lineLogger{l: logger, level: "warn"})
	}
	res, err := run.Exec(ctx, []string{"sh", "-c", snippet}, opts)
	if err != nil {
		return fmt.Errorf("postCreateCommand: %w", err)
	}
	stdout := res.Stdout
	stderr := res.Stderr
	if logger != nil {
		stdout = bufOut.Bytes()
		stderr = bufErr.Bytes()
	}
	if res.ExitCode != 0 {
		return fmt.Errorf(
			"postCreateCommand exited %d:\nstdout:\n%s\nstderr:\n%s",
			res.ExitCode, string(stdout), string(stderr),
		)
	}
	return nil
}

// lineLogger fans bytes into a leveled iterlog logger one line at a time.
// Designed for live-streaming subprocess output where each newline
// completes a logical message. Partial trailing lines are buffered
// until the next newline arrives or Close() is called (we don't call
// Close because Run.Exec terminates the writer anyway when the process
// exits).
type lineLogger struct {
	l     *iterlog.Logger
	level string
	buf   bytes.Buffer
}

func (lw *lineLogger) Write(p []byte) (int, error) {
	lw.buf.Write(p)
	for {
		idx := bytes.IndexByte(lw.buf.Bytes(), '\n')
		if idx < 0 {
			break
		}
		line := string(lw.buf.Next(idx + 1))
		line = line[:len(line)-1] // drop newline
		if line == "" {
			continue
		}
		switch lw.level {
		case "warn":
			lw.l.Warn("sandbox: postCreate: %s", line)
		default:
			lw.l.Info("sandbox: postCreate: %s", line)
		}
	}
	return len(p), nil
}
