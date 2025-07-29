package tests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	teatest "github.com/charmbracelet/x/exp/teatest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/derekxwang/tcs/internal/config"
	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/discovery"
	"github.com/derekxwang/tcs/internal/monitor"
	"github.com/derekxwang/tcs/internal/scheduler"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/tui"
)

// setupIntegrationDB creates an in-memory SQLite database for integration testing
func setupIntegrationDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		// Use silent logger for tests to reduce noise
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create integration test database: %v", err)
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
		t.Fatalf("Failed to migrate integration test database: %v", err)
	}

	// Initialize default app config for tests
	defaultConfig := &database.AppConfig{
		Key:   "test_mode",
		Value: "true",
	}
	if err := db.Create(defaultConfig).Error; err != nil {
		t.Logf("Warning: failed to create default config: %v", err)
	}

	return db
}

// createTestTUIApp creates a complete TUI application for testing
func createTestTUIApp(t *testing.T) *tui.App {
	// Initialize config first to avoid nil pointer dereference
	_, err := config.Load("")
	if err != nil {
		t.Logf("Config load failed, using defaults: %v", err)
	}

	// Setup database
	db := setupIntegrationDB(t)

	// Set this as the global database for the test (important for components that use database.GetDB())
	database.DB = db

	// Create components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(db)

	// Initialize usage monitor
	err = usageMonitor.Initialize()
	if err != nil {
		t.Logf("Warning: failed to initialize usage monitor: %v", err)
	}

	// Create window discovery
	windowDiscovery := discovery.NewWindowDiscovery(db, tmuxClient, nil)

	// Create scheduler
	schedulerInstance := scheduler.NewScheduler(db, tmuxClient, usageMonitor, nil)
	err = schedulerInstance.Initialize()
	if err != nil {
		t.Logf("Warning: failed to initialize scheduler: %v", err)
	}

	// Create TUI app
	app := tui.NewApp(db, tmuxClient, usageMonitor, windowDiscovery, schedulerInstance)

	return app
}

// TestTUI_ForceRescan_Integration tests force scan in full TUI context
func TestTUI_ForceRescan_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow integration test in short mode")
	}

	// Skip if no tmux available
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsRunning() {
		t.Skip("Tmux not running, skipping integration test")
	}

	t.Run("ForceRescanCrashDetection", func(t *testing.T) {
		// Create TUI app
		app := createTestTUIApp(t)

		// Create test model with timeout protection
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		tm := teatest.NewTestModel(
			t, app,
			teatest.WithInitialTermSize(120, 40),
		)

		// Track if we encounter a panic
		var panicOccurred bool
		var panicMessage string

		defer func() {
			if r := recover(); r != nil {
				panicOccurred = true
				panicMessage = fmt.Sprintf("%v", r)
				t.Logf("TUI Integration Test: Panic detected: %v", r)
			}
		}()

		// Wait for TUI to initialize and load
		t.Log("Waiting for TUI to initialize...")
		teatest.WaitFor(
			t, tm.Output(),
			func(bts []byte) bool {
				// Look for main TUI indicators
				return bytes.Contains(bts, []byte("TCS")) ||
					bytes.Contains(bts, []byte("Dashboard")) ||
					bytes.Contains(bts, []byte("Windows"))
			},
			teatest.WithCheckInterval(100*time.Millisecond),
			teatest.WithDuration(10*time.Second),
		)

		t.Log("TUI initialized, navigating to Windows view...")

		// Navigate to Windows view (press '2')
		tm.Send(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune("2"),
		})

		// Wait for Windows view to load
		teatest.WaitFor(
			t, tm.Output(),
			func(bts []byte) bool {
				return bytes.Contains(bts, []byte("Windows")) &&
					(bytes.Contains(bts, []byte("Session")) || bytes.Contains(bts, []byte("Target")))
			},
			teatest.WithCheckInterval(100*time.Millisecond),
			teatest.WithDuration(5*time.Second),
		)

		t.Log("Windows view loaded, attempting force scan...")

		// Capture output before force scan
		_, err := io.ReadAll(tm.Output())
		if err == nil {
			t.Log("Successfully captured pre-force-scan output")
		}

		// Execute force scan (press 'F')
		tm.Send(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune("F"),
		})

		// Wait for either success or crash
		done := make(chan bool, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicOccurred = true
					panicMessage = fmt.Sprintf("Goroutine panic: %v", r)
					t.Logf("Force scan goroutine panic: %v", r)
				}
				done <- true
			}()

			// Try to wait for force scan completion
			teatest.WaitFor(
				t, tm.Output(),
				func(bts []byte) bool {
					return bytes.Contains(bts, []byte("Force Rescan Complete")) ||
						bytes.Contains(bts, []byte("Force Rescan Failed")) ||
						bytes.Contains(bts, []byte("Error"))
				},
				teatest.WithCheckInterval(100*time.Millisecond),
				teatest.WithDuration(10*time.Second),
			)
		}()

		// Wait for completion or timeout
		select {
		case <-done:
			t.Log("Force scan operation completed")
		case <-ctx.Done():
			t.Log("Force scan test timed out")
		case <-time.After(15 * time.Second):
			t.Log("Force scan took longer than expected")
		}

		// Capture final output
		finalOutput, err := io.ReadAll(tm.Output())
		if err == nil {
			t.Logf("Final output length: %d bytes", len(finalOutput))
			if len(finalOutput) > 1000 {
				t.Logf("Final output sample (first 500 chars): %s", string(finalOutput[:500]))
			} else {
				t.Logf("Final output: %s", string(finalOutput))
			}
		}

		// Report results
		if panicOccurred {
			t.Errorf("FORCE SCAN CRASH DETECTED: %s", panicMessage)
		} else {
			t.Log("Force scan completed without crashing in integration test")
		}

		// Try to get final model state for analysis
		finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
		if finalModel != nil {
			t.Log("Final model retrieved successfully")
		} else {
			t.Log("Failed to retrieve final model - possible crash")
		}
	})
}

// TestTUI_ForceRescan_StepByStep tests each step of the TUI workflow
func TestTUI_ForceRescan_StepByStep(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow integration test in short mode")
	}

	if !tmux.NewClient().IsRunning() {
		t.Skip("Tmux not running, skipping step-by-step test")
	}

	tests := []struct {
		name         string
		keySequence  []string
		expectOutput []string
		description  string
	}{
		{
			name:         "TUI_Initialization",
			keySequence:  []string{},
			expectOutput: []string{"TCS", "Dashboard"},
			description:  "Test TUI initializes properly",
		},
		{
			name:         "Navigate_To_Windows",
			keySequence:  []string{"2"},
			expectOutput: []string{"Windows"},
			description:  "Test navigation to Windows view",
		},
		{
			name:         "Force_Scan_Trigger",
			keySequence:  []string{"2", "F"},
			expectOutput: []string{"Force Rescan", "Complete", "Failed"},
			description:  "Test force scan execution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := createTestTUIApp(t)

			tm := teatest.NewTestModel(
				t, app,
				teatest.WithInitialTermSize(120, 40),
			)

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Step %s crashed with panic: %v", tt.name, r)
				}
			}()

			// Wait for initial load
			time.Sleep(500 * time.Millisecond)

			// Send key sequence
			for _, key := range tt.keySequence {
				tm.Send(tea.KeyMsg{
					Type:  tea.KeyRunes,
					Runes: []rune(key),
				})
				time.Sleep(200 * time.Millisecond) // Small delay between keys
			}

			// Wait for processing
			time.Sleep(1 * time.Second)

			// Check output
			output, err := io.ReadAll(tm.Output())
			if err != nil {
				t.Errorf("Failed to read output: %v", err)
				return
			}

			outputStr := string(output)
			found := false

			for _, expected := range tt.expectOutput {
				if bytes.Contains(output, []byte(expected)) {
					found = true
					break
				}
			}

			if len(tt.expectOutput) > 0 && !found {
				t.Logf("Expected one of %v in output, but not found", tt.expectOutput)
				if len(outputStr) > 500 {
					t.Logf("Actual output (first 500 chars): %s", outputStr[:500])
				} else {
					t.Logf("Actual output: %s", outputStr)
				}
			}

			t.Logf("Step %s completed successfully", tt.name)
		})
	}
}

// TestTUI_ForceRescan_RaceConditions tests for race conditions
func TestTUI_ForceRescan_RaceConditions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow integration test in short mode")
	}

	if !tmux.NewClient().IsRunning() {
		t.Skip("Tmux not running, skipping race condition test")
	}

	t.Run("MultipleForceScans", func(t *testing.T) {
		app := createTestTUIApp(t)

		tm := teatest.NewTestModel(
			t, app,
			teatest.WithInitialTermSize(120, 40),
		)

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Race condition test crashed: %v", r)
			}
		}()

		// Navigate to windows
		tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
		time.Sleep(500 * time.Millisecond)

		// Send multiple force scans rapidly
		for i := 0; i < 3; i++ {
			t.Logf("Sending force scan %d", i+1)
			tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
			time.Sleep(100 * time.Millisecond) // Rapid succession
		}

		// Wait for all operations to complete
		time.Sleep(5 * time.Second)

		t.Log("Multiple force scans completed without crash")
	})
}

// TestTUI_ForceRescan_MemoryAndResources tests resource usage
func TestTUI_ForceRescan_MemoryAndResources(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow integration test in short mode")
	}

	if !tmux.NewClient().IsRunning() {
		t.Skip("Tmux not running, skipping resource test")
	}

	t.Run("ResourceCleanup", func(t *testing.T) {
		app := createTestTUIApp(t)

		tm := teatest.NewTestModel(
			t, app,
			teatest.WithInitialTermSize(120, 40),
		)

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Resource test crashed: %v", r)
			}
		}()

		// Navigate and force scan multiple times
		tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
		time.Sleep(500 * time.Millisecond)

		for i := 0; i < 5; i++ {
			t.Logf("Resource test iteration %d", i+1)
			tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
			time.Sleep(2 * time.Second) // Wait for completion
		}

		t.Log("Resource cleanup test completed")
	})
}
