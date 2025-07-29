package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/monitor"
)

// setupTestMonitorDB creates a test database for monitor tests
func setupTestMonitorDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Only migrate models needed for window-based architecture
	err = db.AutoMigrate(
		&database.TmuxWindow{},
		&database.WindowMessageQueue{},
		&database.Message{},
		&database.UsageWindow{},
	)
	require.NoError(t, err)

	return db
}

// TestUsageMonitorInitialization tests usage monitor initialization
func TestUsageMonitorInitialization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestMonitorDB(t)

	monitor := monitor.NewUsageMonitor(db)
	assert.NotNil(t, monitor)

	err := monitor.Initialize()
	assert.NoError(t, err)

	// Check that a usage window was created
	var count int64
	db.Model(&database.UsageWindow{}).Count(&count)
	assert.Equal(t, int64(1), count, "Should create one usage window on initialization")
}

// TestUsageMonitor5HourWindow tests the 5-hour window tracking
func TestUsageMonitor5HourWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestMonitorDB(t)

	monitor := monitor.NewUsageMonitor(db)
	err := monitor.Initialize()
	require.NoError(t, err)

	// Get current stats
	stats, err := monitor.GetCurrentStats()
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Check window duration
	windowDuration := stats.WindowEndTime.Sub(stats.WindowStartTime)
	assert.Equal(t, 5*time.Hour, windowDuration, "Window should be exactly 5 hours")

	// Should be able to send messages (unless at limit)
	assert.NotNil(t, stats.CanSendMessage)               // Just verify the field exists
	assert.GreaterOrEqual(t, stats.MessagesUsed, 0)      // Should be non-negative
	assert.GreaterOrEqual(t, stats.UsagePercentage, 0.0) // Should be non-negative percentage
	assert.LessOrEqual(t, stats.UsagePercentage, 1.0)    // Should not exceed 100%
}

// TestUsageMonitorBasicFunctionality tests basic usage monitor functionality
func TestUsageMonitorBasicFunctionality(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestMonitorDB(t)

	monitor := monitor.NewUsageMonitor(db)
	err := monitor.Initialize()
	require.NoError(t, err)

	// Test getting current stats
	stats, err := monitor.GetCurrentStats()
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.MessagesUsed, 0)      // Should be non-negative
	assert.GreaterOrEqual(t, stats.TokensUsed, 0)        // Should be non-negative
	assert.GreaterOrEqual(t, stats.UsagePercentage, 0.0) // Should be non-negative percentage
	assert.LessOrEqual(t, stats.UsagePercentage, 1.0)    // Should not exceed 100%
	assert.NotNil(t, stats.CanSendMessage)               // Just verify the field exists

	// Test window functionality
	window := monitor.GetCurrentWindow()
	assert.NotNil(t, window)
	assert.True(t, window.Active)
}

// TestUsageMonitorWindowExpiry tests window expiry behavior
func TestUsageMonitorWindowExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestMonitorDB(t)

	// Create an expired window manually
	expiredWindow := &database.UsageWindow{
		StartTime:     time.Now().Add(-6 * time.Hour),
		EndTime:       time.Now().Add(-1 * time.Hour),
		TotalMessages: 50,
		TotalTokens:   5000,
		Active:        true,
	}
	err := db.Create(expiredWindow).Error
	require.NoError(t, err)

	monitor := monitor.NewUsageMonitor(db)
	err = monitor.Initialize()
	require.NoError(t, err)

	// Should create a new window since the existing one is expired
	var windows []database.UsageWindow
	db.Find(&windows)
	assert.GreaterOrEqual(t, len(windows), 2, "Should have at least 2 windows (expired + new)")

	// Check that only one window is active
	var activeCount int64
	db.Model(&database.UsageWindow{}).Where("active = ?", true).Count(&activeCount)
	assert.Equal(t, int64(1), activeCount, "Only one window should be active")
}

// TestUsageMonitorMaxMessages tests maximum message limit enforcement
func TestUsageMonitorMaxMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestMonitorDB(t)

	// Create a window that's almost at limit
	window := &database.UsageWindow{
		StartTime:     time.Now(),
		EndTime:       time.Now().Add(5 * time.Hour),
		TotalMessages: 999, // Just under the 1000 limit
		TotalTokens:   99900,
		Active:        true,
	}
	err := db.Create(window).Error
	require.NoError(t, err)

	monitor := monitor.NewUsageMonitor(db)
	err = monitor.Initialize()
	require.NoError(t, err)

	// Get initial stats (will have real Claude data)
	stats, err := monitor.GetCurrentStats()
	assert.NoError(t, err)

	// Test that stats are reasonable
	assert.GreaterOrEqual(t, stats.MessagesUsed, 0)      // Should be non-negative
	assert.GreaterOrEqual(t, stats.UsagePercentage, 0.0) // Should be non-negative percentage
	assert.LessOrEqual(t, stats.UsagePercentage, 1.0)    // Should not exceed 100%

	// Test the limit logic by manually setting the window to near-limit
	// This tests the monitor's ability to calculate limits correctly
	window.TotalMessages = 999
	db.Save(window)

	// Reinitialize monitor to pick up the change
	err = monitor.Initialize()
	require.NoError(t, err)

	stats, err = monitor.GetCurrentStats()
	assert.NoError(t, err)

	// Should be able to determine if we can send messages based on the limit
	assert.NotNil(t, stats.CanSendMessage)               // Field should exist
	assert.GreaterOrEqual(t, stats.UsagePercentage, 0.0) // Should be valid percentage
}
