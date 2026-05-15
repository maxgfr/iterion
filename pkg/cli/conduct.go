package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/SocialGouv/iterion/pkg/conductor"
	"github.com/SocialGouv/iterion/pkg/conductor/native"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/server"
	"github.com/SocialGouv/iterion/pkg/store"
)

// ConductOptions captures CLI flags for `iterion conduct`.
type ConductOptions struct {
	ConfigPath string
	StoreDir   string
	Port       int  // overrides cfg.Server.Port if > 0
	NoServer   bool // overrides cfg.Server.Port to disable HTTP
}

// RunConduct loads the config, opens the necessary stores, builds a
// Manager + starts a conductor, then serves the REST/WS surface until
// SIGINT/SIGTERM.
//
// This is the standalone CLI path — same Manager primitive that the
// editor server uses, just driven by a YAML on disk instead of the
// SPA's PUT /api/v1/conductor/config endpoint.
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

	wsRoot := cfg.Workspace.Root
	if wsRoot == "" {
		wsRoot = filepath.Join(storeDir, "conductor", "workspaces")
	}
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		return fmt.Errorf("conductor: mkdir workspace root: %w", err)
	}
	lockPath := filepath.Join(wsRoot, ".conductor.lock")
	lk, err := store.AcquireFileLock(lockPath, "conductor workspace "+wsRoot)
	if err != nil {
		return err
	}
	defer func() {
		if err := lk.Unlock(); err != nil {
			logger.Warn("conductor: unlock %s: %v", lockPath, err)
		}
	}()

	mgr, err := conductor.NewManager(conductor.ManagerOptions{
		StoreDir:    storeDir,
		NativeStore: nativeStore,
		Logger:      logger,
	})
	if err != nil {
		return err
	}
	if err := mgr.SaveConfig(cfg); err != nil {
		return err
	}
	if err := mgr.Start(); err != nil {
		return err
	}
	defer mgr.Stop()

	watcher, err := conductor.NewConfigWatcher(opts.ConfigPath, logger)
	if err != nil {
		return err
	}
	if err := watcher.Start(func(c *conductor.Config) {
		if err := mgr.SaveConfig(c); err != nil {
			logger.Warn("conductor: reload: %v", err)
		}
	}); err != nil {
		return err
	}
	defer watcher.Stop()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	port := pickPort(cfg, opts)
	var httpSrv *http.Server
	if port > 0 {
		mux := http.NewServeMux()
		mgr.RegisterRoutes(mux, "/api/v1/conductor")
		nativeStore.RegisterRoutes(mux, "/api/v1/native")
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		mux.HandleFunc("/api/server/info", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"mode":"conduct","auth_required":false,"limits":{"upload":{}},"native_tracker_enabled":true,"conductor_enabled":true}`))
		})
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
	p.Line("iterion conduct: tracker %s", cfg.Tracker.Kind)
	p.Line("iterion conduct: workspaces under %s", wsRoot)
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

func pickPort(cfg *conductor.Config, opts ConductOptions) int {
	if opts.NoServer {
		return 0
	}
	if opts.Port > 0 {
		return opts.Port
	}
	return cfg.Server.Port
}
