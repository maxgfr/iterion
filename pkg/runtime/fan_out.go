package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/SocialGouv/iterion/pkg/backend/model"
	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	"github.com/SocialGouv/iterion/pkg/store"
)

// ---------------------------------------------------------------------------
// Fan-out / Join — parallel branch scheduler
// ---------------------------------------------------------------------------

// execFanOut handles a fan_out_all router node by spawning parallel
// branches for each outgoing edge, bounded by MaxParallelBranches.
// It returns the next node ID to continue from (after the join).
func (e *Engine) execFanOut(ctx context.Context, rs *runState, routerNodeID string) (string, error) {
	// Emit router node_started.
	if err := e.emit(rs.ctx, rs.runID, store.EventNodeStarted, routerNodeID, map[string]interface{}{
		"kind":      "router",
		"mode":      "fan_out_all",
		"iteration": e.currentLoopIteration(routerNodeID, rs.loopCounters),
	}); err != nil {
		return "", err
	}

	// Router is a pass-through: its output = its input from incoming edges.
	routerInput := e.buildNodeInputRS(routerNodeID, rs.vars, rs.outputs, rs.runInputs, rs.artifacts, rs)
	rs.outputs[routerNodeID] = routerInput

	// Emit router node_finished.
	if err := e.emit(rs.ctx, rs.runID, store.EventNodeFinished, routerNodeID, nil); err != nil {
		return "", err
	}

	// Collect all outgoing edges from the router.
	var fanEdges []*ir.Edge
	for _, edge := range e.workflow.Edges {
		if edge.From == routerNodeID {
			fanEdges = append(fanEdges, edge)
		}
	}
	if len(fanEdges) == 0 {
		return "", fmt.Errorf("fan_out_all router %q has no outgoing edges", routerNodeID)
	}

	// Validate workspace safety: at most one mutating branch in parallel.
	if err := e.validateWorkspaceSafety(routerNodeID, fanEdges); err != nil {
		return "", err
	}

	// Determine concurrency limit from budget.
	maxParallel := len(fanEdges)
	if e.workflow.Budget != nil && e.workflow.Budget.MaxParallelBranches > 0 && e.workflow.Budget.MaxParallelBranches < maxParallel {
		maxParallel = e.workflow.Budget.MaxParallelBranches
	}

	// Pre-compute convergence point so branches know where to stop.
	preComputedConvergence := e.findConvergencePoint(routerNodeID, fanEdges)

	// Decide the sibling-cancellation policy. Under wait_all (the default
	// strategy at the convergence node) any branch failure dooms the
	// whole run — so once one branch errors we should cancel siblings to
	// stop them spending tokens/USD on work whose result will be
	// discarded. Under best_effort, sibling failures are tolerated and
	// the convergence aggregator can still consume successful branches,
	// so we MUST NOT cancel siblings on a peer failure (only on budget
	// exhaustion or parent ctx cancellation, which apply globally).
	cancelOnFirstFailure := true
	if preComputedConvergence != "" {
		if convNode, ok := e.workflow.Nodes[preComputedConvergence]; ok {
			if mode := nodeAwaitMode(convNode); mode == ir.AwaitBestEffort {
				cancelOnFirstFailure = false
			}
		}
	}

	// Deep-copy parent outputs and artifacts so branches can't mutate shared state.
	parentOutputs := copyOutputs(rs.outputs)
	parentArtifacts := copyOutputs(rs.artifacts)

	// Derive a cancellable context for the whole fan-out. When any branch
	// trips the budget (or the parent ctx is cancelled — Ctrl-C), we cancel
	// branchCtx so siblings stop racking up tokens/USD on subsequent LLM
	// calls. Without this, a fan_out_all with N branches and a $10 cap would
	// burn N * $10 in the worst case before stopping.
	branchCtx, cancelBranches := context.WithCancel(ctx)
	defer cancelBranches()

	// Launch branches with bounded concurrency.
	sem := make(chan struct{}, maxParallel)
	resultsCh := make(chan *branchResult, len(fanEdges))

	for _, edge := range fanEdges {
		branchID := fmt.Sprintf("branch_%s_%s", routerNodeID, edge.To)

		go func(edge *ir.Edge, branchID string) {
			// Register the panic-recovery defer FIRST, before the
			// semaphore acquire — otherwise a panic between the goroutine
			// starting and the recover() defer being registered (e.g. if
			// sem is ever closed externally) would be unrecoverable.
			defer func() {
				if r := recover(); r != nil {
					resultsCh <- &branchResult{
						branchID: branchID,
						outputs:  make(map[string]map[string]interface{}),
						err:      fmt.Errorf("panic in branch %s: %v", branchID, r),
					}
				}
			}()
			// Acquire a semaphore slot, but bail if the fan-out is already
			// cancelled (budget trip, sibling failure with wait_all, or
			// parent cancel) — otherwise a branch queued behind
			// maxParallel would block here waiting for a slot held by a
			// branch wedged in executor.Execute, even though its result is
			// already doomed. Emitting a cancelled result keeps the
			// collector's count balanced; it's equivalent to what
			// execBranch returns at its top-of-loop ctx check, minus the
			// wait. (No branch_started was emitted yet, so there's no
			// started/finished imbalance.)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }() // release
			case <-branchCtx.Done():
				resultsCh <- &branchResult{
					branchID: branchID,
					outputs:  make(map[string]map[string]interface{}),
					err:      e.wrapContextErr(branchCtx.Err()),
				}
				return
			}

			result := e.execBranch(branchCtx, rs, branchID, edge, parentOutputs, parentArtifacts, preComputedConvergence)
			// Cancel siblings to stop them calling executor.Execute when:
			//   1. this branch tripped the global budget — applies to
			//      every fan_out regardless of await mode; or
			//   2. this branch failed for any other reason AND the
			//      convergence is wait_all — siblings' results would be
			//      discarded anyway, so paying for them is pure waste.
			// Sibling branches see the cancellation via the ctx.Done()
			// select at the top of their per-iteration loop and return
			// a wrapped ctx error.
			if result != nil && result.err != nil {
				if errors.Is(result.err, ErrBudgetExceeded) || cancelOnFirstFailure {
					cancelBranches()
				}
			}
			resultsCh <- result
		}(edge, branchID)
	}

	// Collect all results. The collector is ctx-aware: if the parent ctx
	// fires (run cancellation, timeout) we still need to drain branches that
	// already started but won't honour cancellation immediately (e.g. a
	// claude_code subprocess that swallows SIGINT). To avoid leaking
	// goroutines, we keep waiting on resultsCh — but we record the
	// cancellation so the final aggregate error reflects it.
	results := make([]*branchResult, 0, len(fanEdges))
	var ctxErr error
	for i := 0; i < len(fanEdges); i++ {
		select {
		case r := <-resultsCh:
			results = append(results, r)
		case <-ctx.Done():
			if ctxErr == nil {
				ctxErr = ctx.Err()
				cancelBranches()
			}
			// Re-receive on resultsCh; we MUST drain to avoid goroutine
			// leaks (a goroutine blocked on `resultsCh <- result` would
			// otherwise leak). The buffered channel of size len(fanEdges)
			// guarantees no producer ever blocks on send.
			results = append(results, <-resultsCh)
		}
	}
	if ctxErr != nil {
		return "", e.wrapContextErr(ctxErr)
	}

	// Determine convergence point. Prefer the one reported by successful branches;
	// if all branches failed, discover it from the graph topology.
	//
	// Under best_effort, branches may legitimately end at different
	// nodes — e.g. one branch hits a fail node, another times out and
	// is cancelled, a third completes. We keep the first non-empty
	// joinNodeID and log the divergence rather than aborting the whole
	// fan-out (which would discard the successful branches and is
	// exactly the failure mode best_effort exists to avoid).
	convergenceNodeID := ""
	isBestEffort := !cancelOnFirstFailure
	for _, r := range results {
		if r.joinNodeID == "" {
			continue
		}
		if convergenceNodeID == "" {
			convergenceNodeID = r.joinNodeID
			continue
		}
		if convergenceNodeID != r.joinNodeID {
			if isBestEffort {
				if e.logger != nil {
					e.logger.Warn("fan_out from %s: branches converge to different nodes (%s vs %s in branch %s) — best_effort, keeping first",
						routerNodeID, convergenceNodeID, r.joinNodeID, r.branchID)
				}
				continue
			}
			return "", fmt.Errorf("branches converge to different nodes: %s vs %s", convergenceNodeID, r.joinNodeID)
		}
	}
	if convergenceNodeID == "" {
		// All-done topology under best_effort: every branch ran to its
		// own *ir.DoneNode without sharing a convergence point and no
		// branch failed. Hand a terminal node ID back to the engine's
		// main loop so it routes to run_finished — falling through to
		// preComputedConvergence (empty here, since the branches diverge)
		// would synthesize a "no convergence point" error and waste the
		// successful work.
		if isBestEffort && allTerminatedAtDone(results) {
			return e.processConvergenceTerminal(rs, results)
		}
		// All branches failed before reaching convergence. Use the
		// pre-computed convergence point (already computed above).
		convergenceNodeID = preComputedConvergence
		if convergenceNodeID == "" {
			return "", fmt.Errorf("no convergence point found after fan_out from %s", routerNodeID)
		}
	}

	// Process convergence.
	return e.processConvergence(rs, convergenceNodeID, results)
}

// allTerminatedAtDone reports whether every branch finished cleanly at
// an *ir.DoneNode. Branches with err != nil count as non-terminating —
// best_effort tolerates them but the all-done shortcut requires every
// branch to have produced a terminal exit.
func allTerminatedAtDone(results []*branchResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, r := range results {
		if r.err != nil || !r.terminatedAtDone || r.terminalNodeID == "" {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Branch execution
// ---------------------------------------------------------------------------

// branchResult holds the outcome of a single parallel branch.
type branchResult struct {
	branchID         string
	outputs          map[string]map[string]interface{}
	artifacts        map[string]map[string]interface{} // publish name → output
	artifactVersions map[string]int
	joinNodeID       string // the join node this branch converged to (empty if terminal)
	err              error
	eventErrors      int // count of event emission failures (best-effort events)
	// terminatedAtDone is true when the branch loop exited at an
	// *ir.DoneNode rather than at a convergence/error/cancel. Used by
	// best_effort fan_out to recognise the "every branch finished at
	// its own done" topology — without this flag the post-loop
	// convergence search would fail because joinNodeID is empty on each
	// branch and preComputedConvergence is also empty (the branches
	// diverge to distinct terminals).
	terminatedAtDone bool
	terminalNodeID   string // the *ir.DoneNode ID when terminatedAtDone
}

// execBranch runs a single parallel branch starting from the target of
// the given edge. It executes nodes sequentially until it reaches a
// convergence point, a terminal node, or encounters an error.
// convergenceNodeID is the pre-computed convergence point (may be empty
// if unknown; in that case, AwaitMode on individual nodes is checked).
func (e *Engine) execBranch(ctx context.Context, rs *runState, branchID string, startEdge *ir.Edge, parentOutputs map[string]map[string]interface{}, parentArtifacts map[string]map[string]interface{}, convergenceNodeID string) *branchResult {
	// Copy parent artifact versions so branches continue incrementing from
	// the correct version, rather than resetting to 0 on each fan-out cycle.
	branchArtifactVersions := make(map[string]int, len(rs.artifactVersions))
	for k, v := range rs.artifactVersions {
		branchArtifactVersions[k] = v
	}

	result := &branchResult{
		branchID:         branchID,
		outputs:          make(map[string]map[string]interface{}),
		artifacts:        make(map[string]map[string]interface{}),
		artifactVersions: branchArtifactVersions,
	}

	runID := rs.runID
	vars := rs.vars
	runInputs := rs.runInputs

	// branchCostUSD is this branch's cumulative LLM spend. It is recorded
	// into the shared daily-cap ledger under a per-branch key
	// ("<runID>#<branchID>") rather than the bare runID: branches run
	// concurrently, and the ledger's AddSpend keys by a single ID with
	// monotonic-max, so two branches recording under the same runID would
	// clobber each other (only the largest single-branch cumulative would
	// survive). Distinct keys are summed into the day total, so parallel
	// branch spend aggregates correctly and stays idempotent on resume
	// (re-running a branch overwrites its own key monotonically). Without
	// this, all fan-out spend escapes the per-day cap entirely.
	var branchCostUSD float64

	// Emit branch_started (best-effort — branch can proceed without the event).
	if err := e.emitBranch(ctx, runID, branchID, store.EventBranchStarted, startEdge.To, nil); err != nil {
		e.logger.Warn("branch %s: failed to emit branch_started: %v", branchID, err)
		result.eventErrors++
	}

	// Always emit branch_finished, regardless of how the branch exits.
	// Observers (e.g. the Prometheus exporter's parallel-branches gauge)
	// rely on the started/finished pair to track in-flight concurrency.
	defer func() {
		data := map[string]interface{}{}
		if result.err != nil {
			data["error"] = result.err.Error()
		}
		if result.joinNodeID != "" {
			data["join_node"] = result.joinNodeID
		}
		if err := e.emitBranch(ctx, runID, branchID, store.EventBranchFinished, startEdge.To, data); err != nil {
			e.logger.Warn("branch %s: failed to emit branch_finished: %v", branchID, err)
			result.eventErrors++
		}
	}()

	currentNodeID := startEdge.To

	for {
		select {
		case <-ctx.Done():
			result.err = e.wrapContextErr(ctx.Err())
			return result
		default:
		}

		node, ok := e.workflow.Nodes[currentNodeID]
		if !ok {
			result.err = fmt.Errorf("node %q not found in branch %s", currentNodeID, branchID)
			return result
		}

		// Stop at convergence point — the branch has reached the
		// pre-computed node where parallel branches reconverge.
		if convergenceNodeID != "" && currentNodeID == convergenceNodeID {
			result.joinNodeID = currentNodeID
			return result
		}

		// Stop at terminal nodes within a branch.
		switch node.(type) {
		case *ir.DoneNode:
			result.terminatedAtDone = true
			result.terminalNodeID = currentNodeID
			return result
		case *ir.FailNode:
			result.err = fmt.Errorf("branch %s reached fail node %q", branchID, currentNodeID)
			return result
		}

		// Check budget before execution.
		if rs.budget != nil {
			checks := rs.budget.Check()
			if exc := findExceeded(checks); exc != nil {
				if err := e.emitBranch(ctx, runID, branchID, store.EventBudgetExceeded, currentNodeID, map[string]interface{}{
					"dimension": exc.dimension,
					"used":      exc.used,
					"limit":     exc.limit,
				}); err != nil {
					e.logger.Warn("branch %s: failed to emit budget_exceeded: %v", branchID, err)
					result.eventErrors++
				}
				result.err = fmt.Errorf("%w: %s (%.0f/%.0f)", ErrBudgetExceeded, exc.dimension, exc.used, exc.limit)
				return result
			}
			if hl := findHardLimited(checks); hl != nil {
				if err := e.emitBranch(ctx, runID, branchID, store.EventBudgetExceeded, currentNodeID, map[string]interface{}{
					"dimension":  hl.dimension,
					"used":       hl.used,
					"limit":      hl.limit,
					"hard_limit": true,
				}); err != nil {
					e.logger.Warn("branch %s: failed to emit budget hard limit: %v", branchID, err)
					result.eventErrors++
				}
				result.err = fmt.Errorf("%w: hard limit %s at %.0f%% (%.0f/%.0f)", ErrBudgetExceeded, hl.dimension, (hl.used/hl.limit)*100, hl.used, hl.limit)
				return result
			}
		}

		// Loop edges inside fan-out branches are skipped (see helpers.go),
		// so iteration here reflects the parent loop counters only.
		iter := e.currentLoopIteration(currentNodeID, rs.loopCounters)

		// Emit node_started.
		if err := e.emitBranch(ctx, runID, branchID, store.EventNodeStarted, currentNodeID, map[string]interface{}{
			"kind":      node.NodeKind().String(),
			"iteration": iter,
		}); err != nil {
			e.logger.Warn("branch %s: failed to emit node_started: %v", branchID, err)
			result.eventErrors++
		}

		// Build input: merge parent outputs with branch-local outputs so
		// refs to upstream nodes (before the router) still resolve. We
		// pass the parent rs so {{loop.*}} / {{run.*}} read the parent's
		// loop snapshots (read-only — branches never traverse loop edges).
		merged := mergeOutputs(parentOutputs, result.outputs)
		mergedArt := mergeOutputs(parentArtifacts, result.artifacts)
		nodeInput := e.buildNodeInputRS(currentNodeID, vars, merged, runInputs, mergedArt, rs)

		execCtx := model.WithLoopIteration(ctx, iter)
		output, err := e.executor.Execute(execCtx, node, nodeInput)
		if err != nil {
			result.err = fmt.Errorf("node %q in branch %s: %w", currentNodeID, branchID, err)
			if emitErr := e.emitBranch(ctx, runID, branchID, store.EventNodeFinished, currentNodeID, map[string]interface{}{
				"error": err.Error(),
			}); emitErr != nil {
				e.logger.Warn("branch %s: failed to emit node_finished: %v", branchID, emitErr)
				result.eventErrors++
			}
			return result
		}

		result.outputs[currentNodeID] = output

		// Validate output against declared schema (optional).
		if err := e.validateNodeOutput(currentNodeID, node, output); err != nil {
			result.err = fmt.Errorf("node %q in branch %s: %w", currentNodeID, branchID, err)
			return result
		}

		// Record budget usage and check limits.
		tokens, costUSD := extractUsage(output)

		// Daily spend-cap accounting for this branch. Independent of the
		// per-run budget so it works for workflows with no `budget:` block.
		// The pause decision still happens on the trunk's pre-exec path
		// (checkBudgetBeforeExec) so the checkpoint anchors at a not-yet-
		// executed node; branches only contribute spend. The per-branch
		// ledger key keeps concurrent branches from clobbering each other
		// (see branchCostUSD above).
		if e.dailyCap != nil && costUSD > 0 {
			branchCostUSD += costUSD
			if _, err := e.dailyCap.Record(ctx, runID+"#"+branchID, branchCostUSD); err != nil {
				e.logger.Warn("branch %s: daily spend cap record failed: %v", branchID, err)
			}
		}

		if rs.budget != nil {
			checks := rs.budget.RecordUsage(tokens, costUSD)

			// Emit warnings.
			for _, w := range findWarnings(checks) {
				if err := e.emitBranch(ctx, runID, branchID, store.EventBudgetWarning, currentNodeID, map[string]interface{}{
					"dimension": w.dimension,
					"used":      w.used,
					"limit":     w.limit,
				}); err != nil {
					e.logger.Warn("branch %s: failed to emit budget_warning: %v", branchID, err)
					result.eventErrors++
				}
			}

			// Fail on exceeded.
			if exc := findExceeded(checks); exc != nil {
				if err := e.emitBranch(ctx, runID, branchID, store.EventBudgetExceeded, currentNodeID, map[string]interface{}{
					"dimension": exc.dimension,
					"used":      exc.used,
					"limit":     exc.limit,
				}); err != nil {
					e.logger.Warn("branch %s: failed to emit budget_exceeded: %v", branchID, err)
					result.eventErrors++
				}
				result.err = fmt.Errorf("%w: %s (%.0f/%.0f)", ErrBudgetExceeded, exc.dimension, exc.used, exc.limit)
				return result
			}
		}

		// Persist artifact if node has publish.
		if pub := nodePublish(node); pub != "" {
			version := result.artifactVersions[currentNodeID]
			artifact := &store.Artifact{
				RunID:   runID,
				NodeID:  currentNodeID,
				Version: version,
				Data:    output,
			}
			if err := e.store.WriteArtifact(ctx, artifact); err != nil {
				result.err = fmt.Errorf("node %q in branch %s: write artifact: %w", currentNodeID, branchID, err)
				return result
			}
			result.artifactVersions[currentNodeID] = version + 1
			result.artifacts[pub] = output
			if err := e.emitBranch(ctx, runID, branchID, store.EventArtifactWritten, currentNodeID, map[string]interface{}{
				"publish": pub,
				"version": version,
			}); err != nil {
				e.logger.Warn("branch %s: failed to emit artifact_written: %v", branchID, err)
				result.eventErrors++
			}
		}

		// Emit node_finished with usage data.
		if err := e.emitBranch(ctx, runID, branchID, store.EventNodeFinished, currentNodeID, buildNodeFinishedData(e.sanitizeOutputForEvent(node, output))); err != nil {
			e.logger.Warn("branch %s: failed to emit node_finished: %v", branchID, err)
			result.eventErrors++
		}
		if e.onNodeFinished != nil {
			e.onNodeFinished(runID, currentNodeID, output)
		}

		// Select next edge (branch-local, no loop counters needed in branches).
		nextNodeID, err := e.selectEdgeBranch(ctx, runID, branchID, currentNodeID, output)
		if err != nil {
			result.err = err
			return result
		}

		currentNodeID = nextNodeID
	}
}

// selectEdgeBranch picks the next node for a branch. It is simpler than
// selectEdge: no loop counter enforcement, events carry a branch ID.
func (e *Engine) selectEdgeBranch(ctx context.Context, runID, branchID, fromNodeID string, output map[string]interface{}) (string, error) {
	selected := e.evaluateEdges(fromNodeID, fmt.Sprintf("branch %s", branchID), output)
	if selected == nil {
		return "", fmt.Errorf("no outgoing edge from node %q in branch %s", fromNodeID, branchID)
	}

	if err := e.emitBranch(ctx, runID, branchID, store.EventEdgeSelected, "", map[string]interface{}{
		"from": selected.From,
		"to":   selected.To,
	}); err != nil {
		e.logger.Warn("branch %s: failed to emit edge_selected: %v", branchID, err)
	}

	return selected.To, nil
}

// ---------------------------------------------------------------------------
// Convergence / Join
// ---------------------------------------------------------------------------

// processConvergence aggregates branch results according to the convergence
// node's await strategy, merges outputs into the run state, builds the
// convergence node's input from multi-edge with-mappings, and returns
// the convergence node ID for the main loop to continue execution.
func (e *Engine) processConvergence(rs *runState, convergenceNodeID string, results []*branchResult) (string, error) {
	convNode, ok := e.workflow.Nodes[convergenceNodeID]
	if !ok {
		return "", fmt.Errorf("convergence node %q not found", convergenceNodeID)
	}

	// Determine await strategy: use node's explicit setting, default to wait_all.
	strategy := nodeAwaitMode(convNode)
	if strategy == ir.AwaitNone {
		strategy = ir.AwaitWaitAll
	}

	// Collect failed branches metadata.
	var failedBranches []map[string]interface{}
	for _, r := range results {
		if r.err != nil {
			failedBranches = append(failedBranches, map[string]interface{}{
				"branch_id": r.branchID,
				"error":     r.err.Error(),
			})
		}
	}

	// Apply await strategy.
	switch strategy {
	case ir.AwaitWaitAll:
		if len(failedBranches) > 0 {
			return "", fmt.Errorf("convergence at %s (wait_all): %d branch(es) failed: %v",
				convergenceNodeID, len(failedBranches), failedBranches[0]["error"])
		}
	case ir.AwaitBestEffort:
		// Proceed even with failures — failed branch metadata is exposed.
	}

	// Merge successful branch outputs into the run state.
	for _, r := range results {
		if r.err != nil {
			continue
		}
		for nodeID, output := range r.outputs {
			rs.outputs[nodeID] = output
		}
		for name, output := range r.artifacts {
			rs.artifacts[name] = output
		}
		for nodeID, version := range r.artifactVersions {
			rs.artifactVersions[nodeID] = version
		}
	}

	// Add failed branches metadata to outputs so it's available via with-mappings.
	if len(failedBranches) > 0 {
		// Expose as a special output on the convergence node.
		if rs.outputs[convergenceNodeID] == nil {
			rs.outputs[convergenceNodeID] = make(map[string]interface{})
		}
		rs.outputs[convergenceNodeID]["_failed_branches"] = failedBranches
	}

	// Emit convergence_ready event.
	convData := map[string]interface{}{
		"strategy": strategy.String(),
	}
	if len(failedBranches) > 0 {
		convData["failed_branches"] = failedBranches
	}
	if err := e.emit(rs.ctx, rs.runID, store.EventJoinReady, convergenceNodeID, convData); err != nil {
		e.logger.Warn("failed to emit convergence_ready: %v", err)
	}

	// Return the convergence node ID — the main loop will execute it normally.
	return convergenceNodeID, nil
}

// processConvergenceTerminal handles the best_effort all-done topology
// (every branch ran to its own *ir.DoneNode and no branch failed).
// Merges branch outputs/artifacts into the run state and hands back one
// of the terminal node IDs so the engine's main loop emits run_finished.
func (e *Engine) processConvergenceTerminal(rs *runState, results []*branchResult) (string, error) {
	for _, r := range results {
		for nodeID, output := range r.outputs {
			rs.outputs[nodeID] = output
		}
		for name, output := range r.artifacts {
			rs.artifacts[name] = output
		}
		for nodeID, version := range r.artifactVersions {
			rs.artifactVersions[nodeID] = version
		}
	}
	// Use the first branch's terminal node — the engine treats any Done
	// node as run_finished, so picking one is unambiguous.
	terminal := results[0].terminalNodeID
	if err := e.emit(rs.ctx, rs.runID, store.EventJoinReady, terminal, map[string]interface{}{
		"strategy":       ir.AwaitBestEffort.String(),
		"terminal_join":  true,
		"branches_total": len(results),
	}); err != nil {
		e.logger.Warn("failed to emit terminal convergence join_ready: %v", err)
	}
	return terminal, nil
}

// findConvergencePoint walks outgoing edges from the router's targets to
// find a downstream convergence point (a node with AwaitMode != AwaitNone,
// or a node that receives edges from multiple distinct sources).
// Terminal nodes (done/fail) can be convergence points when multiple
// branches target them directly.
// This is also called pre-emptively before branches start so that each
// branch knows where to stop.
func (e *Engine) findConvergencePoint(routerNodeID string, fanEdges []*ir.Edge) string {
	// Build in-degree map: count distinct sources per target.
	inSources := make(map[string]map[string]bool)
	for _, edge := range e.workflow.Edges {
		if _, ok := inSources[edge.To]; !ok {
			inSources[edge.To] = make(map[string]bool)
		}
		inSources[edge.To][edge.From] = true
	}

	// BFS from each fan-out target to find a convergence point.
	// maxVisits guards against a malformed graph where a cycle slipped
	// past compile-time validation (C012/C013): without it the queue
	// could grow without bound. Cap at the workflow's node count —
	// any honest BFS visits each node at most once.
	maxVisits := len(e.workflow.Nodes) + 1
	for _, startEdge := range fanEdges {
		visited := map[string]bool{}
		queue := []string{startEdge.To}
		for len(queue) > 0 {
			if len(visited) > maxVisits {
				if e.logger != nil {
					e.logger.Warn("findConvergencePoint: BFS exceeded %d visits — likely an undetected graph cycle, aborting search", maxVisits)
				}
				break
			}
			nodeID := queue[0]
			queue = queue[1:]
			if visited[nodeID] {
				continue
			}
			visited[nodeID] = true

			node, ok := e.workflow.Nodes[nodeID]
			if !ok {
				continue
			}
			// Convergence point: explicitly marked OR has multiple distinct incoming sources.
			if nodeAwaitMode(node) != ir.AwaitNone || len(inSources[nodeID]) > 1 {
				return nodeID
			}
			// Follow outgoing edges.
			for _, edge := range e.workflow.Edges {
				if edge.From == nodeID {
					queue = append(queue, edge.To)
				}
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Output copy helpers
// ---------------------------------------------------------------------------

// mergeOutputs creates a merged view of parent and branch outputs.
// Branch outputs take precedence over parent outputs.
func mergeOutputs(parent, branch map[string]map[string]interface{}) map[string]map[string]interface{} {
	merged := make(map[string]map[string]interface{}, len(parent)+len(branch))
	for k, v := range parent {
		merged[k] = v
	}
	for k, v := range branch {
		merged[k] = v
	}
	return merged
}

// copyOutputs creates a deep copy of the outputs map so that concurrent
// branches cannot mutate shared parent state. Naive two-level copying
// (the previous implementation) left nested maps and slices aliased
// between branches: a fan-out where two branches both received an
// upstream output containing a nested map would race on that map's
// internal hashtable.
func copyOutputs(src map[string]map[string]interface{}) map[string]map[string]interface{} {
	dst := make(map[string]map[string]interface{}, len(src))
	for k, v := range src {
		inner := make(map[string]interface{}, len(v))
		for ik, iv := range v {
			inner[ik] = deepCopyValue(iv)
		}
		dst[k] = inner
	}
	return dst
}

// deepCopyValue recursively copies a value tree of the shapes produced
// by JSON unmarshalling (map[string]interface{}, []interface{}, plus
// scalars). Other concrete types pass through unchanged — the runtime
// only stores JSON-shaped values in node outputs, so this covers the
// real cases without paying the cost of reflection-based cloning.
func deepCopyValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = deepCopyValue(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = deepCopyValue(val)
		}
		return out
	default:
		return v
	}
}

// ---------------------------------------------------------------------------
// Workspace mutation safety
// ---------------------------------------------------------------------------

// readOnlyTools is the set of built-in tool names that are guaranteed to
// never modify the workspace. These are safe for parallel execution.
var readOnlyTools = map[string]bool{
	"git_diff":        true,
	"git_status":      true,
	"read_file":       true,
	"list_files":      true,
	"search_codebase": true,
	"tree":            true,
}

// isMutatingNode returns true if the node may modify the workspace.
// Tool nodes are always mutating. Agent/judge nodes are mutating only
// if they have at least one tool that is not in the read-only set.
// Nodes with Readonly=true are never considered mutating.
func isMutatingNode(node ir.Node) bool {
	switch n := node.(type) {
	case *ir.ToolNode:
		return true
	case *ir.AgentNode:
		if n.Readonly {
			return false
		}
		for _, t := range n.Tools {
			if !readOnlyTools[t] {
				return true
			}
		}
	case *ir.JudgeNode:
		if n.Readonly {
			return false
		}
		for _, t := range n.Tools {
			if !readOnlyTools[t] {
				return true
			}
		}
	}
	return false
}

// branchContainsMutation walks from startNodeID to globalConvergence (or to a
// terminal node) and returns true if any node along the path may mutate the
// workspace.
//
// The previous implementation stopped walking at the FIRST node with
// AwaitMode != AwaitNone — i.e. at any intermediate join — which meant that
// in a topology like
//
//	router(fan_out_all) -> A -> joinA -> mutA -> globalJoin
//	                    -> B -> joinB -> mutB -> globalJoin
//
// the BFS treated `joinA` / `joinB` as the stopping point and never saw
// `mutA` or `mutB`. Both branches passed validateWorkspaceSafety, then ran
// in parallel and raced on the shared workspace (e.g. git index).
//
// The correct stopping condition is the GLOBAL convergence point of the
// fan-out (the node where all branches reconverge), not the first
// intermediate join. We pass that in explicitly. Terminal nodes (done/fail)
// also stop the walk because the branch ends there.
func (e *Engine) branchContainsMutation(startNodeID, globalConvergence string) bool {
	visited := map[string]bool{}
	queue := []string{startNodeID}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if visited[nodeID] {
			continue
		}
		visited[nodeID] = true

		// Stop at the global convergence point — beyond it, nodes are
		// post-fan-out and shared by all branches sequentially.
		if globalConvergence != "" && nodeID == globalConvergence {
			continue
		}

		node, ok := e.workflow.Nodes[nodeID]
		if !ok {
			continue
		}
		// Stop walking at terminal nodes.
		if isTerminalNode(node) {
			continue
		}
		if isMutatingNode(node) {
			return true
		}
		for _, edge := range e.workflow.Edges {
			if edge.From == nodeID {
				queue = append(queue, edge.To)
			}
		}
	}
	return false
}

// validateWorkspaceSafety checks that at most one branch in a fan-out
// contains mutating nodes. Returns an error if the topology is unsafe.
//
// routerNodeID + fanEdges are used to compute the global convergence point
// up-front; we pass it down to branchContainsMutation so the BFS doesn't
// stop early at intermediate joins.
func (e *Engine) validateWorkspaceSafety(routerNodeID string, fanEdges []*ir.Edge) error {
	globalConvergence := e.findConvergencePoint(routerNodeID, fanEdges)
	mutatingCount := 0
	var mutatingBranches []string
	for _, edge := range fanEdges {
		if e.branchContainsMutation(edge.To, globalConvergence) {
			mutatingCount++
			mutatingBranches = append(mutatingBranches, edge.To)
		}
	}
	if mutatingCount > 1 {
		return &RuntimeError{
			Code:    ErrCodeWorkspaceSafety,
			Message: fmt.Sprintf("workspace safety violation: %d branches contain mutating nodes %v", mutatingCount, mutatingBranches),
			Hint:    "at most 1 mutating branch is allowed in parallel on the same workspace; move tool nodes to separate sequential steps",
		}
	}
	return nil
}
