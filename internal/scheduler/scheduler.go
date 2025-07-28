package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/monitor"
	"github.com/derekxwang/tcs/internal/tmux"
)

// Scheduler coordinates message scheduling across different strategies
type Scheduler struct {
	db             *gorm.DB
	tmuxClient     *tmux.Client
	messageSender  *tmux.MessageSender
	usageMonitor   *monitor.UsageMonitor
	smartScheduler *SmartScheduler
	cronScheduler  *CronScheduler

	// Configuration
	config *Config

	// State management
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Statistics
	stats *SchedulerStats

	// Callbacks
	messageCallbacks []func(*MessageEvent)
	errorCallbacks   []func(error)
}

// Config holds scheduler configuration
type Config struct {
	SmartSchedulerEnabled bool          `json:"smart_scheduler_enabled"`
	CronSchedulerEnabled  bool          `json:"cron_scheduler_enabled"`
	ProcessingInterval    time.Duration `json:"processing_interval"`
	MaxConcurrentMessages int           `json:"max_concurrent_messages"`
	RetryAttempts         int           `json:"retry_attempts"`
	RetryDelay            time.Duration `json:"retry_delay"`
	HealthCheckInterval   time.Duration `json:"health_check_interval"`
	StatsUpdateInterval   time.Duration `json:"stats_update_interval"`
}

// DefaultConfig returns default scheduler configuration
func DefaultConfig() *Config {
	return &Config{
		SmartSchedulerEnabled: true,
		CronSchedulerEnabled:  true,
		ProcessingInterval:    10 * time.Second,
		MaxConcurrentMessages: 3,
		RetryAttempts:         3,
		RetryDelay:            30 * time.Second,
		HealthCheckInterval:   60 * time.Second,
		StatsUpdateInterval:   30 * time.Second,
	}
}

// MessageEvent represents events in message processing
type MessageEvent struct {
	Type      string               `json:"type"` // queued, processing, sent, failed, retrying
	Message   *database.Message    `json:"message"`
	Window    *database.TmuxWindow `json:"window"`
	Result    *tmux.SendResult     `json:"result,omitempty"`
	Error     error                `json:"error,omitempty"`
	Timestamp time.Time            `json:"timestamp"`
	Attempt   int                  `json:"attempt"`
}

// SchedulerStats tracks scheduler performance
type SchedulerStats struct {
	TotalProcessed   int64         `json:"total_processed"`
	TotalSent        int64         `json:"total_sent"`
	TotalFailed      int64         `json:"total_failed"`
	TotalRetries     int64         `json:"total_retries"`
	QueueSize        int           `json:"queue_size"`
	ProcessingRate   float64       `json:"processing_rate"` // messages per minute
	SuccessRate      float64       `json:"success_rate"`
	AverageDelay     time.Duration `json:"average_delay"`
	LastProcessed    time.Time     `json:"last_processed"`
	ActiveSchedulers []string      `json:"active_schedulers"`
	Uptime           time.Duration `json:"uptime"`
}

// NewScheduler creates a new scheduler
func NewScheduler(
	db *gorm.DB,
	tmuxClient *tmux.Client,
	usageMonitor *monitor.UsageMonitor,
	config *Config,
) *Scheduler {
	if config == nil {
		config = DefaultConfig()
	}

	messageSender := tmux.NewMessageSender(tmuxClient)
	smartScheduler := NewSmartScheduler(db, messageSender, usageMonitor)
	cronScheduler := NewCronScheduler(db, messageSender)

	return &Scheduler{
		db:             db,
		tmuxClient:     tmuxClient,
		messageSender:  messageSender,
		usageMonitor:   usageMonitor,
		smartScheduler: smartScheduler,
		cronScheduler:  cronScheduler,
		config:         config,
		stats:          &SchedulerStats{},
	}
}

// Initialize initializes the scheduler
func (s *Scheduler) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize sub-schedulers
	if s.config.SmartSchedulerEnabled {
		if err := s.smartScheduler.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize smart scheduler: %w", err)
		}
	}

	if s.config.CronSchedulerEnabled {
		if err := s.cronScheduler.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize cron scheduler: %w", err)
		}
	}

	// Initialize stats
	s.stats.ActiveSchedulers = []string{}
	if s.config.SmartSchedulerEnabled {
		s.stats.ActiveSchedulers = append(s.stats.ActiveSchedulers, "smart")
	}
	if s.config.CronSchedulerEnabled {
		s.stats.ActiveSchedulers = append(s.stats.ActiveSchedulers, "cron")
	}

	log.Printf("Scheduler initialized with active schedulers: %v", s.stats.ActiveSchedulers)
	return nil
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.running = true
	s.stats.LastProcessed = time.Now()

	// Start sub-schedulers
	if s.config.SmartSchedulerEnabled {
		if err := s.smartScheduler.Start(); err != nil {
			return fmt.Errorf("failed to start smart scheduler: %w", err)
		}
	}

	if s.config.CronSchedulerEnabled {
		if err := s.cronScheduler.Start(); err != nil {
			return fmt.Errorf("failed to start cron scheduler: %w", err)
		}
	}

	// Start main processing loop
	go s.processingLoop()

	// Start health check loop
	go s.healthCheckLoop()

	// Start stats update loop
	go s.statsUpdateLoop()

	log.Printf("Scheduler started with processing interval: %v", s.config.ProcessingInterval)
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("scheduler is not running")
	}

	// Cancel context to stop all loops
	s.cancel()
	s.running = false

	// Stop sub-schedulers
	if s.config.SmartSchedulerEnabled {
		if err := s.smartScheduler.Stop(); err != nil {
			log.Printf("Error stopping smart scheduler: %v", err)
		}
	}

	if s.config.CronSchedulerEnabled {
		if err := s.cronScheduler.Stop(); err != nil {
			log.Printf("Error stopping cron scheduler: %v", err)
		}
	}

	log.Printf("Scheduler stopped")
	return nil
}

// ScheduleMessage schedules a message for delivery to a tmux window target
func (s *Scheduler) ScheduleMessage(target, content string, scheduledTime time.Time, priority int) (*database.Message, error) {
	// Get or create tmux window
	window, err := database.GetTmuxWindow(s.db, target)
	if err != nil {
		// If window doesn't exist, try to discover it
		if err == gorm.ErrRecordNotFound {
			// Parse target to get session name and window index
			parts := strings.Split(target, ":")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid target format '%s', expected 'session:window'", target)
			}

			sessionName := parts[0]
			windowIndex := 0
			if idx, parseErr := strconv.Atoi(parts[1]); parseErr == nil {
				windowIndex = idx
			}

			// Create window entry (will be updated by discovery later)
			window, err = database.CreateOrUpdateTmuxWindow(s.db, sessionName, windowIndex, "", false)
			if err != nil {
				return nil, fmt.Errorf("failed to create window entry: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to get window: %w", err)
		}
	}

	// Create message
	message := &database.Message{
		WindowID:      window.ID,
		Content:       content,
		ScheduledTime: scheduledTime,
		Priority:      priority,
		Status:        database.MessageStatusPending,
	}

	if err := s.db.Create(message).Error; err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	// Load window relationship
	if err := s.db.Preload("Window").First(message, message.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load message with window: %w", err)
	}

	// Emit event
	s.emitMessageEvent("queued", message, nil, nil, 0)

	log.Printf("Scheduled message for target '%s' at %v with priority %d",
		target, scheduledTime, priority)

	return message, nil
}

// ScheduleMessageNow schedules a message for immediate delivery
func (s *Scheduler) ScheduleMessageNow(target, content string, priority int) (*database.Message, error) {
	return s.ScheduleMessage(target, content, time.Now(), priority)
}

// ScheduleMessageWithCron schedules a message using a cron expression
func (s *Scheduler) ScheduleMessageWithCron(target, content, cronExpr string, priority int) error {
	if !s.config.CronSchedulerEnabled {
		return fmt.Errorf("cron scheduler is disabled")
	}

	return s.cronScheduler.ScheduleMessage(target, content, cronExpr, priority)
}

// processingLoop is the main message processing loop
func (s *Scheduler) processingLoop() {
	ticker := time.NewTicker(s.config.ProcessingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processMessages()
		}
	}
}

// processMessages processes pending messages
func (s *Scheduler) processMessages() {
	// Check if we have available usage
	availableUsage := s.usageMonitor.GetAvailableUsage()
	if availableUsage <= 0 {
		return // No usage available
	}

	// Get pending messages for all windows
	limit := min(s.config.MaxConcurrentMessages, availableUsage)
	messages, err := database.GetPendingMessagesForAllWindows(s.db, limit)
	if err != nil {
		s.emitError(fmt.Errorf("failed to get pending messages: %w", err))
		return
	}

	if len(messages) == 0 {
		return // No messages to process
	}

	// Process messages concurrently
	semaphore := make(chan struct{}, s.config.MaxConcurrentMessages)
	var wg sync.WaitGroup

	for _, message := range messages {
		select {
		case <-s.ctx.Done():
			return
		case semaphore <- struct{}{}:
			wg.Add(1)
			go func(msg database.Message) {
				defer wg.Done()
				defer func() { <-semaphore }()
				s.processMessage(&msg)
			}(message)
		}
	}

	wg.Wait()
}

// processMessage processes a single message
func (s *Scheduler) processMessage(message *database.Message) {
	s.emitMessageEvent("processing", message, nil, nil, message.Retries+1)

	// Send the message to the window target
	result, err := s.messageSender.SendQueuedMessage(
		message.Window.Target,
		message.Content,
		message.Priority,
	)

	// Update statistics
	s.updateMessageStats(result, err)

	if err != nil {
		s.handleMessageFailure(message, err)
		return
	}

	// Success - update message status
	if err := database.UpdateMessageStatus(s.db, message.ID, database.MessageStatusSent, ""); err != nil {
		s.emitError(fmt.Errorf("failed to update message status: %w", err))
		return
	}

	// Record usage for the window
	if err := s.usageMonitor.RecordMessageSent(message.WindowID, 0); err != nil {
		s.emitError(fmt.Errorf("failed to record usage: %w", err))
	}

	// Update window activity
	if err := s.updateWindowActivity(message.Window.ID); err != nil {
		log.Printf("Warning: failed to update window activity: %v", err)
	}

	s.emitMessageEvent("sent", message, result, nil, message.Retries+1)
	s.stats.TotalSent++
	s.stats.TotalProcessed++
}

// updateWindowActivity updates the last activity time for a window
func (s *Scheduler) updateWindowActivity(windowID uint) error {
	now := time.Now()
	return s.db.Model(&database.TmuxWindow{}).
		Where("id = ?", windowID).
		Update("last_activity", &now).Error
}

// handleMessageFailure handles a failed message
func (s *Scheduler) handleMessageFailure(message *database.Message, err error) {
	s.stats.TotalFailed++
	s.stats.TotalProcessed++

	// Check if we can retry
	if message.CanRetry() {
		// Schedule retry
		retryTime := time.Now().Add(s.config.RetryDelay)
		if err := s.db.Model(message).Updates(map[string]interface{}{
			"scheduled_time": retryTime,
			"retries":        message.Retries + 1,
			"error":          err.Error(),
		}).Error; err != nil {
			s.emitError(fmt.Errorf("failed to schedule retry: %w", err))
			return
		}

		s.emitMessageEvent("retrying", message, nil, err, message.Retries+1)
		s.stats.TotalRetries++
		log.Printf("Scheduled retry for message %d (attempt %d/%d)",
			message.ID, message.Retries+1, message.MaxRetries)
	} else {
		// Mark as permanently failed
		if dbErr := database.UpdateMessageStatus(s.db, message.ID, database.MessageStatusFailed, err.Error()); dbErr != nil {
			s.emitError(fmt.Errorf("failed to mark message as failed: %w", dbErr))
			return
		}

		s.emitMessageEvent("failed", message, nil, err, message.Retries+1)
		log.Printf("Message %d permanently failed after %d attempts: %v",
			message.ID, message.Retries, err)
	}
}

// healthCheckLoop performs periodic health checks
func (s *Scheduler) healthCheckLoop() {
	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.performHealthCheck()
		}
	}
}

// performHealthCheck performs a health check
func (s *Scheduler) performHealthCheck() {
	// Check tmux connectivity
	if !s.tmuxClient.IsRunning() {
		s.emitError(fmt.Errorf("tmux server is not running"))
		return
	}

	// Check database connectivity
	if err := database.Health(); err != nil {
		s.emitError(fmt.Errorf("database health check failed: %w", err))
		return
	}

	// Check usage monitor
	if _, err := s.usageMonitor.GetCurrentStats(); err != nil {
		s.emitError(fmt.Errorf("usage monitor health check failed: %w", err))
		return
	}

	// Log health status
	stats, _ := s.GetStats()
	log.Printf("Scheduler health check: %d messages in queue, %.2f%% success rate",
		stats.QueueSize, stats.SuccessRate*100)
}

// statsUpdateLoop updates statistics periodically
func (s *Scheduler) statsUpdateLoop() {
	startTime := time.Now()
	ticker := time.NewTicker(s.config.StatsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateStats(startTime)
		}
	}
}

// updateStats updates scheduler statistics
func (s *Scheduler) updateStats(startTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get queue size
	var queueSize int64
	s.db.Model(&database.Message{}).Where("status = ?", database.MessageStatusPending).Count(&queueSize)
	s.stats.QueueSize = int(queueSize)

	// Calculate success rate
	total := s.stats.TotalProcessed
	if total > 0 {
		s.stats.SuccessRate = float64(s.stats.TotalSent) / float64(total)
	}

	// Calculate processing rate (messages per minute)
	elapsed := time.Since(startTime).Minutes()
	if elapsed > 0 {
		s.stats.ProcessingRate = float64(s.stats.TotalProcessed) / elapsed
	}

	// Update uptime
	s.stats.Uptime = time.Since(startTime)
}

// updateMessageStats updates statistics from message sending
func (s *Scheduler) updateMessageStats(result *tmux.SendResult, err error) {
	if result != nil {
		s.stats.AverageDelay = (s.stats.AverageDelay + result.Duration) / 2
	}
	if err != nil {
		s.stats.TotalFailed++
	}
}

// AddMessage adds a message directly to the scheduler queue
func (s *Scheduler) AddMessage(message *database.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.smartScheduler != nil {
		s.smartScheduler.AddMessage(message)
	}
}

// TriggerImmediateProcessing triggers immediate message processing
func (s *Scheduler) TriggerImmediateProcessing() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.smartScheduler != nil {
		s.smartScheduler.TriggerImmediateProcessing()
	}
}

// GetStats returns current scheduler statistics
func (s *Scheduler) GetStats() (*SchedulerStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy to avoid race conditions
	statsCopy := *s.stats
	return &statsCopy, nil
}

// GetQueueSize returns the current queue size
func (s *Scheduler) GetQueueSize() (int, error) {
	var count int64
	err := s.db.Model(&database.Message{}).Where("status = ?", database.MessageStatusPending).Count(&count).Error
	return int(count), err
}

// GetPendingMessages returns pending messages
func (s *Scheduler) GetPendingMessages(limit int) ([]database.Message, error) {
	return database.GetPendingMessages(s.db, limit)
}

// AddMessageCallback adds a callback for message events
func (s *Scheduler) AddMessageCallback(callback func(*MessageEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageCallbacks = append(s.messageCallbacks, callback)
}

// AddErrorCallback adds a callback for error events
func (s *Scheduler) AddErrorCallback(callback func(error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorCallbacks = append(s.errorCallbacks, callback)
}

// emitMessageEvent emits a message event
func (s *Scheduler) emitMessageEvent(eventType string, message *database.Message, result *tmux.SendResult, err error, attempt int) {
	event := &MessageEvent{
		Type:      eventType,
		Message:   message,
		Window:    &message.Window, // Window is already loaded via foreign key
		Result:    result,
		Error:     err,
		Timestamp: time.Now(),
		Attempt:   attempt,
	}

	for _, callback := range s.messageCallbacks {
		go callback(event)
	}
}

// emitError emits an error event
func (s *Scheduler) emitError(err error) {
	for _, callback := range s.errorCallbacks {
		go callback(err)
	}
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
