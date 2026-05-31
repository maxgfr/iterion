package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/SocialGouv/iterion/pkg/notify"
)

// config is the adapter's runtime configuration, read from env.
type config struct {
	listenAddr  string // adapter HTTP bind, e.g. :8090
	callbackURL string // public URL iterion POSTs completion webhooks to
	actionURL   string // public URL Mattermost POSTs consent clicks to

	iterionBase  string // iterion server base URL
	iterionToken string // optional bearer for iterion auth
	clarifyPath  string // path to examples/clarify/main.bot
	model        string // optional model override

	mmHTTPBase  string
	mmWSURL     string
	mmToken     string
	mmBotUserID string
}

func loadConfig() (config, error) {
	c := config{
		listenAddr:   env("CLARIFY_LISTEN_ADDR", ":8090"),
		callbackURL:  os.Getenv("CLARIFY_CALLBACK_URL"),
		actionURL:    os.Getenv("CLARIFY_ACTION_URL"),
		iterionBase:  env("ITERION_BASE_URL", "http://localhost:8080"),
		iterionToken: os.Getenv("ITERION_AUTH_TOKEN"),
		clarifyPath:  env("CLARIFY_BOT_PATH", "examples/clarify/main.bot"),
		model:        os.Getenv("CLARIFY_MODEL"),
		mmHTTPBase:   os.Getenv("MM_HTTP_BASE"),
		mmWSURL:      os.Getenv("MM_WS_URL"),
		mmToken:      os.Getenv("MM_BOT_TOKEN"),
		mmBotUserID:  os.Getenv("MM_BOT_USER_ID"),
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	botSource, err := os.ReadFile(cfg.clarifyPath)
	if err != nil {
		log.Fatalf("read clarify bot %q: %v", cfg.clarifyPath, err)
	}

	launchClient := NewLaunchClient(LaunchConfig{
		BaseURL:     cfg.iterionBase,
		AuthToken:   cfg.iterionToken,
		BotSource:   string(botSource),
		CallbackURL: cfg.callbackURL,
		Model:       cfg.model,
	})
	driver := NewMattermostDriver(MattermostConfig{
		HTTPBase:  cfg.mmHTTPBase,
		WSURL:     cfg.mmWSURL,
		Token:     cfg.mmToken,
		BotUserID: cfg.mmBotUserID,
		ActionURL: cfg.actionURL,
	})
	coord := NewCoordinator(driver, newHeuristicFilter(), launchClient)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// HTTP server: completion callback + Mattermost action endpoint.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /callback", completionHandler(ctx, coord))
	mux.HandleFunc("POST /mm/actions", actionHandler(coord))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	srv := &http.Server{Addr: cfg.listenAddr, Handler: mux}
	go func() {
		log.Printf("mattermost-clarify: HTTP listening on %s", cfg.listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	// Drive the Mattermost WS loop.
	go func() {
		if err := coord.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("coordinator stopped: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("mattermost-clarify: shutting down")
	_ = srv.Shutdown(context.Background())
}

// completionHandler receives iterion's run-completion webhook and posts
// the final answer back into the originating thread.
func completionHandler(ctx context.Context, coord *Coordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload notify.CompletionPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		if err := coord.HandleCompletion(ctx, payload); err != nil {
			log.Printf("completion for run %s: %v", payload.RunID, err)
		}
		w.WriteHeader(http.StatusOK)
	}
}

// mmAction is the Mattermost interactive-action request body.
type mmAction struct {
	UserID  string `json:"user_id"`
	Context struct {
		Action    string `json:"action"`
		Granted   bool   `json:"granted"`
		UserID    string `json:"user_id"`
		ChannelID string `json:"channel_id"`
		RootID    string `json:"root_id"`
	} `json:"context"`
}

// actionHandler translates a Mattermost consent button click into a
// ConsentAction and acknowledges it.
func actionHandler(coord *Coordinator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var a mmAction
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, "bad action", http.StatusBadRequest)
			return
		}
		if a.Context.Action == "consent" {
			coord.HandleConsent(ConsentAction{
				Thread:  ThreadRef{ChannelID: a.Context.ChannelID, RootID: a.Context.RootID},
				UserID:  a.Context.UserID,
				Granted: a.Context.Granted,
			})
		}
		// Update the ephemeral prompt so the user sees their choice stuck.
		msg := "✅ Thanks — your messages in this thread will be sent to @clarify-bot."
		if !a.Context.Granted {
			msg = "🚫 Understood — your messages in this thread will NOT be sent."
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ephemeral_text": msg})
	}
}
