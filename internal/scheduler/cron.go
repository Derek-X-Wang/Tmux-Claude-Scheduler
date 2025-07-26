package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	cron "github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/tmux"
)

// CronScheduler handles exact-time scheduling using cron expressions
type CronScheduler struct {
	db            *gorm.DB
	messageSender *tmux.MessageSender

	// Cron instance
	cron *cron.Cron
	mu   sync.RWMutex

	// State
	running bool
	jobs    map[string]cron.EntryID // job name -> cron entry ID
}

// CronJob represents a scheduled cron job
type CronJob struct {
	ID          string     `json:"id"`
	SessionName string     `json:"session_name"`
	Content     string     `json:"content"`
	CronExpr    string     `json:"cron_expr"`
	Priority    int        `json:"priority"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	LastRun     *time.Time `json:"last_run"`
	NextRun     *time.Time `json:"next_run"`
	RunCount    int        `json:"run_count"`
}

// NewCronScheduler creates a new cron scheduler
func NewCronScheduler(
	db *gorm.DB,
	messageSender *tmux.MessageSender,
) *CronScheduler {
	// Create cron with seconds support
	c := cron.New(cron.WithSeconds())

	return &CronScheduler{
		db:            db,
		messageSender: messageSender,
		cron:          c,
		jobs:          make(map[string]cron.EntryID),
	}
}

// Initialize initializes the cron scheduler
func (cs *CronScheduler) Initialize() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Load existing cron jobs from database would go here
	// For now, we'll start with an empty scheduler

	log.Printf("Cron scheduler initialized")
	return nil
}

// Start starts the cron scheduler
func (cs *CronScheduler) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return fmt.Errorf("cron scheduler already running")
	}

	cs.cron.Start()
	cs.running = true

	log.Printf("Cron scheduler started")
	return nil
}

// Stop stops the cron scheduler
func (cs *CronScheduler) Stop() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return fmt.Errorf("cron scheduler not running")
	}

	ctx := cs.cron.Stop()
	<-ctx.Done() // Wait for all jobs to complete
	cs.running = false

	log.Printf("Cron scheduler stopped")
	return nil
}

// ScheduleMessage schedules a message using a cron expression
func (cs *CronScheduler) ScheduleMessage(target, content, cronExpr string, priority int) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	jobID := fmt.Sprintf("%s-%d", target, time.Now().Unix())

	// Create the job function
	jobFunc := func() {
		cs.executeCronJob(jobID, target, content, priority)
	}

	// Add to cron
	entryID, err := cs.cron.AddFunc(cronExpr, jobFunc)
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	// Store job mapping
	cs.jobs[jobID] = entryID

	log.Printf("Scheduled cron job '%s' for target '%s' with expression '%s'",
		jobID, target, cronExpr)

	return nil
}

// executeCronJob executes a cron job
func (cs *CronScheduler) executeCronJob(jobID, target, content string, priority int) {
	log.Printf("Executing cron job '%s' for target '%s'", jobID, target)

	// Get or create window
	window, err := database.GetTmuxWindow(cs.db, target)
	if err != nil {
		log.Printf("Cron job '%s' failed: target '%s' not found: %v", jobID, target, err)
		return
	}

	// Create message record
	message := &database.Message{
		WindowID:      window.ID,
		Content:       content,
		ScheduledTime: time.Now(),
		Priority:      priority,
		Status:        database.MessageStatusPending,
	}

	if err := cs.db.Create(message).Error; err != nil {
		log.Printf("Cron job '%s' failed: could not create message: %v", jobID, err)
		return
	}

	// Load window relationship
	if err := cs.db.Preload("Window").First(message, message.ID).Error; err != nil {
		log.Printf("Cron job '%s' failed: could not load message: %v", jobID, err)
		return
	}

	// Send the message immediately (cron jobs are exact-time)
	result, err := cs.messageSender.SendQueuedMessage(
		window.Target,
		content,
		priority,
	)

	if err != nil {
		// Mark as failed
		errorMsg := err.Error()
		if result != nil {
			errorMsg = result.Error
		}

		if dbErr := database.UpdateMessageStatus(cs.db, message.ID, database.MessageStatusFailed, errorMsg); dbErr != nil {
			log.Printf("Error updating message status: %v", dbErr)
		}

		log.Printf("Cron job '%s' failed to send message: %v", jobID, err)
		return
	}

	// Success
	if err := database.UpdateMessageStatus(cs.db, message.ID, database.MessageStatusSent, ""); err != nil {
		log.Printf("Error updating message status: %v", err)
	}

	// Update window activity
	if err := cs.updateWindowActivity(window.ID); err != nil {
		log.Printf("Warning: failed to update window activity: %v", err)
	}

	log.Printf("Cron job '%s' successfully executed", jobID)
}

// RemoveJob removes a cron job
func (cs *CronScheduler) RemoveJob(jobID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	entryID, exists := cs.jobs[jobID]
	if !exists {
		return fmt.Errorf("job '%s' not found", jobID)
	}

	cs.cron.Remove(entryID)
	delete(cs.jobs, jobID)

	log.Printf("Removed cron job '%s'", jobID)
	return nil
}

// ListJobs returns all active cron jobs
func (cs *CronScheduler) ListJobs() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	jobs := make([]string, 0, len(cs.jobs))
	for jobID := range cs.jobs {
		jobs = append(jobs, jobID)
	}

	return jobs
}

// GetStatus returns the status of the cron scheduler
func (cs *CronScheduler) GetStatus() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	entries := cs.cron.Entries()
	nextRuns := make([]time.Time, len(entries))
	for i, entry := range entries {
		nextRuns[i] = entry.Next
	}

	return map[string]interface{}{
		"running":   cs.running,
		"job_count": len(cs.jobs),
		"next_runs": nextRuns,
	}
}

// AddRecurringMessage adds a recurring message with cron expression
func (cs *CronScheduler) AddRecurringMessage(target, content, cronExpr string, priority int) (string, error) {
	// Validate cron expression
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return "", fmt.Errorf("invalid cron expression '%s': %w", cronExpr, err)
	}

	// Validate window exists or can be created
	_, err = database.GetTmuxWindow(cs.db, target)
	if err != nil {
		return "", fmt.Errorf("target '%s' not found: %w", target, err)
	}

	// Generate job ID
	jobID := fmt.Sprintf("recurring-%s-%d", target, time.Now().Unix())

	// Create job function
	jobFunc := func() {
		cs.executeCronJob(jobID, target, content, priority)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Add to cron
	entryID, err := cs.cron.AddFunc(cronExpr, jobFunc)
	if err != nil {
		return "", fmt.Errorf("failed to add recurring job: %w", err)
	}

	// Store job mapping
	cs.jobs[jobID] = entryID

	nextRun := schedule.Next(time.Now())
	log.Printf("Added recurring job '%s' for target '%s', next run: %v",
		jobID, target, nextRun)

	return jobID, nil
}

// IsRunning returns whether the cron scheduler is running
func (cs *CronScheduler) IsRunning() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.running
}

// updateWindowActivity updates the last activity time for a window
func (cs *CronScheduler) updateWindowActivity(windowID uint) error {
	now := time.Now()
	return cs.db.Model(&database.TmuxWindow{}).
		Where("id = ?", windowID).
		Update("last_activity", &now).Error
}
