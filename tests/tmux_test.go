package tests

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/derekxwang/tcs/internal/tmux"
)

// skipIfNoTmux skips the test if tmux is not available
func skipIfNoTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
}

// setupTestTmuxSession creates a test tmux session
func setupTestTmuxSession(t *testing.T, sessionName string) {
	skipIfNoTmux(t)

	// Kill any existing session with the same name
	_ = exec.Command("tmux", "kill-session", "-t", sessionName).Run()

	// Create new session
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName)
	err := cmd.Run()
	require.NoError(t, err, "Failed to create tmux session")

	// Add cleanup
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})
}

// TestTmuxClientIsRunning tests checking if tmux is running
func TestTmuxClientIsRunning(t *testing.T) {
	skipIfNoTmux(t)

	client := tmux.NewClient()

	// Before creating any sessions, it might not be running
	// Create a test session to ensure tmux is running
	setupTestTmuxSession(t, "test-running")

	running := client.IsRunning()
	assert.True(t, running, "Tmux should be running")
}

// TestTmuxClientListSessions tests listing tmux sessions
func TestTmuxClientListSessions(t *testing.T) {
	skipIfNoTmux(t)

	// Create test sessions
	setupTestTmuxSession(t, "test-list-1")
	setupTestTmuxSession(t, "test-list-2")

	client := tmux.NewClient()
	sessions, err := client.ListSessions()
	assert.NoError(t, err)
	assert.NotEmpty(t, sessions)

	// Check that our test sessions exist
	var foundSession1, foundSession2 bool
	for _, session := range sessions {
		if session.Name == "test-list-1" {
			foundSession1 = true
		}
		if session.Name == "test-list-2" {
			foundSession2 = true
		}
	}

	assert.True(t, foundSession1, "Should find test-list-1 session")
	assert.True(t, foundSession2, "Should find test-list-2 session")
}

// TestTmuxClientSessionExists tests checking if a session exists
func TestTmuxClientSessionExists(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-exists"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()

	// Test existing session
	exists := client.SessionExists(sessionName)
	assert.True(t, exists, "Session should exist")

	// Test non-existing session
	exists = client.SessionExists("non-existent-session")
	assert.False(t, exists, "Non-existent session should not exist")
}

// TestTmuxClientWindowOperations tests window-related operations
func TestTmuxClientWindowOperations(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-windows"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()

	// List windows
	windows, err := client.ListWindows(sessionName)
	assert.NoError(t, err)
	assert.Len(t, windows, 1, "New session should have one window")

	// Check window properties
	window := windows[0]
	assert.Equal(t, sessionName, window.SessionName)
	assert.Equal(t, 0, window.WindowIndex)
	assert.Equal(t, fmt.Sprintf("%s:0", sessionName), window.Target)

	// Check window exists
	exists := client.WindowExists(sessionName, 0)
	assert.True(t, exists, "Window 0 should exist")

	exists = client.WindowExists(sessionName, 99)
	assert.False(t, exists, "Window 99 should not exist")
}

// TestTmuxClientParseTarget tests parsing tmux targets
func TestTmuxClientParseTarget(t *testing.T) {
	// Test valid targets
	sessionName, windowIndex, err := tmux.ParseTarget("mysession:0")
	assert.NoError(t, err)
	assert.Equal(t, "mysession", sessionName)
	assert.Equal(t, 0, windowIndex)

	sessionName, windowIndex, err = tmux.ParseTarget("another-session:5")
	assert.NoError(t, err)
	assert.Equal(t, "another-session", sessionName)
	assert.Equal(t, 5, windowIndex)

	// Test invalid targets
	_, _, err = tmux.ParseTarget("invalid")
	assert.Error(t, err)

	_, _, err = tmux.ParseTarget("session:window")
	assert.Error(t, err)

	_, _, err = tmux.ParseTarget("")
	assert.Error(t, err)
}

// TestTmuxClientSendKeys tests sending keys to a tmux window
func TestTmuxClientSendKeys(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-sendkeys"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()
	target := fmt.Sprintf("%s:0", sessionName)

	// Send some text
	err := client.SendKeys(target, "echo 'Hello from test'")
	assert.NoError(t, err)

	// Send Enter key
	err = client.SendKeys(target, "Enter")
	assert.NoError(t, err)

	// Give it a moment to process
	time.Sleep(100 * time.Millisecond)

	// Capture pane content
	content, err := client.CapturePane(target, 10)
	assert.NoError(t, err)
	assert.Contains(t, content, "echo 'Hello from test'")
}

// TestTmuxMessageSender tests the message sender with proper delay
func TestTmuxMessageSender(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-sender"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()
	sender := tmux.NewMessageSender(client)
	target := fmt.Sprintf("%s:0", sessionName)

	// Record start time
	startTime := time.Now()

	// Send message
	result, err := sender.SendMessage(target, "Test message with delay")

	// Record end time
	duration := time.Since(startTime)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Empty(t, result.Error)

	// Verify the critical 500ms delay was included
	assert.GreaterOrEqual(t, duration, 500*time.Millisecond,
		"SendMessage should include at least 500ms delay")

	// Check that message appears in pane
	time.Sleep(100 * time.Millisecond) // Give it time to render
	content, err := client.CapturePane(target, 10)
	assert.NoError(t, err)
	assert.Contains(t, content, "Test message with delay")
}

// TestTmuxClientDiscoverClaudeSessions tests discovering Claude sessions
func TestTmuxClientDiscoverClaudeSessions(t *testing.T) {
	skipIfNoTmux(t)

	// Create a session and add Claude-like content
	sessionName := "test-claude-discovery"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()
	target := fmt.Sprintf("%s:0", sessionName)

	// Send Claude-like content
	err := client.SendKeys(target, "Human: Hello Claude")
	assert.NoError(t, err)
	err = client.SendKeys(target, "Enter")
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Discover Claude sessions
	claudeWindows, err := client.DiscoverClaudeSessions()
	assert.NoError(t, err)

	// Check if our session was discovered
	found := false
	for _, window := range claudeWindows {
		if window.SessionName == sessionName {
			found = true
			break
		}
	}

	assert.True(t, found, "Should discover session with Claude content")
}

// TestTmuxClientMonitorWindow tests window monitoring
func TestTmuxClientMonitorWindow(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-monitor"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()
	target := fmt.Sprintf("%s:0", sessionName)

	// Start monitoring
	updates, err := client.MonitorWindow(target, 100*time.Millisecond)
	assert.NoError(t, err)
	assert.NotNil(t, updates)

	// Wait for initial update
	select {
	case update := <-updates:
		assert.NotEmpty(t, update)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for initial update")
	}

	// Send some content
	err = client.SendKeys(target, "New content")
	assert.NoError(t, err)

	// Wait for update
	select {
	case update := <-updates:
		assert.Contains(t, update, "New content")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for content update")
	}
}

// TestTmuxClientGetActiveWindow tests getting the active window
func TestTmuxClientGetActiveWindow(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-active"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()

	// Get active window
	activeWindow, err := client.GetActiveWindow(sessionName)
	assert.NoError(t, err)
	assert.NotNil(t, activeWindow)
	assert.Equal(t, sessionName, activeWindow.SessionName)
	assert.True(t, activeWindow.Active)
}

// TestTmuxClientValidateTarget tests target validation
func TestTmuxClientValidateTarget(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-validate"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()

	// Test valid target
	err := client.ValidateTarget(fmt.Sprintf("%s:0", sessionName))
	assert.NoError(t, err)

	// Test invalid session
	err = client.ValidateTarget("non-existent:0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")

	// Test invalid window
	err = client.ValidateTarget(fmt.Sprintf("%s:99", sessionName))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "window")

	// Test invalid format
	err = client.ValidateTarget("invalid-format")
	assert.Error(t, err)
}

// TestTmuxClientCreateAndKillSession tests session lifecycle
func TestTmuxClientCreateAndKillSession(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-lifecycle"
	client := tmux.NewClient()

	// Ensure session doesn't exist
	_ = client.KillSession(sessionName)

	// Create session
	err := client.CreateSession(sessionName)
	assert.NoError(t, err)

	// Verify it exists
	exists := client.SessionExists(sessionName)
	assert.True(t, exists)

	// Kill session
	err = client.KillSession(sessionName)
	assert.NoError(t, err)

	// Verify it's gone
	exists = client.SessionExists(sessionName)
	assert.False(t, exists)

	// Try to kill non-existent session
	err = client.KillSession("non-existent-session")
	assert.Error(t, err)
}

// TestTmuxMessageSenderConcurrency tests concurrent message sending
func TestTmuxMessageSenderConcurrency(t *testing.T) {
	skipIfNoTmux(t)

	sessionName := "test-concurrent"
	setupTestTmuxSession(t, sessionName)

	client := tmux.NewClient()
	target := fmt.Sprintf("%s:0", sessionName)

	// Create multiple message senders
	numSenders := 3
	results := make(chan *tmux.SendResult, numSenders)
	errors := make(chan error, numSenders)

	// Send messages concurrently
	for i := 0; i < numSenders; i++ {
		go func(id int) {
			sender := tmux.NewMessageSender(client)
			result, err := sender.SendMessage(target, fmt.Sprintf("Message %d", id))
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numSenders; i++ {
		select {
		case result := <-results:
			assert.True(t, result.Success)
			successCount++
		case err := <-errors:
			t.Errorf("Concurrent send error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent sends")
		}
	}

	assert.Equal(t, numSenders, successCount, "All messages should be sent successfully")
}

// TestTmuxClientGetServerInfo tests getting server information
func TestTmuxClientGetServerInfo(t *testing.T) {
	skipIfNoTmux(t)

	// Create a couple of test sessions
	setupTestTmuxSession(t, "test-info-1")
	setupTestTmuxSession(t, "test-info-2")

	client := tmux.NewClient()
	info, err := client.GetServerInfo()
	assert.NoError(t, err)
	assert.NotNil(t, info)

	// Check that info contains expected fields
	assert.Contains(t, info, "total_sessions")
	assert.Contains(t, info, "total_windows")
	assert.Contains(t, info, "server_running")
	assert.Contains(t, info, "last_checked")

	// Verify values
	serverRunning, ok := info["server_running"].(bool)
	require.True(t, ok, "server_running should be a bool")
	assert.True(t, serverRunning)

	totalSessions, ok := info["total_sessions"].(int)
	require.True(t, ok, "total_sessions should be an int")
	assert.GreaterOrEqual(t, totalSessions, 2)
}
