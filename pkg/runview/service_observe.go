package runview

import (
	"context"
	"errors"

	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/store"
	"github.com/SocialGouv/iterion/pkg/supervise"
)

// ObserveRun streams a run's events for an external observer (the
// supervisor coordinator): a catch-up replay of everything persisted so
// far — so a late-attaching observer can reconstruct the currently
// active node — followed by live events, deduplicated by seq. The
// returned channel is closed when the run terminates (broker CloseRun)
// or ctx is cancelled.
//
// The caller MUST invoke release exactly once when done; it cancels the
// live subscription and releases the on-demand file tailer (started for
// runs this process did not launch in-process, e.g. a dispatcher- or
// CLI-spawned run observed from a studio process).
//
// Local broker mode only — cloud event-source mode is out of scope for
// the supervisor's local attach path and returns a typed error.
func (s *Service) ObserveRun(ctx context.Context, runID string) (<-chan *store.Event, func(), error) {
	if runID == "" {
		return nil, nil, errors.New("runview: run_id is required")
	}
	if s.broker == nil {
		return nil, nil, errors.New("runview: no broker wired (cannot observe run)")
	}
	// Subscribe BEFORE the catch-up read so any event persisted during
	// the read is buffered on the live channel and deduped by seq —
	// never dropped between snapshot and live.
	var releaseSrc func()
	if !s.Active(runID) {
		// Bridge events.jsonl -> broker for runs not produced in-process.
		releaseSrc = s.EnsureEventSource(runID)
	}
	sub := s.broker.Subscribe(runID)

	out := make(chan *store.Event, subscriberBufferSize)
	release := func() {
		sub.Cancel()
		if releaseSrc != nil {
			releaseSrc()
		}
	}

	go func() {
		defer close(out)
		var lastSeq int64 = -1
		// Catch-up replay from disk.
		if events, err := s.store.LoadEvents(ctx, runID); err == nil {
			for _, e := range events {
				if e == nil {
					continue
				}
				select {
				case out <- e:
					if e.Seq > lastSeq {
						lastSeq = e.Seq
					}
				case <-ctx.Done():
					return
				}
			}
		}
		// Live events, deduped against the catch-up tail.
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-sub.C:
				if !ok {
					return
				}
				if e == nil || e.Seq <= lastSeq {
					continue
				}
				lastSeq = e.Seq
				select {
				case out <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, release, nil
}

// Inject enqueues a steering message into runID, scoped to nodeID when
// non-empty (delivered only while that node is the active executing
// node). It wraps QueueMessage + WithMessageNode so a supervisor
// coordinator (pkg/supervise) can drive *Service through the
// supervise.Injector seam without pkg/supervise importing runview.
func (s *Service) Inject(ctx context.Context, runID, nodeID, text string) error {
	_, err := s.QueueMessage(ctx, runID, text, WithMessageNode(nodeID))
	return err
}

// startDeclaredSupervisors spawns a supervise.Coordinator for every
// `supervisor NAME:` block on the workflow, each bound to this run's
// lifetime via ctx. They observe through the broker (in-process) and
// steer via Inject. Returns a stop func the caller defers to drain them
// before the run goroutine exits. A no-op when the workflow declares
// none — supervision is an enhancement, never a hard dependency, so any
// individual coordinator that fails to construct is simply skipped.
func (s *Service) startDeclaredSupervisors(ctx context.Context, runID string, wf *ir.Workflow, logger *iterlog.Logger) (stop func()) {
	if wf == nil || len(wf.Supervisors) == 0 {
		return func() {}
	}
	var coords []*supervise.Coordinator
	for _, sup := range wf.Supervisors {
		spec := supervise.Spec{
			Name:     sup.Name,
			Model:    sup.Model,
			System:   resolvePromptBody(wf, sup.System),
			Watches:  sup.Watches,
			Cooldown: sup.Cooldown,
			MaxEvals: sup.MaxEvals,
		}
		coord := supervise.New(s, s, runID, spec, nil, logger)
		if coord == nil {
			continue
		}
		coord.Start(ctx)
		coords = append(coords, coord)
	}
	return func() {
		for _, c := range coords {
			c.Close()
		}
	}
}

// resolvePromptBody returns the raw text of a workflow prompt by name,
// or "" when the name is empty/undeclared (the supervisor then runs with
// only its built-in framing).
func resolvePromptBody(wf *ir.Workflow, name string) string {
	if name == "" || wf == nil {
		return ""
	}
	if p, ok := wf.Prompts[name]; ok && p != nil {
		return p.Body
	}
	return ""
}
