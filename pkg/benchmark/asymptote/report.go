package asymptote

import (
	"fmt"
	"strings"
	"time"
)

// RenderMarkdown produces a human-readable markdown report from a Comparison.
// The format is intentionally text-only (no embedded SVG or PNG) so it
// renders well on GitHub, in editors, and via `iterion bench asymptote
// --output - | less`.
func RenderMarkdown(cmp Comparison, opts RenderOptions) string {
	var b strings.Builder

	if opts.Title == "" {
		opts.Title = "Asymptote Benchmark"
	}
	b.WriteString("# ")
	b.WriteString(opts.Title)
	b.WriteString("\n\n")

	if opts.GeneratedAt.IsZero() {
		opts.GeneratedAt = time.Now().UTC()
	}
	fmt.Fprintf(&b, "_Generated %s_\n\n", opts.GeneratedAt.Format("2006-01-02 15:04 UTC"))

	primaryLabel := cmp.Single.Label
	if primaryLabel == "" {
		primaryLabel = "primary"
	}
	variantLabel := cmp.Alternated.Label
	if variantLabel == "" {
		variantLabel = "variant"
	}
	hasVariant := len(cmp.Alternated.Runs) > 0

	b.WriteString("## Inputs\n\n")
	fmt.Fprintf(&b, "- %s runs: %d (max iter %d)\n", primaryLabel, len(cmp.Single.Runs), cmp.Single.MaxIter)
	if hasVariant {
		fmt.Fprintf(&b, "- %s runs: %d (max iter %d)\n", variantLabel, len(cmp.Alternated.Runs), cmp.Alternated.MaxIter)
	}
	fmt.Fprintf(&b, "- Approval threshold: %.2f\n\n", opts.ApprovalThreshold)

	b.WriteString("## Per-iteration aggregate\n\n")
	if hasVariant {
		fmt.Fprintf(&b, "| Iter | %s mean | %s pass-rate | %s n | %s mean | %s pass-rate | %s n | Δ pass-rate (%s − %s) |\n",
			primaryLabel, primaryLabel, primaryLabel, variantLabel, variantLabel, variantLabel, variantLabel, primaryLabel)
		b.WriteString("|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	} else {
		fmt.Fprintf(&b, "| Iter | %s mean | %s pass-rate | %s n |\n", primaryLabel, primaryLabel, primaryLabel)
		b.WriteString("|---:|---:|---:|---:|\n")
	}

	for iter := 0; iter <= cmp.MaxIter; iter++ {
		s := cellAt(cmp.Single.PerIter, iter)
		if hasVariant {
			a := cellAt(cmp.Alternated.PerIter, iter)
			delta := ""
			if s.Count > 0 && a.Count > 0 {
				delta = fmt.Sprintf("%+0.2f", a.PassRate-s.PassRate)
			}
			fmt.Fprintf(&b, "| %d | %s | %s | %d | %s | %s | %d | %s |\n",
				iter, fmtFloat(s), fmtPct(s), s.Count,
				fmtFloat(a), fmtPct(a), a.Count, delta)
		} else {
			fmt.Fprintf(&b, "| %d | %s | %s | %d |\n",
				iter, fmtFloat(s), fmtPct(s), s.Count)
		}
	}
	b.WriteString("\n")

	b.WriteString("## Pass-rate sparkline\n\n```\n")
	b.WriteString("iter:  ")
	for iter := 0; iter <= cmp.MaxIter; iter++ {
		fmt.Fprintf(&b, "%2d ", iter)
	}
	fmt.Fprintf(&b, "\n%-7s", abbrev(primaryLabel)+":")
	for iter := 0; iter <= cmp.MaxIter; iter++ {
		s := cellAt(cmp.Single.PerIter, iter)
		fmt.Fprintf(&b, "%s ", spark(s))
	}
	if hasVariant {
		fmt.Fprintf(&b, "\n%-7s", abbrev(variantLabel)+":")
		for iter := 0; iter <= cmp.MaxIter; iter++ {
			a := cellAt(cmp.Alternated.PerIter, iter)
			fmt.Fprintf(&b, "%s ", spark(a))
		}
	}
	b.WriteString("\n```\n\n")

	b.WriteString("Legend: `_` no data · `0`–`9` pass-rate (0.0–0.9) · `*` 1.0\n\n")

	if opts.IncludePerRun {
		b.WriteString("## Per-run series\n\n")
		writeRunGroup(&b, cmp.Single)
		writeRunGroup(&b, cmp.Alternated)
	}

	b.WriteString("## Reading guide\n\n")
	b.WriteString("- **Mean score** is the average judge verdict at iteration N across runs that reached iteration N.\n")
	b.WriteString("- **Pass-rate** is the fraction of runs whose verdict at iteration N was approved (score ≥ threshold).\n")
	b.WriteString("- The asymptote is the iteration N from which the mean stops climbing — ideally with a tight `n` so the stabilisation isn't an artefact of attrition.\n")
	if hasVariant {
		b.WriteString("- A positive Δ at the asymptote means the variant *raises* the reliability ceiling for this task — useful evidence for adopting it on critical work.\n")
	}
	b.WriteString("- Treat n < 5 with caution; small samples are noisy. Treat a curve that never stabilises as evidence the judge prompt is over-strict (chasing nits) or the task is too open-ended.\n")

	return b.String()
}

// abbrev shortens long labels for the sparkline left margin.
func abbrev(label string) string {
	if len(label) <= 6 {
		return label
	}
	return label[:6]
}

// RenderOptions tunes the rendered report.
type RenderOptions struct {
	Title             string
	GeneratedAt       time.Time
	ApprovalThreshold float64
	IncludePerRun     bool
}

func writeRunGroup(b *strings.Builder, g GroupAggregate) {
	fmt.Fprintf(b, "### %s\n\n", g.Label)
	if len(g.Runs) == 0 {
		b.WriteString("_no runs_\n\n")
		return
	}
	for _, r := range g.Runs {
		fmt.Fprintf(b, "- `%s` (loop=%s, status=%s):", r.RunID, r.LoopName, r.Status)
		if len(r.Scores) == 0 {
			b.WriteString(" _no judge verdicts found_\n")
			continue
		}
		var parts []string
		for _, s := range r.Scores {
			marker := "x"
			if s.Approved {
				marker = "✓"
			}
			parts = append(parts, fmt.Sprintf("%d%s(%.2f)", s.Iteration, marker, s.Score))
		}
		b.WriteString(" ")
		b.WriteString(strings.Join(parts, " "))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func cellAt(per []IterationAggregate, iter int) IterationAggregate {
	if iter < 0 || iter >= len(per) {
		return IterationAggregate{Iteration: iter}
	}
	return per[iter]
}

func fmtFloat(c IterationAggregate) string {
	if c.Count == 0 {
		return "—"
	}
	return fmt.Sprintf("%.2f", c.MeanScore)
}

func fmtPct(c IterationAggregate) string {
	if c.Count == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", c.PassRate*100)
}

// spark maps a pass-rate to a single ASCII char for inline sparkline.
func spark(c IterationAggregate) string {
	if c.Count == 0 {
		return "_ "
	}
	if c.PassRate >= 1.0 {
		return "* "
	}
	d := int(c.PassRate * 10)
	if d > 9 {
		d = 9
	}
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d ", d)
}
