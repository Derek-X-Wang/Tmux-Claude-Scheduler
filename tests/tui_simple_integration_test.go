package tests

import (
	"io"
	"log"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/derekxwang/tcs/internal/config"
	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/tui/views"
)

// setupSimpleTestDB creates a test database with minimal logging
func setupSimpleTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

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

	// Set as global database
	database.DB = db

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	return db
}

// TestForceRescan_WindowsView_Direct tests force scan directly on the Windows view
func TestForceRescan_WindowsView_Direct(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Silence log output during tests
	log.SetOutput(io.Discard)
	defer log.SetOutput(log.Writer())

	// Initialize config
	_, err := config.Load("")
	if err != nil {
		t.Logf("Config load failed, using defaults: %v", err)
	}

	// Setup test database
	db := setupSimpleTestDB(t)
	tmuxClient := tmux.NewClient()

	// Skip if no tmux available
	if !tmuxClient.IsRunning() {
		t.Skip("Tmux not running, skipping integration test")
	}

	// Create Windows view directly
	w := views.NewWindows(db, nil, tmuxClient)
	w.Init()

	// Test that the view can handle a force rescan key press
	t.Run("ForceRescanKeyHandling", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Force rescan panicked: %v", r)
			}
		}()

		// Simulate the 'F' key press
		keyMsg := tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{'F'},
		}

		// Call Update method (this simulates what happens in the TUI)
		model, cmd := w.Update(keyMsg)
		assert.NotNil(t, model)

		if cmd != nil {
			// Execute the command to test it doesn't crash
			result := cmd()
			t.Logf("Force rescan command executed successfully, result: %+v", result)

			// Process the result
			finalModel, finalCmd := model.Update(result)
			assert.NotNil(t, finalModel)
			t.Logf("Force rescan result processed, has follow-up command: %v", finalCmd != nil)
		} else {
			t.Log("Force rescan key press did not generate a command")
		}
	})

	// Test force rescan method directly
	t.Run("ForceRescanMethod", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Force rescan method panicked: %v", r)
			}
		}()

		// Call the force rescan method directly
		result := w.PerformForceRescan()
		assert.NotNil(t, result)
		t.Logf("Direct force rescan result: %+v", result)
	})
}

// TestTUI_ComponentIntegration tests individual TUI components
func TestTUI_ComponentIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Silence logging
	log.SetOutput(io.Discard)
	defer log.SetOutput(log.Writer())

	// Initialize config
	_, err := config.Load("")
	if err != nil {
		t.Logf("Config load failed, using defaults: %v", err)
	}

	db := setupSimpleTestDB(t)
	tmuxClient := tmux.NewClient()

	t.Run("WindowsViewCreation", func(t *testing.T) {
		w := views.NewWindows(db, nil, tmuxClient)
		assert.NotNil(t, w)

		// Test initialization
		cmd := w.Init()
		if cmd != nil {
			result := cmd()
			t.Logf("Windows view init command result: %+v", result)
		}
	})

	t.Run("WindowsViewKeyBindings", func(t *testing.T) {
		w := views.NewWindows(db, nil, tmuxClient)
		w.Init()

		// Test various key bindings
		keys := []rune{'r', 'R', 'f', 'F', 'q', 'Q'}
		for _, key := range keys {
			keyMsg := tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{key},
			}

			model, cmd := w.Update(keyMsg)
			assert.NotNil(t, model)
			t.Logf("Key '%c' processed, has command: %v", key, cmd != nil)
		}
	})
}

// TestTUI_BasicWorkflow tests basic TUI workflow without teatest
func TestTUI_BasicWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Silence logging
	log.SetOutput(io.Discard)
	defer log.SetOutput(log.Writer())

	db := setupSimpleTestDB(t)
	tmuxClient := tmux.NewClient()

	if !tmuxClient.IsRunning() {
		t.Skip("Tmux not running, skipping workflow test")
	}

	// Test the workflow: create view -> init -> key press -> command execution
	t.Run("CompleteWorkflow", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Workflow panicked: %v", r)
			}
		}()

		// Step 1: Create and initialize view
		w := views.NewWindows(db, nil, tmuxClient)
		initCmd := w.Init()
		if initCmd != nil {
			initResult := initCmd()
			t.Logf("Init result: %+v", initResult)
		}

		// Step 2: Process key input
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}}
		model, cmd := w.Update(keyMsg)
		assert.NotNil(t, model)

		// Step 3: Execute command if generated
		if cmd != nil {
			result := cmd()
			t.Logf("Command result: %+v", result)

			// Step 4: Process command result
			finalModel, finalCmd := model.Update(result)
			assert.NotNil(t, finalModel)
			t.Logf("Final workflow step completed, has follow-up: %v", finalCmd != nil)
		}

		t.Log("Complete workflow test passed")
	})
}
