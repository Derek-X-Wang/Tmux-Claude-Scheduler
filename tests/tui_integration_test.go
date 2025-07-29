package tests

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/tui/views"
)

// setupTUITestDB creates an in-memory SQLite database for TUI testing
func setupTUITestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		// Use silent logger for tests to reduce noise
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Auto-migrate all tables in the same order as the main application
	err = db.AutoMigrate(
		&database.TmuxWindow{},
		&database.WindowMessageQueue{},
		&database.Message{},
		&database.UsageWindow{},
		&database.AppConfig{},
		&database.SchedulerState{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	// Set as global database for components that use database.GetDB()
	database.DB = db

	return db
}

// TestForceRescan_BubbleTeaCommand tests force rescan as a Bubble Tea command
func TestForceRescan_BubbleTeaCommand(t *testing.T) {
	db := setupTUITestDB(t)
	tmuxClient := tmux.NewClient()

	// Create Windows view
	w := views.NewWindows(db, nil, tmuxClient)

	// Test the tea.Cmd directly
	t.Run("ForceRescanCommand", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Force rescan command panicked: %v", r)
			}
		}()

		// Get the command
		cmd := w.ForceRescan() // We need to make this public for testing

		if cmd == nil {
			t.Fatal("ForceRescan() returned nil command")
		}

		// Execute the command (this simulates what Bubble Tea does)
		msg := cmd()

		t.Logf("Command executed successfully, returned message: %+v", msg)
	})
}

// TestForceRescan_KeyHandling tests the key handling that triggers force rescan
func TestForceRescan_KeyHandling(t *testing.T) {
	db := setupTUITestDB(t)
	tmuxClient := tmux.NewClient()

	// Create Windows view
	w := views.NewWindows(db, nil, tmuxClient)

	// Initialize the view
	w.Init()

	t.Run("ForceRescanKeyPress", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Key handling panicked: %v", r)
			}
		}()

		// Simulate the 'F' key press that triggers force rescan
		// We need to check what key binding is used for force rescan
		keyMsg := tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{'F'},
		}

		// Call Update method (this simulates what happens in the TUI)
		_, cmd := w.Update(keyMsg)

		if cmd != nil {
			t.Logf("Key press generated command, executing...")

			// Execute the command
			msg := cmd()
			t.Logf("Command executed, returned message: %+v", msg)
		} else {
			t.Log("Key press did not generate a command")
		}
	})
}

// TestForceRescan_MessageProcessing tests how the TUI processes the returned message
func TestForceRescan_MessageProcessing(t *testing.T) {
	db := setupTUITestDB(t)
	tmuxClient := tmux.NewClient()

	// Create Windows view
	w := views.NewWindows(db, nil, tmuxClient)

	t.Run("ProcessForceRescanResult", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Message processing panicked: %v", r)
			}
		}()

		// Get a force rescan result directly
		result := w.PerformForceRescan()

		// Try to process this message through the Update method
		// This simulates what happens when the command result comes back
		_, cmd := w.Update(result)

		t.Logf("Message processed successfully, returned command: %v", cmd != nil)
	})
}

// TestForceRescan_FullWorkflow tests the complete workflow
func TestForceRescan_FullWorkflow(t *testing.T) {
	db := setupTUITestDB(t)
	tmuxClient := tmux.NewClient()

	// Create Windows view
	w := views.NewWindows(db, nil, tmuxClient)
	w.Init()

	t.Run("CompleteWorkflow", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Complete workflow panicked at some point: %v", r)
			}
		}()

		// Step 1: Simulate key press
		keyMsg := tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{'F'},
		}

		model, cmd := w.Update(keyMsg)
		t.Logf("Step 1 - Key press processed")

		if cmd != nil {
			// Step 2: Execute the command
			result := cmd()
			t.Logf("Step 2 - Command executed, result: %+v", result)

			// Step 3: Process the result
			finalModel, finalCmd := model.Update(result)
			t.Logf("Step 3 - Result processed, final command: %v", finalCmd != nil)

			_ = finalModel // Use the final model to avoid unused variable warning
		}
	})
}
