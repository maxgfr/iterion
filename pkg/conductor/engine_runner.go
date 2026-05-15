package conductor

import (
	"context"
	"fmt"
	"io"

	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/runtime"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/store"
)

// EngineRunner is the production Runner: each Dispatch compiles a
// fresh executor for the requested RunID and drives the iterion
// runtime engine until completion or cancellation.
type EngineRunner struct {
	workflow     *ir.Workflow
	workflowPath string
	workflowHash string
	logger       *iterlog.Logger
}

// NewEngineRunner pre-compiles the workflow at workflowPath. The
// resulting EngineRunner can dispatch concurrently across many issues
// using the same compiled IR.
func NewEngineRunner(workflowPath string, logger *iterlog.Logger) (*EngineRunner, error) {
	if workflowPath == "" {
		return nil, fmt.Errorf("engine runner: workflow path required")
	}
	wf, hash, err := runview.CompileWorkflowWithHash(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("engine runner: compile %s: %w", workflowPath, err)
	}
	return &EngineRunner{
		workflow:     wf,
		workflowPath: workflowPath,
		workflowHash: hash,
		logger:       logger,
	}, nil
}

// Workflow returns the compiled IR. Useful for callers that want to
// validate dispatch.vars keys against the workflow's declared vars at
// startup.
func (r *EngineRunner) Workflow() *ir.Workflow { return r.workflow }

// Dispatch implements Runner. Opens the store, builds an executor for
// this RunID, registers an event observer that bridges every event to
// spec.OnEvent, and drives engine.Run to completion.
func (r *EngineRunner) Dispatch(ctx context.Context, spec DispatchSpec) error {
	if spec.StoreDir == "" {
		return fmt.Errorf("engine runner: spec.StoreDir is required")
	}
	s, err := store.New(spec.StoreDir, store.WithLogger(r.logger))
	if err != nil {
		return fmt.Errorf("engine runner: open store: %w", err)
	}

	exec, err := runview.BuildExecutor(runview.ExecutorSpec{
		Ctx:      ctx,
		Workflow: r.workflow,
		Store:    s,
		RunID:    spec.RunID,
		Logger:   r.logger,
		StoreDir: spec.StoreDir,
	})
	if err != nil {
		return fmt.Errorf("engine runner: build executor: %w", err)
	}
	if c, ok := any(exec).(io.Closer); ok {
		defer func() {
			if cerr := c.Close(); cerr != nil {
				r.logger.Warn("engine runner: executor close: %v", cerr)
			}
		}()
	}

	eng := runtime.New(r.workflow, s, exec,
		runtime.WithLogger(r.logger),
		runtime.WithWorkflowHash(r.workflowHash),
		runtime.WithFilePath(r.workflowPath),
		runtime.WithRunName(spec.RunID),
		runtime.WithEventObserver(func(evt store.Event) {
			if spec.OnEvent != nil {
				spec.OnEvent(string(evt.Type))
			}
		}),
	)

	return eng.Run(ctx, spec.RunID, spec.Vars)
}
