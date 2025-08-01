package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/tui/views"
	"github.com/derekxwang/tcs/internal/types"
)

// TestTmuxClient implements TmuxInterface for testing
type TestTmuxClient struct {
	isRunning    bool
	sessions     []tmux.SessionInfo
	sessionsErr  error
	paneContents map[string]string // target -> content
	paneErrors   map[string]error  // target -> error
	shouldPanic  string            // which method should panic
}

func NewTestTmuxClient() *TestTmuxClient {
	return &TestTmuxClient{
		isRunning:    true,
		sessions:     []tmux.SessionInfo{},
		paneContents: make(map[string]string),
		paneErrors:   make(map[string]error),
	}
}

func (t *TestTmuxClient) IsRunning() bool {
	if t.shouldPanic == "IsRunning" {
		panic("test panic in IsRunning")
	}
	return t.isRunning
}

func (t *TestTmuxClient) ListSessions() ([]tmux.SessionInfo, error) {
	if t.shouldPanic == "ListSessions" {
		panic("test panic in ListSessions")
	}
	return t.sessions, t.sessionsErr
}

func (t *TestTmuxClient) CapturePane(target string, lines int) (string, error) {
	if t.shouldPanic == "CapturePane" {
		panic("test panic in CapturePane")
	}
	if err, exists := t.paneErrors[target]; exists {
		return "", err
	}
	if content, exists := t.paneContents[target]; exists {
		return content, nil
	}
	return "default content", nil
}

// setupForceTestDB creates an in-memory SQLite database for testing
func setupForceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Auto-migrate the tables
	err = db.AutoMigrate(
		&database.TmuxWindow{},
		&database.WindowMessageQueue{},
		&database.Message{},
		&database.UsageWindow{},
		&database.AppConfig{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

// TestForceRescan_Success tests successful force rescan
func TestForceRescan_Success(t *testing.T) {
	db := setupForceTestDB(t)
	testTmux := NewTestTmuxClient()

	// Configure test tmux client
	testTmux.sessions = []tmux.SessionInfo{
		{
			Name: "test-session",
			Windows: []tmux.WindowInfo{
				{
					SessionName: "test-session",
					WindowIndex: 0,
					WindowName:  "regular-window",
					Target:      "test-session:0",
				},
				{
					SessionName: "test-session",
					WindowIndex: 1,
					WindowName:  "claude-window",
					Target:      "test-session:1",
				},
			},
		},
	}

	// Configure pane contents
	testTmux.paneContents["test-session:0"] = "bash terminal content"
	testTmux.paneContents["test-session:1"] = "I'm Claude, an AI assistant created by Anthropic"

	// Create Windows view with mock client
	w := views.NewWindows(db, nil, testTmux)

	// Execute force rescan
	result := w.PerformForceRescan()

	// Verify success
	successMsg, ok := result.(types.SuccessMsg)
	assert.True(t, ok, "Expected SuccessMsg, got %T: %+v", result, result)
	assert.Contains(t, successMsg.Title, "Force Rescan Complete")
	assert.Contains(t, successMsg.Message, "Found 2 windows")
	assert.Contains(t, successMsg.Message, "1 with Claude")

	// Verify database was updated
	var windows []database.TmuxWindow
	err := db.Find(&windows).Error
	assert.NoError(t, err)
	assert.Len(t, windows, 2, "Should have 2 windows in database")

	// Find windows and verify data
	var claudeWindow, regularWindow database.TmuxWindow
	for _, window := range windows {
		if window.Target == "test-session:1" {
			claudeWindow = window
		} else if window.Target == "test-session:0" {
			regularWindow = window
		}
	}

	assert.NotZero(t, claudeWindow.ID, "Claude window should exist")
	assert.True(t, claudeWindow.HasClaude, "Claude window should have Claude detected")
	assert.Equal(t, "claude-window", claudeWindow.WindowName)

	assert.NotZero(t, regularWindow.ID, "Regular window should exist")
	assert.False(t, regularWindow.HasClaude, "Regular window should not have Claude detected")
	assert.Equal(t, "regular-window", regularWindow.WindowName)
}

// TestForceRescan_TmuxNotRunning tests when tmux is not running
func TestForceRescan_TmuxNotRunning(t *testing.T) {
	db := setupForceTestDB(t)
	testTmux := NewTestTmuxClient()
	testTmux.isRunning = false

	w := views.NewWindows(db, nil, testTmux)
	result := w.PerformForceRescan()

	errorMsg, ok := result.(types.ErrorMsg)
	assert.True(t, ok, "Expected ErrorMsg, got %T: %+v", result, result)
	assert.Equal(t, "Error", errorMsg.Title)
	assert.Contains(t, errorMsg.Message, "Tmux server is not running")
}

// TestForceRescan_NilTmuxClient tests with nil tmux client
func TestForceRescan_NilTmuxClient(t *testing.T) {
	db := setupForceTestDB(t)

	w := views.NewWindows(db, nil, nil)
	result := w.PerformForceRescan()

	errorMsg, ok := result.(types.ErrorMsg)
	assert.True(t, ok, "Expected ErrorMsg, got %T: %+v", result, result)
	assert.Equal(t, "Error", errorMsg.Title)
	assert.Contains(t, errorMsg.Message, "Tmux client not available")
}

// TestForceRescan_ListSessionsError tests when ListSessions fails
func TestForceRescan_ListSessionsError(t *testing.T) {
	db := setupForceTestDB(t)
	testTmux := NewTestTmuxClient()
	testTmux.sessionsErr = fmt.Errorf("tmux connection failed")

	w := views.NewWindows(db, nil, testTmux)
	result := w.PerformForceRescan()

	errorMsg, ok := result.(types.ErrorMsg)
	assert.True(t, ok, "Expected ErrorMsg, got %T: %+v", result, result)
	assert.Equal(t, "Rescan failed", errorMsg.Title)
	assert.Contains(t, errorMsg.Message, "tmux connection failed")
}

// TestForceRescan_PanicRecovery tests panic recovery in various methods
func TestForceRescan_PanicRecovery(t *testing.T) {
	tests := []struct {
		name        string
		panicMethod string
	}{
		{"ListSessions panic", "ListSessions"},
		{"CapturePane panic", "CapturePane"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupForceTestDB(t)
			testTmux := NewTestTmuxClient()
			testTmux.shouldPanic = tt.panicMethod

			// Add some session data if needed
			if tt.panicMethod == "CapturePane" {
				testTmux.sessions = []tmux.SessionInfo{
					{
						Name: "test-session",
						Windows: []tmux.WindowInfo{
							{
								SessionName: "test-session",
								WindowIndex: 0,
								WindowName:  "test-window",
								Target:      "test-session:0",
							},
						},
					},
				}
			}

			w := views.NewWindows(db, nil, testTmux)

			// This should not panic - the panic recovery should catch it
			result := w.PerformForceRescan()

			errorMsg, ok := result.(types.ErrorMsg)
			assert.True(t, ok, "Expected ErrorMsg from panic recovery, got %T: %+v", result, result)
			assert.Equal(t, "Force Rescan Failed", errorMsg.Title)
			assert.Contains(t, errorMsg.Message, "test panic")
		})
	}
}

// TestForceRescan_EmptySessions tests with no sessions
func TestForceRescan_EmptySessions(t *testing.T) {
	db := setupForceTestDB(t)
	testTmux := NewTestTmuxClient()
	// sessions is already empty by default

	w := views.NewWindows(db, nil, testTmux)
	result := w.PerformForceRescan()

	successMsg, ok := result.(types.SuccessMsg)
	assert.True(t, ok, "Expected SuccessMsg, got %T: %+v", result, result)
	assert.Contains(t, successMsg.Message, "Found 0 windows (0 with Claude) across 0 sessions")
}
