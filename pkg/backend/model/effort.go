package model

import (
	"github.com/SocialGouv/claw-code-go/pkg/apikit"
)

// effortRank returns a comparable ordinal for reasoning_effort levels.
// Higher rank = more compute. Unknown values get rank 0 so they sort
// below every known level — callers that pass a typo can still get a
// sensible coerced value rather than an outright drop.
func effortRank(e string) int {
	switch e {
	case "minimal":
		return 1
	case "low":
		return 2
	case "medium":
		return 3
	case "high":
		return 4
	case "xhigh":
		return 5
	case "max":
		return 6
	default:
		return 0
	}
}

// coerceEffort returns effort unchanged when it appears in supported.
// Otherwise it returns the highest supported level whose rank is <=
// effort's rank — the convention "fall back to the highest supported
// at or below" already used by the codex SDK adapter. When effort is
// below every supported level, the lowest supported level is returned
// (we always send *something* if the model accepts the parameter).
// When supported is empty the input is passed through unchanged: the
// model is unknown to apikit and the caller's value is trusted.
func coerceEffort(effort string, supported []string, def string) string {
	if effort == "" || len(supported) == 0 {
		return effort
	}
	for _, s := range supported {
		if s == effort {
			return effort
		}
	}
	target := effortRank(effort)
	bestAtOrBelow := ""
	bestRankAtOrBelow := 0
	minSupported := ""
	minSupportedRank := 0
	for _, s := range supported {
		r := effortRank(s)
		if minSupportedRank == 0 || r < minSupportedRank {
			minSupported = s
			minSupportedRank = r
		}
		if r <= target && r > bestRankAtOrBelow {
			bestAtOrBelow = s
			bestRankAtOrBelow = r
		}
	}
	if bestAtOrBelow != "" {
		return bestAtOrBelow
	}
	if minSupported != "" {
		return minSupported
	}
	return def
}

// coerceEffortForModel resolves the model's effort matrix via
// apikit.EffortCapabilities and coerces effort to a level the model
// accepts. Models claw-code-go does not recognise return the input
// unchanged; the provider call will surface a 400 with enough context
// to add the model to the registry.
func coerceEffortForModel(effort, modelID string) string {
	if effort == "" {
		return ""
	}
	supported, def := apikit.EffortCapabilities(modelID)
	return coerceEffort(effort, supported, def)
}
