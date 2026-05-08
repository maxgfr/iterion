package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SocialGouv/iterion/pkg/benchmark/asymptote"
	"github.com/SocialGouv/iterion/pkg/store"
)

// BenchAsymptoteOptions configures the `iterion bench asymptote` command.
//
// The asymptote thesis (see docs/why-iterion.md) holds that running the
// same workflow across N independent sessions produces a quality curve
// that climbs and stabilises — that stabilisation is the asymptote, and
// it measures the (model + recipe)'s reliability ceiling for the task.
//
// Primary use: feed N runs of the same workflow via --runs and observe
// the per-iteration aggregate + cross-session distribution.
//
// Secondary use: feed an alternative recipe variant via --variant-runs
// to quantify a lift (typically a multi-family alternation variant for
// security-critical or complex tasks). Multi-family alternation is *not*
// the default thesis — it is the optional refinement.
type BenchAsymptoteOptions struct {
	StoreDir          string
	Runs              []string // canonical asymptote subjects (same workflow, N independent sessions)
	VariantRuns       []string // optional: alternative recipe variant for comparison
	Label             string   // primary group label (default: "asymptote")
	VariantLabel      string   // variant group label (default: "variant")
	JudgeNode         string   // IR node ID of the judge whose verdicts we score
	JudgeField        string   // output field name (default "approved")
	LoopName          string   // optional: pin to one loop (default: first observed)
	ApprovalThreshold float64  // default 0.5
	Output            string   // markdown path; "-" or "" → stdout
	Title             string
	IncludePerRun     bool
}

// RunBenchAsymptote loads the requested runs, parses each with the asymptote
// pipeline, aggregates per group, renders, and writes the report.
func RunBenchAsymptote(opts BenchAsymptoteOptions, p *Printer) error {
	if opts.JudgeNode == "" {
		return fmt.Errorf("--judge-node is required (the IR node ID of the judge whose verdicts will be scored)")
	}
	if len(opts.Runs) == 0 && len(opts.VariantRuns) == 0 {
		return fmt.Errorf("at least one of --runs or --variant-runs must be provided")
	}
	if opts.JudgeField == "" {
		opts.JudgeField = asymptote.DefaultJudgeField
	}
	if opts.ApprovalThreshold == 0 {
		opts.ApprovalThreshold = asymptote.DefaultApprovalThreshold
	}
	if opts.Label == "" {
		opts.Label = "asymptote"
	}
	if opts.VariantLabel == "" {
		opts.VariantLabel = "variant"
	}

	cwd, _ := os.Getwd()
	storeDir := store.ResolveStoreDir(cwd, opts.StoreDir)

	s, err := store.New(storeDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	parseOpts := asymptote.ParseOptions{
		JudgeNodeID:       opts.JudgeNode,
		JudgeField:        opts.JudgeField,
		LoopName:          opts.LoopName,
		ApprovalThreshold: opts.ApprovalThreshold,
	}

	ctx := context.Background()

	primarySeries, err := parseRuns(ctx, s, opts.Runs, parseOpts)
	if err != nil {
		return err
	}
	variantSeries, err := parseRuns(ctx, s, opts.VariantRuns, parseOpts)
	if err != nil {
		return err
	}

	cmp := asymptote.Compare(
		asymptote.AggregateGroup(opts.Label, primarySeries),
		asymptote.AggregateGroup(opts.VariantLabel, variantSeries),
	)

	if p.Format == OutputJSON {
		p.JSON(cmp)
		return nil
	}

	md := asymptote.RenderMarkdown(cmp, asymptote.RenderOptions{
		Title:             opts.Title,
		GeneratedAt:       time.Now().UTC(),
		ApprovalThreshold: opts.ApprovalThreshold,
		IncludePerRun:     opts.IncludePerRun,
	})

	if opts.Output == "" || opts.Output == "-" {
		_, _ = fmt.Fprint(p.W, md)
		return nil
	}

	if err := os.WriteFile(opts.Output, []byte(md), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	p.Line("Report written to %s", opts.Output)
	return nil
}

// SplitRunIDs parses a comma-separated CLI flag value into a slice of run IDs,
// trimming whitespace and skipping empty entries.
func SplitRunIDs(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseRuns(ctx context.Context, s store.RunStore, ids []string, opts asymptote.ParseOptions) ([]asymptote.RunSeries, error) {
	out := make([]asymptote.RunSeries, 0, len(ids))
	for _, id := range ids {
		rs, err := asymptote.ParseRun(ctx, s, id, opts)
		if err != nil {
			return nil, fmt.Errorf("parse run %s: %w", id, err)
		}
		out = append(out, *rs)
	}
	return out, nil
}
