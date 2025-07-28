package scheduler

import (
	"container/heap"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/monitor"
	"github.com/derekxwang/tcs/internal/tmux"
)

// SmartScheduler implements priority-based message scheduling
type SmartScheduler struct {
	db            *gorm.DB
	messageSender *tmux.MessageSender
	usageMonitor  *monitor.UsageMonitor

	// Priority queue
	queue *MessageQueue
	mu    sync.RWMutex

	// State
	running     bool
	stopChan    chan bool
	triggerChan chan bool // Channel to trigger immediate processing
}

// MessageQueueItem represents an item in the priority queue
type MessageQueueItem struct {
	Message  *database.Message `json:"message"`
	Priority int               `json:"priority"`
	Index    int               `json:"index"` // heap index
}

// MessageQueue implements a priority queue for messages
type MessageQueue []*MessageQueueItem

// Implement heap.Interface
func (mq MessageQueue) Len() int { return len(mq) }

func (mq MessageQueue) Less(i, j int) bool {
	// Higher priority first, then earlier scheduled time
	if mq[i].Priority != mq[j].Priority {
		return mq[i].Priority > mq[j].Priority
	}
	return mq[i].Message.ScheduledTime.Before(mq[j].Message.ScheduledTime)
}

func (mq MessageQueue) Swap(i, j int) {
	mq[i], mq[j] = mq[j], mq[i]
	mq[i].Index = i
	mq[j].Index = j
}

func (mq *MessageQueue) Push(x interface{}) {
	n := len(*mq)
	item, ok := x.(*MessageQueueItem)
	if !ok {
		panic("MessageQueue.Push: invalid type, expected *MessageQueueItem")
	}
	item.Index = n
	*mq = append(*mq, item)
}

func (mq *MessageQueue) Pop() interface{} {
	old := *mq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*mq = old[0 : n-1]
	return item
}

// NewSmartScheduler creates a new smart scheduler
func NewSmartScheduler(
	db *gorm.DB,
	messageSender *tmux.MessageSender,
	usageMonitor *monitor.UsageMonitor,
) *SmartScheduler {
	queue := &MessageQueue{}
	heap.Init(queue)

	return &SmartScheduler{
		db:            db,
		messageSender: messageSender,
		usageMonitor:  usageMonitor,
		queue:         queue,
		stopChan:      make(chan bool),
		triggerChan:   make(chan bool, 1), // Buffered channel to avoid blocking
	}
}

// Initialize initializes the smart scheduler
func (ss *SmartScheduler) Initialize() error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Load pending messages into queue
	return ss.loadPendingMessages()
}

// loadPendingMessages loads pending messages from database into queue
func (ss *SmartScheduler) loadPendingMessages() error {
	messages, err := database.GetPendingMessages(ss.db, 0) // Get all pending
	if err != nil {
		return fmt.Errorf("failed to load pending messages: %w", err)
	}

	for _, message := range messages {
		messageCopy := message // Avoid pointer issues
		item := &MessageQueueItem{
			Message:  &messageCopy,
			Priority: messageCopy.Priority,
		}
		heap.Push(ss.queue, item)
	}

	log.Printf("Loaded %d pending messages into smart scheduler queue", len(messages))
	return nil
}

// Start starts the smart scheduler
func (ss *SmartScheduler) Start() error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.running {
		return fmt.Errorf("smart scheduler already running")
	}

	ss.running = true
	go ss.processQueue()

	log.Printf("Smart scheduler started")
	return nil
}

// Stop stops the smart scheduler
func (ss *SmartScheduler) Stop() error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if !ss.running {
		return fmt.Errorf("smart scheduler not running")
	}

	ss.running = false
	close(ss.stopChan)

	log.Printf("Smart scheduler stopped")
	return nil
}

// processQueue processes the priority queue
func (ss *SmartScheduler) processQueue() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ss.stopChan:
			return
		case <-ticker.C:
			ss.processNextMessage()
		case <-ss.triggerChan:
			// Immediate processing triggered
			ss.processNextMessage()
		}
	}
}

// processNextMessage processes the next highest priority message
func (ss *SmartScheduler) processNextMessage() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Check if we have available usage
	availableUsage := ss.usageMonitor.GetAvailableUsage()
	if availableUsage <= 0 {
		return // No usage available
	}

	// Check if queue has messages
	if ss.queue.Len() == 0 {
		// Reload from database
		if err := ss.loadPendingMessages(); err != nil {
			log.Printf("Error reloading messages: %v", err)
		}
		return
	}

	// Get highest priority message
	popped := heap.Pop(ss.queue)
	item, ok := popped.(*MessageQueueItem)
	if !ok {
		log.Printf("Error: invalid type from heap.Pop, expected *MessageQueueItem")
		return
	}
	message := item.Message

	// Check if message is ready to be sent
	if time.Now().Before(message.ScheduledTime) {
		// Put it back and wait
		heap.Push(ss.queue, item)
		return
	}

	// Ensure message has a valid window
	if message.WindowID == 0 {
		log.Printf("Message %d has no window assigned, skipping", message.ID)
		return
	}

	// Verify window is active and has Claude
	var window database.TmuxWindow
	if err := ss.db.First(&window, message.WindowID).Error; err != nil {
		log.Printf("Window %d not found for message %d: %v", message.WindowID, message.ID, err)
		return
	}

	if !window.Active {
		log.Printf("Message %d rejected: Window %s is not active", message.ID, window.Target)
		return
	}

	if !window.HasClaude {
		log.Printf("Message %d rejected: No Claude detected in window %s (try running 'tcs window scan' if Claude Code is running)", message.ID, window.Target)
		return
	}

	// Send the message
	go ss.sendMessage(message)
}

// sendMessage sends a message asynchronously
func (ss *SmartScheduler) sendMessage(message *database.Message) {
	// Get window information
	var window database.TmuxWindow
	if err := ss.db.First(&window, message.WindowID).Error; err != nil {
		log.Printf("Error loading window for message %d: %v", message.ID, err)
		return
	}

	// Send the message to the window
	result, err := ss.messageSender.SendQueuedMessage(
		window.Target,
		message.Content,
		message.Priority,
	)

	if err != nil {
		// Handle failure
		errorMsg := err.Error()
		if result != nil {
			errorMsg = result.Error
		}

		if message.CanRetry() {
			// Schedule retry
			retryTime := time.Now().Add(30 * time.Second)
			updates := map[string]interface{}{
				"scheduled_time": retryTime,
				"retries":        message.Retries + 1,
				"error":          errorMsg,
			}
			if err := ss.db.Model(message).Updates(updates).Error; err != nil {
				log.Printf("Error scheduling retry: %v", err)
			} else {
				// Add back to queue with lower priority
				ss.mu.Lock()
				item := &MessageQueueItem{
					Message:  message,
					Priority: max(1, message.Priority-1),
				}
				heap.Push(ss.queue, item)
				ss.mu.Unlock()
			}
		} else {
			// Mark as failed
			if err := database.UpdateMessageStatus(ss.db, message.ID, database.MessageStatusFailed, errorMsg); err != nil {
				log.Printf("Error marking message as failed: %v", err)
			}
		}

		log.Printf("Message %d failed: %v", message.ID, err)
		return
	}

	// Success
	if err := database.UpdateMessageStatus(ss.db, message.ID, database.MessageStatusSent, ""); err != nil {
		log.Printf("Error updating message status: %v", err)
		return
	}

	// Record usage tracking for window
	if err := ss.usageMonitor.RecordMessageSent(message.WindowID, 0); err != nil {
		log.Printf("Error recording usage: %v", err)
	}

	// Update window activity
	now := time.Now()
	if err := ss.db.Model(&window).Update("last_activity", &now).Error; err != nil {
		log.Printf("Warning: failed to update window activity: %v", err)
	}

	log.Printf("Successfully sent message %d to window '%s'", message.ID, window.Target)
}

// AddMessage adds a message to the priority queue
func (ss *SmartScheduler) AddMessage(message *database.Message) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	item := &MessageQueueItem{
		Message:  message,
		Priority: message.Priority,
	}
	heap.Push(ss.queue, item)

	// Trigger processing if running
	if ss.running {
		go ss.TriggerImmediateProcessing()
	}
}

// TriggerImmediateProcessing triggers immediate message processing
func (ss *SmartScheduler) TriggerImmediateProcessing() {
	select {
	case ss.triggerChan <- true:
		// Trigger sent successfully
	default:
		// Channel is full (trigger already pending), skip
	}
}

// GetQueueStatus returns current queue status
func (ss *SmartScheduler) GetQueueStatus() map[string]interface{} {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	// Count messages by priority
	priorityCounts := make(map[int]int)
	for _, item := range *ss.queue {
		priorityCounts[item.Priority]++
	}

	return map[string]interface{}{
		"queue_size":      ss.queue.Len(),
		"priority_counts": priorityCounts,
		"running":         ss.running,
	}
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
