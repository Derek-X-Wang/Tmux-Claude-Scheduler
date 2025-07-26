package monitor

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
)

// UsageMonitor tracks Claude subscription usage across 5-hour windows
type UsageMonitor struct {
	db              *gorm.DB
	mu              sync.RWMutex
	currentWindow   *database.UsageWindow
	lastWindowCheck time.Time
	usageCallbacks  []func(*UsageStats)
	windowCallbacks []func(*database.UsageWindow)
	maxMessages     int // Maximum messages per 5-hour window
	maxTokens       int // Maximum tokens per 5-hour window (if available)
}

// UsageStats represents current usage statistics
type UsageStats struct {
	CurrentWindow   *database.UsageWindow `json:"current_window"`
	MessagesUsed    int                   `json:"messages_used"`
	TokensUsed      int                   `json:"tokens_used"`
	WindowsActive   int                   `json:"windows_active"`
	TimeRemaining   time.Duration         `json:"time_remaining"`
	UsagePercentage float64               `json:"usage_percentage"`
	CanSendMessage  bool                  `json:"can_send_message"`
	WindowStartTime time.Time             `json:"window_start_time"`
	WindowEndTime   time.Time             `json:"window_end_time"`
	LastActivity    *time.Time            `json:"last_activity"`
	EstimatedReset  time.Time             `json:"estimated_reset"`
}

// NewUsageMonitor creates a new usage monitor
func NewUsageMonitor(db *gorm.DB) *UsageMonitor {
	return &UsageMonitor{
		db:              db,
		lastWindowCheck: time.Now(),
		maxMessages:     1000,   // Default limit, should be configurable
		maxTokens:       100000, // Default limit, should be configurable
	}
}

// Initialize initializes the usage monitor
func (um *UsageMonitor) Initialize() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	// Load or create current usage window
	if err := um.loadCurrentWindow(); err != nil {
		return fmt.Errorf("failed to load current window: %w", err)
	}

	log.Printf("Usage monitor initialized with window: %v to %v",
		um.currentWindow.StartTime, um.currentWindow.EndTime)

	return nil
}

// loadCurrentWindow loads the current active usage window or creates a new one
func (um *UsageMonitor) loadCurrentWindow() error {
	// Try to find current active window
	var window database.UsageWindow
	err := um.db.Where("active = ? AND start_time <= ? AND end_time >= ?",
		true, time.Now(), time.Now()).First(&window).Error

	if err == gorm.ErrRecordNotFound {
		// No active window found, create a new one
		return um.createNewWindow()
	} else if err != nil {
		return fmt.Errorf("error querying current window: %w", err)
	}

	um.currentWindow = &window
	return nil
}

// createNewWindow creates a new 5-hour usage window
func (um *UsageMonitor) createNewWindow() error {
	// Deactivate any existing windows
	if err := um.db.Model(&database.UsageWindow{}).
		Where("active = ?", true).
		Update("active", false).Error; err != nil {
		return fmt.Errorf("failed to deactivate old windows: %w", err)
	}

	// Create new window
	now := time.Now()
	window := &database.UsageWindow{
		StartTime: now,
		EndTime:   now.Add(5 * time.Hour),
		Active:    true,
	}

	if err := um.db.Create(window).Error; err != nil {
		return fmt.Errorf("failed to create new window: %w", err)
	}

	um.currentWindow = window
	log.Printf("Created new 5-hour usage window: %v to %v", window.StartTime, window.EndTime)

	// Notify callbacks about new window
	for _, callback := range um.windowCallbacks {
		go callback(window)
	}

	return nil
}

// GetCurrentStats returns current usage statistics
func (um *UsageMonitor) GetCurrentStats() (*UsageStats, error) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.currentWindow == nil {
		return nil, fmt.Errorf("no current window available")
	}

	// Check if window has expired
	if um.currentWindow.IsExpired() {
		if err := um.createNewWindow(); err != nil {
			return nil, fmt.Errorf("failed to create new window: %w", err)
		}
	}

	// Get active windows count
	var activeWindows int64
	um.db.Model(&database.TmuxWindow{}).Where("active = ? AND has_claude = ?", true, true).Count(&activeWindows)

	// Calculate usage percentage based on messages (primary metric)
	usagePercentage := 0.0
	if um.maxMessages > 0 {
		usagePercentage = float64(um.currentWindow.TotalMessages) / float64(um.maxMessages)
	}

	// Get last activity from windows
	var lastActivity *time.Time
	var latestWindow database.TmuxWindow
	err := um.db.Where("active = ? AND has_claude = ?", true, true).
		Order("last_activity DESC").
		First(&latestWindow).Error
	if err == nil && latestWindow.LastActivity != nil {
		lastActivity = latestWindow.LastActivity
	}

	// Estimate next reset time (5 hours from current window start)
	estimatedReset := um.currentWindow.EndTime

	stats := &UsageStats{
		CurrentWindow:   um.currentWindow,
		MessagesUsed:    um.currentWindow.TotalMessages,
		TokensUsed:      um.currentWindow.TotalTokens,
		WindowsActive:   int(activeWindows),
		TimeRemaining:   um.currentWindow.TimeRemaining(),
		UsagePercentage: usagePercentage,
		CanSendMessage:  um.currentWindow.TotalMessages < um.maxMessages,
		WindowStartTime: um.currentWindow.StartTime,
		WindowEndTime:   um.currentWindow.EndTime,
		LastActivity:    lastActivity,
		EstimatedReset:  estimatedReset,
	}

	return stats, nil
}

// RecordMessageSent records that a message was sent in the current window
// This is the CRITICAL function that tracks when Claude windows start counting
func (um *UsageMonitor) RecordMessageSent(windowID uint, estimatedTokens int) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.currentWindow == nil {
		if err := um.loadCurrentWindow(); err != nil {
			return fmt.Errorf("failed to load current window: %w", err)
		}
	}

	// Update window last activity
	var window database.TmuxWindow
	if err := um.db.First(&window, windowID).Error; err != nil {
		return fmt.Errorf("window not found: %w", err)
	}

	// Update window activity
	now := time.Now()
	window.LastActivity = &now

	// If this is the very first message in the window, this starts the 5-hour timer
	if um.currentWindow.TotalMessages == 0 {
		log.Printf("FIRST MESSAGE in window - 5-hour timer starts now!")
		um.currentWindow.StartTime = now
		um.currentWindow.EndTime = now.Add(5 * time.Hour)
	}

	// Update window in database
	if err := um.db.Save(&window).Error; err != nil {
		return fmt.Errorf("failed to update window: %w", err)
	}

	// Update usage window
	um.currentWindow.TotalMessages++
	if estimatedTokens > 0 {
		um.currentWindow.TotalTokens += estimatedTokens
	}

	// Track window usage (simplified - no longer counting sessions)
	// The window count is managed separately in the WindowMessageQueue

	// Save updated window
	if err := um.db.Save(um.currentWindow).Error; err != nil {
		return fmt.Errorf("failed to update usage window: %w", err)
	}

	// Check if we're approaching limits
	if um.currentWindow.TotalMessages >= int(float64(um.maxMessages)*0.9) {
		log.Printf("WARNING: Approaching message limit (%d/%d)",
			um.currentWindow.TotalMessages, um.maxMessages)
	}

	log.Printf("Recorded message for window %d. Usage total: %d messages, %d tokens",
		windowID, um.currentWindow.TotalMessages, um.currentWindow.TotalTokens)

	// Notify callbacks (outside the lock to avoid deadlock)
	go func() {
		stats, err := um.GetCurrentStats()
		if err != nil {
			return
		}
		for _, callback := range um.usageCallbacks {
			go callback(stats)
		}
	}()

	return nil
}

// GetAvailableUsage returns how many messages can still be sent
func (um *UsageMonitor) GetAvailableUsage() int {
	um.mu.RLock()
	defer um.mu.RUnlock()

	if um.currentWindow == nil {
		return 0
	}

	if um.currentWindow.IsExpired() {
		return um.maxMessages // New window available
	}

	remaining := um.maxMessages - um.currentWindow.TotalMessages
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetTimeUntilReset returns time until usage window resets
func (um *UsageMonitor) GetTimeUntilReset() time.Duration {
	um.mu.RLock()
	defer um.mu.RUnlock()

	if um.currentWindow == nil {
		return 0
	}

	if um.currentWindow.IsExpired() {
		return 0
	}

	return um.currentWindow.TimeRemaining()
}

// SetLimits sets the usage limits for the monitor
func (um *UsageMonitor) SetLimits(maxMessages, maxTokens int) {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.maxMessages = maxMessages
	um.maxTokens = maxTokens

	log.Printf("Updated usage limits: %d messages, %d tokens", maxMessages, maxTokens)
}

// AddUsageCallback adds a callback for usage updates
func (um *UsageMonitor) AddUsageCallback(callback func(*UsageStats)) {
	um.mu.Lock()
	defer um.mu.Unlock()
	um.usageCallbacks = append(um.usageCallbacks, callback)
}

// AddWindowCallback adds a callback for new window events
func (um *UsageMonitor) AddWindowCallback(callback func(*database.UsageWindow)) {
	um.mu.Lock()
	defer um.mu.Unlock()
	um.windowCallbacks = append(um.windowCallbacks, callback)
}

// GetHistoricalUsage returns usage statistics for past windows
func (um *UsageMonitor) GetHistoricalUsage(days int) ([]database.UsageWindow, error) {
	cutoff := time.Now().AddDate(0, 0, -days)

	var windows []database.UsageWindow
	err := um.db.Where("start_time >= ?", cutoff).
		Order("start_time DESC").
		Find(&windows).Error

	return windows, err
}

// PredictUsage predicts when the current window will be exhausted
func (um *UsageMonitor) PredictUsage() (*UsagePrediction, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	if um.currentWindow == nil {
		return nil, fmt.Errorf("no current window")
	}

	// Get usage rate from the last hour
	oneHourAgo := time.Now().Add(-time.Hour)
	var recentMessages int64
	err := um.db.Model(&database.Message{}).
		Where("sent_at >= ? AND sent_at IS NOT NULL", oneHourAgo).
		Count(&recentMessages).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get recent message count: %w", err)
	}

	// Calculate messages per hour
	messagesPerHour := float64(recentMessages)
	if messagesPerHour == 0 {
		messagesPerHour = 1 // Minimum rate to avoid division by zero
	}

	// Predict when limit will be reached
	remaining := um.maxMessages - um.currentWindow.TotalMessages
	hoursToLimit := float64(remaining) / messagesPerHour

	prediction := &UsagePrediction{
		CurrentUsage:       um.currentWindow.TotalMessages,
		Limit:              um.maxMessages,
		Remaining:          remaining,
		MessagesPerHour:    messagesPerHour,
		EstimatedDepletion: time.Now().Add(time.Duration(hoursToLimit * float64(time.Hour))),
		WindowEnds:         um.currentWindow.EndTime,
		RiskLevel:          calculateRiskLevel(float64(um.currentWindow.TotalMessages), float64(um.maxMessages)),
	}

	return prediction, nil
}

// UsagePrediction represents usage prediction data
type UsagePrediction struct {
	CurrentUsage       int       `json:"current_usage"`
	Limit              int       `json:"limit"`
	Remaining          int       `json:"remaining"`
	MessagesPerHour    float64   `json:"messages_per_hour"`
	EstimatedDepletion time.Time `json:"estimated_depletion"`
	WindowEnds         time.Time `json:"window_ends"`
	RiskLevel          string    `json:"risk_level"`
}

// calculateRiskLevel determines the risk level based on usage
func calculateRiskLevel(current, max float64) string {
	percentage := current / max

	switch {
	case percentage >= 0.9:
		return "CRITICAL"
	case percentage >= 0.7:
		return "HIGH"
	case percentage >= 0.5:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// ForceNewWindow forces creation of a new usage window (for testing/admin)
func (um *UsageMonitor) ForceNewWindow() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	return um.createNewWindow()
}

// GetCurrentWindow returns the current usage window
func (um *UsageMonitor) GetCurrentWindow() *database.UsageWindow {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.currentWindow
}

// StartMonitoring starts the background monitoring routine
func (um *UsageMonitor) StartMonitoring(interval time.Duration) {
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			// Check if window has expired and create new one if needed
			um.mu.RLock()
			windowExpired := um.currentWindow != nil && um.currentWindow.IsExpired()
			um.mu.RUnlock()

			if windowExpired {
				if err := func() error {
					um.mu.Lock()
					defer um.mu.Unlock()
					return um.createNewWindow()
				}(); err != nil {
					log.Printf("Error creating new usage window: %v", err)
				}
			}

			// Update statistics and notify callbacks
			if stats, err := um.GetCurrentStats(); err == nil {
				for _, callback := range um.usageCallbacks {
					go callback(stats)
				}
			}
		}
	}()

	log.Printf("Started usage monitoring with %v interval", interval)
}

// Cleanup performs maintenance on usage data
func (um *UsageMonitor) Cleanup(olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)

	// Delete old inactive windows
	result := um.db.Where("active = false AND created_at < ?", cutoff).
		Delete(&database.UsageWindow{})

	if result.Error != nil {
		return fmt.Errorf("failed to cleanup old windows: %w", result.Error)
	}

	log.Printf("Cleaned up %d old usage windows", result.RowsAffected)
	return nil
}
