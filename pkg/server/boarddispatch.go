package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/SocialGouv/iterion/pkg/dispatcher/boardmongo"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/store"
)

// boardCoordinator is the cross-tenant board view the cloud dispatcher needs.
// *boardmongo.Coordinator satisfies it; tests pass a fake.
type boardCoordinator interface {
	ListEligible(ctx context.Context, eligible []string, limit int) ([]boardmongo.Candidate, error)
	Claim(ctx context.Context, tenant, id, marker string) error
	SetState(ctx context.Context, tenant, id, state string) error
	Release(ctx context.Context, tenant, id, marker string) error
}

// boardDispatcher polls the cloud board for eligible cards and runs each via
// the injected process func (launch + poll-to-terminal). Multi-replica-safe
// WITHOUT leader election: the per-card Claim is a CAS, so each card is claimed
// by exactly one replica; the rest skip. In-flight cards are bounded by a
// semaphore shared across ticks, so a slow run never starves polling.
type boardDispatcher struct {
	coord   boardCoordinator
	process func(ctx context.Context, tenant string, iss native.Issue) error
	marker  string

	eligible        []string
	inProgressState string
	doneState       string
	blockedState    string

	interval time.Duration
	sem      chan struct{}
	logger   *iterlog.Logger
	wg       sync.WaitGroup // tracks in-flight processCard goroutines (for tests + drain)
}

// newBoardDispatcher wires a cloud board dispatcher with sensible defaults.
func newBoardDispatcher(coord boardCoordinator, process func(context.Context, string, native.Issue) error, marker string, concurrency int, logger *iterlog.Logger) *boardDispatcher {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &boardDispatcher{
		coord:           coord,
		process:         process,
		marker:          marker,
		eligible:        []string{native.StateReady},
		inProgressState: native.StateInProgress,
		doneState:       native.StateDone,
		blockedState:    native.StateBlocked,
		interval:        5 * time.Second,
		sem:             make(chan struct{}, concurrency),
		logger:          logger,
	}
}

// tick claims as many eligible cards as there are free slots and dispatches
// each in a detached goroutine. Returns the number it claimed this tick.
func (d *boardDispatcher) tick(ctx context.Context) int {
	cands, err := d.coord.ListEligible(ctx, d.eligible, cap(d.sem)*2)
	if err != nil {
		d.warn("list eligible: %v", err)
		return 0
	}
	claimed := 0
	for _, c := range cands {
		select {
		case d.sem <- struct{}{}: // acquired a slot
		default:
			return claimed // no free slots; the rest wait for the next tick
		}
		if err := d.coord.Claim(ctx, c.Tenant, c.Issue.ID, d.marker); err != nil {
			<-d.sem // claim lost (another replica won, or conflict) — release the slot
			continue
		}
		claimed++
		d.wg.Add(1)
		go d.processCard(ctx, c)
	}
	return claimed
}

func (d *boardDispatcher) processCard(ctx context.Context, c boardmongo.Candidate) {
	defer d.wg.Done()
	defer func() { <-d.sem }()
	// Move to in-progress for board visibility (best-effort).
	if err := d.coord.SetState(ctx, c.Tenant, c.Issue.ID, d.inProgressState); err != nil {
		d.warn("card %s/%s → in_progress: %v", c.Tenant, c.Issue.ID, err)
	}
	runErr := d.process(ctx, c.Tenant, c.Issue)
	final := d.doneState
	if runErr != nil {
		final = d.blockedState
		d.warn("card %s/%s run failed: %v", c.Tenant, c.Issue.ID, runErr)
	}
	if err := d.coord.SetState(ctx, c.Tenant, c.Issue.ID, final); err != nil {
		d.warn("card %s/%s → %s: %v", c.Tenant, c.Issue.ID, final, err)
	}
	if err := d.coord.Release(ctx, c.Tenant, c.Issue.ID, d.marker); err != nil {
		d.warn("card %s/%s release: %v", c.Tenant, c.Issue.ID, err)
	}
}

// run loops tick every interval until ctx is cancelled, then drains in-flight
// cards. Start one per replica.
func (d *boardDispatcher) run(ctx context.Context) {
	t := time.NewTicker(d.interval)
	defer t.Stop()
	for {
		d.tick(ctx)
		select {
		case <-ctx.Done():
			d.wg.Wait() // let in-flight cards finish their state transition
			return
		case <-t.C:
		}
	}
}

func (d *boardDispatcher) warn(format string, args ...any) {
	if d.logger != nil {
		d.logger.Warn("board dispatcher: "+format, args...)
	}
}

// processBoardCard is the cloud board dispatcher's process func: launch the
// card's bot for its tenant through the run service (→ publisher), then poll
// the run record until it terminates. Returns nil on a clean finish, an error
// on failure or pause (the dispatcher then moves the card to blocked). The
// tenant identity is stamped on ctx so the publisher seals credentials.
func (s *Server) processBoardCard(ctx context.Context, tenant string, iss native.Issue) error {
	if s.runs == nil {
		return errors.New("run service unavailable")
	}
	if iss.Bot == "" {
		return fmt.Errorf("card %s has no bot", iss.ID)
	}
	ctx = store.WithIdentity(ctx, tenant, "board-dispatcher")
	path, source, err := s.resolveBotSource(iss.Bot)
	if err != nil {
		return err
	}
	res, err := s.runs.Launch(ctx, runview.LaunchSpec{
		FilePath: path,
		Source:   source,
		BotID:    iss.Bot,
		Vars:     iss.BotArgs,
	})
	if err != nil {
		return err
	}
	runID := res.RunID
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		if run, lerr := s.runs.LoadRunCtx(ctx, runID); lerr == nil {
			switch st := run.Status; {
			case st == store.RunStatusFinished:
				return nil
			case st.IsTerminal():
				return fmt.Errorf("run %s ended %s", runID, st)
			case st == store.RunStatusPausedWaitingHuman || st == store.RunStatusPausedOperator:
				// Parked on a human/operator gate — stop waiting; the card
				// goes to blocked and the operator resumes the run.
				return fmt.Errorf("run %s paused (%s)", runID, st)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
