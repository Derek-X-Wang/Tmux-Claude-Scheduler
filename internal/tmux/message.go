package tmux

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// MessageSender handles sending messages to tmux windows with proper timing
type MessageSender struct {
	client *Client
}

// NewMessageSender creates a new message sender
func NewMessageSender(client *Client) *MessageSender {
	return &MessageSender{
		client: client,
	}
}

// SendResult represents the result of sending a message
type SendResult struct {
	Target        string        `json:"target"`
	Message       string        `json:"message"`
	Success       bool          `json:"success"`
	Error         string        `json:"error,omitempty"`
	Duration      time.Duration `json:"duration"`
	Timestamp     time.Time     `json:"timestamp"`
	ContentBefore string        `json:"content_before,omitempty"`
	ContentAfter  string        `json:"content_after,omitempty"`
}

// SendMessage sends a message to a tmux window with the critical 500ms delay
// This follows the exact pattern from the Tmux-Orchestrator example
func (ms *MessageSender) SendMessage(target, message string) (*SendResult, error) {
	start := time.Now()
	result := &SendResult{
		Target:    target,
		Message:   message,
		Timestamp: start,
	}

	// CRITICAL: Verify window exists first (as specified in PRP)
	if err := ms.client.ValidateTarget(target); err != nil {
		result.Error = fmt.Sprintf("target validation failed: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	// Capture content before sending for verification
	contentBefore, err := ms.client.CapturePane(target, 5)
	if err != nil {
		log.Printf("Warning: could not capture content before sending: %v", err)
	} else {
		result.ContentBefore = contentBefore
	}

	// CRITICAL: Send message with proper delay (exactly as in send-claude-message.sh)
	// Step 1: Send the message text
	if err := ms.client.SendKeys(target, message); err != nil {
		result.Error = fmt.Sprintf("failed to send message: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	// CRITICAL: 500ms delay before Enter (this is the magic from the example)
	time.Sleep(500 * time.Millisecond)

	// Step 2: Send Enter key to submit
	if err := ms.client.SendKeys(target, "Enter"); err != nil {
		result.Error = fmt.Sprintf("failed to send Enter key: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	// Give a moment for the command to be processed
	time.Sleep(100 * time.Millisecond)

	// Capture content after sending for verification
	contentAfter, err := ms.client.CapturePane(target, 5)
	if err != nil {
		log.Printf("Warning: could not capture content after sending: %v", err)
	} else {
		result.ContentAfter = contentAfter
	}

	result.Success = true
	result.Duration = time.Since(start)

	log.Printf("Message sent to %s: %s (took %v)", target, message, result.Duration)
	return result, nil
}

// SendMessageWithRetry sends a message with retry logic
func (ms *MessageSender) SendMessageWithRetry(target, message string, maxRetries int) (*SendResult, error) {
	var lastResult *SendResult
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt*attempt) * time.Second
			log.Printf("Retrying message send to %s after %v (attempt %d/%d)", target, backoff, attempt+1, maxRetries+1)
			time.Sleep(backoff)
		}

		result, err := ms.SendMessage(target, message)
		lastResult = result
		lastErr = err

		if err == nil && result.Success {
			return result, nil
		}

		log.Printf("Message send attempt %d failed: %v", attempt+1, err)
	}

	return lastResult, fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
}

// SendClaudeMessage sends a message specifically formatted for Claude
func (ms *MessageSender) SendClaudeMessage(target, message string) (*SendResult, error) {
	// Ensure the message is properly formatted for Claude
	cleanMessage := ms.formatClaudeMessage(message)
	return ms.SendMessage(target, cleanMessage)
}

// formatClaudeMessage formats a message for Claude interaction
func (ms *MessageSender) formatClaudeMessage(message string) string {
	// Remove any trailing newlines or extra whitespace
	message = strings.TrimSpace(message)

	// Ensure the message doesn't contain characters that might interfere with tmux
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\r", " ")

	// Collapse multiple spaces
	for strings.Contains(message, "  ") {
		message = strings.ReplaceAll(message, "  ", " ")
	}

	return message
}

// VerifyDelivery verifies that a message was successfully delivered
func (ms *MessageSender) VerifyDelivery(target, expectedContent string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		content, err := ms.client.CapturePane(target, 10)
		if err != nil {
			return false, fmt.Errorf("failed to capture pane for verification: %w", err)
		}

		if strings.Contains(content, expectedContent) {
			return true, nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return false, fmt.Errorf("message not found in target after %v", timeout)
}

// BatchSendMessages sends multiple messages to different targets
func (ms *MessageSender) BatchSendMessages(messages map[string]string) (map[string]*SendResult, error) {
	results := make(map[string]*SendResult)

	for target, message := range messages {
		result, err := ms.SendMessage(target, message)
		results[target] = result

		if err != nil {
			log.Printf("Failed to send message to %s: %v", target, err)
		}

		// Small delay between messages to avoid overwhelming tmux
		time.Sleep(100 * time.Millisecond)
	}

	return results, nil
}

// SendInteractiveMessage sends a message and waits for a response
func (ms *MessageSender) SendInteractiveMessage(target, message string, responseTimeout time.Duration) (*SendResult, string, error) {
	// Capture initial content
	initialContent, err := ms.client.CapturePane(target, 20)
	if err != nil {
		return nil, "", fmt.Errorf("failed to capture initial content: %w", err)
	}

	// Send the message
	result, err := ms.SendMessage(target, message)
	if err != nil {
		return result, "", err
	}

	// Wait for response
	deadline := time.Now().Add(responseTimeout)
	for time.Now().Before(deadline) {
		currentContent, err := ms.client.CapturePane(target, 20)
		if err != nil {
			continue
		}

		// Check if content has changed (indicating a response)
		if currentContent != initialContent && len(currentContent) > len(initialContent) {
			// Extract the new content (response)
			response := strings.TrimPrefix(currentContent, initialContent)
			response = strings.TrimSpace(response)
			return result, response, nil
		}

		time.Sleep(1 * time.Second)
	}

	return result, "", fmt.Errorf("no response received within %v", responseTimeout)
}

// IsClaudeResponding checks if Claude is actively responding in a window
func (ms *MessageSender) IsClaudeResponding(target string) (bool, error) {
	// Capture content multiple times to detect changes
	content1, err := ms.client.CapturePane(target, 10)
	if err != nil {
		return false, err
	}

	time.Sleep(2 * time.Second)

	content2, err := ms.client.CapturePane(target, 10)
	if err != nil {
		return false, err
	}

	// If content is changing, Claude is likely responding
	return content1 != content2, nil
}

// WaitForClaudeReady waits for Claude to be ready to receive messages
func (ms *MessageSender) WaitForClaudeReady(target string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if Claude is responding (busy)
		responding, err := ms.IsClaudeResponding(target)
		if err != nil {
			return fmt.Errorf("failed to check Claude status: %w", err)
		}

		if !responding {
			// Claude appears ready
			return nil
		}

		log.Printf("Claude is busy in %s, waiting...", target)
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("Claude not ready after %v", timeout)
}

// SendQueuedMessage sends a message and handles common error cases
func (ms *MessageSender) SendQueuedMessage(target, message string, priority int) (*SendResult, error) {
	// Validate target first
	if err := ms.client.ValidateTarget(target); err != nil {
		return nil, fmt.Errorf("invalid target: %w", err)
	}

	// Wait for Claude to be ready if it's a high priority message
	if priority >= 8 {
		if err := ms.WaitForClaudeReady(target, 30*time.Second); err != nil {
			log.Printf("Warning: proceeding with message send despite Claude not being ready: %v", err)
		}
	}

	// Send the message with retry for high priority messages
	if priority >= 7 {
		return ms.SendMessageWithRetry(target, message, 2)
	}

	return ms.SendMessage(target, message)
}

// GetMessageSendStats returns statistics about message sending
type SendStats struct {
	TotalSent     int           `json:"total_sent"`
	SuccessRate   float64       `json:"success_rate"`
	AverageDelay  time.Duration `json:"average_delay"`
	LastSent      time.Time     `json:"last_sent"`
	FailedTargets []string      `json:"failed_targets"`
}

// stats tracks message sending statistics
var stats = struct {
	totalSent     int
	totalFailed   int
	totalDelay    time.Duration
	lastSent      time.Time
	failedTargets map[string]int
}{
	failedTargets: make(map[string]int),
}

// UpdateStats updates the message sending statistics
func (ms *MessageSender) UpdateStats(result *SendResult) {
	stats.totalSent++
	stats.totalDelay += result.Duration
	stats.lastSent = result.Timestamp

	if !result.Success {
		stats.totalFailed++
		stats.failedTargets[result.Target]++
	}
}

// GetStats returns current message sending statistics
func (ms *MessageSender) GetStats() *SendStats {
	successRate := 0.0
	if stats.totalSent > 0 {
		successRate = float64(stats.totalSent-stats.totalFailed) / float64(stats.totalSent)
	}

	averageDelay := time.Duration(0)
	if stats.totalSent > 0 {
		averageDelay = stats.totalDelay / time.Duration(stats.totalSent)
	}

	var failedTargets []string
	for target := range stats.failedTargets {
		failedTargets = append(failedTargets, target)
	}

	return &SendStats{
		TotalSent:     stats.totalSent,
		SuccessRate:   successRate,
		AverageDelay:  averageDelay,
		LastSent:      stats.lastSent,
		FailedTargets: failedTargets,
	}
}
