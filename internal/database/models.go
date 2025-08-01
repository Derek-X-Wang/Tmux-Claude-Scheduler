package database

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Message represents a scheduled message (window-based architecture)
type Message struct {
	gorm.Model
	WindowID      uint       `gorm:"not null;index" json:"window_id"`
	Window        TmuxWindow `gorm:"foreignKey:WindowID" json:"window"`
	Content       string     `gorm:"type:text;not null" json:"content"`
	ScheduledTime time.Time  `gorm:"index" json:"scheduled_time"`
	Priority      int        `gorm:"default:5;index" json:"priority"`       // 1-10, higher = more important
	Status        string     `gorm:"default:'pending';index" json:"status"` // pending, sent, failed
	Error         string     `gorm:"type:text" json:"error"`
	SentAt        *time.Time `json:"sent_at"`
	Retries       int        `gorm:"default:0" json:"retries"`
	MaxRetries    int        `gorm:"default:3" json:"max_retries"`
}

// UsageWindow tracks 5-hour usage windows
type UsageWindow struct {
	gorm.Model
	StartTime     time.Time `gorm:"index;not null" json:"start_time"`
	EndTime       time.Time `gorm:"index;not null" json:"end_time"`
	TotalMessages int       `gorm:"default:0" json:"total_messages"`
	TotalTokens   int       `gorm:"default:0" json:"total_tokens"`
	SessionCount  int       `gorm:"default:0" json:"session_count"`
	Active        bool      `gorm:"default:true" json:"active"`
}

// Configuration for the app stored in database
type AppConfig struct {
	gorm.Model
	Key   string `gorm:"uniqueIndex;not null" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

// TmuxSession tracks discovered tmux sessions
type TmuxSession struct {
	gorm.Model
	Name      string    `gorm:"uniqueIndex;not null" json:"name"`
	Target    string    `gorm:"not null" json:"target"` // session:window format
	Active    bool      `gorm:"default:true" json:"active"`
	LastSeen  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"last_seen"`
	HasClaude bool      `gorm:"default:false" json:"has_claude"` // detected Claude instance
}

// TmuxWindow represents a discovered tmux window (new window-based architecture)
type TmuxWindow struct {
	gorm.Model
	SessionName  string     `gorm:"index;not null" json:"session_name"`         // tmux session name
	WindowIndex  int        `gorm:"not null" json:"window_index"`               // window index (0, 1, 2...)
	WindowName   string     `json:"window_name"`                                // window title
	Target       string     `gorm:"uniqueIndex;not null" json:"target"`         // "session:window" format
	HasClaude    bool       `gorm:"default:false" json:"has_claude"`            // detected Claude instance
	Priority     int        `gorm:"default:5" json:"priority"`                  // queue priority (1-10)
	Active       bool       `gorm:"default:true" json:"active"`                 // enabled for scheduling
	LastSeen     time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"last_seen"` // last discovery time
	LastActivity *time.Time `json:"last_activity"`                              // last message activity
}

// WindowMessageQueue represents a message queue for a specific tmux window
type WindowMessageQueue struct {
	gorm.Model
	WindowID      uint       `gorm:"not null;index" json:"window_id"` // links to TmuxWindow
	Window        TmuxWindow `gorm:"foreignKey:WindowID" json:"window"`
	Priority      int        `gorm:"default:5" json:"priority"`      // queue priority (1-10)
	Active        bool       `gorm:"default:true" json:"active"`     // queue enabled
	MessageCount  int        `gorm:"default:0" json:"message_count"` // pending messages
	LastProcessed *time.Time `json:"last_processed"`                 // last message processed
}

// SchedulerState tracks the state of different schedulers
type SchedulerState struct {
	gorm.Model
	Name       string     `gorm:"uniqueIndex;not null" json:"name"` // "smart", "cron"
	Enabled    bool       `gorm:"default:true" json:"enabled"`
	LastRun    *time.Time `json:"last_run"`
	NextRun    *time.Time `json:"next_run"`
	RunCount   int        `gorm:"default:0" json:"run_count"`
	ErrorCount int        `gorm:"default:0" json:"error_count"`
	Status     string     `gorm:"default:'idle'" json:"status"` // idle, running, error
}

// Constants for message statuses
const (
	MessageStatusPending = "pending"
	MessageStatusSent    = "sent"
	MessageStatusFailed  = "failed"
	MessageStatusRetry   = "retry"
)

// Constants for scheduler states
const (
	SchedulerStatusIdle    = "idle"
	SchedulerStatusRunning = "running"
	SchedulerStatusError   = "error"
)

// Constants for scheduler names
const (
	SchedulerSmart = "smart"
	SchedulerCron  = "cron"
)

// Helper methods

// IsExpired checks if a usage window has expired (5-hour limit)
func (uw *UsageWindow) IsExpired() bool {
	return time.Now().After(uw.EndTime)
}

// TimeRemaining returns the time remaining in the usage window
func (uw *UsageWindow) TimeRemaining() time.Duration {
	if uw.IsExpired() {
		return 0
	}
	return time.Until(uw.EndTime)
}

// CanRetry checks if a message can be retried
func (m *Message) CanRetry() bool {
	return m.Status == MessageStatusFailed && m.Retries < m.MaxRetries
}

// IsScheduled checks if a message is ready to be sent
func (m *Message) IsScheduled() bool {
	return m.Status == MessageStatusPending && time.Now().After(m.ScheduledTime)
}

// GetCurrentUsageWindow returns the current active usage window
func GetCurrentUsageWindow(db *gorm.DB) (*UsageWindow, error) {
	var window UsageWindow
	err := db.Where("active = ? AND start_time <= ? AND end_time >= ?",
		true, time.Now(), time.Now()).First(&window).Error
	if err != nil {
		return nil, err
	}
	return &window, nil
}

// CreateNewUsageWindow creates a new 5-hour usage window
func CreateNewUsageWindow(db *gorm.DB) (*UsageWindow, error) {
	now := time.Now()
	window := &UsageWindow{
		StartTime: now,
		EndTime:   now.Add(5 * time.Hour),
		Active:    true,
	}

	if err := db.Create(window).Error; err != nil {
		return nil, err
	}

	return window, nil
}

// GetPendingMessages returns all pending messages ordered by priority and scheduled time
func GetPendingMessages(db *gorm.DB, limit int) ([]Message, error) {
	var messages []Message
	query := db.Where("status = ? AND scheduled_time <= ?",
		MessageStatusPending, time.Now()).
		Order("priority DESC, scheduled_time ASC").
		Preload("Window")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// UpdateMessageStatus updates the status of a message
func UpdateMessageStatus(db *gorm.DB, messageID uint, status string, errorMsg string) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if status == MessageStatusSent {
		now := time.Now()
		updates["sent_at"] = &now
	}

	if errorMsg != "" {
		updates["error"] = errorMsg
		updates["retries"] = gorm.Expr("retries + 1")
	}

	return db.Model(&Message{}).Where("id = ?", messageID).Updates(updates).Error
}

// CleanupOldData removes old data to keep database size manageable
func CleanupOldData(db *gorm.DB, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)

	// Clean up old sent messages
	if err := db.Where("status = ? AND sent_at < ?", MessageStatusSent, cutoff).
		Delete(&Message{}).Error; err != nil {
		return err
	}

	// Clean up old usage windows
	if err := db.Where("active = false AND created_at < ?", cutoff).
		Delete(&UsageWindow{}).Error; err != nil {
		return err
	}

	return nil
}

// Helper functions for window-based architecture

// GetTmuxWindow returns a tmux window by target
func GetTmuxWindow(db *gorm.DB, target string) (*TmuxWindow, error) {
	var window TmuxWindow
	err := db.Where("target = ?", target).First(&window).Error
	if err != nil {
		return nil, err
	}
	return &window, nil
}

// Cache for GetActiveTmuxWindows to reduce database queries
var (
	activeTmuxWindowsCache      []TmuxWindow
	activeTmuxWindowsCacheTime  time.Time
	activeTmuxWindowsCacheMutex sync.RWMutex
)

// GetActiveTmuxWindows returns all active tmux windows with Claude
func GetActiveTmuxWindows(db *gorm.DB) ([]TmuxWindow, error) {
	// Check cache first (valid for 2 seconds)
	activeTmuxWindowsCacheMutex.RLock()
	if time.Since(activeTmuxWindowsCacheTime) < 2*time.Second && activeTmuxWindowsCache != nil {
		// Return cached copy
		cached := make([]TmuxWindow, len(activeTmuxWindowsCache))
		copy(cached, activeTmuxWindowsCache)
		activeTmuxWindowsCacheMutex.RUnlock()
		return cached, nil
	}
	activeTmuxWindowsCacheMutex.RUnlock()

	// Cache miss or expired, query database
	var windows []TmuxWindow
	err := db.Where("active = ? AND has_claude = ?", true, true).
		Order("priority DESC, session_name ASC, window_index ASC").
		Find(&windows).Error

	if err == nil {
		// Update cache
		activeTmuxWindowsCacheMutex.Lock()
		activeTmuxWindowsCache = make([]TmuxWindow, len(windows))
		copy(activeTmuxWindowsCache, windows)
		activeTmuxWindowsCacheTime = time.Now()
		activeTmuxWindowsCacheMutex.Unlock()
	}

	return windows, err
}

// InvalidateActiveTmuxWindowsCache invalidates the cache when windows are modified
func InvalidateActiveTmuxWindowsCache() {
	activeTmuxWindowsCacheMutex.Lock()
	activeTmuxWindowsCache = nil
	activeTmuxWindowsCacheTime = time.Time{}
	activeTmuxWindowsCacheMutex.Unlock()
}

// GetAllActiveTmuxWindows returns all active tmux windows (regardless of Claude status)
func GetAllActiveTmuxWindows(db *gorm.DB) ([]TmuxWindow, error) {
	var windows []TmuxWindow
	err := db.Where("active = ?", true).
		Order("session_name ASC, window_index ASC").
		Find(&windows).Error
	return windows, err
}

// CreateOrUpdateTmuxWindow creates or updates a tmux window entry
func CreateOrUpdateTmuxWindow(db *gorm.DB, sessionName string, windowIndex int, windowName string, hasClaude bool) (*TmuxWindow, error) {
	target := fmt.Sprintf("%s:%d", sessionName, windowIndex)

	var window TmuxWindow
	// Use silent logger for expected "record not found" during window discovery
	err := db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)}).
		Where("target = ?", target).First(&window).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new window
			window = TmuxWindow{
				SessionName: sessionName,
				WindowIndex: windowIndex,
				WindowName:  windowName,
				Target:      target,
				HasClaude:   hasClaude,
				Priority:    5, // default priority
				Active:      true,
				LastSeen:    time.Now(),
			}
			if err := db.Create(&window).Error; err != nil {
				return nil, err
			}
			// Invalidate cache since we added a new window
			InvalidateActiveTmuxWindowsCache()
		} else {
			return nil, err
		}
	} else {
		// Update existing window
		updates := map[string]interface{}{
			"window_name": windowName,
			"has_claude":  hasClaude,
			"last_seen":   time.Now(),
		}
		if err := db.Model(&window).Updates(updates).Error; err != nil {
			return nil, err
		}
		// Invalidate cache since we updated the window
		InvalidateActiveTmuxWindowsCache()
	}

	return &window, nil
}

// GetOrCreateWindowMessageQueue gets or creates a message queue for a window
func GetOrCreateWindowMessageQueue(db *gorm.DB, windowID uint) (*WindowMessageQueue, error) {
	var queue WindowMessageQueue
	// Use silent logger for expected "record not found" during queue creation
	err := db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)}).
		Where("window_id = ?", windowID).Preload("Window").First(&queue).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new queue
			queue = WindowMessageQueue{
				WindowID:     windowID,
				Priority:     5, // default priority
				Active:       true,
				MessageCount: 0,
			}
			if err := db.Create(&queue).Error; err != nil {
				return nil, err
			}
			// Reload with preloaded window (no need for silent logger here since record should exist)
			err = db.Where("window_id = ?", windowID).Preload("Window").First(&queue).Error
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return &queue, nil
}

// GetPendingMessagesForWindow returns pending messages for a specific window
func GetPendingMessagesForWindow(db *gorm.DB, windowID uint, limit int) ([]Message, error) {
	var messages []Message
	query := db.Where("window_id = ? AND status = ? AND scheduled_time <= ?",
		windowID, MessageStatusPending, time.Now()).
		Order("priority DESC, scheduled_time ASC").
		Preload("Window")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// GetPendingMessagesForAllWindows returns all pending messages ordered by window priority and message priority
func GetPendingMessagesForAllWindows(db *gorm.DB, limit int) ([]Message, error) {
	var messages []Message
	query := db.Table("messages").
		Select("messages.*").
		Joins("JOIN tmux_windows ON messages.window_id = tmux_windows.id").
		Joins("JOIN window_message_queues ON tmux_windows.id = window_message_queues.window_id").
		Where("messages.status = ? AND messages.scheduled_time <= ? AND tmux_windows.active = ? AND window_message_queues.active = ?",
			MessageStatusPending, time.Now(), true, true).
		Order("window_message_queues.priority DESC, messages.priority DESC, messages.scheduled_time ASC").
		Preload("Window")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&messages).Error
	return messages, err
}
