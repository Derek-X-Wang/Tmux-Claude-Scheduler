package utils

import (
	"testing"
)

func TestIsClaudeWindow(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "claude lowercase",
			content:  "starting claude session",
			expected: true,
		},
		{
			name:     "Claude uppercase",
			content:  "Claude is ready",
			expected: true,
		},
		{
			name:     "anthropic mention",
			content:  "using anthropic's api",
			expected: true,
		},
		{
			name:     "Assistant prompt",
			content:  "Assistant: How can I help?",
			expected: true,
		},
		{
			name:     "Human prompt",
			content:  "Human: Hello there",
			expected: true,
		},
		{
			name:     "I'm Claude response",
			content:  "I'm Claude, an AI assistant",
			expected: true,
		},
		{
			name:     "claude-3 model",
			content:  "using claude-3-sonnet model",
			expected: true,
		},
		{
			name:     "Claude Code mention",
			content:  "running Claude Code",
			expected: true,
		},
		{
			name:     "claude-code command",
			content:  "$ claude-code start",
			expected: true,
		},
		{
			name:     "Sonnet model",
			content:  "Sonnet is responding",
			expected: true,
		},
		{
			name:     "no claude indicators",
			content:  "just regular terminal output",
			expected: false,
		},
		{
			name:     "false positive prevention",
			content:  "claudia is a user name",
			expected: false,
		},
		{
			name:     "mixed case detection",
			content:  "CLAUDE IS RUNNING",
			expected: true,
		},
		{
			name:     "partial match",
			content:  "this contains the word anthropic somewhere",
			expected: true,
		},
		{
			name:     "command line usage",
			content:  "$ claude help",
			expected: true,
		},
		{
			name:     "comment usage",
			content:  "# claude session started",
			expected: true,
		},
		{
			name:     "shell prompt",
			content:  "> claude --version",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsClaudeWindow(tt.content)
			if result != tt.expected {
				t.Errorf("IsClaudeWindow(%q) = %v, expected %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestGetClaudeIndicators(t *testing.T) {
	indicators := GetClaudeIndicators()

	// Should return a copy, not the original slice
	if len(indicators) == 0 {
		t.Error("GetClaudeIndicators() returned empty slice")
	}

	// Should contain expected indicators
	expectedIndicators := []string{"claude", "Claude", "anthropic", "Assistant:", "Human:"}
	for _, expected := range expectedIndicators {
		found := false
		for _, indicator := range indicators {
			if indicator == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetClaudeIndicators() missing expected indicator: %s", expected)
		}
	}

	// Modifying returned slice should not affect original
	originalLen := len(indicators)
	indicators[0] = "modified"
	newIndicators := GetClaudeIndicators()
	if newIndicators[0] == "modified" {
		t.Error("GetClaudeIndicators() does not return a copy - original was modified")
	}
	if len(newIndicators) != originalLen {
		t.Error("GetClaudeIndicators() length changed after modification")
	}
}

// Benchmark tests for performance validation
func BenchmarkIsClaudeWindow(b *testing.B) {
	testContent := "This is a test string with claude mentioned in it for performance testing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsClaudeWindow(testContent)
	}
}

func BenchmarkIsClaudeWindowNoMatch(b *testing.B) {
	testContent := "This is a test string with no matching indicators for performance testing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsClaudeWindow(testContent)
	}
}

func BenchmarkIsClaudeWindowEarlyMatch(b *testing.B) {
	testContent := "claude is mentioned early in this test string for performance testing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsClaudeWindow(testContent)
	}
}
