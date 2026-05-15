package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/SocialGouv/iterion/pkg/conductor"
	"github.com/SocialGouv/iterion/pkg/conductor/native"
	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/server"
	"github.com/SocialGouv/iterion/pkg/store"

	"io/fs"
)

// ConductOptions captures CLI flags for `iterion conduct`.
type ConductOptions struct {
	ConfigPath string
	StoreDir   string
	Port       int  // overrides cfg.Server.Port if > 0
	NoServer   bool // overrides cfg.Server.Port to disable HTTP
}

// RunConduct loads the config, opens the necessary stores, builds the
// conductor and HTTP surface, and runs until SIGINT/SIGTERM.
func RunConduct(p *Printer, opts ConductOptions) error {
	logger := iterlog.New(iterlog.LevelInfo, os.Stderr)

	cfg, err := conductor.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	storeDir := store.ResolveStoreDir(cwd, opts.StoreDir)

	nativeStore, err := native.NewStore(filepath.Join(storeDir, "conductor"))
	if err != nil {
		return fmt.Errorf("native store: %w", err)
	}

	trk, err := buildTracker(cfg, nativeStore)
	if err != nil {
		return err
	}

	wsRoot := cfg.Workspace.Root
	if wsRoot == "" {
		wsRoot = filepath.Join(storeDir, "conductor", "workspaces")
	}
	workspaces, err := conductor.NewWorkspaces(wsRoot)
	if err != nil {
		return err
	}

	runner, err := conductor.NewEngineRunner(cfg.Workflow, logger)
	if err != nil {
		return err
	}

	lockPath := filepath.Join(wsRoot, ".conductor.lock")
	lk, err := conductor.Lock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := lk.Unlock(); err != nil {
			logger.Warn("conductor: unlock %s: %v", lockPath, err)
		}
	}()

	c, err := conductor.New(conductor.Options{
		Config:     cfg,
		Tracker:    trk,
		Runner:     runner,
		Workspaces: workspaces,
		Logger:     logger,
		StoreDir:   storeDir,
	})
	if err != nil {
		return err
	}

	// Hot-reload wiring.
	watcher, err := conductor.NewConfigWatcher(opts.ConfigPath, logger)
	if err != nil {
		return err
	}
	if err := watcher.Start(c.Reload); err != nil {
		return err
	}
	defer watcher.Stop()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	c.Start(ctx)
	defer c.Stop()

	port := pickPort(cfg, opts)
	var httpSrv *http.Server
	if port > 0 {
		mux := http.NewServeMux()
		c.RegisterRoutes(mux, "/api/v1/conductor")
		nativeStore.RegisterRoutes(mux, "/api/v1/native")
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		// /api/server/info minimally so the SPA detects available views.
		mux.HandleFunc("/api/server/info", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"mode":"conduct","auth_required":false,"limits":{"upload":{}},"native_tracker_enabled":true,"conductor_enabled":true}`))
		})
		// SPA: serve the embedded editor so users can open the
		// dashboard at the same URL without running `iterion editor`.
		if sub, err := fs.Sub(server.StaticFS, "static"); err == nil {
			mux.Handle("/", server.SPAHandler(sub))
		} else {
			logger.Warn("conductor: SPA assets not available: %v", err)
		}

		httpSrv = &http.Server{
			Addr:              fmt.Sprintf("127.0.0.1:%d", port),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		p.Line("iterion conduct: HTTP on http://%s", httpSrv.Addr)
		go func() {
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("conductor http server: %v", err)
				cancel()
			}
		}()
	} else {
		p.Line("iterion conduct: HTTP disabled (cfg.server.port=0)")
	}

	p.Line("iterion conduct: workflow %s", cfg.Workflow)
	p.Line("iterion conduct: tracker %s", trk.Name())
	p.Line("iterion conduct: workspaces under %s", workspaces.Root())
	p.Line("iterion conduct: polling every %s, stall timeout %s", cfg.PollingInterval(), cfg.StallTimeout())
	p.Line("iterion conduct: ctrl-c to stop")

	<-ctx.Done()
	p.Line("iterion conduct: shutting down…")
	if httpSrv != nil {
		shutdownCtx, sc := context.WithTimeout(context.Background(), 5*time.Second)
		defer sc()
		_ = httpSrv.Shutdown(shutdownCtx)
	}
	return nil
}

func buildTracker(cfg *conductor.Config, ns *native.Store) (tracker.Tracker, error) {
	switch cfg.Tracker.Kind {
	case "native":
		return native.NewAdapter(ns), nil
	case "github":
		return buildGitHubTracker(cfg.Tracker.GitHub)
	case "forgejo":
		return buildForgejoTracker(cfg.Tracker.Forgejo)
	default:
		return nil, fmt.Errorf("conductor: unsupported tracker kind %q", cfg.Tracker.Kind)
	}
}

func buildGitHubTracker(cfg *conductor.GitHubTrackerConfig) (tracker.Tracker, error) {
	if cfg == nil {
		return nil, errors.New("conductor: tracker.kind=github requires tracker.github block")
	}
	mapping := make(map[string]tracker.LabelSelector, len(cfg.StateMapping))
	for state, sel := range cfg.StateMapping {
		mapping[state] = tracker.LabelSelector{
			LabelsInclude: sel.LabelsInclude,
			LabelsExclude: sel.LabelsExclude,
		}
	}
	return tracker.NewGitHub(tracker.GitHubOptions{
		Repo:          cfg.Repo,
		Token:         cfg.Token,
		IncludeLabels: cfg.IncludeLabels,
		ExcludeLabels: cfg.ExcludeLabels,
		ClaimedLabel:  cfg.ClaimedLabel,
		StateMapping:  mapping,
	})
}

func buildForgejoTracker(cfg *conductor.ForgejoTrackerConfig) (tracker.Tracker, error) {
	if cfg == nil {
		return nil, errors.New("conductor: tracker.kind=forgejo requires tracker.forgejo block")
	}
	mapping := make(map[string]tracker.LabelSelector, len(cfg.StateMapping))
	for state, sel := range cfg.StateMapping {
		mapping[state] = tracker.LabelSelector{
			LabelsInclude: sel.LabelsInclude,
			LabelsExclude: sel.LabelsExclude,
		}
	}
	return tracker.NewForgejo(tracker.ForgejoOptions{
		Host:          cfg.Host,
		Repo:          cfg.Repo,
		Token:         cfg.Token,
		IncludeLabels: cfg.IncludeLabels,
		ExcludeLabels: cfg.ExcludeLabels,
		ClaimedLabel:  cfg.ClaimedLabel,
		StateMapping:  mapping,
	})
}

func pickPort(cfg *conductor.Config, opts ConductOptions) int {
	if opts.NoServer {
		return 0
	}
	if opts.Port > 0 {
		return opts.Port
	}
	return cfg.Server.Port
}
