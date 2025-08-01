package tests

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/tui/views"
)

// setupCrashTestDB creates an in-memory SQLite database for testing
func setupCrashTestDB(t *testing.T) *gorm.DB {
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

// TestForceRescan_CrashDebug tests force rescan to identify where it crashes
func TestForceRescan_CrashDebug(t *testing.T) {
	// Setup
	db := setupCrashTestDB(t)
	tmuxClient := tmux.NewClient()

	// Create Windows view
	w := views.NewWindows(db, nil, tmuxClient)

	// This should help us identify where the crash occurs
	// We'll catch any panic and examine it
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Force rescan crashed with panic: %v", r)
			t.Logf("This helps us identify the exact location of the crash")
			// Don't fail the test, just log the panic details
		}
	}()

	// Call the method that's crashing
	result := w.PerformForceRescan()

	t.Logf("Force rescan completed without panic. Result: %+v", result)
}

// TestForceRescan_ComponentsIndividually tests individual components
func TestForceRescan_ComponentsIndividually(t *testing.T) {
	// Test tmux client individually
	t.Run("TmuxClient_IsRunning", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("IsRunning panicked: %v", r)
			}
		}()

		tmuxClient := tmux.NewClient()
		running := tmuxClient.IsRunning()
		t.Logf("Tmux is running: %v", running)
	})

	t.Run("TmuxClient_ListSessions", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("ListSessions panicked: %v", r)
			}
		}()

		tmuxClient := tmux.NewClient()
		if tmuxClient.IsRunning() {
			sessions, err := tmuxClient.ListSessions()
			t.Logf("ListSessions result: %d sessions, error: %v", len(sessions), err)
		} else {
			t.Log("Tmux not running, skipping ListSessions test")
		}
	})

	t.Run("Database_CreateWindow", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("CreateOrUpdateTmuxWindow panicked: %v", r)
			}
		}()

		db := setupCrashTestDB(t)

		_, err := database.CreateOrUpdateTmuxWindow(
			db,
			"test-session",
			0,
			"test-window",
			false,
		)
		t.Logf("CreateOrUpdateTmuxWindow result: error: %v", err)
	})

	t.Run("ClaudeDetection", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Claude detection panicked: %v", r)
			}
		}()

		// Import the utils package directly
		// This will test if the Claude detection itself is causing issues
		t.Log("Claude detection test - this will be implemented if needed")
	})
}

// TestForceRescan_StepByStep tests each step of force rescan individually
func TestForceRescan_StepByStep(t *testing.T) {
	db := setupCrashTestDB(t)
	tmuxClient := tmux.NewClient()

	t.Run("Step1_TmuxClientCheck", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Step 1 panicked: %v", r)
			}
		}()

		// Test: if w.tmuxClient == nil
		w := views.NewWindows(db, nil, nil)
		result := w.PerformForceRescan()
		t.Logf("Nil tmux client result: %+v", result)
	})

	t.Run("Step2_TmuxRunningCheck", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Step 2 panicked: %v", r)
			}
		}()

		// Test: w.tmuxClient.IsRunning()
		w := views.NewWindows(db, nil, tmuxClient)
		isRunning := tmuxClient.IsRunning()
		t.Logf("Tmux running check: %v", isRunning)

		if !isRunning {
			result := w.PerformForceRescan()
			t.Logf("Not running result: %+v", result)
			return
		}
	})

	t.Run("Step3_ListSessions", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Step 3 panicked: %v", r)
			}
		}()

		if !tmuxClient.IsRunning() {
			t.Skip("Tmux not running")
		}

		sessions, err := tmuxClient.ListSessions()
		t.Logf("List sessions result: %d sessions, error: %v", len(sessions), err)
	})
}
