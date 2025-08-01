package utils

import (
	"strings"
)

// claudeIndicators contains patterns that indicate a Claude session
var claudeIndicators = []string{
	"claude",
	"Claude",
	"anthropic",
	"Assistant:",
	"Human:",
	"I'm Claude",
	"claude-3",
	"I'm an AI assistant",
	"Claude Code",
	"claude-code",
	"claude code",
	"$ claude",
	"> claude",
	"# claude",
	"Sonnet",
	"claude-sonnet",
	"model named",
}

// Pre-computed lowercase indicators for performance
var lowerClaudeIndicators []string

// High-priority indicators for early exit optimization
var priorityIndicators = []string{
	"claude",
	"anthropic",
	"assistant:",
	"human:",
}

func init() {
	// Pre-compute lowercase versions of all indicators
	lowerClaudeIndicators = make([]string, len(claudeIndicators))
	for i, indicator := range claudeIndicators {
		lowerClaudeIndicators[i] = strings.ToLower(indicator)
	}
}

// IsClaudeWindow checks if window content indicates a Claude session
// Optimized version with pre-computed patterns and early exit strategies
func IsClaudeWindow(content string) bool {
	if content == "" {
		return false
	}

	// Convert to lowercase once
	lowerContent := strings.ToLower(content)

	// First check high-priority indicators for early exit (most common patterns)
	for _, indicator := range priorityIndicators {
		if strings.Contains(lowerContent, indicator) {
			return true
		}
	}

	// Check remaining pre-computed lowercase indicators
	for _, indicator := range lowerClaudeIndicators {
		// Skip priority indicators we already checked
		if indicator == "claude" || indicator == "anthropic" ||
			indicator == "assistant:" || indicator == "human:" {
			continue
		}

		if strings.Contains(lowerContent, indicator) {
			return true
		}
	}

	return false
}

// GetClaudeIndicators returns a copy of the Claude detection patterns
// This is useful for testing or extending detection logic
func GetClaudeIndicators() []string {
	indicators := make([]string, len(claudeIndicators))
	copy(indicators, claudeIndicators)
	return indicators
}
