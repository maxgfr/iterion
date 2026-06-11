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
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SocialGouv/iterion/pkg/audit"
	iterauth "github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/auth/oidc"
	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/SocialGouv/iterion/pkg/cloud/metrics"
	"github.com/SocialGouv/iterion/pkg/cloud/tracing"
	iterconfig "github.com/SocialGouv/iterion/pkg/config"
	"github.com/SocialGouv/iterion/pkg/identity"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/mail"
	"github.com/SocialGouv/iterion/pkg/orgusage"
	"github.com/SocialGouv/iterion/pkg/pat"
	natsq "github.com/SocialGouv/iterion/pkg/queue/nats"
	"github.com/SocialGouv/iterion/pkg/runview"
	"github.com/SocialGouv/iterion/pkg/runview/eventstream"
	"github.com/SocialGouv/iterion/pkg/secrets"
	"github.com/SocialGouv/iterion/pkg/server"
	"github.com/SocialGouv/iterion/pkg/server/cloudpublisher"
	"github.com/SocialGouv/iterion/pkg/store/blob"
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

	bc, err := blob.NewS3(rootCtx, blob.Config{
		Endpoint:        cfg.S3.Endpoint,
		Region:          cfg.S3.Region,
		Bucket:          cfg.S3.Bucket,
		AccessKeyID:     cfg.S3.AccessKeyID,
		SecretAccessKey: cfg.S3.SecretAccessKey,
		UsePathStyle:    cfg.S3.UsePathStyle,
	})
	if err != nil {
		return fmt.Errorf("server: build blob client: %w", err)
	}
	defer func() { _ = bc.Close() }()

	// Server-side store: no NATS lock provider — the server never
	// executes runs, only publishes them. The runner pod is the
	// only place that takes leases.
	st, err := mongostore.New(rootCtx, mongostore.Config{
		URI:           cfg.Mongo.URI,
		Database:      cfg.Mongo.DB,
		EventsTTLDays: cfg.Mongo.EventsTTLDays,
		Logger:        logger,
		Blob:          bc,
	})
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
	orgUsageCounter := orgusage.NewMongoCounter(st.DB())
	if err := orgusage.EnsureSchema(rootCtx, st.DB()); err != nil {
		return fmt.Errorf("server: ensure org_usage schema: %w", err)
	}
	auditStore := audit.NewMongoStore(st.DB())
	if err := audit.EnsureSchema(rootCtx, st.DB()); err != nil {
		return fmt.Errorf("server: ensure audit schema: %w", err)
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
	// SMTP: ITERION_SMTP_HOST switches the real mailer on; otherwise
	// the log fallback keeps flows testable and server_info reports
	// email_enabled=false so the SPA hides forgot-password.
	var mailer mail.Mailer = &mail.LogMailer{Logger: logger}
	if host := os.Getenv("ITERION_SMTP_HOST"); host != "" {
		port, _ := strconv.Atoi(os.Getenv("ITERION_SMTP_PORT"))
		startTLS := true
		if v := os.Getenv("ITERION_SMTP_STARTTLS"); v != "" {
			startTLS, _ = strconv.ParseBool(v)
		}
		smtpMailer, merr := mail.NewSMTP(mail.Config{
			Host:     host,
			Port:     port,
			Username: os.Getenv("ITERION_SMTP_USERNAME"),
			Password: os.Getenv("ITERION_SMTP_PASSWORD"),
			From:     os.Getenv("ITERION_SMTP_FROM"),
			StartTLS: startTLS,
		})
		if merr != nil {
			return fmt.Errorf("server: smtp config: %w", merr)
		}
		mailer = smtpMailer
		logger.Info("server: SMTP mailer enabled (host=%s)", host)
	}
	resetStore := iterauth.NewMongoPasswordResetStore(st.DB())
	if err := resetStore.EnsureSchema(rootCtx); err != nil {
		return fmt.Errorf("server: ensure password_resets schema: %w", err)
	}

	authSvc, err := iterauth.NewService(iterauth.Config{
		Store:      identityStore,
		Sessions:   sessions,
		Signer:     signer,
		SignupMode: iterauth.SignupMode(cfg.Auth.SignupMode),
		RefreshTTL: cfg.Auth.RefreshTTL,
		Logger:     logger,
		Resets:     resetStore,
		Mailer:     mailer,
		PublicURL:  cfg.Auth.PublicURL,
	})
	if err != nil {
		return fmt.Errorf("server: build auth service: %w", err)
	}

	if email := cfg.Auth.BootstrapAdminEmail; email != "" && !disableAuth {
		existing, getErr := identityStore.GetUserByEmail(rootCtx, email)
		switch {
		case getErr == nil && existing.Status == identity.UserStatusPendingPasswordChange:
			// The configured bootstrap admin exists but was never activated.
			// Re-issue a fresh temp password so an operator who lost the first
			// one (e.g. the pod restarted before it was captured) can recover
			// by restarting, instead of being permanently locked out. An
			// already-active admin's password is never reset this way.
			pw, err := randomBootstrapPassword()
			if err != nil {
				return fmt.Errorf("server: bootstrap password: %w", err)
			}
			hash, err := iterauth.HashPassword(pw)
			if err != nil {
				return fmt.Errorf("server: hash bootstrap password: %w", err)
			}
			existing.PasswordHash = hash
			if err := identityStore.UpdateUser(rootCtx, existing); err != nil {
				return fmt.Errorf("server: re-issue bootstrap admin: %w", err)
			}
			logger.Warn("server: BOOTSTRAP super-admin %s still pending — re-issued temp_password=%s (rotate via POST /api/auth/password/change)", email, pw)
		case errors.Is(getErr, identity.ErrNotFound):
			// First boot with an empty users collection → create the admin.
			count, err := identityStore.UserCount(rootCtx)
			if err != nil {
				return fmt.Errorf("server: user count: %w", err)
			}
			if count == 0 {
				pw, err := randomBootstrapPassword()
				if err != nil {
					return fmt.Errorf("server: bootstrap password: %w", err)
				}
				if _, _, err := authSvc.CreateUserAndPersonalTeam(rootCtx, email, "Bootstrap admin", pw, true, identity.UserStatusPendingPasswordChange); err != nil {
					return fmt.Errorf("server: bootstrap admin: %w", err)
				}
				logger.Warn("server: BOOTSTRAP super-admin created — email=%s temp_password=%s (rotate via POST /api/auth/password/change)", email, pw)
			}
		case getErr != nil:
			return fmt.Errorf("server: bootstrap admin lookup: %w", getErr)
		}
		// getErr == nil && status active/disabled → already onboarded; no-op.
	}

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
		ApiKeys:                apiKeysStore,
		GenericSecrets:         genericSecretsStore,
		BotBindings:            botBindingsStore,
		WebhookConfigs:         webhookStores.Configs,
		WebhookDeliveries:      webhookStores.Deliveries,
		WebhookCounter:         webhookStores.Counter,
		OrgUsage:               orgUsageCounter,
		OrgDefaults:            orgLimitDefaultsFromEnv(),
		Audit:                  auditStore,
		PATs:                   patStore,
		PATMaxTTL:              patMaxTTL,
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
