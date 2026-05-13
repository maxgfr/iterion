package model

import "testing"

func TestCoerceEffort(t *testing.T) {
	openaiSupported := []string{"minimal", "low", "medium", "high"}
	anthropicOpus47 := []string{"low", "medium", "high", "xhigh", "max"}

	tests := []struct {
		name      string
		effort    string
		supported []string
		def       string
		want      string
	}{
		{
			name:      "openai passes through high",
			effort:    "high",
			supported: openaiSupported,
			def:       "medium",
			want:      "high",
		},
		{
			name:      "openai coerces max down to high",
			effort:    "max",
			supported: openaiSupported,
			def:       "medium",
			want:      "high",
		},
		{
			name:      "openai coerces xhigh down to high",
			effort:    "xhigh",
			supported: openaiSupported,
			def:       "medium",
			want:      "high",
		},
		{
			name:      "anthropic opus 4.7 keeps max",
			effort:    "max",
			supported: anthropicOpus47,
			def:       "xhigh",
			want:      "max",
		},
		{
			name:      "anthropic opus 4.7 keeps xhigh",
			effort:    "xhigh",
			supported: anthropicOpus47,
			def:       "xhigh",
			want:      "xhigh",
		},
		{
			name:      "empty effort passes through",
			effort:    "",
			supported: openaiSupported,
			def:       "medium",
			want:      "",
		},
		{
			name:      "unknown model passes through",
			effort:    "max",
			supported: nil,
			def:       "",
			want:      "max",
		},
		{
			name:      "below-minimum gets minimum supported",
			effort:    "minimal",
			supported: anthropicOpus47,
			def:       "xhigh",
			want:      "low",
		},
		{
			name:      "unknown effort value falls to model default via lowest",
			effort:    "blah",
			supported: openaiSupported,
			def:       "medium",
			want:      "minimal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceEffort(tt.effort, tt.supported, tt.def)
			if got != tt.want {
				t.Errorf("coerceEffort(%q, %v, %q) = %q, want %q", tt.effort, tt.supported, tt.def, got, tt.want)
			}
		})
	}
}

func TestCoerceEffortForModel(t *testing.T) {
	tests := []struct {
		name   string
		effort string
		model  string
		want   string
	}{
		{
			name:   "max on gpt-5.5 collapses to high",
			effort: "max",
			model:  "gpt-5.5",
			want:   "high",
		},
		{
			name:   "xhigh on gpt-5.4 collapses to high",
			effort: "xhigh",
			model:  "gpt-5.4",
			want:   "high",
		},
		{
			name:   "max on claude-opus-4-7 stays max",
			effort: "max",
			model:  "claude-opus-4-7",
			want:   "max",
		},
		{
			name:   "xhigh on claude-opus-4-7 stays xhigh",
			effort: "xhigh",
			model:  "claude-opus-4-7",
			want:   "xhigh",
		},
		{
			name:   "unknown model passes through max",
			effort: "max",
			model:  "totally-fake-model",
			want:   "max",
		},
		{
			name:   "empty effort stays empty",
			effort: "",
			model:  "gpt-5.5",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coerceEffortForModel(tt.effort, tt.model)
			if got != tt.want {
				t.Errorf("coerceEffortForModel(%q, %q) = %q, want %q", tt.effort, tt.model, got, tt.want)
			}
		})
	}
}
