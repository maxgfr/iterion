package model

import "strings"

// capabilitiesForModel returns static capabilities for a given provider and model ID.
// This replaces sdk.ModelCapabilitiesOf() which required a runtime interface assertion.
func capabilitiesForModel(provider, modelID string) ModelCapabilities {
	switch provider {
	case "anthropic":
		return anthropicCapabilities(modelID)
	case "openai":
		return openaiCapabilities(modelID)
	default:
		// Conservative default: tool calling + temperature, no reasoning.
		return ModelCapabilities{
			ToolCall:    true,
			Temperature: true,
		}
	}
}

func anthropicCapabilities(modelID string) ModelCapabilities {
	lower := strings.ToLower(modelID)

	// z.ai's GLM models are served through the Anthropic-compatible
	// endpoint, so they arrive here as "anthropic/glm-X". They are a
	// distinct family with their own context windows — handle them
	// before the Claude reasoning heuristics below.
	if strings.Contains(lower, "glm") {
		return glmCapabilities(lower)
	}

	// Claude 3.5+ and Claude 4+ support reasoning via extended thinking.
	hasReasoning := strings.Contains(lower, "claude-3-5") ||
		strings.Contains(lower, "claude-3.5") ||
		strings.Contains(lower, "claude-sonnet-4") ||
		strings.Contains(lower, "claude-opus-4") ||
		strings.Contains(lower, "claude-4")

	return ModelCapabilities{
		Reasoning:   hasReasoning,
		ToolCall:    true,
		Temperature: true,
	}
}

// glmContextWindow returns the context window for a GLM model served via
// z.ai's Anthropic-compatible endpoint. GLM-5.2 ships a 1M-token window
// (released 2026-06-13, ~5x its GLM-5.1 predecessor); GLM-5.1 / GLM-4.6 and
// earlier are 200K-class. modelID arrives lowercased.
func glmContextWindow(modelID string) int {
	if strings.Contains(modelID, "glm-5.2") {
		return 1_000_000
	}
	return 200_000
}

// glmCapabilities returns static capabilities for a GLM model. GLM models
// support tool calling, temperature, and extended thinking via the
// Anthropic-compatible endpoint.
func glmCapabilities(modelID string) ModelCapabilities {
	return ModelCapabilities{
		Reasoning:     true,
		ToolCall:      true,
		Temperature:   true,
		ContextWindow: glmContextWindow(modelID),
	}
}

func openaiCapabilities(modelID string) ModelCapabilities {
	lower := strings.ToLower(modelID)

	// o1, o3, o4 series are reasoning models that don't accept temperature.
	isReasoning := strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4")

	return ModelCapabilities{
		Reasoning:   isReasoning,
		ToolCall:    true,
		Temperature: !isReasoning,
	}
}
