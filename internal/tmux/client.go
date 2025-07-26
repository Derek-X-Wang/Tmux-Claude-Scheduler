package tmux

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Client provides tmux functionality using shell commands
type Client struct {
	// No internal state needed for shell-based approach
}

// WindowInfo represents information about a tmux window
type WindowInfo struct {
	SessionName string `json:"session_name"`
	WindowIndex int    `json:"window_index"`
	WindowName  string `json:"window_name"`
	Active      bool   `json:"active"`
	Target      string `json:"target"` // session:window format
}

// SessionInfo represents information about a tmux session
type SessionInfo struct {
	Name     string       `json:"name"`
	Attached bool         `json:"attached"`
	Windows  []WindowInfo `json:"windows"`
}

// NewClient creates a new tmux client
func NewClient() *Client {
	return &Client{}
}

// IsRunning checks if tmux server is running
func (c *Client) IsRunning() bool {
	cmd := exec.Command("tmux", "list-sessions")
	err := cmd.Run()
	return err == nil
}

// ListSessions returns all tmux sessions with their windows
func (c *Client) ListSessions() ([]SessionInfo, error) {
	// Get sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_attached}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	var sessionInfos []SessionInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}

		sessionName := parts[0]
		attached := parts[1] == "1"

		sessionInfo := SessionInfo{
			Name:     sessionName,
			Attached: attached,
		}

		// Get windows for this session
		windows, err := c.ListWindows(sessionName)
		if err != nil {
			log.Printf("Warning: failed to list windows for session %s: %v", sessionName, err)
			continue
		}
		sessionInfo.Windows = windows

		sessionInfos = append(sessionInfos, sessionInfo)
	}

	return sessionInfos, nil
}

// ListWindows returns all windows for a given session
func (c *Client) ListWindows(sessionName string) ([]WindowInfo, error) {
	cmd := exec.Command("tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}:#{window_name}:#{window_active}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows for session %s: %w", sessionName, err)
	}

	var windowInfos []WindowInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}

		windowIndex, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		windowInfo := WindowInfo{
			SessionName: sessionName,
			WindowIndex: windowIndex,
			WindowName:  parts[1],
			Active:      parts[2] == "1",
			Target:      fmt.Sprintf("%s:%d", sessionName, windowIndex),
		}
		windowInfos = append(windowInfos, windowInfo)
	}

	return windowInfos, nil
}

// SessionExists checks if a session exists
func (c *Client) SessionExists(sessionName string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	err := cmd.Run()
	return err == nil
}

// WindowExists checks if a window exists in a session
func (c *Client) WindowExists(sessionName string, windowIndex int) bool {
	target := fmt.Sprintf("%s:%d", sessionName, windowIndex)
	cmd := exec.Command("tmux", "list-windows", "-t", target)
	err := cmd.Run()
	return err == nil
}

// ParseTarget parses a target string (session:window) into components
func ParseTarget(target string) (sessionName string, windowIndex int, err error) {
	parts := strings.Split(target, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid target format, expected 'session:window', got: %s", target)
	}

	sessionName = parts[0]
	windowIndex, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid window index in target %s: %w", target, err)
	}

	return sessionName, windowIndex, nil
}

// ValidateTarget checks if a target is valid and the window exists
func (c *Client) ValidateTarget(target string) error {
	sessionName, windowIndex, err := ParseTarget(target)
	if err != nil {
		return err
	}

	if !c.SessionExists(sessionName) {
		return fmt.Errorf("session '%s' does not exist", sessionName)
	}

	if !c.WindowExists(sessionName, windowIndex) {
		return fmt.Errorf("window %d does not exist in session '%s'", windowIndex, sessionName)
	}

	return nil
}

// GetWindowInfo returns detailed information about a specific window
func (c *Client) GetWindowInfo(target string) (*WindowInfo, error) {
	sessionName, windowIndex, err := ParseTarget(target)
	if err != nil {
		return nil, err
	}

	windows, err := c.ListWindows(sessionName)
	if err != nil {
		return nil, err
	}

	for _, window := range windows {
		if window.WindowIndex == windowIndex {
			return &window, nil
		}
	}

	return nil, fmt.Errorf("window %d not found in session '%s'", windowIndex, sessionName)
}

// CapturePane captures the content of a tmux pane
func (c *Client) CapturePane(target string, numLines int) (string, error) {
	if err := c.ValidateTarget(target); err != nil {
		return "", err
	}

	var cmd *exec.Cmd
	if numLines > 0 {
		cmd = exec.Command("tmux", "capture-pane", "-t", target, "-p", "-S", fmt.Sprintf("-%d", numLines))
	} else {
		cmd = exec.Command("tmux", "capture-pane", "-t", target, "-p")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane content: %w", err)
	}

	return string(output), nil
}

// SendKeys sends keys to a tmux window
func (c *Client) SendKeys(target string, keys string) error {
	if err := c.ValidateTarget(target); err != nil {
		return err
	}

	cmd := exec.Command("tmux", "send-keys", "-t", target, keys)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys to %s: %w", target, err)
	}

	return nil
}

// DiscoverClaudeSessions attempts to discover tmux sessions that might contain Claude
func (c *Client) DiscoverClaudeSessions() ([]WindowInfo, error) {
	sessions, err := c.ListSessions()
	if err != nil {
		return nil, err
	}

	var claudeWindows []WindowInfo

	for _, session := range sessions {
		for _, window := range session.Windows {
			// Try to capture content and look for Claude indicators
			content, err := c.CapturePane(window.Target, 50)
			if err != nil {
				continue
			}

			// Look for Claude indicators in the content
			if c.isClaudeWindow(content) {
				claudeWindows = append(claudeWindows, window)
			}
		}
	}

	return claudeWindows, nil
}

// isClaudeWindow checks if window content indicates a Claude session
func (c *Client) isClaudeWindow(content string) bool {
	claudeIndicators := []string{
		"claude",
		"Claude",
		"anthropic",
		"Assistant:",
		"Human:",
		"I'm Claude",
		"claude-3",
		"I'm an AI assistant",
	}

	contentLower := strings.ToLower(content)
	for _, indicator := range claudeIndicators {
		if strings.Contains(contentLower, strings.ToLower(indicator)) {
			return true
		}
	}

	return false
}

// MonitorWindow monitors a window for changes and returns a channel of updates
func (c *Client) MonitorWindow(target string, interval time.Duration) (<-chan string, error) {
	if err := c.ValidateTarget(target); err != nil {
		return nil, err
	}

	updates := make(chan string, 10)

	go func() {
		defer close(updates)

		var lastContent string
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				content, err := c.CapturePane(target, 10)
				if err != nil {
					log.Printf("Error monitoring window %s: %v", target, err)
					continue
				}

				if content != lastContent {
					updates <- content
					lastContent = content
				}
			}
		}
	}()

	return updates, nil
}

// GetActiveWindow returns the currently active window in a session
func (c *Client) GetActiveWindow(sessionName string) (*WindowInfo, error) {
	windows, err := c.ListWindows(sessionName)
	if err != nil {
		return nil, err
	}

	for _, window := range windows {
		if window.Active {
			return &window, nil
		}
	}

	return nil, fmt.Errorf("no active window found in session '%s'", sessionName)
}

// CreateSession creates a new tmux session
func (c *Client) CreateSession(sessionName string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create session '%s': %w", sessionName, err)
	}

	return nil
}

// KillSession kills a tmux session
func (c *Client) KillSession(sessionName string) error {
	if !c.SessionExists(sessionName) {
		return fmt.Errorf("session '%s' does not exist", sessionName)
	}

	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill session '%s': %w", sessionName, err)
	}

	return nil
}

// GetServerInfo returns information about the tmux server
func (c *Client) GetServerInfo() (map[string]interface{}, error) {
	sessions, err := c.ListSessions()
	if err != nil {
		return nil, err
	}

	totalWindows := 0
	activeSessions := 0

	for _, session := range sessions {
		totalWindows += len(session.Windows)
		if session.Attached {
			activeSessions++
		}
	}

	info := map[string]interface{}{
		"total_sessions":  len(sessions),
		"active_sessions": activeSessions,
		"total_windows":   totalWindows,
		"server_running":  true,
		"last_checked":    time.Now().Format(time.RFC3339),
	}

	return info, nil
}
