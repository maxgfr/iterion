package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SocialGouv/iterion/pkg/audit"
	iterauth "github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/auth/oidc"
	"github.com/SocialGouv/iterion/pkg/auth/orgsso"
	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/SocialGouv/iterion/pkg/cloud/metrics"
	"github.com/SocialGouv/iterion/pkg/cloud/tracing"
	iterconfig "github.com/SocialGouv/iterion/pkg/config"
	"github.com/SocialGouv/iterion/pkg/forge"
	"github.com/SocialGouv/iterion/pkg/identity"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/mail"
	"github.com/SocialGouv/iterion/pkg/marketplace"
	"github.com/SocialGouv/iterion/pkg/orgusage"
	"github.com/SocialGouv/iterion/pkg/pat"
	natsq "github.com/SocialGouv/iterion/pkg/queue/nats"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/runview/eventstream"
	"github.com/SocialGouv/iterion/pkg/secrets"
	"github.com/SocialGouv/iterion/pkg/server"
	"github.com/SocialGouv/iterion/pkg/server/cloudpublisher"
	mongostore "github.com/SocialGouv/iterion/pkg/store/mongo"
	"github.com/SocialGouv/iterion/pkg/webhooks"
)

// `iterion server` is the cloud-mode HTTP server entry point. In
// local mode it delegates to cli.RunStudio (same handler tree as
// `iterion studio`). In cloud mode it builds a Mongo+S3 store + a
// NATS-backed LaunchPublisher and feeds them into pkg/server.Server
// so handleLaunchRun publishes to the queue instead of spawning the
// runtime in-process.
//
// Differences from `iterion studio` regardless of mode:
//   - default --bind is 0.0.0.0 (cloud pods need LAN exposure);
//   - --no-browser is forced on (no display in a container).
//
// Cloud-ready plan §F (T-30, T-31, T-32, T-33).

var serverOpts struct {
	port       int
	bind       string
	dir        string
	storeDir   string
	configPath string
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the iterion HTTP server (studio + run console + cloud API)",
	Long: `iterion server is the cloud-deployment HTTP entry point. It serves the
studio, the run console (REST + WebSocket), and the launch /
resume / cancel API on a single port. Health endpoints (/healthz,
/readyz) live alongside the API.

Mode is chosen by ITERION_MODE:
  - local (default): in-process engine; same as 'iterion studio'.
  - cloud: persists to Mongo+S3, publishes runs onto NATS for the
    runner pool to consume.

For local dev, prefer 'iterion studio' which keeps the loopback bind
default and opens the browser.`,
	Args: cobra.NoArgs,
	RunE: runServer,
}

func init() {
	f := serverCmd.Flags()
	f.IntVar(&serverOpts.port, "port", 4891, "HTTP port")
	f.StringVar(&serverOpts.bind, "bind", "0.0.0.0", "Bind address (default 0.0.0.0 for cloud pods)")
	f.StringVar(&serverOpts.dir, "dir", "", "Working directory")
	f.StringVar(&serverOpts.storeDir, "store-dir", "", "Run store directory (local mode only)")
	f.StringVar(&serverOpts.configPath, "config", "", "Path to YAML config (env vars take precedence)")
	rootCmd.AddCommand(serverCmd)
}

// randomBootstrapPassword returns a URL-safe random temporary password for the
// bootstrap super-admin. base64 of 18 bytes (~24 chars) is comfortably above
// the MinPasswordLen the rotation endpoint enforces.
// orgLimitDefaultsFromEnv reads the platform-wide launch limits
// applied to teams without a per-org override. Unset / invalid /
// zero values mean "no limit" — the safe default for existing
// deployments. Per-org overrides live on the Team document and are
// managed via PATCH /api/admin/orgs/{id}.
func orgLimitDefaultsFromEnv() server.OrgLimitDefaults {
	intEnv := func(key string) int {
		n, err := strconv.Atoi(os.Getenv(key))
		if err != nil || n < 0 {
			return 0
		}
		return n
	}
	var d server.OrgLimitDefaults
	d.MonthlyRunQuota = intEnv("ITERION_ORG_DEFAULT_MONTHLY_RUN_QUOTA")
	d.MaxConcurrentRuns = intEnv("ITERION_ORG_DEFAULT_MAX_CONCURRENT_RUNS")
	d.LaunchRatePerMin = intEnv("ITERION_ORG_DEFAULT_LAUNCH_RATE_PER_MIN")
	if f, err := strconv.ParseFloat(os.Getenv("ITERION_ORG_DEFAULT_MONTHLY_COST_CAP_USD"), 64); err == nil && f > 0 {
		d.MonthlyCostCapUSD = f
	}
	return d
}

// forgeGitHubAppFromEnv reads the GitHub-App identity for the
// installation-token connect mode. The PEM private key is loaded from a file
// (the canonical k8s-secret mount), falling back to an inline env value.
// Empty AppID → the App mode is unavailable (OAuth/PAT still work).
func forgeGitHubAppFromEnv() server.ForgeGitHubAppConfig {
	appID, _ := strconv.ParseInt(strings.TrimSpace(os.Getenv("ITERION_FORGE_GITHUB_APP_ID")), 10, 64)
	key := strings.TrimSpace(os.Getenv("ITERION_FORGE_GITHUB_APP_PRIVATE_KEY"))
	if path := strings.TrimSpace(os.Getenv("ITERION_FORGE_GITHUB_APP_PRIVATE_KEY_FILE")); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			key = string(b)
		}
	}
	return server.ForgeGitHubAppConfig{
		AppID:      appID,
		PrivateKey: key,
		AppSlug:    strings.TrimSpace(os.Getenv("ITERION_FORGE_GITHUB_APP_SLUG")),
	}
}

func randomBootstrapPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func runServer(cmd *cobra.Command, _ []string) error {
	cfg, err := iterconfig.Load(iterconfig.LoadOptions{
		YAMLPath:         serverOpts.configPath,
		DefaultLogFormat: iterconfig.LogFormatJSON,
	})
	if err != nil {
		return fmt.Errorf("server: load config: %w", err)
	}

	// Local mode: keep the existing studio handlers; only difference
	// from `iterion studio` is the cloud-friendly --bind default.
	if cfg.Mode == iterconfig.ModeLocal {
		return cli.RunStudio(cmd.Context(), cli.StudioOptions{
			Port:      serverOpts.port,
			Bind:      serverOpts.bind,
			Dir:       serverOpts.dir,
			StoreDir:  serverOpts.storeDir,
			NoBrowser: true,
		}, newPrinter())
	}

	// Cloud mode: build Mongo+S3 store + NATS publisher + server
	// directly. We bypass cli.RunStudio because it auto-discovers a
	// filesystem store, which doesn't make sense when persistence
	// lives in Mongo.
	logger := iterlog.NewWithFormat(parseLevel(cfg.Log.Level), cmd.ErrOrStderr(), parseLogFormat(cfg.Log.Format))
	logger.Info("server: starting (mode=cloud)")

	rootCtx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	traceShutdown, err := tracing.Init(rootCtx, "iterion-server", logger)
	if err != nil {
		return fmt.Errorf("server: init tracing: %w", err)
	}
	defer func() {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = traceShutdown(shutCtx)
	}()

	natsConn, err := natsq.Connect(rootCtx, natsq.Config{
		URL:        cfg.NATS.URL,
		StreamName: cfg.NATS.Stream,
		DLQStream:  cfg.NATS.DLQStream,
		KVBucket:   cfg.NATS.KVBucket,
		Logger:     logger,
	})
	if err != nil {
		return fmt.Errorf("server: connect NATS: %w", err)
	}
	defer natsConn.Close()

	bc, err := newCloudBlob(rootCtx, cfg.S3)
	if err != nil {
		return fmt.Errorf("server: build blob client: %w", err)
	}
	defer func() { _ = bc.Close() }()

	// Server-side store: no NATS lock provider — the server never
	// executes runs, only publishes them. The runner pod is the
	// only place that takes leases.
	st, err := newCloudMongoStore(rootCtx, cfg.Mongo, bc, logger, nil)
	if err != nil {
		return fmt.Errorf("server: build mongo store: %w", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = st.Close(closeCtx)
	}()

	// Prometheus registry: built early so cloudpublisher + eventstream
	// + the run-console WS handler all share the same registry.
	mreg := metrics.New()

	// AES-GCM master key for sealing BYOK + OAuth credentials at
	// rest. Built early so the publisher can pick up the BYOK store.
	sealer, err := secrets.NewAESGCMSealerFromBase64(cfg.Auth.SecretsKey)
	if err != nil {
		return fmt.Errorf("server: build sealer: %w", err)
	}
	apiKeysStore := secrets.NewMongoApiKeyStore(st.DB())
	if err := apiKeysStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure api_keys schema: %w", err)
	}
	genericSecretsStore := secrets.NewMongoGenericSecretStore(st.DB())
	if err := genericSecretsStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure generic_secrets schema: %w", err)
	}
	runSecretsStore := secrets.NewMongoRunSecretsStore(st.DB())
	if err := runSecretsStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure run_secrets schema: %w", err)
	}
	oauthStore := secrets.NewMongoOAuthStore(st.DB())
	if err := oauthStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure oauth schema: %w", err)
	}
	botBindingsStore := secrets.NewMongoBotSecretBindingStore(st.DB())
	if err := botBindingsStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure bot_secret_bindings schema: %w", err)
	}
	webhookStores := webhooks.NewMongoStores(st.DB())
	if err := webhooks.EnsureSchema(rootCtx, st.DB()); err != nil {
		return fmt.Errorf("server: ensure webhooks schema: %w", err)
	}
	forgeConnStore := forge.NewMongoConnectionStore(st.DB())
	if err := forgeConnStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure forge_connections schema: %w", err)
	}
	forgeIntegrationStore := forge.NewMongoRepoIntegrationStore(st.DB())
	if err := forgeIntegrationStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure repo_integrations schema: %w", err)
	}
	forgeOAuthAppStore := forge.NewMongoOAuthAppStore(st.DB())
	if err := forgeOAuthAppStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure forge_oauth_apps schema: %w", err)
	}
	orgSSOStore := orgsso.NewMongoStore(st.DB())
	if err := orgSSOStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure org_sso_providers schema: %w", err)
	}
	orgDomainStore := orgsso.NewMongoDomainStore(st.DB())
	if err := orgDomainStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure org_verified_domains schema: %w", err)
	}
	// Mongo-backed OIDC state store: PendingAuth must survive across replicas
	// (an OIDC /start on pod A and /callback on pod B), which the per-process
	// memory store can't guarantee in HA.
	oidcStateStore := oidc.NewMongoStateStore(st.DB(), 10*time.Minute)
	if err := oidcStateStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure oidc_states schema: %w", err)
	}
	orgUsageCounter := orgusage.NewMongoCounter(st.DB())
	if err := orgusage.EnsureSchema(rootCtx, st.DB()); err != nil {
		return fmt.Errorf("server: ensure org_usage schema: %w", err)
	}
	auditStore := audit.NewMongoStore(st.DB())
	if err := audit.EnsureSchema(rootCtx, st.DB()); err != nil {
		return fmt.Errorf("server: ensure audit schema: %w", err)
	}
	// Hosted marketplace (Mongo-backed) — opt-in for cloud via
	// ITERION_CLOUD_MARKETPLACE because the submit/install paths are
	// local-mode only today (cloud is rejected pending a vetted submission
	// flow — see pkg/server/marketplace_routes.go). When enabled it surfaces
	// the read-only browse view + sets marketplace_enabled. Local self-host
	// wires a JSONStore unconditionally in pkg/cli/studio.go.
	var marketplaceStore marketplace.Store
	if enabled, _ := strconv.ParseBool(os.Getenv("ITERION_CLOUD_MARKETPLACE")); enabled {
		if err := marketplace.EnsureSchema(rootCtx, st.DB()); err != nil {
			return fmt.Errorf("server: ensure marketplace schema: %w", err)
		}
		marketplaceStore = marketplace.NewMongoStore(st.DB())
	}
	patStore := pat.NewMongoStore(st.DB())
	if err := pat.EnsureSchema(rootCtx, st.DB()); err != nil {
		return fmt.Errorf("server: ensure pat schema: %w", err)
	}
	// ITERION_PAT_MAX_TTL (Go duration, e.g. "2160h" = 90 days) caps
	// every personal access token's lifetime. Unset = no platform cap.
	var patMaxTTL time.Duration
	if v := os.Getenv("ITERION_PAT_MAX_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			patMaxTTL = d
		} else {
			logger.Warn("server: invalid ITERION_PAT_MAX_TTL %q ignored", v)
		}
	}
	memStore := mongostore.NewMongoMemoryStore(st.DB())
	if err := memStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure memory schema: %w", err)
	}

	pub, err := cloudpublisher.New(cloudpublisher.Config{
		NATS:           natsConn,
		Store:          st,
		MongoColl:      st.RunsCollection(),
		Logger:         logger,
		Metrics:        mreg,
		ApiKeys:        apiKeysStore,
		GenericSecrets: genericSecretsStore,
		BotBindings:    botBindingsStore,
		RunSecrets:     runSecretsStore,
		Sealer:         sealer,
		OAuthForfait:   oauthStore,
	})
	if err != nil {
		return fmt.Errorf("server: build cloud publisher: %w", err)
	}

	// Mongo change-stream event source so the WS handler streams
	// runner-pod events (the local broker would only see this
	// process's writes). Plan §F (T-21, T-22).
	mongoSource := eventstream.NewMongo(st.EventsCollection(), logger).WithMetrics(mreg)
	eventSrc := runview.NewEventSourceAdapter(mongoSource)

	disableAuth, _ := strconv.ParseBool(os.Getenv("ITERION_DISABLE_AUTH"))

	// Wire auth (identity store, JWT signer, refresh sessions) and
	// the OIDC connector registry. Bootstrap a super-admin if the
	// env var is set and no user matches yet.
	identityStore := identity.NewMongoStore(st.DB())
	if err := identityStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure identity schema: %w", err)
	}
	sessions := iterauth.NewMongoSessionStore(st.DB())
	if err := sessions.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure sessions schema: %w", err)
	}

	signer, err := iterauth.NewJWTSigner(cfg.Auth.JWTSecret, cfg.Auth.AccessTTL)
	if err != nil {
		return fmt.Errorf("server: build jwt signer: %w", err)
	}
	// SMTP: ITERION_SMTP_HOST switches the real mailer on; otherwise the log
	// fallback keeps flows testable and server_info reports email_enabled=false
	// so the SPA hides forgot-password.
	mailer, err := buildMailer(logger)
	if err != nil {
		return err
	}
	resetStore := iterauth.NewMongoPasswordResetStore(st.DB())
	if err := resetStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure password_resets schema: %w", err)
	}

	authSvc, err := iterauth.NewService(iterauth.Config{
		Store:                    identityStore,
		Sessions:                 sessions,
		Signer:                   signer,
		SignupMode:               iterauth.SignupMode(cfg.Auth.SignupMode),
		RefreshTTL:               cfg.Auth.RefreshTTL,
		Logger:                   logger,
		Resets:                   resetStore,
		Mailer:                   mailer,
		PublicURL:                cfg.Auth.PublicURL,
		OrgSSO:                   orgSSOStore,
		Domains:                  orgDomainStore,
		TrustedAutoLinkProviders: cfg.Auth.TrustedAutoLinkProviders,
	})
	if err != nil {
		return fmt.Errorf("server: build auth service: %w", err)
	}

	if err := bootstrapAdmin(rootCtx, cfg, identityStore, authSvc, disableAuth, logger); err != nil {
		return err
	}

	registry := buildOIDCRegistry(cfg)

	if disableAuth {
		logger.Warn("server: ITERION_DISABLE_AUTH set — /api/* endpoints are unauthenticated; do not expose the server publicly")
	}

	// Run-health alerting: webhook (Slack/Discord) + always-on browser
	// toast sink. Deep links use the externally-reachable PublicURL so
	// webhook recipients get a clickable /runs/<id> link. The desktop
	// sink is nil here (cloud pods have no Wails runtime).
	alertSettings := &runview.AlertSettings{
		WebhookURL:   cfg.Alerts.Webhook.URL,
		StallTimeout: cfg.Alerts.StallTimeout,
		BaseURL:      cfg.Auth.PublicURL,
	}

	// Bots: where the inbound-webhook bot resolution (botregistry.ResolveBotPath)
	// looks for recipes. The official image ships the catalog at /opt/iterion/bots
	// and sets ITERION_BOTS_PATH; operators may override with a colon-separated
	// list. Empty → no webhook bot resolution (studio still discovers via WorkDir).
	var botsPaths []string
	if bp := os.Getenv("ITERION_BOTS_PATH"); bp != "" {
		botsPaths = filepath.SplitList(bp)
	}

	srv := server.New(server.Config{
		Port:                   serverOpts.port,
		Bind:                   serverOpts.bind,
		Bots:                   server.BotsConfig{Paths: botsPaths},
		WorkDir:                serverOpts.dir,
		Store:                  st,
		Alerts:                 alertSettings,
		LaunchPublisher:        pub,
		EventSource:            eventSrc,
		Mode:                   string(iterconfig.ModeCloud),
		AuthService:            authSvc,
		AuthSigner:             signer,
		OIDCRegistry:           registry,
		OIDCStates:             oidcStateStore,
		OrgSSO:                 orgSSOStore,
		OrgDomains:             orgDomainStore,
		ApiKeys:                apiKeysStore,
		GenericSecrets:         genericSecretsStore,
		BotBindings:            botBindingsStore,
		ForgeConnections:       forgeConnStore,
		ForgeIntegrations:      forgeIntegrationStore,
		ForgeOAuthApps:         forgeOAuthAppStore,
		ForgeGitHubApp:         forgeGitHubAppFromEnv(),
		WebhookConfigs:         webhookStores.Configs,
		WebhookDeliveries:      webhookStores.Deliveries,
		WebhookCounter:         webhookStores.Counter,
		OrgUsage:               orgUsageCounter,
		OrgDefaults:            orgLimitDefaultsFromEnv(),
		Audit:                  auditStore,
		Marketplace:            marketplaceStore,
		PATs:                   patStore,
		PATMaxTTL:              patMaxTTL,
		Queue:                  natsConn,
		MemoryStore:            memStore,
		RunSecrets:             runSecretsStore,
		Sealer:                 sealer,
		OAuthForfait:           oauthStore,
		AnthropicOAuthClientID: cfg.Auth.OAuthForfait.AnthropicClientID,
		CodexOAuthClientID:     cfg.Auth.OAuthForfait.CodexClientID,
		AccessTTL:              cfg.Auth.AccessTTL,
		RefreshTTL:             cfg.Auth.RefreshTTL,
		PublicURL:              cfg.Auth.PublicURL,
		SignupMode:             cfg.Auth.SignupMode,
		CookieDomain:           cfg.Auth.CookieDomain,
		CookieSecure:           cfg.Auth.CookieSecure,
		DisableAuth:            disableAuth,
		Metrics:                mreg,
		// /readyz pings each dependency under a 1s deadline so kubelet
		// readiness probes flip to "not ready" the moment a backend
		// drops, instead of returning 200 against a stub.
		ReadinessChecks: map[string]server.ReadinessCheck{
			"mongo": st.Ping,
			"nats":  natsConn.Ping,
			"s3":    bc.Ping,
		},
	}, logger)

	// Prometheus metrics on a dedicated port (plan §F T-40). Bound
	// synchronously so a port-conflict surfaces at boot, not later.
	metricsAddr := fmt.Sprintf(":%d", cfg.Metrics.Port)
	metricsSrv, err := mreg.StartServer(metricsAddr, logger)
	if err != nil {
		return fmt.Errorf("server: start metrics: %w", err)
	}
	defer func() { _ = metrics.ShutdownServer(metricsSrv) }()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-rootCtx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// bootstrapAdmin reconciles the configured super-admin per the
// ITERION_BOOTSTRAP_ADMIN_* policy. No-op when no email is configured or auth
// is disabled. A declared password (ITERION_BOOTSTRAP_ADMIN_PASSWORD, a k8s
// secret) is AUTHORITATIVE and reconciled idempotently; an un-activated admin
// gets a fresh temp password re-issued; an empty users collection gets the
// admin created. Passwords are never logged on the declarative path.
func bootstrapAdmin(ctx context.Context, cfg iterconfig.Config, identityStore *identity.MongoStore, authSvc *iterauth.Service, disableAuth bool, logger *iterlog.Logger) error {
	email := cfg.Auth.BootstrapAdminEmail
	if email == "" || disableAuth {
		return nil
	}
	declaredPW := strings.TrimSpace(cfg.Auth.BootstrapAdminPassword)
	existing, getErr := identityStore.GetUserByEmail(ctx, email)
	switch {
	case getErr == nil:
		switch {
		case declaredPW != "":
			// Declarative admin: the secret is AUTHORITATIVE — ensure an active
			// super-admin whose password matches it, resetting only on drift so
			// idempotent restarts are no-ops. The password is never logged.
			changed := false
			if !existing.IsSuperAdmin {
				existing.IsSuperAdmin = true
				changed = true
			}
			if existing.Status != identity.UserStatusActive {
				existing.Status = identity.UserStatusActive
				changed = true
			}
			if ok, _ := iterauth.VerifyPassword(declaredPW, existing.PasswordHash); !ok {
				hash, err := iterauth.HashPassword(declaredPW)
				if err != nil {
					return fmt.Errorf("server: hash bootstrap password: %w", err)
				}
				existing.PasswordHash = hash
				changed = true
				logger.Info("server: BOOTSTRAP super-admin %s password reconciled to ITERION_BOOTSTRAP_ADMIN_PASSWORD", email)
			}
			if changed {
				if err := identityStore.UpdateUser(ctx, existing); err != nil {
					return fmt.Errorf("server: reconcile bootstrap admin: %w", err)
				}
			}
		case existing.Status == identity.UserStatusPendingPasswordChange:
			// No declared password and the admin was never activated. Re-issue
			// a fresh temp password so an operator who lost the first one (e.g.
			// the pod restarted before it was captured) can recover by
			// restarting. An already-active admin's password is never reset.
			pw, err := randomBootstrapPassword()
			if err != nil {
				return fmt.Errorf("server: bootstrap password: %w", err)
			}
			hash, err := iterauth.HashPassword(pw)
			if err != nil {
				return fmt.Errorf("server: hash bootstrap password: %w", err)
			}
			existing.PasswordHash = hash
			if err := identityStore.UpdateUser(ctx, existing); err != nil {
				return fmt.Errorf("server: re-issue bootstrap admin: %w", err)
			}
			logger.Warn("server: BOOTSTRAP super-admin %s still pending — re-issued temp_password=%s (rotate via POST /api/auth/password/change, or set ITERION_BOOTSTRAP_ADMIN_PASSWORD)", email, pw)
		}
		// getErr == nil && active && no declared password → no-op.
	case errors.Is(getErr, identity.ErrNotFound):
		// First boot with an empty users collection → create the admin.
		count, err := identityStore.UserCount(ctx)
		if err != nil {
			return fmt.Errorf("server: user count: %w", err)
		}
		if count == 0 {
			if declaredPW != "" {
				// Declarative: create an ACTIVE super-admin with the secret
				// password — no temp-password dance, GitOps-friendly.
				if _, _, err := authSvc.CreateUserAndPersonalTeam(ctx, email, "Bootstrap admin", declaredPW, true, identity.UserStatusActive); err != nil {
					return fmt.Errorf("server: bootstrap admin: %w", err)
				}
				logger.Info("server: BOOTSTRAP super-admin created (active) from ITERION_BOOTSTRAP_ADMIN_PASSWORD — email=%s", email)
			} else {
				pw, err := randomBootstrapPassword()
				if err != nil {
					return fmt.Errorf("server: bootstrap password: %w", err)
				}
				if _, _, err := authSvc.CreateUserAndPersonalTeam(ctx, email, "Bootstrap admin", pw, true, identity.UserStatusPendingPasswordChange); err != nil {
					return fmt.Errorf("server: bootstrap admin: %w", err)
				}
				logger.Warn("server: BOOTSTRAP super-admin created — email=%s temp_password=%s (rotate via POST /api/auth/password/change, or set ITERION_BOOTSTRAP_ADMIN_PASSWORD)", email, pw)
			}
		}
	case getErr != nil:
		return fmt.Errorf("server: bootstrap admin lookup: %w", getErr)
	}
	return nil
}

// buildMailer returns an SMTP mailer when ITERION_SMTP_HOST is set, else a
// log-only fallback (which keeps auth flows testable and reports
// email_enabled=false to the SPA).
func buildMailer(logger *iterlog.Logger) (mail.Mailer, error) {
	host := os.Getenv("ITERION_SMTP_HOST")
	if host == "" {
		return &mail.LogMailer{Logger: logger}, nil
	}
	port, _ := strconv.Atoi(os.Getenv("ITERION_SMTP_PORT"))
	startTLS := true
	if v := os.Getenv("ITERION_SMTP_STARTTLS"); v != "" {
		startTLS, _ = strconv.ParseBool(v)
	}
	smtpMailer, err := mail.NewSMTP(mail.Config{
		Host:     host,
		Port:     port,
		Username: os.Getenv("ITERION_SMTP_USERNAME"),
		Password: os.Getenv("ITERION_SMTP_PASSWORD"),
		From:     os.Getenv("ITERION_SMTP_FROM"),
		StartTLS: startTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("server: smtp config: %w", err)
	}
	logger.Info("server: SMTP mailer enabled (host=%s)", host)
	return smtpMailer, nil
}

// buildOIDCRegistry wires the enabled OIDC connectors (Google, GitHub,
// generic) into a fresh registry.
func buildOIDCRegistry(cfg iterconfig.Config) *oidc.Registry {
	registry := oidc.NewRegistry()
	if cfg.Auth.OIDC.Google.Enabled {
		registry.Register(oidc.NewGoogleConnector(cfg.Auth.OIDC.Google.ClientID, cfg.Auth.OIDC.Google.ClientSecret, cfg.Auth.OIDC.Google.DisplayName))
	}
	if cfg.Auth.OIDC.GitHub.Enabled {
		registry.Register(oidc.NewGitHubConnector(cfg.Auth.OIDC.GitHub.ClientID, cfg.Auth.OIDC.GitHub.ClientSecret, cfg.Auth.OIDC.GitHub.DisplayName))
	}
	if cfg.Auth.OIDC.Generic.Enabled {
		registry.Register(oidc.NewGenericConnector(cfg.Auth.OIDC.Generic.IssuerURL, cfg.Auth.OIDC.Generic.ClientID, cfg.Auth.OIDC.Generic.ClientSecret, cfg.Auth.OIDC.Generic.DisplayName, cfg.Auth.OIDC.Generic.Scopes))
	}
	return registry
}
