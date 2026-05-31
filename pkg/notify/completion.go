// Package notify delivers run-completion webhooks — a generic
// "this run reached a terminal state, here is its final answer"
// callback POSTed to a URL supplied at launch time.
//
// This is the engine-side primitive that lets an external integration
// (a chat adapter, a CI bridge, anything) trigger a run asynchronously
// and be told when it finished without polling. It is deliberately
// neutral: the payload is a plain JSON envelope; the platform-specific
// glue (Slack/Mattermost formatting, thread routing) lives entirely in
// the receiver.
//
// The callback URL is treated as attacker-influenced (it arrives over
// the launch API), so every delivery passes an SSRF guard: only
// http/https, and — unless explicitly opted in — never a loopback,
// link-local, RFC-1918, or cloud-metadata host. Resolution fails closed.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	iterlog "github.com/SocialGouv/iterion/pkg/log"
	"github.com/SocialGouv/iterion/pkg/store"
)

// PayloadVersion is the schema version of CompletionPayload. Bumped on
// any breaking change so receivers can gate on it.
const PayloadVersion = 1

// DefaultAnswerField is the artifact-data key the notifier reads to
// populate CompletionPayload.FinalAnswer when no explicit field is
// configured on the run.
const DefaultAnswerField = "final_answer"

// CompletionPayload is the JSON body POSTed to a run's callback URL
// when the run reaches a terminal state. Stable wire contract — see
// docs/completion-webhooks.md.
type CompletionPayload struct {
	V             int    `json:"v"`
	RunID         string `json:"run_id"`
	Status        string `json:"status"`
	WorkflowName  string `json:"workflow_name,omitempty"`
	FinalAnswer   string `json:"final_answer,omitempty"`
	FinalAnswerN  string `json:"final_answer_node,omitempty"`
	Error         string `json:"error,omitempty"`
	CallbackToken string `json:"callback_token,omitempty"`
}

// Notifier delivers CompletionPayloads. Construct once and share; it is
// safe for concurrent use. A nil *Notifier is a valid no-op so callers
// can hold one unconditionally.
type Notifier struct {
	client       *http.Client
	logger       *iterlog.Logger
	allowPrivate bool
}

// Option configures a Notifier.
type Option func(*Notifier)

// WithAllowPrivate permits callback URLs that resolve to loopback,
// link-local, RFC-1918, or cloud-metadata addresses. Off by default
// (SSRF guard). Turn it on only for self-hosted deployments where the
// callback receiver genuinely lives on a private network alongside
// iterion.
func WithAllowPrivate(allow bool) Option {
	return func(n *Notifier) { n.allowPrivate = allow }
}

// New builds a Notifier. timeout bounds each delivery attempt; zero
// applies a 15s default.
func New(logger *iterlog.Logger, timeout time.Duration, opts ...Option) *Notifier {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	n := &Notifier{
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}
	for _, o := range opts {
		o(n)
	}
	return n
}

// FireForRun loads the run, and if it carries a callback URL and has
// reached a terminal-for-notification state, resolves its final answer
// and POSTs a CompletionPayload. Best-effort: every failure is logged,
// none is returned — a webhook delivery must never affect run outcome.
//
// Paused states (paused_waiting_human / paused_operator) are skipped:
// the run is not done, it is waiting. Resume will call FireForRun again
// when the run actually terminates.
func (n *Notifier) FireForRun(ctx context.Context, st store.RunStore, runID string) {
	if n == nil || st == nil {
		return
	}
	run, err := st.LoadRun(ctx, runID)
	if err != nil {
		if n.logger != nil {
			n.logger.Warn("completion webhook: load run %s: %v", runID, err)
		}
		return
	}
	if run.CallbackURL == "" {
		return // no callback requested — the common case
	}
	if !shouldNotify(run.Status) {
		return
	}

	answer, node := n.resolveFinalAnswer(ctx, st, run)
	payload := CompletionPayload{
		V:             PayloadVersion,
		RunID:         run.ID,
		Status:        string(run.Status),
		WorkflowName:  run.WorkflowName,
		FinalAnswer:   answer,
		FinalAnswerN:  node,
		Error:         run.Error,
		CallbackToken: run.CallbackToken,
	}
	n.deliver(ctx, run.CallbackURL, payload)
}

// shouldNotify reports whether a status warrants a completion callback.
// Running and the two paused states are excluded — they are not
// terminal-for-notification.
func shouldNotify(s store.RunStatus) bool {
	switch s {
	case store.RunStatusFinished,
		store.RunStatusFailed,
		store.RunStatusFailedResumable,
		store.RunStatusCancelled:
		return true
	default:
		return false
	}
}

// resolveFinalAnswer extracts the run's user-facing answer string.
//
//   - When the run names CallbackAnswerNode, the latest artifact for
//     that node is read and its DefaultAnswerField returned.
//   - Otherwise every node in Run.ArtifactIndex is probed (latest
//     version) and the first DefaultAnswerField string wins.
//
// Returns ("", "") when no answer artifact is present — a legitimate
// outcome (e.g. a facilitator bot that chose to stay silent), which the
// receiver interprets as "post nothing".
func (n *Notifier) resolveFinalAnswer(ctx context.Context, st store.RunStore, run *store.Run) (answer, node string) {
	if run.CallbackAnswerNode != "" {
		// Pinned: read ONLY the named node. The hard-stop here is
		// deliberate — a caller that named a node wants that node's
		// answer or none. Falling back to scanning other nodes could
		// post an answer the caller never intended (e.g. an internal
		// reasoning artifact), so a named-but-empty node correctly
		// yields silence.
		if s, ok := n.answerFromNode(ctx, st, run.ID, run.CallbackAnswerNode); ok {
			return s, run.CallbackAnswerNode
		}
		return "", ""
	}
	for nodeID := range run.ArtifactIndex {
		if s, ok := n.answerFromNode(ctx, st, run.ID, nodeID); ok {
			return s, nodeID
		}
	}
	return "", ""
}

// answerFromNode reads the node's latest artifact and returns its
// DefaultAnswerField as a string when present and non-empty.
func (n *Notifier) answerFromNode(ctx context.Context, st store.RunStore, runID, nodeID string) (string, bool) {
	art, err := st.LoadLatestArtifact(ctx, runID, nodeID)
	if err != nil || art == nil || art.Data == nil {
		return "", false
	}
	raw, ok := art.Data[DefaultAnswerField]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// deliver POSTs the payload after vetting the URL. All paths are
// best-effort with logging; the callback URL itself is never logged
// (it can embed a secret token in its query string).
func (n *Notifier) deliver(ctx context.Context, rawURL string, payload CompletionPayload) {
	if err := n.vetURL(rawURL); err != nil {
		if n.logger != nil {
			n.logger.Warn("completion webhook: rejected callback URL for run %s: %v", payload.RunID, err)
		}
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		if n.logger != nil {
			n.logger.Warn("completion webhook: marshal payload for run %s: %v", payload.RunID, err)
		}
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		if n.logger != nil {
			n.logger.Warn("completion webhook: build request for run %s failed", payload.RunID)
		}
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "iterion-completion-webhook/1")

	resp, err := n.client.Do(req)
	if err != nil {
		if n.logger != nil {
			n.logger.Warn("completion webhook: delivery failed for run %s", payload.RunID)
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && n.logger != nil {
		n.logger.Warn("completion webhook: receiver returned %d for run %s", resp.StatusCode, payload.RunID)
	}
}

// vetURL enforces the SSRF guard: http/https only, a host must be
// present, and — unless allowPrivate is set — the host must resolve
// exclusively to public-unicast addresses. Resolution fails closed (an
// unresolvable host is rejected). Mirrors the blocklist in
// pkg/server.isPublicUnicast.
func (n *Notifier) vetURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed (want http/https)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if n.allowPrivate {
		return nil
	}

	// Numeric literal: validate directly.
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicUnicast(ip) {
			return fmt.Errorf("address %s is not a public unicast IP", ip)
		}
		return nil
	}

	// Refuse cluster-internal aliases outright (service meshes re-route
	// these even when DNS has no record).
	lower := strings.ToLower(host)
	if strings.HasSuffix(lower, ".svc.cluster.local") ||
		strings.HasSuffix(lower, ".svc") ||
		lower == "kubernetes.default" ||
		lower == "metadata.google.internal" {
		return fmt.Errorf("hostname %q is reserved for cluster-internal services", host)
	}

	addrs, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", host, err) // fail closed
	}
	if len(addrs) == 0 {
		return fmt.Errorf("resolve %q: no addresses", host)
	}
	for _, a := range addrs {
		if !isPublicUnicast(a) {
			return fmt.Errorf("resolved address %s is not a public unicast IP", a)
		}
	}
	return nil
}

// isPublicUnicast reports whether ip is safe to POST to from the
// iterion host/pod. Blocks the standard SSRF categories plus cloud
// metadata endpoints. Mirrors pkg/server.isPublicUnicast.
func isPublicUnicast(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	switch ip.String() {
	case "169.254.169.254", "fe80::a9fe:a9fe", "100.100.100.200":
		return false
	}
	return true
}
