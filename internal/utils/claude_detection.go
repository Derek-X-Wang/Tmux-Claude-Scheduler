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

// IsClaudeWindow checks if window content indicates a Claude session
// It performs case-insensitive matching for better detection accuracy
func IsClaudeWindow(content string) bool {
	if content == "" {
		return false
	}

	// Convert to lowercase for case-insensitive matching
	lowerContent := strings.ToLower(content)

	for _, indicator := range claudeIndicators {
		if strings.Contains(lowerContent, strings.ToLower(indicator)) {
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
