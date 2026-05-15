// Package conductor implements iterion's long-running dispatcher.
// It polls a tracker.Tracker for eligible issues and dispatches a
// workflow run per issue, with retry, stall detection, hooks, and
// per-state concurrency limits.
//
// Concurrency model: a single goroutine (the actor) owns all mutable
// state. External callers — fsnotify watcher, HTTP handlers, retry
// timers, dispatch goroutines — interact through typed commands sent
// on Conductor.cmds. This mirrors Symphony's GenServer design with
// fewer moving parts.
package conductor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

// Options is the construction-time wiring for a Conductor.
type Options struct {
	Config     *Config
	Tracker    tracker.Tracker
	Runner     Runner
	Workspaces *Workspaces
	Logger     *iterlog.Logger
	StoreDir   string

	// HostMarker is the claim marker the conductor writes to the
	// tracker when it claims an issue. Defaults to "<hostname>-<pid>".
	HostMarker string

	// SnapshotPublisher, when non-nil, is invoked with each fresh
	// snapshot after a tick. Used to fan snapshots out over WebSocket.
	SnapshotPublisher func(Snapshot)
}

// Conductor is the long-running dispatcher.
type Conductor struct {
	cfg     atomic.Pointer[Config]
	tracker tracker.Tracker
	runner  Runner

	workspaces *Workspaces
	hooks      Hooks
	logger     *iterlog.Logger
	storeDir   string
	hostMarker string

	state *state
	cmds  chan cmd

	publishMu sync.Mutex
	publisher func(Snapshot)

	startOnce sync.Once
	stopOnce  sync.Once
	stop      chan struct{}
	done      chan struct{}

	ws *wsBridge
}

// New constructs a Conductor with the given Options. It does not start
// the actor goroutine; call Start.
func New(opts Options) (*Conductor, error) {
	if opts.Config == nil {
		return nil, errors.New("conductor: config required")
	}
	if opts.Tracker == nil {
		return nil, errors.New("conductor: tracker required")
	}
	if opts.Runner == nil {
		return nil, errors.New("conductor: runner required")
	}
	if opts.Workspaces == nil {
		return nil, errors.New("conductor: workspaces required")
	}
	if opts.Logger == nil {
		return nil, errors.New("conductor: logger required")
	}
	if opts.HostMarker == "" {
		opts.HostMarker = defaultHostMarker()
	}
	c := &Conductor{
		tracker:    opts.Tracker,
		runner:     opts.Runner,
		workspaces: opts.Workspaces,
		hooks:      opts.Config.Hooks,
		logger:     opts.Logger,
		storeDir:   opts.StoreDir,
		hostMarker: opts.HostMarker,
		state:      newState(),
		cmds:       make(chan cmd, 64),
		publisher:  opts.SnapshotPublisher,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		ws:         newWsBridge(),
	}
	c.cfg.Store(opts.Config)
	return c, nil
}

// Start runs the actor loop and the polling ticker. Returns
// immediately; use Stop to shut down.
func (c *Conductor) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		go c.actorLoop(ctx)
	})
}

// Stop signals the actor to exit and waits for it. Safe to call more
// than once.
func (c *Conductor) Stop() {
	c.stopOnce.Do(func() {
		close(c.stop)
	})
	<-c.done
}

// Refresh enqueues an immediate poll tick, bypassing the regular cadence.
func (c *Conductor) Refresh() {
	select {
	case c.cmds <- cmdRefresh{}:
	default:
		// channel full → a tick is already pending, nothing to do.
	}
}

// Cancel asks the conductor to cancel an in-flight dispatch for the
// given issue. The corresponding worker goroutine receives ctx.Done()
// and the issue is released for re-dispatch on the next tick (subject
// to tracker state).
func (c *Conductor) Cancel(issueID string) {
	c.cmds <- cmdCancel{issueID: issueID}
}

// Reload swaps in a fresh config. Typically wired to ConfigWatcher.
func (c *Conductor) Reload(cfg *Config) {
	c.cmds <- cmdReload{cfg: cfg}
}

// Snapshot returns a consistent view of the actor's state.
func (c *Conductor) Snapshot() Snapshot {
	reply := make(chan Snapshot, 1)
	c.cmds <- cmdSnapshot{reply: reply}
	return <-reply
}

// SetSnapshotPublisher swaps the fan-out hook (used to wire/unwire WS).
func (c *Conductor) SetSnapshotPublisher(fn func(Snapshot)) {
	c.publishMu.Lock()
	c.publisher = fn
	c.publishMu.Unlock()
}

// Config returns the currently-active config pointer.
func (c *Conductor) Config() *Config { return c.cfg.Load() }

// ---------------------------------------------------------------------------
// actor loop
// ---------------------------------------------------------------------------

func (c *Conductor) actorLoop(ctx context.Context) {
	defer close(c.done)

	cfg := c.cfg.Load()
	ticker := time.NewTicker(cfg.PollingInterval())
	defer ticker.Stop()

	// Kick off an immediate first tick so the user sees activity
	// without waiting for the cadence.
	c.tick(ctx)

	for {
		select {
		case <-c.stop:
			c.shutdown()
			return
		case <-ctx.Done():
			c.shutdown()
			return
		case <-ticker.C:
			c.tick(ctx)
		case cmd, ok := <-c.cmds:
			if !ok {
				c.shutdown()
				return
			}
			cmd.apply(c, ctx)
			// Re-tick ticker cadence when polling interval changes via Reload.
			if cur := c.cfg.Load(); cur.PollingInterval() != cfg.PollingInterval() {
				ticker.Reset(cur.PollingInterval())
				cfg = cur
			}
		}
	}
}

func (c *Conductor) shutdown() {
	for _, r := range c.state.running {
		if r.Cancel != nil {
			r.Cancel()
		}
	}
	for _, t := range c.state.retryTimers {
		t.Stop()
	}
}

// fireSnapshot publishes the current snapshot to the WS bridge and to
// the optional user-supplied publisher.
func (c *Conductor) fireSnapshot() {
	snap := c.buildSnapshot()
	c.ws.broadcast(snap)
	c.publishMu.Lock()
	pub := c.publisher
	c.publishMu.Unlock()
	if pub != nil {
		pub(snap)
	}
}

func (c *Conductor) buildSnapshot() Snapshot {
	cfg := c.cfg.Load()
	snap := Snapshot{
		Name:             cfg.Name,
		Tracker:          c.tracker.Name(),
		GeneratedAt:      time.Now().UTC(),
		PollingIntervalS: cfg.PollingInterval().Seconds(),
		StallTimeoutS:    cfg.StallTimeout().Seconds(),
		Slots: SlotsView{
			GlobalMax:    cfg.Agent.MaxConcurrent,
			GlobalUsed:   len(c.state.running),
			PerStateMax:  copyIntMap(cfg.Agent.MaxConcurrentByState),
			PerStateUsed: copyIntMap(c.state.slotsByState),
		},
	}
	ids := make([]string, 0, len(c.state.running))
	for id := range c.state.running {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := c.state.running[id]
		snap.Running = append(snap.Running, RunningView{
			IssueID:       r.IssueID,
			Identifier:    r.Identifier,
			RunID:         r.RunID,
			WorkflowState: r.WorkflowState,
			WorkspacePath: r.WorkspacePath,
			StartedAt:     r.StartedAt,
			LastEventAt:   r.LastEventAt,
			LastEventName: r.LastEventName,
			Attempt:       r.Attempt,
		})
	}
	rids := make([]string, 0, len(c.state.retryTimers))
	for id := range c.state.retryTimers {
		rids = append(rids, id)
	}
	sort.Strings(rids)
	for _, id := range rids {
		snap.Retries = append(snap.Retries, RetryView{
			IssueID: id,
			Attempt: c.state.retryAttempts[id],
		})
	}
	return snap
}

func copyIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// utilities
// ---------------------------------------------------------------------------

func defaultHostMarker() string {
	host, err := osHostname()
	if err != nil || host == "" {
		host = "conductor"
	}
	return fmt.Sprintf("%s-%d", host, osPid())
}
