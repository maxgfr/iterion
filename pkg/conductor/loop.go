package conductor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
)

// tick is the conductor's heartbeat. Runs entirely on the actor
// goroutine: reconciles stalled runs, refreshes tracker states for
// running issues, then dispatches new candidates until slots fill up.
func (c *Conductor) tick(ctx context.Context) {
	cfg := c.cfg.Load()

	c.reconcileStalled(ctx, cfg)
	c.refreshRunningStates(ctx)

	candidates, err := c.tracker.ListCandidates(ctx)
	if err != nil {
		c.logger.Warn("conductor: tracker ListCandidates: %v", err)
		c.fireSnapshot()
		return
	}
	sortCandidates(candidates)

	for _, iss := range candidates {
		if !c.hasSlot(iss.WorkflowState, cfg) {
			break
		}
		if c.state.isClaimed(iss.ID) {
			continue
		}
		c.dispatch(ctx, iss)
	}
	c.fireSnapshot()
}

func sortCandidates(in []tracker.Issue) {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Priority != in[j].Priority {
			return in[i].Priority > in[j].Priority
		}
		if !in[i].CreatedAt.Equal(in[j].CreatedAt) {
			return in[i].CreatedAt.Before(in[j].CreatedAt)
		}
		return in[i].Identifier < in[j].Identifier
	})
}

// reconcileStalled cancels any run whose LastEventAt is older than the
// configured stall timeout. The dispatch goroutine will eventually post
// cmdRunFinished with context.Canceled, which schedules the retry.
func (c *Conductor) reconcileStalled(_ context.Context, cfg *Config) {
	timeout := cfg.StallTimeout()
	if timeout <= 0 {
		return
	}
	now := time.Now()
	for id, r := range c.state.running {
		if now.Sub(r.LastEventAt) > timeout {
			c.logger.Warn("conductor: %s stalled (no event for %s) — cancelling", r.Identifier, now.Sub(r.LastEventAt))
			if r.Cancel != nil {
				r.Cancel()
			}
			_ = id // keep entry; cmdRunFinished will remove it
		}
	}
}

// refreshRunningStates queries the tracker for each running issue and
// cancels any whose state moved out of the eligible set externally
// (operator closed it, blocker added, …).
func (c *Conductor) refreshRunningStates(ctx context.Context) {
	if len(c.state.running) == 0 {
		return
	}
	ids := make([]string, 0, len(c.state.running))
	for id := range c.state.running {
		ids = append(ids, id)
	}
	states, err := c.tracker.RefreshStates(ctx, ids)
	if err != nil {
		c.logger.Warn("conductor: tracker RefreshStates: %v", err)
		return
	}
	for _, id := range ids {
		r := c.state.running[id]
		newState, ok := states[id]
		if !ok {
			c.logger.Info("conductor: %s disappeared from tracker — cancelling", r.Identifier)
			if r.Cancel != nil {
				r.Cancel()
			}
			continue
		}
		if newState != r.WorkflowState {
			c.logger.Info("conductor: %s moved %s → %s externally — cancelling", r.Identifier, r.WorkflowState, newState)
			if r.Cancel != nil {
				r.Cancel()
			}
		}
	}
}

func (c *Conductor) hasSlot(state string, cfg *Config) bool {
	if len(c.state.running) >= cfg.Agent.MaxConcurrent {
		return false
	}
	if cap, ok := cfg.Agent.MaxConcurrentByState[state]; ok {
		if c.state.slotsByState[state] >= cap {
			return false
		}
	}
	return true
}

// dispatch claims the issue, allocates a workspace, and spawns the
// worker goroutine. Runs on the actor goroutine — must be fast.
func (c *Conductor) dispatch(ctx context.Context, iss tracker.Issue) {
	cfg := c.cfg.Load()
	if err := c.tracker.Claim(ctx, iss.ID, c.hostMarker); err != nil {
		if errors.Is(err, tracker.ErrClaimConflict) {
			c.logger.Info("conductor: %s already claimed elsewhere, skipping", iss.Identifier)
			return
		}
		c.logger.Warn("conductor: claim %s: %v", iss.Identifier, err)
		return
	}

	wsPath, created, err := c.workspaces.Create(iss.ID)
	if err != nil {
		c.logger.Warn("conductor: workspace create %s: %v", iss.Identifier, err)
		_ = c.tracker.Release(ctx, iss.ID, c.hostMarker)
		return
	}

	attempt := 0
	if cur, ok := c.state.retries[iss.ID]; ok {
		attempt = cur.Attempt
	}
	runID := newRunID(iss.ID, attempt)
	runCtx, cancel := context.WithCancel(ctx)

	entry := &runningEntry{
		IssueID:       iss.ID,
		Identifier:    iss.Identifier,
		RunID:         runID,
		WorkflowState: iss.WorkflowState,
		WorkspacePath: wsPath,
		StartedAt:     time.Now().UTC(),
		LastEventAt:   time.Now().UTC(),
		Attempt:       attempt,
		Cancel:        cancel,
		issueSnapshot: iss,
	}
	c.state.running[iss.ID] = entry
	c.state.slotsByState[iss.WorkflowState]++

	spec := c.buildSpec(cfg, iss, runID, wsPath, attempt)

	c.logger.Info("conductor: dispatching %s → run=%s (attempt=%d, workspace=%s)", iss.Identifier, runID, attempt, wsPath)

	go c.runWorker(runCtx, entry, created, spec)
}

func (c *Conductor) buildSpec(cfg *Config, iss tracker.Issue, runID, wsPath string, attempt int) DispatchSpec {
	tplCtx := TemplateContext{
		Issue: iss,
		Conductor: ConductorVars{
			Name:          cfg.Name,
			RunID:         runID,
			WorkspacePath: wsPath,
			Attempt:       attempt,
		},
	}
	vars := map[string]any{}
	for k, src := range cfg.Dispatch.Vars {
		tpl, err := ParseTemplate(src)
		if err != nil {
			c.logger.Warn("conductor: dispatch.vars[%s]: %v", k, err)
			continue
		}
		v, err := tpl.Render(tplCtx)
		if err != nil {
			c.logger.Warn("conductor: render dispatch.vars[%s]: %v", k, err)
			continue
		}
		vars[k] = v
	}
	attachments := map[string]any{}
	for k, src := range cfg.Dispatch.Attachments {
		tpl, err := ParseTemplate(src)
		if err != nil {
			c.logger.Warn("conductor: dispatch.attachments[%s]: %v", k, err)
			continue
		}
		v, err := tpl.Render(tplCtx)
		if err != nil {
			c.logger.Warn("conductor: render dispatch.attachments[%s]: %v", k, err)
			continue
		}
		attachments[k] = v
	}
	return DispatchSpec{
		WorkflowPath:  cfg.Workflow,
		RunID:         runID,
		WorkspacePath: wsPath,
		StoreDir:      c.storeDir,
		Vars:          vars,
		Attachments:   attachments,
		OnEvent: func(name string) {
			select {
			case c.cmds <- cmdEvent{issueID: iss.ID, eventName: name}:
			case <-c.stop:
			}
		},
	}
}

// runWorker is the dispatch goroutine. Runs all hooks and the workflow,
// then posts cmdRunFinished. Hook failures fail the run.
//
// All sends on c.cmds are guarded by c.stop so a shutdown that races
// a late-finishing worker doesn't leak a goroutine blocked forever on
// a full channel.
func (c *Conductor) runWorker(ctx context.Context, entry *runningEntry, created bool, spec DispatchSpec) {
	env := c.dispatchEnv(entry, spec)

	if created && c.hooks.AfterCreate != nil {
		if err := c.hooks.AfterCreate.Run(ctx, c.logger, "after_create", entry.WorkspacePath, env); err != nil {
			c.postFinished(entry.IssueID, fmt.Errorf("after_create hook: %w", err))
			return
		}
	}
	if err := c.hooks.BeforeRun.Run(ctx, c.logger, "before_run", entry.WorkspacePath, env); err != nil {
		c.postFinished(entry.IssueID, fmt.Errorf("before_run hook: %w", err))
		return
	}

	dispatchErr := c.runner.Dispatch(ctx, spec)

	// after_run is best-effort: log failures but don't override the
	// dispatch result.
	if err := c.hooks.AfterRun.Run(ctx, c.logger, "after_run", entry.WorkspacePath, env); err != nil {
		c.logger.Warn("conductor: after_run hook for %s: %v", entry.Identifier, err)
	}

	c.postFinished(entry.IssueID, dispatchErr)
}

func (c *Conductor) postFinished(issueID string, err error) {
	select {
	case c.cmds <- cmdRunFinished{issueID: issueID, err: err}:
	case <-c.stop:
	}
}

func (c *Conductor) dispatchEnv(entry *runningEntry, spec DispatchSpec) []string {
	env := []string{
		"ITERION_ISSUE_ID=" + entry.IssueID,
		"ITERION_ISSUE_IDENTIFIER=" + entry.Identifier,
		"ITERION_ISSUE_STATE=" + entry.WorkflowState,
		"ITERION_RUN_ID=" + spec.RunID,
		"ITERION_WORKSPACE=" + spec.WorkspacePath,
	}
	if spec.StoreDir != "" {
		env = append(env, "ITERION_STORE_DIR="+spec.StoreDir)
	}
	return env
}

// newRunID produces a deterministic-ish, sortable run ID for dispatch.
func newRunID(issueID string, attempt int) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			return r
		}
		return '_'
	}, issueID)
	return fmt.Sprintf("conductor-%s-%d-%d", clean, attempt, time.Now().UnixMilli())
}
