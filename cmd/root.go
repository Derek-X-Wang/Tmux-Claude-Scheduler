package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/derekxwang/tcs/internal/config"
	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/monitor"
	"github.com/derekxwang/tcs/internal/scheduler"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/tui"
	"github.com/derekxwang/tcs/internal/utils"
)

var (
	configFile string
	verbose    bool
	dryRun     bool

	// Version information
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tcs",
	Short: "TCS (Tmux Claude Scheduler) - Maximize Claude subscription usage",
	Long: `TCS (Tmux Claude Scheduler) is a CLI tool that helps maximize Claude subscription usage
by monitoring 5-hour usage windows, scheduling messages to tmux sessions, and managing
multiple Claude sessions with smart priority-based scheduling.

Features:
- Monitor Claude subscription usage in 5-hour windows
- Auto-discover tmux windows and create message queues
- Schedule messages to tmux windows with smart priority queuing
- Beautiful TUI dashboard for monitoring and control
- Cron-based exact-time scheduling
- Automatic window discovery and health monitoring`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is $HOME/.tcs/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be done without executing")

	// Add subcommands
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(windowCmd)
	rootCmd.AddCommand(queueCmd)
	rootCmd.AddCommand(messageCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(sendCmd)
}

// initConfig reads in config file and ENV variables
func initConfig() {
	// Load configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		// Use defaults if config loading fails
		cfg, _ = config.Load("")
	}

	if verbose {
		fmt.Printf("Loaded configuration from: %s\n", configFile)
		fmt.Printf("Database: %s\n", cfg.Database.Path)
	}
}

// Init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize TCS with complete setup",
	Long: `Initialize TCS by setting up configuration, scanning tmux windows, and preparing the system for use.
This command will:
- Generate default configuration file
- Scan and discover tmux windows with Claude detection
- Initialize the database
- Show initial status and next steps`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

// TUI command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Start the terminal user interface",
	Long:  `Launch the interactive TUI dashboard for monitoring and controlling Claude usage.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

// Daemon command
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run TCS scheduler in daemon mode",
	Long:  `Run the TCS scheduler continuously in the background to process scheduled messages.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon()
	},
}

// Window management commands
var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Manage tmux windows",
	Long:  `List and manage auto-discovered tmux windows with Claude detection.`,
}

var windowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all discovered windows",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWindowList()
	},
}

var windowScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Force scan for new windows",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWindowScan()
	},
}

// Queue management commands
var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage message queues",
	Long:  `View and manage message queues grouped by tmux session.`,
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List message queues grouped by session",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runQueueList()
	},
}

var queueStatusCmd = &cobra.Command{
	Use:   "status [session]",
	Short: "Show queue status for session(s)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := ""
		if len(args) > 0 {
			sessionName = args[0]
		}
		return runQueueStatus(sessionName)
	},
}

// Message scheduling commands
var messageCmd = &cobra.Command{
	Use:   "message",
	Short: "Schedule messages",
	Long:  `Schedule messages to be sent to Claude sessions.`,
}

var messageAddCmd = &cobra.Command{
	Use:   "add <target> <content>",
	Short: "Schedule a message to a window target",
	Long:  `Schedule a message to a tmux window target (format: session:window, e.g., "project:0")`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		priority, _ := cmd.Flags().GetInt("priority")
		when, _ := cmd.Flags().GetString("when")
		return runMessageAdd(args[0], args[1], priority, when)
	},
}

var messageListCmd = &cobra.Command{
	Use:   "list [target]",
	Short: "List scheduled messages",
	Long:  `List all scheduled messages, optionally filtered by target window`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return runMessageList(target)
	},
}

var messageEditCmd = &cobra.Command{
	Use:   "edit <message-id>",
	Short: "Edit a scheduled message",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content, _ := cmd.Flags().GetString("content")
		priority, _ := cmd.Flags().GetInt("priority")
		when, _ := cmd.Flags().GetString("when")
		target, _ := cmd.Flags().GetString("target")
		return runMessageEdit(args[0], content, target, priority, when)
	},
}

var messageDeleteCmd = &cobra.Command{
	Use:   "delete <message-id>",
	Short: "Delete a scheduled message",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMessageDelete(args[0])
	},
}

// Status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show usage and scheduler status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus()
	},
}

// Config commands
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigInit()
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigShow()
	},
}

// Send command for immediate message sending
var sendCmd = &cobra.Command{
	Use:   "send <target> <message>",
	Short: "Send a message immediately to a tmux target",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSend(args[0], args[1])
	},
}

func init() {
	// Window subcommands
	windowCmd.AddCommand(windowListCmd)
	windowCmd.AddCommand(windowScanCmd)

	// Queue subcommands
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueStatusCmd)

	// Message subcommands
	messageCmd.AddCommand(messageAddCmd)
	messageCmd.AddCommand(messageListCmd)
	messageCmd.AddCommand(messageEditCmd)
	messageCmd.AddCommand(messageDeleteCmd)

	// Message flags
	messageAddCmd.Flags().Int("priority", 5, "Message priority (1-10)")
	messageAddCmd.Flags().String("when", "now", "When to send (now, +5m, 14:30, etc.)")

	messageEditCmd.Flags().String("content", "", "New message content")
	messageEditCmd.Flags().String("target", "", "New target window")
	messageEditCmd.Flags().Int("priority", -1, "New priority (1-10)")
	messageEditCmd.Flags().String("when", "", "New schedule time")

	// Config subcommands
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
}

// Implementation functions

func runWindowList() error {
	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Get all discovered windows (both with and without Claude)
	var windows []database.TmuxWindow
	err := database.GetDB().Where("active = ?", true).
		Order("session_name ASC, window_index ASC").
		Find(&windows).Error
	if err != nil {
		return fmt.Errorf("failed to get windows: %w", err)
	}

	if len(windows) == 0 {
		fmt.Println("No windows discovered. Use 'window scan' to force discovery or check tmux is running.")
		return nil
	}

	// Group windows by session for display
	sessionWindows := make(map[string][]database.TmuxWindow)
	for _, window := range windows {
		sessionWindows[window.SessionName] = append(sessionWindows[window.SessionName], window)
	}

	fmt.Printf("Found %d windows across %d sessions:\n\n", len(windows), len(sessionWindows))

	for sessionName, sessionWins := range sessionWindows {
		fmt.Printf("Session: %s (%d windows)\n", sessionName, len(sessionWins))
		for _, window := range sessionWins {
			claudeStatus := "No Claude"
			if window.HasClaude {
				claudeStatus = "Has Claude"
			}

			fmt.Printf("  Window %d: %s (%s)\n", window.WindowIndex, window.WindowName, claudeStatus)
			fmt.Printf("    Target: %s\n", window.Target)
			fmt.Printf("    Priority: %d\n", window.Priority)
			fmt.Printf("    Last Seen: %s\n", window.LastSeen.Format(time.RFC3339))
			if window.LastActivity != nil {
				fmt.Printf("    Last Activity: %s\n", window.LastActivity.Format(time.RFC3339))
			}
		}
		fmt.Println()
	}

	return nil
}

func runWindowScan() error {
	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Import discovery package at the top of file
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsRunning() {
		return fmt.Errorf("tmux server is not running")
	}

	fmt.Println("Scanning for tmux windows...")

	// Perform manual discovery
	sessions, err := tmuxClient.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	windowCount := 0
	claudeCount := 0

	for _, session := range sessions {
		for _, window := range session.Windows {
			// Detect Claude using configured detection method
			hasClaude := false
			cfg := config.Get()
			detectionMethod := cfg.Tmux.ClaudeDetectionMethod
			processNames := cfg.Tmux.ClaudeProcessNames

			switch detectionMethod {
			case "process":
				// Process-based detection only
				processDetected, err := tmuxClient.DetectClaudeProcessWithNames(window.Target, processNames)
				if err == nil && processDetected {
					hasClaude = true
				}
			case "text":
				// Content-based detection only
				content, err := tmuxClient.CapturePane(window.Target, 50)
				if err == nil {
					hasClaude = utils.IsClaudeWindow(content)
				}
			case "both":
				fallthrough
			default:
				// Try process detection first (more reliable for Claude Code)
				processDetected, err := tmuxClient.DetectClaudeProcessWithNames(window.Target, processNames)
				if err == nil && processDetected {
					hasClaude = true
				} else {
					// Fallback to content-based detection
					content, err := tmuxClient.CapturePane(window.Target, 50)
					if err == nil {
						hasClaude = utils.IsClaudeWindow(content)
					}
				}
			}

			// Create or update window
			_, err = database.CreateOrUpdateTmuxWindow(
				database.GetDB(),
				window.SessionName,
				window.WindowIndex,
				window.WindowName,
				hasClaude,
			)
			if err != nil {
				fmt.Printf("Warning: failed to save window %s: %v\n", window.Target, err)
				continue
			}

			windowCount++
			if hasClaude {
				claudeCount++
			}
		}
	}

	fmt.Printf("Scan complete: found %d windows (%d with Claude) across %d sessions\n",
		windowCount, claudeCount, len(sessions))
	return nil
}

func runQueueList() error {
	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Get all windows with their queues
	windows, err := database.GetActiveTmuxWindows(database.GetDB())
	if err != nil {
		return fmt.Errorf("failed to get windows: %w", err)
	}

	if len(windows) == 0 {
		fmt.Println("No active windows found.")
		return nil
	}

	// Group by session as requested
	sessionQueues := make(map[string][]database.TmuxWindow)
	for _, window := range windows {
		sessionQueues[window.SessionName] = append(sessionQueues[window.SessionName], window)
	}

	fmt.Printf("Message Queues by Session:\n\n")

	for sessionName, sessionWins := range sessionQueues {
		fmt.Printf("Session: %s\n", sessionName)

		totalPending := 0
		for _, window := range sessionWins {
			// Get pending message count for this window
			var pendingCount int64
			database.GetDB().Model(&database.Message{}).
				Where("window_id = ? AND status = ?", window.ID, database.MessageStatusPending).
				Count(&pendingCount)

			totalPending += int(pendingCount)

			fmt.Printf("  %s (priority: %d) - %d pending messages\n",
				window.Target, window.Priority, pendingCount)
		}

		fmt.Printf("  Total pending: %d messages\n\n", totalPending)
	}

	return nil
}

func runQueueStatus(sessionName string) error {
	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Get windows, optionally filtered by session
	var windows []database.TmuxWindow
	var err error

	if sessionName != "" {
		err = database.GetDB().Where("session_name = ? AND active = ?", sessionName, true).Find(&windows).Error
	} else {
		windows, err = database.GetActiveTmuxWindows(database.GetDB())
	}

	if err != nil {
		return fmt.Errorf("failed to get windows: %w", err)
	}

	if len(windows) == 0 {
		if sessionName != "" {
			fmt.Printf("No windows found for session '%s'\n", sessionName)
		} else {
			fmt.Println("No active windows found.")
		}
		return nil
	}

	if sessionName != "" {
		fmt.Printf("Queue Status for Session: %s\n\n", sessionName)
	} else {
		fmt.Printf("Queue Status for All Sessions:\n\n")
	}

	// Group by session
	sessionQueues := make(map[string][]database.TmuxWindow)
	for _, window := range windows {
		sessionQueues[window.SessionName] = append(sessionQueues[window.SessionName], window)
	}

	for sessName, sessionWins := range sessionQueues {
		fmt.Printf("%s:\n", sessName)

		for _, window := range sessionWins {
			// Get detailed queue info
			queue, err := database.GetOrCreateWindowMessageQueue(database.GetDB(), window.ID)
			if err != nil {
				fmt.Printf("  %s: Error getting queue: %v\n", window.Target, err)
				continue
			}

			// Get pending messages
			messages, err := database.GetPendingMessagesForWindow(database.GetDB(), window.ID, 0)
			if err != nil {
				fmt.Printf("  %s: Error getting messages: %v\n", window.Target, err)
				continue
			}

			fmt.Printf("  %s:\n", window.Target)
			fmt.Printf("    Queue Priority: %d\n", queue.Priority)
			fmt.Printf("    Pending Messages: %d\n", len(messages))
			fmt.Printf("    Last Processed: %v\n", queue.LastProcessed)
			fmt.Printf("    Has Claude: %t\n", window.HasClaude)

			if len(messages) > 0 {
				fmt.Printf("    Next Messages:\n")
				for i, msg := range messages {
					if i >= 3 { // Show first 3 messages
						fmt.Printf("      ... and %d more\n", len(messages)-3)
						break
					}
					fmt.Printf("      %d. [P:%d] %s (scheduled: %s)\n",
						msg.ID, msg.Priority,
						truncateString(msg.Content, 50),
						msg.ScheduledTime.Format(time.RFC3339))
				}
			}
		}
		fmt.Println()
	}

	return nil
}

func runMessageAdd(target, content string, priority int, when string) error {
	// Validate target format (session:window)
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}
	if !strings.Contains(target, ":") {
		return fmt.Errorf("target must be in format 'session:window' (e.g., 'project:0'), got: %s", target)
	}
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid target format '%s'. Use 'session:window' (e.g., 'project:0')", target)
	}

	// Validate content length (reasonable limit for CLI usage)
	if len(content) == 0 {
		return fmt.Errorf("message content cannot be empty")
	}
	if len(content) > 100000 { // 100KB limit
		return fmt.Errorf("message content too long (%d characters, max 100,000)", len(content))
	}

	// Parse schedule time with comprehensive validation
	scheduledTime, err := parseScheduleTime(when)
	if err != nil {
		return fmt.Errorf("invalid schedule time: %w", err)
	}

	// Initialize components
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(database.GetDB())

	if err := usageMonitor.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize usage monitor: %w", err)
	}

	// Create scheduler
	schedulerInstance := scheduler.NewScheduler(
		database.GetDB(),
		tmuxClient,
		usageMonitor,
		nil,
	)

	if err := schedulerInstance.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize scheduler: %w", err)
	}

	// Schedule message
	message, err := schedulerInstance.ScheduleMessage(target, content, scheduledTime, priority)
	if err != nil {
		return fmt.Errorf("failed to schedule message: %w", err)
	}

	fmt.Printf("Scheduled message (ID: %d) for target '%s' at %s with priority %d\n",
		message.ID, target, scheduledTime.Format(time.RFC3339), priority)

	if when == "now" {
		// For immediate messages, start scheduler temporarily to process them
		fmt.Println("Sending message immediately...")
		if err := schedulerInstance.Start(); err != nil {
			return fmt.Errorf("failed to start scheduler: %w", err)
		}

		// Give the scheduler a moment to initialize and load the queue
		time.Sleep(100 * time.Millisecond)

		// Add the message directly to the scheduler queue for immediate processing
		schedulerInstance.AddMessage(message)

		// Trigger immediate processing
		schedulerInstance.TriggerImmediateProcessing()

		// Wait for scheduler to process the message
		fmt.Print("Processing")
		messageProcessed := false
		for i := 0; i < 10; i++ { // Wait up to 10 seconds for immediate processing
			time.Sleep(500 * time.Millisecond)
			fmt.Print(".")

			// Check if THIS specific message was processed
			var updatedMessage database.Message
			err := database.GetDB().First(&updatedMessage, message.ID).Error
			if err == nil && updatedMessage.Status != database.MessageStatusPending {
				// Our specific message was processed
				if updatedMessage.Status == database.MessageStatusSent {
					messageProcessed = true
				}
				break
			}
		}
		fmt.Println()

		// Stop the scheduler
		if err := schedulerInstance.Stop(); err != nil {
			fmt.Printf("Warning: failed to stop scheduler: %v\n", err)
		}

		if messageProcessed {
			fmt.Println("Message sent!")
		} else {
			fmt.Println("Message queued - run 'tcs tui' or 'tcs daemon' to process pending messages")
		}
	} else {
		fmt.Printf("Message will be sent in %s\n", time.Until(scheduledTime).Round(time.Second))
		fmt.Println("Note: Run 'tcs tui' to start the scheduler and process scheduled messages.")
	}

	return nil
}

func runStatus() error {
	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Get database stats
	stats, err := database.GetStats()
	if err != nil {
		return fmt.Errorf("failed to get database stats: %w", err)
	}

	// Initialize usage monitor for current stats
	usageMonitor := monitor.NewUsageMonitor(database.GetDB())
	if err := usageMonitor.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize usage monitor: %w", err)
	}

	usageStats, err := usageMonitor.GetCurrentStats()
	if err != nil {
		return fmt.Errorf("failed to get usage stats: %w", err)
	}

	// Check tmux connectivity
	tmuxClient := tmux.NewClient()
	tmuxRunning := tmuxClient.IsRunning()

	// Display status
	fmt.Println("Claude Usage Manager Status")
	fmt.Println("===========================")
	fmt.Println()

	fmt.Printf("Database: %s\n", config.Get().Database.Path)
	fmt.Printf("  Windows: %d total, %d active\n", stats.Windows, stats.ActiveWindows)
	fmt.Printf("  Messages: %d total, %d pending\n", stats.Messages, stats.PendingMessages)
	fmt.Printf("  Usage Windows: %d\n", stats.UsageWindows)
	fmt.Println()

	fmt.Printf("Tmux: %s\n", map[bool]string{true: "Connected", false: "Disconnected"}[tmuxRunning])
	if tmuxRunning {
		if info, err := tmuxClient.GetServerInfo(); err == nil {
			fmt.Printf("  Sessions: %v\n", info["total_sessions"])
			fmt.Printf("  Windows: %v\n", info["total_windows"])
		}
	}
	fmt.Println()

	fmt.Println("Current Usage Window:")
	if usageStats.CurrentWindow != nil {
		fmt.Printf("  Messages Used: %d/%d (%.1f%%)\n",
			usageStats.MessagesUsed,
			config.Get().Usage.MaxMessages,
			usageStats.UsagePercentage*100)
		fmt.Printf("  Time Remaining: %s\n", usageStats.TimeRemaining.Round(time.Minute))
		fmt.Printf("  Window: %s to %s\n",
			usageStats.WindowStartTime.Format("15:04:05"),
			usageStats.WindowEndTime.Format("15:04:05"))
		fmt.Printf("  Can Send Messages: %t\n", usageStats.CanSendMessage)
	} else {
		fmt.Println("  No active usage window")
	}

	return nil
}

func runConfigInit() error {
	configPath := configFile
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		configPath = fmt.Sprintf("%s/.tcs/config.yaml", homeDir)
	}

	if err := config.GenerateDefaultConfig(configPath); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	fmt.Printf("Generated default configuration at: %s\n", configPath)
	return nil
}

func runConfigShow() error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("no configuration loaded")
	}

	fmt.Println("Current Configuration:")
	fmt.Println("=====================")
	fmt.Printf("Database Path: %s\n", cfg.Database.Path)
	fmt.Printf("TUI Refresh Rate: %s\n", cfg.TUI.RefreshRate)
	fmt.Printf("Max Messages: %d\n", cfg.Usage.MaxMessages)
	fmt.Printf("Usage Window: %s\n", cfg.Usage.WindowDuration)
	fmt.Printf("Smart Scheduler: %t\n", cfg.Scheduler.SmartEnabled)
	fmt.Printf("Cron Scheduler: %t\n", cfg.Scheduler.CronEnabled)
	fmt.Printf("Log Level: %s\n", cfg.Logging.Level)

	return nil
}

func runSend(target, message string) error {
	// Create tmux client
	tmuxClient := tmux.NewClient()

	// Check if tmux is running
	if !tmuxClient.IsRunning() {
		return fmt.Errorf("tmux server is not running")
	}

	// Validate target
	if err := tmuxClient.ValidateTarget(target); err != nil {
		return fmt.Errorf("invalid target '%s': %w", target, err)
	}

	// Create message sender
	messageSender := tmux.NewMessageSender(tmuxClient)

	// Send message
	fmt.Printf("Sending message to %s: %s\n", target, message)

	result, err := messageSender.SendMessage(target, message)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	if result.Success {
		fmt.Printf("Message sent successfully in %s\n", result.Duration)
	} else {
		fmt.Printf("Message failed: %s\n", result.Error)
	}

	return nil
}

func runMessageList(target string) error {
	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Get messages, optionally filtered by target
	var messages []database.Message
	var err error

	if target != "" {
		// Get specific window
		window, err := database.GetTmuxWindow(database.GetDB(), target)
		if err != nil {
			return fmt.Errorf("target '%s' not found: %w", target, err)
		}

		messages, err = database.GetPendingMessagesForWindow(database.GetDB(), window.ID, 0)
		if err != nil {
			return fmt.Errorf("failed to get messages for target: %w", err)
		}
	} else {
		messages, err = database.GetPendingMessages(database.GetDB(), 0)
		if err != nil {
			return fmt.Errorf("failed to get messages: %w", err)
		}
	}

	if len(messages) == 0 {
		if target != "" {
			fmt.Printf("No pending messages for target '%s'\n", target)
		} else {
			fmt.Println("No pending messages found.")
		}
		return nil
	}

	// Group by session for display as requested
	sessionMessages := make(map[string][]database.Message)
	for _, msg := range messages {
		// Load window info
		err := database.GetDB().Preload("Window").First(&msg, msg.ID).Error
		if err != nil {
			continue
		}
		sessionMessages[msg.Window.SessionName] = append(sessionMessages[msg.Window.SessionName], msg)
	}

	if target != "" {
		fmt.Printf("Pending Messages for Target: %s\n\n", target)
	} else {
		fmt.Printf("All Pending Messages (grouped by session):\n\n")
	}

	for sessionName, sessionMsgs := range sessionMessages {
		fmt.Printf("Session: %s (%d messages)\n", sessionName, len(sessionMsgs))
		for _, msg := range sessionMsgs {
			fmt.Printf("  [%d] Target: %s, Priority: %d\n", msg.ID, msg.Window.Target, msg.Priority)
			fmt.Printf("      Content: %s\n", truncateString(msg.Content, 80))
			fmt.Printf("      Scheduled: %s\n", msg.ScheduledTime.Format(time.RFC3339))
			if msg.Retries > 0 {
				fmt.Printf("      Retries: %d\n", msg.Retries)
			}
		}
		fmt.Println()
	}

	return nil
}

func runMessageEdit(messageIDStr, content, target string, priority int, when string) error {
	// Parse message ID
	messageID, err := strconv.ParseUint(messageIDStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid message ID: %s", messageIDStr)
	}

	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Get existing message
	var message database.Message
	err = database.GetDB().Preload("Window").First(&message, uint(messageID)).Error
	if err != nil {
		return fmt.Errorf("message %d not found: %w", messageID, err)
	}

	// Check if message can be edited (not sent)
	if message.Status == database.MessageStatusSent {
		return fmt.Errorf("cannot edit message %d: already sent", messageID)
	}

	// Prepare updates
	updates := make(map[string]interface{})

	if content != "" {
		updates["content"] = content
	}

	if priority != 0 { // 0 means not specified (default flag value)
		if priority < 1 || priority > 10 {
			return fmt.Errorf("priority must be between 1 and 10, got %d", priority)
		}
		updates["priority"] = priority
	}

	if when != "" {
		scheduledTime, err := parseScheduleTime(when)
		if err != nil {
			return fmt.Errorf("invalid schedule time: %w", err)
		}
		updates["scheduled_time"] = scheduledTime
	}

	if target != "" {
		// Get new target window
		window, err := database.GetTmuxWindow(database.GetDB(), target)
		if err != nil {
			return fmt.Errorf("target '%s' not found: %w", target, err)
		}
		updates["window_id"] = window.ID
	}

	if len(updates) == 0 {
		return fmt.Errorf("no changes specified")
	}

	// Apply updates
	err = database.GetDB().Model(&message).Updates(updates).Error
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	fmt.Printf("Updated message %d\n", messageID)
	return nil
}

func runMessageDelete(messageIDStr string) error {
	// Parse message ID
	messageID, err := strconv.ParseUint(messageIDStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid message ID: %s", messageIDStr)
	}

	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Check if message exists
	var message database.Message
	err = database.GetDB().First(&message, uint(messageID)).Error
	if err != nil {
		return fmt.Errorf("message %d not found: %w", messageID, err)
	}

	// Check if message can be deleted (not sent)
	if message.Status == database.MessageStatusSent {
		return fmt.Errorf("cannot delete message %d: already sent", messageID)
	}

	// Delete message
	err = database.GetDB().Delete(&message).Error
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	fmt.Printf("Deleted message %d\n", messageID)
	return nil
}

// Helper functions (using optimized detection from utils package)

// parseScheduleTime parses time input with comprehensive validation
// Supports: "now", "+duration", "HH:MM", "YYYY-MM-DD HH:MM"
func parseScheduleTime(when string) (time.Time, error) {
	if when == "" {
		return time.Time{}, fmt.Errorf("time cannot be empty")
	}

	switch when {
	case "now":
		return time.Now(), nil
	default:
		// Handle relative time (+duration)
		if len(when) > 1 && when[0] == '+' {
			duration, err := time.ParseDuration(when[1:])
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid duration format '%s': %w (use formats like +1h, +30m, +5s)", when, err)
			}

			// Validate reasonable duration limits
			if duration < 0 {
				return time.Time{}, fmt.Errorf("duration cannot be negative: %s", when)
			}
			if duration > 30*24*time.Hour { // 30 days
				return time.Time{}, fmt.Errorf("duration too large (max 30 days): %s", when)
			}

			return time.Now().Add(duration), nil
		}

		// Try parsing as time in different formats
		now := time.Now()

		// Try HH:MM format first (most common)
		if t, err := time.Parse("15:04", when); err == nil {
			// Combine with today's date
			scheduledTime := time.Date(now.Year(), now.Month(), now.Day(),
				t.Hour(), t.Minute(), 0, 0, now.Location())

			// If time is in the past, schedule for tomorrow
			if scheduledTime.Before(now) {
				scheduledTime = scheduledTime.AddDate(0, 0, 1)
			}
			return scheduledTime, nil
		}

		// Try full datetime format: YYYY-MM-DD HH:MM
		if t, err := time.Parse("2006-01-02 15:04", when); err == nil {
			if t.Before(now) {
				return time.Time{}, fmt.Errorf("scheduled time cannot be in the past: %s", when)
			}
			return t, nil
		}

		// Try date only format: YYYY-MM-DD (schedule for 9 AM)
		if t, err := time.Parse("2006-01-02", when); err == nil {
			scheduledTime := time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, now.Location())
			if scheduledTime.Before(now) {
				return time.Time{}, fmt.Errorf("scheduled date cannot be in the past: %s", when)
			}
			return scheduledTime, nil
		}

		return time.Time{}, fmt.Errorf("invalid time format '%s'. Supported formats: 'now', '+duration', 'HH:MM', 'YYYY-MM-DD HH:MM', 'YYYY-MM-DD'", when)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func runTUI() error {
	return tui.Run()
}

func runDaemon() error {
	fmt.Println("🚀 Starting TCS Scheduler Daemon")
	fmt.Println("=================================")

	// Initialize database
	if err := database.Initialize(nil); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()

	// Initialize components
	tmuxClient := tmux.NewClient()
	usageMonitor := monitor.NewUsageMonitor(database.GetDB())

	if err := usageMonitor.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize usage monitor: %w", err)
	}

	// Create scheduler
	schedulerInstance := scheduler.NewScheduler(
		database.GetDB(),
		tmuxClient,
		usageMonitor,
		nil,
	)

	if err := schedulerInstance.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize scheduler: %w", err)
	}

	// Start the scheduler
	fmt.Println("Starting scheduler...")
	if err := schedulerInstance.Start(); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	fmt.Println("✅ TCS Scheduler is running")
	fmt.Println("Press Ctrl+C to stop")

	// Keep daemon running
	select {}
}

func runInit() error {
	fmt.Println("🚀 Initializing TCS (Tmux Claude Scheduler)")
	fmt.Println("==========================================")
	fmt.Println()

	// Step 1: Generate configuration if it doesn't exist
	fmt.Print("📝 Setting up configuration... ")
	configPath := configFile
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		configPath = fmt.Sprintf("%s/.tcs/config.yaml", homeDir)
	}

	// Check if config already exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := config.GenerateDefaultConfig(configPath); err != nil {
			fmt.Printf("❌ Failed\n")
			return fmt.Errorf("failed to generate config: %w", err)
		}
		fmt.Printf("✅ Created at %s\n", configPath)
	} else {
		fmt.Printf("✅ Already exists at %s\n", configPath)
	}

	// Step 2: Initialize database (now uses correct ~/.tcs path by default)
	fmt.Print("💾 Initializing database... ")
	if err := database.Initialize(nil); err != nil {
		fmt.Printf("❌ Failed\n")
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer database.Close()
	fmt.Printf("✅ Ready\n")

	// Step 3: Check tmux connectivity
	fmt.Print("🖥️  Checking tmux connection... ")
	tmuxClient := tmux.NewClient()
	if !tmuxClient.IsRunning() {
		fmt.Printf("⚠️  Not running\n")
		fmt.Println("   Please start tmux first: tmux new-session -d")
		fmt.Println("   Then run 'tcs init' again to complete setup")
		return nil
	}
	fmt.Printf("✅ Connected\n")

	// Step 4: Scan tmux windows
	fmt.Print("🔍 Scanning tmux windows... ")
	sessions, err := tmuxClient.ListSessions()
	if err != nil {
		fmt.Printf("❌ Failed\n")
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	windowCount := 0
	claudeCount := 0

	for _, session := range sessions {
		for _, window := range session.Windows {
			// Detect Claude
			hasClaude := false
			content, err := tmuxClient.CapturePane(window.Target, 50)
			if err == nil {
				hasClaude = utils.IsClaudeWindow(content)
			}

			// Create or update window
			_, err = database.CreateOrUpdateTmuxWindow(
				database.GetDB(),
				window.SessionName,
				window.WindowIndex,
				window.WindowName,
				hasClaude,
			)
			if err != nil {
				fmt.Printf("❌ Failed to save window %s\n", window.Target)
				continue
			}

			windowCount++
			if hasClaude {
				claudeCount++
			}
		}
	}
	fmt.Printf("✅ Found %d windows (%d with Claude)\n", windowCount, claudeCount)

	// Step 5: Initialize usage monitor
	fmt.Print("⏱️  Setting up usage monitoring... ")
	usageMonitor := monitor.NewUsageMonitor(database.GetDB())
	if err := usageMonitor.Initialize(); err != nil {
		fmt.Printf("❌ Failed\n")
		return fmt.Errorf("failed to initialize usage monitor: %w", err)
	}
	fmt.Printf("✅ Ready\n")

	// Step 6: Show initial status
	fmt.Println()
	fmt.Println("📊 Initial Status")
	fmt.Println("-----------------")

	usageStats, err := usageMonitor.GetCurrentStats()
	if err != nil {
		fmt.Printf("Warning: Could not get usage stats: %v\n", err)
	} else {
		fmt.Printf("Current Usage Window: %s to %s\n",
			usageStats.WindowStartTime.Format("15:04"),
			usageStats.WindowEndTime.Format("15:04"))
		fmt.Printf("Messages Used: %d/%d (%.1f%%)\n",
			usageStats.MessagesUsed,
			config.Get().Usage.MaxMessages,
			usageStats.UsagePercentage*100)
		fmt.Printf("Time Until Reset: %s\n", usageStats.TimeRemaining.Round(time.Minute))
	}

	fmt.Printf("Tmux Sessions: %d\n", len(sessions))
	fmt.Printf("Windows Discovered: %d\n", windowCount)
	fmt.Printf("Claude Windows: %d\n", claudeCount)

	// Step 7: Show next steps
	fmt.Println()
	fmt.Println("🎉 TCS is now ready!")
	fmt.Println("Next steps:")
	fmt.Println("  • Run 'tcs status' to see current usage")
	fmt.Println("  • Run 'tcs tui' for the interactive dashboard")
	fmt.Println("  • Run 'tcs window list' to see discovered windows")
	fmt.Println("  • Use 'tcs message add' to schedule Claude messages")
	fmt.Println()
	fmt.Println("For help: tcs --help")

	return nil
}

// SetVersionInfo sets the version information for the CLI
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d

	// Update the root command with version info
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}
