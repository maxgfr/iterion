package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/runtime"
	"github.com/SocialGouv/iterion/pkg/store"
)

// ResumeOptions holds the configuration for the resume command.
type ResumeOptions struct {
	RunID       string
	StoreDir    string
	AnswersFile string            // path to JSON answers file
	Answers     map[string]string // --answer key=value overrides
	LogLevel    string            // log level (default: "info", env: ITERION_LOG_LEVEL)
	Force       bool              // allow resume despite workflow hash change
	Executor    runtime.NodeExecutor
	// Background marks this invocation as a managed-runner subprocess
	// spawned by the studio server. The CLI writes a .pid file so the
	// server can detect liveness across its own restart.
	Background bool
}

// RunResumeWithFile resumes a paused run using a workflow file and answers.
// iterFile is optional: when empty, the run's persisted FilePath (recorded at
// launch — for inline launches, this is the server's inline-source cache
// path) is used. This lets the CLI resume an inline-launched run without
// the caller re-supplying the source.
func RunResumeWithFile(ctx context.Context, iterFile string, opts ResumeOptions, p *Printer) error {
	if opts.RunID == "" {
		return fmt.Errorf("--run-id is required")
	}

	level, err := iterlog.ResolveLevel(opts.LogLevel, "ITERION_LOG_LEVEL")
	if err != nil {
		return err
	}
	logger := iterlog.New(level, os.Stderr)

	// When --file is omitted, the store dir cannot be discovered from its
	// parent; the caller must pass --store-dir or be in a directory whose
	// ancestor contains a .iterion.
	storeAnchor := filepath.Dir(iterFile)
	if iterFile == "" {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			storeAnchor = cwd
		}
	}
	storeDir := store.ResolveStoreDir(storeAnchor, opts.StoreDir)

	// Tee log output to run.log so the studio's Logs panel sees
	// output for resumed runs. Resume re-uses the same file via
	// O_APPEND so the original run.log + resume sessions stack into
	// one timeline, matching what the daemon-launched path produces.
	logger, logCloser := teeRunLog(logger, level, filepath.Join(storeDir, "runs", opts.RunID))
	if logCloser != nil {
		defer logCloser.Close()
	}

	s, err := store.New(storeDir, store.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("cannot open store: %w", err)
	}

	r, err := s.LoadRun(context.Background(), opts.RunID)
	if err != nil {
		return fmt.Errorf("cannot load run: %w", err)
	}

	// Fall back to the FilePath persisted at launch when --file was omitted.
	// Inline-launched runs have this pointing at the server's inline-source
	// cache, so resume replays the exact bytes the run was launched with.
	if iterFile == "" {
		iterFile = r.FilePath
		if iterFile == "" {
			return fmt.Errorf("--file is required: run %q has no persisted workflow path", opts.RunID)
		}
	}

	resumingFromFailure := false
	switch r.Status {
	case store.RunStatusPausedWaitingHuman:
		// OK — requires answers
	case store.RunStatusFailedResumable, store.RunStatusCancelled:
		resumingFromFailure = true
	default:
		return fmt.Errorf("run %q cannot be resumed (status: %s)", opts.RunID, r.Status)
	}

	answers, err := buildResumeAnswers(opts, resumingFromFailure)
	if err != nil {
		return err
	}

	wf, wfHash, iterFile, bundleHandle, bundleCleanup, err := resumeOpenWorkflow(r, iterFile)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := bundleCleanup(); cerr != nil {
			logger.Warn("bundle cleanup: %v", cerr)
		}
	}()

	executor, err := buildResumeExecutor(opts, wf, s, storeDir, logger, r)
	if err != nil {
		return err
	}

	eng := runtime.New(wf, s, executor,
		runtime.WithLogger(logger),
		runtime.WithWorkflowHash(wfHash),
		runtime.WithFilePath(iterFile),
		runtime.WithForceResume(opts.Force),
		runtime.WithBundle(bundleHandle),
		runtime.WithPreset(r.Preset),
	)

	// Acquire exclusive run lock to prevent concurrent processes.
	// Use the SIGINT-aware ctx so a contended lock can still be
	// interrupted by Ctrl-C rather than blocking forever.
	lock, err := s.LockRun(ctx, opts.RunID)
	if err != nil {
		return fmt.Errorf("cannot acquire run lock: %w", err)
	}
	defer lock.Unlock()

	// Managed-runner mode: the studio server writes the .pid file on
	// our behalf at spawn time, so we only need to remove it on exit.
	if opts.Background {
		defer func() {
			if rmErr := s.RemovePIDFile(opts.RunID); rmErr != nil {
				logger.Warn("background: remove .pid: %v", rmErr)
			}
		}()
	}

	// Re-check run status under the lock to prevent a TOCTOU race
	// against a concurrent process that flipped the run to terminal.
	r, err = s.LoadRun(context.Background(), opts.RunID)
	if err != nil {
		return fmt.Errorf("cannot reload run: %w", err)
	}
	if r.Status != store.RunStatusPausedWaitingHuman && r.Status != store.RunStatusFailedResumable && r.Status != store.RunStatusCancelled {
		return fmt.Errorf("run %q can no longer be resumed (status: %s)", opts.RunID, r.Status)
	}

	if p.Format == OutputHuman {
		p.Header("Resume: " + opts.RunID)
		if r.Name != "" {
			p.KV("Name", r.Name)
		}
		p.KV("Workflow", wf.Name)
		if r.Checkpoint != nil {
			p.KV("Node", r.Checkpoint.NodeID)
		}
		if resumingFromFailure {
			p.KV("Resuming from", "failed (re-executing failed node)")
			if r.Error != "" {
				p.KV("Previous error", r.Error)
			}
		}
		p.KV("Log Level", level.String())
		p.Blank()
	}

	err = eng.Resume(ctx, opts.RunID, answers)
	return reportResumeOutcome(p, s, opts.RunID, err, map[string]interface{}{
		"run_id":   opts.RunID,
		"workflow": wf.Name,
	})
}

// ParseAnswerFlags parses a slice of "key=value" strings into a map.
func ParseAnswerFlags(flags []string) (map[string]string, error) {
	answers := make(map[string]string)
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --answer format %q (expected key=value)", f)
		}
		answers[parts[0]] = parts[1]
	}
	return answers, nil
}
