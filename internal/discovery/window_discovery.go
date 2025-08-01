package discovery

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/config"
	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/utils"
)

// WindowDiscovery manages automatic discovery and tracking of tmux windows
type WindowDiscovery struct {
	db         *gorm.DB
	tmuxClient *tmux.Client
	config     *Config

	// State management
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Statistics
	stats *DiscoveryStats

	// Callbacks
	onWindowDiscovered []func(*database.TmuxWindow)
	onWindowLost       []func(*database.TmuxWindow)
	onError            []func(error)
}

// Config holds window discovery configuration
type Config struct {
	ScanInterval       time.Duration `json:"scan_interval"`        // How often to scan for windows
	InactiveTimeout    time.Duration `json:"inactive_timeout"`     // How long before marking window inactive
	ClaudeDetection    bool          `json:"claude_detection"`     // Enable Claude detection
	PersistDiscovered  bool          `json:"persist_discovered"`   // Save discovered windows to database
	MaxConcurrentScans int           `json:"max_concurrent_scans"` // Limit concurrent session scans
	RetryAttempts      int           `json:"retry_attempts"`       // Retry failed scans
	RetryDelay         time.Duration `json:"retry_delay"`          // Delay between retries
}

// DefaultConfig returns default window discovery configuration
func DefaultConfig() *Config {
	return &Config{
		ScanInterval:       30 * time.Second,
		InactiveTimeout:    5 * time.Minute,
		ClaudeDetection:    true,
		PersistDiscovered:  true,
		MaxConcurrentScans: 5,
		RetryAttempts:      3,
		RetryDelay:         5 * time.Second,
	}
}

// DiscoveryStats tracks discovery performance and statistics
type DiscoveryStats struct {
	TotalScans         int64         `json:"total_scans"`
	WindowsDiscovered  int64         `json:"windows_discovered"`
	WindowsLost        int64         `json:"windows_lost"`
	ClaudeWindowsFound int64         `json:"claude_windows_found"`
	ActiveWindows      int           `json:"active_windows"`
	LastScanDuration   time.Duration `json:"last_scan_duration"`
	LastScanTime       time.Time     `json:"last_scan_time"`
	ErrorCount         int64         `json:"error_count"`
	LastError          string        `json:"last_error,omitempty"`
}

// NewWindowDiscovery creates a new window discovery service
func NewWindowDiscovery(db *gorm.DB, tmuxClient *tmux.Client, config *Config) *WindowDiscovery {
	if config == nil {
		config = DefaultConfig()
	}

	return &WindowDiscovery{
		db:         db,
		tmuxClient: tmuxClient,
		config:     config,
		stats:      &DiscoveryStats{},
	}
}

// Start starts the window discovery service
func (wd *WindowDiscovery) Start() error {
	wd.mu.Lock()
	defer wd.mu.Unlock()

	if wd.running {
		return fmt.Errorf("window discovery is already running")
	}

	wd.ctx, wd.cancel = context.WithCancel(context.Background())
	wd.running = true

	// Start discovery loop
	go wd.discoveryLoop()

	// Start cleanup loop
	go wd.cleanupLoop()

	log.Printf("Window discovery started with scan interval: %v", wd.config.ScanInterval)
	return nil
}

// Stop stops the window discovery service
func (wd *WindowDiscovery) Stop() error {
	wd.mu.Lock()
	defer wd.mu.Unlock()

	if !wd.running {
		return fmt.Errorf("window discovery is not running")
	}

	wd.cancel()
	wd.running = false

	log.Printf("Window discovery stopped")
	return nil
}

// discoveryLoop is the main discovery loop
func (wd *WindowDiscovery) discoveryLoop() {
	ticker := time.NewTicker(wd.config.ScanInterval)
	defer ticker.Stop()

	// Initial scan
	wd.performScan()

	for {
		select {
		case <-wd.ctx.Done():
			return
		case <-ticker.C:
			wd.performScan()
		}
	}
}

// cleanupLoop periodically cleans up inactive windows
func (wd *WindowDiscovery) cleanupLoop() {
	ticker := time.NewTicker(wd.config.InactiveTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-wd.ctx.Done():
			return
		case <-ticker.C:
			wd.cleanupInactiveWindows()
		}
	}
}

// performScan performs a full scan of tmux windows
func (wd *WindowDiscovery) performScan() {
	startTime := time.Now()

	defer func() {
		wd.mu.Lock()
		wd.stats.TotalScans++
		wd.stats.LastScanDuration = time.Since(startTime)
		wd.stats.LastScanTime = time.Now()
		wd.mu.Unlock()
	}()

	// Check if tmux is running
	if !wd.tmuxClient.IsRunning() {
		wd.emitError(fmt.Errorf("tmux server is not running"))
		return
	}

	// Get all sessions
	sessions, err := wd.tmuxClient.ListSessions()
	if err != nil {
		wd.emitError(fmt.Errorf("failed to list tmux sessions: %w", err))
		return
	}

	// Process sessions concurrently
	semaphore := make(chan struct{}, wd.config.MaxConcurrentScans)
	var wg sync.WaitGroup

	for _, session := range sessions {
		select {
		case <-wd.ctx.Done():
			return
		case semaphore <- struct{}{}:
			wg.Add(1)
			go func(sess tmux.SessionInfo) {
				defer wg.Done()
				defer func() { <-semaphore }()
				wd.processSessions(sess)
			}(session)
		}
	}

	wg.Wait()

	log.Printf("Window discovery scan completed in %v, found %d sessions with %d total windows",
		time.Since(startTime), len(sessions), wd.countTotalWindows(sessions))
}

// processSessions processes a single session and its windows
func (wd *WindowDiscovery) processSessions(session tmux.SessionInfo) {
	for _, window := range session.Windows {
		wd.processWindow(window)
	}
}

// processWindow processes a single window
func (wd *WindowDiscovery) processWindow(windowInfo tmux.WindowInfo) {
	log.Printf("Processing window: %s (session: %s, index: %d)",
		windowInfo.Target, windowInfo.SessionName, windowInfo.WindowIndex)

	// Detect if window has Claude (if enabled)
	hasClaude := false
	if wd.config.ClaudeDetection {
		cfg := config.Get()
		detectionMethod := cfg.Tmux.ClaudeDetectionMethod
		processNames := cfg.Tmux.ClaudeProcessNames

		switch detectionMethod {
		case "process":
			// Process-based detection only
			processDetected, err := wd.tmuxClient.DetectClaudeProcessWithNames(windowInfo.Target, processNames)
			if err == nil && processDetected {
				hasClaude = true
			}
		case "text":
			// Content-based detection only
			content, err := wd.tmuxClient.CapturePane(windowInfo.Target, 50)
			if err == nil {
				hasClaude = wd.isClaudeWindow(content)
			}
		case "both":
		default:
			// Try process detection first (more reliable for Claude Code)
			processDetected, err := wd.tmuxClient.DetectClaudeProcessWithNames(windowInfo.Target, processNames)
			if err == nil && processDetected {
				hasClaude = true
			} else {
				// Fallback to content-based detection
				content, err := wd.tmuxClient.CapturePane(windowInfo.Target, 50)
				if err == nil {
					hasClaude = wd.isClaudeWindow(content)
				}
			}
		}

		if hasClaude {
			wd.mu.Lock()
			wd.stats.ClaudeWindowsFound++
			wd.mu.Unlock()
			log.Printf("Window %s detected as Claude window", windowInfo.Target)
		} else {
			log.Printf("Window %s does NOT have Claude detected", windowInfo.Target)
		}
	}

	if !wd.config.PersistDiscovered {
		log.Printf("Not persisting window %s (PersistDiscovered=false)", windowInfo.Target)
		return // Don't persist to database
	}

	// Create or update window in database
	log.Printf("Saving window %s to database", windowInfo.Target)
	dbWindow, err := database.CreateOrUpdateTmuxWindow(
		wd.db,
		windowInfo.SessionName,
		windowInfo.WindowIndex,
		windowInfo.WindowName,
		hasClaude,
	)
	if err != nil {
		log.Printf("ERROR: Failed to save window %s: %v", windowInfo.Target, err)
		wd.emitError(fmt.Errorf("failed to create/update window %s: %w", windowInfo.Target, err))
		return
	}
	log.Printf("Successfully saved window %s to database (ID: %d)", windowInfo.Target, dbWindow.ID)

	// Check if this is a newly discovered window
	isNewWindow := dbWindow.CreatedAt.After(time.Now().Add(-wd.config.ScanInterval * 2))
	if isNewWindow {
		wd.mu.Lock()
		wd.stats.WindowsDiscovered++
		wd.mu.Unlock()

		wd.emitWindowDiscovered(dbWindow)
		log.Printf("Discovered new window: %s (session: %s, has_claude: %v)",
			dbWindow.Target, dbWindow.SessionName, dbWindow.HasClaude)
	}

	// Create message queue if it doesn't exist
	_, err = database.GetOrCreateWindowMessageQueue(wd.db, dbWindow.ID)
	if err != nil {
		wd.emitError(fmt.Errorf("failed to create message queue for window %s: %w", dbWindow.Target, err))
	}
}

// cleanupInactiveWindows marks windows as inactive if they haven't been seen recently
func (wd *WindowDiscovery) cleanupInactiveWindows() {
	cutoff := time.Now().Add(-wd.config.InactiveTimeout)

	// Find windows that haven't been seen recently
	var inactiveWindows []database.TmuxWindow
	err := wd.db.Where("active = ? AND last_seen < ?", true, cutoff).Find(&inactiveWindows).Error
	if err != nil {
		wd.emitError(fmt.Errorf("failed to find inactive windows: %w", err))
		return
	}

	// Mark them as inactive
	for _, window := range inactiveWindows {
		err := wd.db.Model(&window).Update("active", false).Error
		if err != nil {
			wd.emitError(fmt.Errorf("failed to mark window %s as inactive: %w", window.Target, err))
			continue
		}

		wd.mu.Lock()
		wd.stats.WindowsLost++
		wd.mu.Unlock()

		wd.emitWindowLost(&window)
		log.Printf("Marked window as inactive: %s (last seen: %v)", window.Target, window.LastSeen)
	}

	// Update active window count
	var activeCount int64
	wd.db.Model(&database.TmuxWindow{}).Where("active = ?", true).Count(&activeCount)
	wd.mu.Lock()
	wd.stats.ActiveWindows = int(activeCount)
	wd.mu.Unlock()
}

// isClaudeWindow checks if window content indicates a Claude session
func (wd *WindowDiscovery) isClaudeWindow(content string) bool {
	return utils.IsClaudeWindow(content)
}

// countTotalWindows counts total windows across all sessions
func (wd *WindowDiscovery) countTotalWindows(sessions []tmux.SessionInfo) int {
	total := 0
	for _, session := range sessions {
		total += len(session.Windows)
	}
	return total
}

// GetStats returns current discovery statistics
func (wd *WindowDiscovery) GetStats() (*DiscoveryStats, error) {
	wd.mu.RLock()
	defer wd.mu.RUnlock()

	// Create a copy to avoid race conditions
	statsCopy := *wd.stats
	return &statsCopy, nil
}

// GetActiveWindows returns all currently active windows from database
func (wd *WindowDiscovery) GetActiveWindows() ([]database.TmuxWindow, error) {
	return database.GetActiveTmuxWindows(wd.db)
}

// ForceRescan forces an immediate scan
func (wd *WindowDiscovery) ForceRescan() error {
	if !wd.IsRunning() {
		return fmt.Errorf("window discovery is not running")
	}

	go wd.performScan()
	return nil
}

// IsRunning returns whether the service is running
func (wd *WindowDiscovery) IsRunning() bool {
	wd.mu.RLock()
	defer wd.mu.RUnlock()
	return wd.running
}

// Event callback management

// OnWindowDiscovered adds a callback for when a new window is discovered
func (wd *WindowDiscovery) OnWindowDiscovered(callback func(*database.TmuxWindow)) {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	wd.onWindowDiscovered = append(wd.onWindowDiscovered, callback)
}

// OnWindowLost adds a callback for when a window becomes inactive
func (wd *WindowDiscovery) OnWindowLost(callback func(*database.TmuxWindow)) {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	wd.onWindowLost = append(wd.onWindowLost, callback)
}

// OnError adds a callback for error events
func (wd *WindowDiscovery) OnError(callback func(error)) {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	wd.onError = append(wd.onError, callback)
}

// emitWindowDiscovered emits a window discovered event
func (wd *WindowDiscovery) emitWindowDiscovered(window *database.TmuxWindow) {
	for _, callback := range wd.onWindowDiscovered {
		go callback(window)
	}
}

// emitWindowLost emits a window lost event
func (wd *WindowDiscovery) emitWindowLost(window *database.TmuxWindow) {
	for _, callback := range wd.onWindowLost {
		go callback(window)
	}
}

// emitError emits an error event
func (wd *WindowDiscovery) emitError(err error) {
	wd.mu.Lock()
	wd.stats.ErrorCount++
	wd.stats.LastError = err.Error()
	wd.mu.Unlock()

	for _, callback := range wd.onError {
		go callback(err)
	}
}
