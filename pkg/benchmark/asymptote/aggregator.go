package asymptote

import "math"

// AggregateGroup pivots a slice of RunSeries (all sharing a single label)
// into per-iteration aggregates. The longest series determines MaxIter.
//
// At each iteration index, only runs that produced a score for that
// iteration count toward the mean — runs that converged earlier and stopped
// emitting verdicts are excluded from later indices rather than padded with
// zeros, which would skew the asymptote shape downward.
func AggregateGroup(label string, runs []RunSeries) GroupAggregate {
	g := GroupAggregate{
		Label: label,
		Runs:  runs,
	}

	maxIter := 0
	for _, r := range runs {
		for _, s := range r.Scores {
			if s.Iteration > maxIter {
				maxIter = s.Iteration
			}
		}
	}
	g.MaxIter = maxIter

	g.PerIter = make([]IterationAggregate, maxIter+1)
	for iter := 0; iter <= maxIter; iter++ {
		var sum, sumSq float64
		count := 0
		passes := 0
		for _, r := range runs {
			s, ok := scoreAt(r.Scores, iter)
			if !ok {
				continue
			}
			sum += s.Score
			sumSq += s.Score * s.Score
			count++
			if s.Approved {
				passes++
			}
		}

		agg := IterationAggregate{Iteration: iter, Count: count}
		if count > 0 {
			agg.MeanScore = sum / float64(count)
			agg.PassRate = float64(passes) / float64(count)
			if count > 1 {
				variance := (sumSq/float64(count)) - (agg.MeanScore * agg.MeanScore)
				if variance < 0 {
					variance = 0
				}
				agg.StdErr = math.Sqrt(variance) / math.Sqrt(float64(count))
			}
		}
		g.PerIter[iter] = agg
	}

	return g
}

// Compare assembles a side-by-side Comparison from two pre-aggregated groups.
func Compare(single, alternated GroupAggregate) Comparison {
	cmp := Comparison{
		Single:     single,
		Alternated: alternated,
	}
	if single.MaxIter > alternated.MaxIter {
		cmp.MaxIter = single.MaxIter
	} else {
		cmp.MaxIter = alternated.MaxIter
	}
	return cmp
}

// scoreAt returns the IterationScore for a given iteration index, if any.
func scoreAt(scores []IterationScore, iter int) (IterationScore, bool) {
	for _, s := range scores {
		if s.Iteration == iter {
			return s, true
		}
	}
	return IterationScore{}, false
}
