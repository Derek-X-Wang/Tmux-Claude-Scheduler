package monitor

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/derekxwang/tcs/internal/claude"
	"github.com/derekxwang/tcs/internal/config"
	"github.com/derekxwang/tcs/internal/database"
)

// UsageMonitor tracks Claude subscription usage across 5-hour windows
type UsageMonitor struct {
	db              *gorm.DB
	claudeReader    *claude.ClaudeDataReader
	mu              sync.RWMutex
	currentWindow   *database.UsageWindow
	lastWindowCheck time.Time
	usageCallbacks  []func(*UsageStats)
	windowCallbacks []func(*database.UsageWindow)
	maxMessages     int     // Maximum messages per 5-hour window
	maxTokens       int     // Maximum tokens per 5-hour window (if available)
	maxCost         float64 // Maximum cost per 5-hour window
	dynamicLimits   bool    // Whether to use dynamic P90-based limits

	// Stats calculation state (protected by mu)
	statsInProgress bool
	cachedStats     *UsageStats
	cacheTime       time.Time
}

// UsageStats represents current usage statistics
type UsageStats struct {
	CurrentWindow   *database.UsageWindow `json:"current_window"`
	MessagesUsed    int                   `json:"messages_used"`
	TokensUsed      int                   `json:"tokens_used"`
	CostUsed        float64               `json:"cost_used"`
	WindowsActive   int                   `json:"windows_active"`
	TimeRemaining   time.Duration         `json:"time_remaining"`
	UsagePercentage float64               `json:"usage_percentage"`
	CanSendMessage  bool                  `json:"can_send_message"`
	WindowStartTime time.Time             `json:"window_start_time"`
	WindowEndTime   time.Time             `json:"window_end_time"`
	LastActivity    *time.Time            `json:"last_activity"`
	EstimatedReset  time.Time             `json:"estimated_reset"`
	MessageLimit    int                   `json:"message_limit"`
	TokenLimit      int                   `json:"token_limit"`
	CostLimit       float64               `json:"cost_limit"`
	DynamicLimits   bool                  `json:"dynamic_limits"`
}

// NewUsageMonitor creates a new usage monitor
func NewUsageMonitor(db *gorm.DB) *UsageMonitor {
	return &UsageMonitor{
		db:              db,
		claudeReader:    claude.NewClaudeDataReader(),
		lastWindowCheck: time.Now(),
		maxMessages:     1000,   // Default limit, overridden by dynamic limits
		maxTokens:       100000, // Default limit, overridden by dynamic limits
		maxCost:         50.0,   // Default cost limit, overridden by dynamic limits
		dynamicLimits:   true,   // Enable P90-based dynamic limits by default
	}
}

// Initialize initializes the usage monitor
func (um *UsageMonitor) Initialize() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	// Update limits with dynamic P90-based calculations if enabled
	if um.dynamicLimits {
		if err := um.updateDynamicLimits(); err != nil {
			log.Printf("Warning: Failed to update dynamic limits, using defaults: %v", err)
		}
	}

	// Load or create current usage window
	if err := um.loadCurrentWindow(); err != nil {
		return fmt.Errorf("failed to load current window: %w", err)
	}

	log.Printf("Usage monitor initialized with window: %v to %v (limits: %d msgs, %d tokens, $%.2f)",
		um.currentWindow.StartTime, um.currentWindow.EndTime,
		um.maxMessages, um.maxTokens, um.maxCost)

	return nil
}

// updateDynamicLimits updates usage limits based on historical P90 calculations
func (um *UsageMonitor) updateDynamicLimits() error {
	messageLimit, tokenLimit, costLimit, err := um.claudeReader.GetDynamicLimits()
	if err != nil {
		return fmt.Errorf("failed to get dynamic limits: %w", err)
	}

	// Update the limits
	um.maxMessages = messageLimit
	um.maxTokens = tokenLimit
	um.maxCost = costLimit

	log.Printf("Updated dynamic limits: %d messages, %d tokens, $%.2f cost (P90-based)",
		messageLimit, tokenLimit, costLimit)

	return nil
}

// loadCurrentWindow loads the current active usage window (dynamic 5-hour windows)
func (um *UsageMonitor) loadCurrentWindow() error {
	// Try to get active Claude session from actual data
	_, windowStart, windowEnd, err := um.claudeReader.GetCurrentWindowEntries(0)
	if err != nil {
		log.Printf("Warning: Could not read Claude data: %v", err)
		// Fallback to database window if Claude data unavailable
		return um.loadCurrentWindowFromDB()
	}

	// Try to find existing window that matches current time period
	var window database.UsageWindow
	// Use Session to silence "record not found" log for this query
	err = um.db.Session(&gorm.Session{Logger: um.db.Logger.LogMode(logger.Silent)}).
		Where("start_time = ? AND end_time = ?", windowStart, windowEnd).First(&window).Error

	if err == gorm.ErrRecordNotFound {
		// Create new window based on Claude's actual session
		return um.createClaudeBasedWindow(windowStart, windowEnd)
	} else if err != nil {
		return fmt.Errorf("error querying current window: %w", err)
	}

	// Update window as active and deactivate others
	if err := um.db.Model(&database.UsageWindow{}).Where("id != ?", window.ID).Update("active", false).Error; err != nil {
		log.Printf("Warning: failed to deactivate old windows: %v", err)
	}
	window.Active = true
	if err := um.db.Save(&window).Error; err != nil {
		log.Printf("Warning: failed to activate current window: %v", err)
	}

	um.currentWindow = &window
	return nil
}

// loadCurrentWindowFromDB is a fallback when Claude data is not available
func (um *UsageMonitor) loadCurrentWindowFromDB() error {
	var window database.UsageWindow
	err := um.db.Where("active = ? AND start_time <= ? AND end_time >= ?",
		true, time.Now(), time.Now()).First(&window).Error

	if err == gorm.ErrRecordNotFound {
		return um.createNewWindow()
	} else if err != nil {
		return fmt.Errorf("error querying current window: %w", err)
	}

	um.currentWindow = &window
	return nil
}

// createClaudeBasedWindow creates a new window based on Claude's actual session
func (um *UsageMonitor) createClaudeBasedWindow(windowStart, windowEnd time.Time) error {
	// Deactivate any existing windows
	if err := um.db.Model(&database.UsageWindow{}).
		Where("active = ?", true).
		Update("active", false).Error; err != nil {
		return fmt.Errorf("failed to deactivate old windows: %w", err)
	}

	// Get actual message count from Claude data (using dynamic window detection)
	messageCount, _, _, err := um.claudeReader.CountMessagesInWindow(0)
	if err != nil {
		log.Printf("Warning: Could not get message count from Claude data: %v", err)
		messageCount = 0 // Fallback to 0
	}

	// Create new window with actual Claude data
	window := &database.UsageWindow{
		StartTime:     windowStart,
		EndTime:       windowEnd,
		TotalMessages: messageCount,
		TotalTokens:   0, // Token counting could be added later
		Active:        true,
	}

	if err := um.db.Create(window).Error; err != nil {
		return fmt.Errorf("failed to create new window: %w", err)
	}

	um.currentWindow = window
	log.Printf("Created Claude-based usage window: %v to %v with %d messages",
		window.StartTime.Format("15:04"), window.EndTime.Format("15:04"), messageCount)

	// Notify callbacks about new window
	for _, callback := range um.windowCallbacks {
		go callback(window)
	}

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

// GetCurrentStats returns current usage statistics using real Claude data
func (um *UsageMonitor) GetCurrentStats() (*UsageStats, error) {
	um.mu.Lock()

	// Check if calculation is already in progress and return cached stats if fresh
	if um.statsInProgress {
		if um.cachedStats != nil && time.Since(um.cacheTime) < 2*time.Second {
			cachedCopy := *um.cachedStats
			um.mu.Unlock()
			return &cachedCopy, nil
		}
		// Cache too old, return error to avoid blocking
		um.mu.Unlock()
		return nil, fmt.Errorf("stats calculation in progress")
	}

	// Mark calculation as in progress while holding the lock
	um.statsInProgress = true

	// Check if we have a current window
	if um.currentWindow == nil {
		um.statsInProgress = false
		um.mu.Unlock()
		return nil, fmt.Errorf("no current window available")
	}

	// Get configuration
	cfg := config.Get()
	if cfg != nil && !um.dynamicLimits {
		// Only use config limits if dynamic limits are disabled
		um.maxMessages = cfg.Usage.MaxMessages
	} else if um.dynamicLimits {
		// Update dynamic limits periodically (every 10 minutes)
		if time.Since(um.lastWindowCheck) > 10*time.Minute {
			if err := um.updateDynamicLimits(); err != nil {
				log.Printf("Warning: Failed to update dynamic limits: %v", err)
			}
			um.lastWindowCheck = time.Now()
		}
	}

	// Get real Claude usage data for current window (dynamic session detection)
	var actualMessageCount int
	var actualTokenCount int
	var actualCost float64
	var windowStart, windowEnd time.Time

	// Get current window entries for detailed analysis
	entries, start, end, err := um.claudeReader.GetCurrentWindowEntries(0)
	if err != nil {
		log.Printf("Warning: Could not get real-time Claude data: %v", err)
		// Fallback to database values
		actualMessageCount = um.currentWindow.TotalMessages
		actualTokenCount = um.currentWindow.TotalTokens
		actualCost = 0.0 // Cost calculation needs Claude data
		windowStart = um.currentWindow.StartTime
		windowEnd = um.currentWindow.EndTime
	} else {
		actualMessageCount = len(entries)
		windowStart = start
		windowEnd = end

		// Calculate tokens and estimated cost from entries
		totalTokens := 0
		totalCost := 0.0
		for _, entry := range entries {
			totalTokens += entry.InputTokens + entry.OutputTokens
			// Estimate cost (rough approximation - $3 per million tokens for Sonnet)
			totalCost += float64(entry.InputTokens+entry.OutputTokens) * 3.0 / 1000000.0
		}
		actualTokenCount = totalTokens
		actualCost = totalCost

		// Update database with real data if significantly different
		if abs(actualMessageCount-um.currentWindow.TotalMessages) > 0 ||
			abs(actualTokenCount-um.currentWindow.TotalTokens) > 1000 {
			um.currentWindow.TotalMessages = actualMessageCount
			um.currentWindow.TotalTokens = actualTokenCount
			um.currentWindow.StartTime = windowStart
			um.currentWindow.EndTime = windowEnd
			if err := um.db.Save(um.currentWindow).Error; err != nil {
				log.Printf("Warning: failed to update window with real data: %v", err)
			}
		}
	}

	// Check if window has expired (based on real Claude schedule)
	now := time.Now()
	if now.After(windowEnd) {
		// Window expired, load new one
		if err := um.loadCurrentWindow(); err != nil {
			log.Printf("Warning: failed to load new window: %v", err)
		}
	}

	// Get active windows count
	var activeWindows int64
	um.db.Model(&database.TmuxWindow{}).Where("active = ? AND has_claude = ?", true, true).Count(&activeWindows)

	// Calculate usage percentage based on the most restrictive metric
	messagePercentage := 0.0
	tokenPercentage := 0.0
	costPercentage := 0.0

	if um.maxMessages > 0 {
		messagePercentage = float64(actualMessageCount) / float64(um.maxMessages)
	}
	if um.maxTokens > 0 {
		tokenPercentage = float64(actualTokenCount) / float64(um.maxTokens)
	}
	if um.maxCost > 0 {
		costPercentage = actualCost / um.maxCost
	}

	// Use the highest percentage as the primary usage metric
	usagePercentage := messagePercentage
	if tokenPercentage > usagePercentage {
		usagePercentage = tokenPercentage
	}
	if costPercentage > usagePercentage {
		usagePercentage = costPercentage
	}

	// Get last activity from windows
	var lastActivity *time.Time
	var windows []database.TmuxWindow
	// Use Limit(1).Find() to avoid "record not found" logs when no records exist
	err = um.db.Where("active = ? AND has_claude = ?", true, true).
		Order("last_activity DESC").
		Limit(1).
		Find(&windows).Error
	if err == nil && len(windows) > 0 && windows[0].LastActivity != nil {
		lastActivity = windows[0].LastActivity
	}

	// Calculate time remaining until next reset
	timeRemaining := time.Until(windowEnd)
	if timeRemaining < 0 {
		timeRemaining = 0
	}

	stats := &UsageStats{
		CurrentWindow:   um.currentWindow,
		MessagesUsed:    actualMessageCount,
		TokensUsed:      actualTokenCount,
		CostUsed:        actualCost,
		WindowsActive:   int(activeWindows),
		TimeRemaining:   timeRemaining,
		UsagePercentage: usagePercentage,
		CanSendMessage:  actualMessageCount < um.maxMessages && actualTokenCount < um.maxTokens && actualCost < um.maxCost,
		WindowStartTime: windowStart,
		WindowEndTime:   windowEnd,
		LastActivity:    lastActivity,
		EstimatedReset:  windowEnd,
		MessageLimit:    um.maxMessages,
		TokenLimit:      um.maxTokens,
		CostLimit:       um.maxCost,
		DynamicLimits:   um.dynamicLimits,
	}

	// Cache the stats for concurrent requests
	um.cachedStats = stats
	um.cacheTime = time.Now()

	// Clear the in-progress flag before returning
	um.statsInProgress = false
	um.mu.Unlock()

	return stats, nil
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
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
	um.dynamicLimits = false // Disable dynamic limits when manually setting

	log.Printf("Updated usage limits: %d messages, %d tokens (dynamic limits disabled)", maxMessages, maxTokens)
}

// SetDynamicLimits enables or disables dynamic P90-based limits
func (um *UsageMonitor) SetDynamicLimits(enabled bool) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.dynamicLimits = enabled

	if enabled {
		// Update limits immediately
		if err := um.updateDynamicLimits(); err != nil {
			return fmt.Errorf("failed to update dynamic limits: %w", err)
		}
		log.Printf("Dynamic P90-based limits enabled: %d messages, %d tokens, $%.2f",
			um.maxMessages, um.maxTokens, um.maxCost)
	} else {
		log.Printf("Dynamic limits disabled, using static limits: %d messages, %d tokens, $%.2f",
			um.maxMessages, um.maxTokens, um.maxCost)
	}

	return nil
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
