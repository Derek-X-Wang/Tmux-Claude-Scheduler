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
	"github.com/derekxwang/tcs/internal/scheduler"
	"github.com/derekxwang/tcs/internal/tmux"
)

// setupTestDB creates a temporary test database
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate all models for window-based architecture
	err = db.AutoMigrate(
		&database.TmuxWindow{},
		&database.WindowMessageQueue{},
		&database.Message{},
		&database.UsageWindow{},
	)
	require.NoError(t, err)

	return db
}

// TestSchedulerInitialization tests scheduler initialization
func TestSchedulerInitialization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestDB(t)

	// Create mock components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(db)

	// Initialize monitor
	err := usageMonitor.Initialize()
	require.NoError(t, err)

	// Create scheduler
	s := scheduler.NewScheduler(db, tmuxClient, usageMonitor, nil)
	assert.NotNil(t, s)

	// Initialize scheduler
	err = s.Initialize()
	assert.NoError(t, err)
}

// TestScheduleMessage tests message scheduling
func TestScheduleMessage(t *testing.T) {
	db := setupTestDB(t)

	// Setup components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(db)

	err := usageMonitor.Initialize()
	require.NoError(t, err)

	// Create a test window first
	window := &database.TmuxWindow{
		SessionName: "test-session",
		WindowIndex: 0,
		Target:      "test-session:0",
		HasClaude:   true,
		Active:      true,
	}
	err = db.Create(window).Error
	require.NoError(t, err)

	// Create and initialize scheduler
	s := scheduler.NewScheduler(db, tmuxClient, usageMonitor, nil)
	err = s.Initialize()
	require.NoError(t, err)

	// Schedule a message with window target
	scheduledTime := time.Now().Add(1 * time.Hour)
	msg, err := s.ScheduleMessage("test-session:0", "Test message", scheduledTime, 7)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "Test message", msg.Content)
	assert.Equal(t, 7, msg.Priority)
	assert.Equal(t, database.MessageStatusPending, msg.Status)
}

// TestPriorityQueue tests the priority queue functionality
func TestPriorityQueue(t *testing.T) {
	db := setupTestDB(t)

	// Setup components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(db)

	err := usageMonitor.Initialize()
	require.NoError(t, err)

	// Create test window
	window := &database.TmuxWindow{
		SessionName: "test-session",
		WindowIndex: 0,
		Target:      "test-session:0",
		HasClaude:   true,
		Active:      true,
	}
	err = db.Create(window).Error
	require.NoError(t, err)

	// Create and initialize scheduler
	s := scheduler.NewScheduler(db, tmuxClient, usageMonitor, nil)
	err = s.Initialize()
	require.NoError(t, err)

	// Schedule messages with different priorities
	now := time.Now()

	msg1, err := s.ScheduleMessage("test-session:0", "Low priority", now, 3)
	require.NoError(t, err)

	msg2, err := s.ScheduleMessage("test-session:0", "High priority", now, 9)
	require.NoError(t, err)

	msg3, err := s.ScheduleMessage("test-session:0", "Medium priority", now, 5)
	require.NoError(t, err)

	// Verify all messages were created
	assert.NotNil(t, msg1)
	assert.NotNil(t, msg2)
	assert.NotNil(t, msg3)

	// Check that messages are stored in database
	var count int64
	db.Model(&database.Message{}).Count(&count)
	assert.Equal(t, int64(3), count)
}

// TestMessageStatusUpdates tests updating message status
func TestMessageStatusUpdates(t *testing.T) {
	db := setupTestDB(t)

	// Create a test window first
	window := &database.TmuxWindow{
		SessionName: "test-session",
		WindowIndex: 0,
		Target:      "test-session:0",
		HasClaude:   true,
		Active:      true,
	}
	err := db.Create(window).Error
	require.NoError(t, err)

	// Create a test message directly
	message := &database.Message{
		WindowID:      window.ID,
		Content:       "Test message",
		Priority:      5,
		Status:        database.MessageStatusPending,
		ScheduledTime: time.Now(),
	}
	err = db.Create(message).Error
	require.NoError(t, err)

	// Update status to sent
	err = db.Model(&database.Message{}).
		Where("id = ?", message.ID).
		Update("status", database.MessageStatusSent).Error
	require.NoError(t, err)

	// Verify update
	var updated database.Message
	err = db.First(&updated, message.ID).Error
	require.NoError(t, err)
	assert.Equal(t, database.MessageStatusSent, updated.Status)
}

// TestSchedulerWithUsageLimits tests scheduler respects usage limits
func TestSchedulerWithUsageLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestDB(t)

	// Setup components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(db)

	err := usageMonitor.Initialize()
	require.NoError(t, err)

	// Create scheduler
	s := scheduler.NewScheduler(db, tmuxClient, usageMonitor, nil)
	err = s.Initialize()
	require.NoError(t, err)

	// Get current usage stats
	stats, err := usageMonitor.GetCurrentStats()
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.True(t, stats.CanSendMessage)
}

// TestCronScheduling tests cron-based scheduling
func TestCronScheduling(t *testing.T) {
	db := setupTestDB(t)

	// Setup components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(db)

	err := usageMonitor.Initialize()
	require.NoError(t, err)

	// Create test window
	window := &database.TmuxWindow{
		SessionName: "cron-session",
		WindowIndex: 0,
		Target:      "cron-session:0",
		HasClaude:   true,
		Active:      true,
	}
	err = db.Create(window).Error
	require.NoError(t, err)

	// Create scheduler with cron enabled
	s := scheduler.NewScheduler(db, tmuxClient, usageMonitor, nil)
	err = s.Initialize()
	require.NoError(t, err)

	// Schedule a message for specific time
	futureTime := time.Now().Add(2 * time.Hour).Truncate(time.Minute)
	msg, err := s.ScheduleMessage("cron-session:0", "Cron message", futureTime, 5)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, futureTime.Unix(), msg.ScheduledTime.Unix())
}

// TestSessionPrioritySelection tests selecting sessions by priority
func TestSessionPrioritySelection(t *testing.T) {
	db := setupTestDB(t)

	// Setup components
	usageMonitor := monitor.NewUsageMonitor(db)

	err := usageMonitor.Initialize()
	require.NoError(t, err)

	// Create windows with different priorities
	window1 := &database.TmuxWindow{
		SessionName: "low-priority",
		WindowIndex: 0,
		Target:      "low-priority:0",
		HasClaude:   true,
		Active:      true,
		Priority:    3,
	}
	err = db.Create(window1).Error
	require.NoError(t, err)

	window2 := &database.TmuxWindow{
		SessionName: "high-priority",
		WindowIndex: 0,
		Target:      "high-priority:0",
		HasClaude:   true,
		Active:      true,
		Priority:    9,
	}
	err = db.Create(window2).Error
	require.NoError(t, err)

	window3 := &database.TmuxWindow{
		SessionName: "medium-priority",
		WindowIndex: 0,
		Target:      "medium-priority:0",
		HasClaude:   true,
		Active:      true,
		Priority:    5,
	}
	err = db.Create(window3).Error
	require.NoError(t, err)

	// Get highest priority window
	var highestPriority database.TmuxWindow
	err = db.Where("active = ?", true).Order("priority DESC").First(&highestPriority).Error
	assert.NoError(t, err)
	assert.Equal(t, window2.ID, highestPriority.ID)

	// Verify all windows exist
	var windows []database.TmuxWindow
	err = db.Find(&windows).Error
	assert.NoError(t, err)
	assert.Len(t, windows, 3)

	// Check window properties
	assert.Equal(t, 3, window1.Priority)
	assert.Equal(t, 9, window2.Priority)
	assert.Equal(t, 5, window3.Priority)
}

// TestMessageProcessingOrder tests that messages are processed in correct order
func TestMessageProcessingOrder(t *testing.T) {
	db := setupTestDB(t)

	// Create a test window first
	window := &database.TmuxWindow{
		SessionName: "test-session",
		WindowIndex: 0,
		Target:      "test-session:0",
		HasClaude:   true,
		Active:      true,
	}
	err := db.Create(window).Error
	require.NoError(t, err)

	// Create messages with different priorities and times
	now := time.Now()
	messages := []database.Message{
		{
			WindowID:      window.ID,
			Content:       "High priority, early",
			Priority:      9,
			Status:        database.MessageStatusPending,
			ScheduledTime: now,
		},
		{
			WindowID:      window.ID,
			Content:       "Low priority, early",
			Priority:      3,
			Status:        database.MessageStatusPending,
			ScheduledTime: now,
		},
		{
			WindowID:      window.ID,
			Content:       "High priority, late",
			Priority:      9,
			Status:        database.MessageStatusPending,
			ScheduledTime: now.Add(1 * time.Hour),
		},
		{
			WindowID:      window.ID,
			Content:       "Medium priority, early",
			Priority:      5,
			Status:        database.MessageStatusPending,
			ScheduledTime: now,
		},
	}

	// Create all messages
	for _, msg := range messages {
		err := db.Create(&msg).Error
		require.NoError(t, err)
	}

	// Query messages ordered by priority and scheduled time
	var orderedMessages []database.Message
	err = db.Model(&database.Message{}).
		Where("status = ?", database.MessageStatusPending).
		Order("priority DESC, scheduled_time ASC").
		Find(&orderedMessages).Error
	require.NoError(t, err)

	// Verify order
	assert.Len(t, orderedMessages, 4)
	assert.Equal(t, "High priority, early", orderedMessages[0].Content)
	assert.Equal(t, "High priority, late", orderedMessages[1].Content)
	assert.Equal(t, "Medium priority, early", orderedMessages[2].Content)
	assert.Equal(t, "Low priority, early", orderedMessages[3].Content)
}

// TestSchedulerShutdown tests graceful scheduler shutdown
func TestSchedulerShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - depends on real Claude data")
	}

	db := setupTestDB(t)

	// Setup components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(db)

	err := usageMonitor.Initialize()
	require.NoError(t, err)

	// Create and start scheduler
	s := scheduler.NewScheduler(db, tmuxClient, usageMonitor, nil)
	err = s.Initialize()
	require.NoError(t, err)

	err = s.Start()
	assert.NoError(t, err)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop scheduler
	err = s.Stop()
	assert.NoError(t, err)
}
