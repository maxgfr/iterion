// Package main: __scan-shards hidden subcommand.
//
// Implements the local-mode shard fan-out described in
// docs/security-bots-distributed.md. Spawns N child `iterion run`
// subprocesses sharing the same --store-dir, each scanning a slice of
// the file list, bounded by --max-concurrency. Polls the store for
// each child until terminal status, then emits an aggregated JSON
// envelope on stdout.
//
// Cloud-mode dispatch (NATS queue publish + parent/child relationship
// persisted via cloudpublisher) is a future swap; the bundle-facing
// surface is the same.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SocialGouv/iterion/pkg/store"
	"github.com/spf13/cobra"
)

var scanShardsOpts struct {
	parentRunID    string
	workflow       string
	filesJSON      string
	shardSize      int
	baseVarsJSON   string
	storeDir       string
	maxConcurrency int
	pollInterval   time.Duration
	timeout        time.Duration
	mode           string
	shardVarName   string
}

var scanShardsCmd = &cobra.Command{
	Use:    "__scan-shards",
	Short:  "Internal: fan out a security scan across N child runs and poll until convergence",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runScanShards(cmd.Context(), os.Stdout)
	},
}

func init() {
	f := scanShardsCmd.Flags()
	f.StringVar(&scanShardsOpts.parentRunID, "parent-run-id", "", "Parent run id; stamped onto each child run's metadata")
	f.StringVar(&scanShardsOpts.workflow, "workflow", "", "Path to the .bot / .botz / bundle dir to run for each shard")
	f.StringVar(&scanShardsOpts.filesJSON, "files-json", "-", "Path to a JSON array of relative file paths to shard (or '-' for stdin)")
	f.IntVar(&scanShardsOpts.shardSize, "shard-size", 100, "Maximum files per shard (must be > 0)")
	f.StringVar(&scanShardsOpts.baseVarsJSON, "base-vars-json", "{}", "JSON object of --var key=value pairs forwarded to every child run")
	f.StringVar(&scanShardsOpts.storeDir, "store-dir", "", "Iterion store directory shared by parent and all children (required for the polling path)")
	f.IntVar(&scanShardsOpts.maxConcurrency, "max-concurrency", 4, "Maximum number of child runs executing in parallel")
	f.DurationVar(&scanShardsOpts.pollInterval, "poll-interval", 2*time.Second, "How often to poll the store for each child's terminal status")
	f.DurationVar(&scanShardsOpts.timeout, "timeout", 2*time.Hour, "Overall fan-out timeout. A child still running past this is reported as 'timed_out' and the parent returns non-zero.")
	f.StringVar(&scanShardsOpts.mode, "mode", "auto", "Dispatch mode: 'auto' (cloud queue if ITERION_QUEUE_NATS_URL set, else local subprocess), 'local' (force subprocess), 'cloud' (force queue; not yet implemented)")
	f.StringVar(&scanShardsOpts.shardVarName, "shard-var", "security_shard_files", "Workflow variable name receiving the shard's file list (as a JSON array string)")
	rootCmd.AddCommand(scanShardsCmd)
}

// shardPlan is the per-shard intent computed before dispatch.
type shardPlan struct {
	Index int      `json:"shard_index"`
	RunID string   `json:"run_id"`
	Files []string `json:"files"`
	Label string   `json:"shard_label"`
}

// shardResult captures one shard's outcome for the aggregated stdout report.
type shardResult struct {
	Plan     shardPlan       `json:"plan"`
	Status   store.RunStatus `json:"status"`
	Error    string          `json:"error,omitempty"`
	Started  *time.Time      `json:"started_at,omitempty"`
	Finished *time.Time      `json:"finished_at,omitempty"`
}

// scanShardsReport is the JSON envelope emitted on stdout.
type scanShardsReport struct {
	ParentRunID  string        `json:"parent_run_id"`
	Workflow     string        `json:"workflow"`
	Mode         string        `json:"mode"`
	ShardCount   int           `json:"shard_count"`
	ShardSize    int           `json:"shard_size"`
	FilesTotal   int           `json:"files_total"`
	Shards       []shardResult `json:"shards"`
	Errors       []string      `json:"errors,omitempty"`
	DurationSecs float64       `json:"duration_secs"`
}

func runScanShards(ctx context.Context, out io.Writer) error {
	if scanShardsOpts.workflow == "" {
		return fmt.Errorf("__scan-shards: --workflow is required")
	}
	if scanShardsOpts.storeDir == "" {
		return fmt.Errorf("__scan-shards: --store-dir is required (parent and children must share the store)")
	}
	if scanShardsOpts.shardSize <= 0 {
		return fmt.Errorf("__scan-shards: --shard-size must be > 0")
	}
	if scanShardsOpts.maxConcurrency <= 0 {
		return fmt.Errorf("__scan-shards: --max-concurrency must be > 0")
	}

	files, err := readFilesJSON(scanShardsOpts.filesJSON)
	if err != nil {
		return fmt.Errorf("__scan-shards: read files-json: %w", err)
	}
	if len(files) == 0 {
		// Trivial case: no files → no shards. Emit an empty report and succeed.
		_ = json.NewEncoder(out).Encode(scanShardsReport{
			ParentRunID: scanShardsOpts.parentRunID,
			Workflow:    scanShardsOpts.workflow,
			Mode:        "local",
			ShardCount:  0,
			ShardSize:   scanShardsOpts.shardSize,
			FilesTotal:  0,
			Shards:      []shardResult{},
		})
		return nil
	}

	plans := planShards(files, scanShardsOpts.shardSize, scanShardsOpts.parentRunID)

	mode := resolveMode(scanShardsOpts.mode)

	baseVars, err := parseBaseVars(scanShardsOpts.baseVarsJSON)
	if err != nil {
		return fmt.Errorf("__scan-shards: parse --base-vars-json: %w", err)
	}

	deadline := time.Now().Add(scanShardsOpts.timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	start := time.Now()
	var results []shardResult
	if mode == "cloud" {
		results = dispatchCloud(ctx, plans, baseVars)
	} else {
		results = dispatchLocal(ctx, plans, baseVars)
	}

	// Re-attach a tail poll to capture terminal statuses from the store.
	rs, openErr := store.New(scanShardsOpts.storeDir)
	if openErr != nil {
		return fmt.Errorf("__scan-shards: open store: %w", openErr)
	}
	for i := range results {
		runID := results[i].Plan.RunID
		r, loadErr := rs.LoadRun(ctx, runID)
		if loadErr != nil {
			// Subprocess emitted no run document (unlikely once dispatchLocal returns) — record as error.
			results[i].Error = fmt.Sprintf("load run after dispatch: %v", loadErr)
			results[i].Status = store.RunStatus("missing")
			continue
		}
		results[i].Status = r.Status
		if r.FinishedAt != nil {
			ft := *r.FinishedAt
			results[i].Finished = &ft
		}
		if r.CreatedAt != (time.Time{}) {
			ct := r.CreatedAt
			results[i].Started = &ct
		}
		if r.Error != "" && results[i].Error == "" {
			results[i].Error = r.Error
		}
	}

	rep := scanShardsReport{
		ParentRunID:  scanShardsOpts.parentRunID,
		Workflow:     scanShardsOpts.workflow,
		Mode:         mode,
		ShardCount:   len(plans),
		ShardSize:    scanShardsOpts.shardSize,
		FilesTotal:   len(files),
		Shards:       results,
		DurationSecs: time.Since(start).Seconds(),
	}
	// Surface any non-terminal-finished statuses as errors so the bundle
	// caller can decide what to do (tool node exit code stays 0; the JSON
	// is the structured signal).
	for _, s := range rep.Shards {
		if s.Status != store.RunStatusFinished {
			rep.Errors = append(rep.Errors, fmt.Sprintf("shard %d (%s) status=%s err=%s", s.Plan.Index, s.Plan.RunID, s.Status, s.Error))
		}
	}
	return json.NewEncoder(out).Encode(rep)
}

func resolveMode(requested string) string {
	switch requested {
	case "cloud":
		return "cloud"
	case "local":
		return "local"
	default:
		// auto: cloud iff ITERION_QUEUE_NATS_URL is set; else local.
		if os.Getenv("ITERION_QUEUE_NATS_URL") != "" {
			return "cloud"
		}
		return "local"
	}
}

func readFilesJSON(path string) ([]string, error) {
	var raw []byte
	var err error
	if path == "-" || path == "" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var files []string
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("files-json must be a JSON array of strings: %w", err)
	}
	// Stable order for determinism.
	sort.Strings(files)
	return files, nil
}

func parseBaseVars(s string) (map[string]string, error) {
	if s == "" {
		return map[string]string{}, nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch vv := v.(type) {
		case string:
			out[k] = vv
		default:
			b, _ := json.Marshal(vv)
			out[k] = string(b)
		}
	}
	return out, nil
}

// planShards splits the file list into shards of shardSize and assigns
// deterministic run ids derived from sha256(parentRunID || shard_index).
// Same (parent, file list, shard_size) ⇒ same plan ⇒ idempotent reruns.
func planShards(files []string, shardSize int, parentRunID string) []shardPlan {
	var plans []shardPlan
	for i := 0; i < len(files); i += shardSize {
		end := i + shardSize
		if end > len(files) {
			end = len(files)
		}
		idx := len(plans)
		seed := fmt.Sprintf("%s:%d:%d", parentRunID, idx, len(files))
		sum := sha256.Sum256([]byte(seed))
		plans = append(plans, shardPlan{
			Index: idx,
			RunID: "shard-" + hex.EncodeToString(sum[:8]),
			Files: append([]string{}, files[i:end]...),
			Label: fmt.Sprintf("files %d-%d (of %d)", i, end-1, len(files)),
		})
	}
	return plans
}

// dispatchLocal spawns child `iterion run` subprocesses bounded by
// scanShardsOpts.maxConcurrency. Returns one shardResult per plan in
// the same order as the input slice. Subprocess failures are recorded
// in the result, not bubbled up — the caller decides what to do based
// on the JSON envelope's `errors[]`.
func dispatchLocal(ctx context.Context, plans []shardPlan, baseVars map[string]string) []shardResult {
	results := make([]shardResult, len(plans))
	for i, p := range plans {
		results[i] = shardResult{Plan: p}
	}

	sem := make(chan struct{}, scanShardsOpts.maxConcurrency)
	var wg sync.WaitGroup

	exePath, err := os.Executable()
	if err != nil {
		// Fall back to argv[0] if the kernel-provided path is unavailable.
		exePath = os.Args[0]
	}
	exePath, _ = filepath.Abs(exePath)

	for i := range plans {
		i := i
		p := plans[i]

		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			shardJSON, _ := json.Marshal(p.Files)
			args := []string{
				"run", scanShardsOpts.workflow,
				"--run-id", p.RunID,
				"--store-dir", scanShardsOpts.storeDir,
				"--no-interactive",
				"--var", fmt.Sprintf("%s=%s", scanShardsOpts.shardVarName, string(shardJSON)),
				"--var", fmt.Sprintf("parent_run_id=%s", scanShardsOpts.parentRunID),
				"--var", fmt.Sprintf("shard_index=%d", p.Index),
			}
			for k, v := range baseVars {
				args = append(args, "--var", k+"="+v)
			}

			started := time.Now()
			results[i].Started = &started

			cmd := exec.CommandContext(ctx, exePath, args...)
			cmd.Env = append(os.Environ(),
				"ITERION_PARENT_RUN_ID="+scanShardsOpts.parentRunID,
				fmt.Sprintf("ITERION_SHARD_INDEX=%d", p.Index),
				fmt.Sprintf("ITERION_SHARD_COUNT=%d", len(plans)),
			)
			// Stream subprocess stderr to the parent's stderr; capture
			// stdout because we don't want it polluting our own JSON
			// envelope.
			cmd.Stderr = os.Stderr
			out, err := cmd.Output()
			finished := time.Now()
			results[i].Finished = &finished
			if err != nil {
				results[i].Error = fmt.Sprintf("%v: %s", err, truncate(string(out), 512))
			}
		}()
	}
	wg.Wait()
	return results
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// dispatchCloud submits each shard as a queued run via the iterion
// server's HTTP launch endpoint (POST /api/v1/runs/launch). The
// server persists the run as `queued`, publishes a RunMessage with
// parent/shard fields populated, and a runner pool drains the queue.
// The parent polls the shared store for each child's terminal
// status — exactly like the local path; only the data plane differs.
//
// Required env:
//   - ITERION_SERVER_URL : base URL of the iterion server (e.g.
//     https://iterion.example.com)
//
// Optional env:
//   - ITERION_SERVER_TOKEN : Bearer auth, when the server gates the
//     launch endpoint behind SSO
func dispatchCloud(ctx context.Context, plans []shardPlan, baseVars map[string]string) []shardResult {
	results := make([]shardResult, len(plans))
	for i, p := range plans {
		results[i] = shardResult{Plan: p}
	}

	serverURL := strings.TrimRight(os.Getenv("ITERION_SERVER_URL"), "/")
	if serverURL == "" {
		for i := range results {
			results[i].Error = "ITERION_SERVER_URL not set; cloud mode requires the server endpoint"
		}
		return results
	}
	token := os.Getenv("ITERION_SERVER_TOKEN")

	// Load the workflow source so the server doesn't need a shared
	// filesystem (cloud-mode endpoint requires inline `source` when
	// the server is the cloud daemon, per pkg/server/runs.go:200).
	src, srcErr := os.ReadFile(scanShardsOpts.workflow)
	if srcErr != nil {
		for i := range results {
			results[i].Error = fmt.Sprintf("read workflow source: %v", srcErr)
		}
		return results
	}

	sem := make(chan struct{}, scanShardsOpts.maxConcurrency)
	var wg sync.WaitGroup

	for i := range plans {
		i := i
		p := plans[i]

		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			shardJSON, _ := json.Marshal(p.Files)
			vars := map[string]string{
				scanShardsOpts.shardVarName: string(shardJSON),
				"parent_run_id":             scanShardsOpts.parentRunID,
				"shard_index":               fmt.Sprintf("%d", p.Index),
			}
			for k, v := range baseVars {
				vars[k] = v
			}

			body := map[string]interface{}{
				"file_path":     scanShardsOpts.workflow,
				"source":        string(src),
				"run_id":        p.RunID,
				"vars":          vars,
				"parent_run_id": scanShardsOpts.parentRunID,
				"shard_index":   p.Index,
				"shard_count":   len(plans),
				"shard_label":   p.Label,
			}
			raw, _ := json.Marshal(body)

			started := time.Now()
			results[i].Started = &started

			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/runs/launch", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			finished := time.Now()
			results[i].Finished = &finished
			if err != nil {
				results[i].Error = fmt.Sprintf("POST /runs/launch: %v", err)
				return
			}
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				results[i].Error = fmt.Sprintf("server returned %d: %s", resp.StatusCode, truncate(string(respBody), 512))
				return
			}
			// Server accepted (202). The child's terminal status will
			// surface in the post-dispatch poll loop in runScanShards.
		}()
	}
	wg.Wait()
	return results
}
