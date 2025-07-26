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
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is $HOME/.config/tcs/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be done without executing")

	// Add subcommands
	rootCmd.AddCommand(tuiCmd)
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

// TUI command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Start the terminal user interface",
	Long:  `Launch the interactive TUI dashboard for monitoring and controlling Claude usage.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
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

	// Get all discovered windows
	windows, err := database.GetActiveTmuxWindows(database.GetDB())
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
			// Detect Claude
			hasClaude := false
			content, err := tmuxClient.CapturePane(window.Target, 50)
			if err == nil {
				hasClaude = isClaudeWindow(content)
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
	// Parse schedule time
	var scheduledTime time.Time
	var err error

	switch when {
	case "now":
		scheduledTime = time.Now()
	default:
		// Try to parse as duration (e.g., "+5m")
		if when[0] == '+' {
			duration, err := time.ParseDuration(when[1:])
			if err != nil {
				return fmt.Errorf("invalid duration format: %s", when)
			}
			scheduledTime = time.Now().Add(duration)
		} else {
			// Try to parse as time (e.g., "14:30")
			scheduledTime, err = time.Parse("15:04", when)
			if err != nil {
				return fmt.Errorf("invalid time format: %s (use HH:MM or +duration)", when)
			}
			// If time is in the past, schedule for tomorrow
			if scheduledTime.Before(time.Now()) {
				scheduledTime = scheduledTime.AddDate(0, 0, 1)
			}
		}
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
		fmt.Println("Message will be sent immediately by the scheduler.")
	} else {
		fmt.Printf("Message will be sent in %s\n", time.Until(scheduledTime).Round(time.Second))
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
		homeDir, _ := os.UserConfigDir()
		configPath = fmt.Sprintf("%s/tcs/config.yaml", homeDir)
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

	if priority >= 1 && priority <= 10 {
		updates["priority"] = priority
	}

	if when != "" {
		var scheduledTime time.Time
		switch when {
		case "now":
			scheduledTime = time.Now()
		default:
			// Parse time (reuse logic from runMessageAdd)
			if when[0] == '+' {
				duration, err := time.ParseDuration(when[1:])
				if err != nil {
					return fmt.Errorf("invalid duration format: %s", when)
				}
				scheduledTime = time.Now().Add(duration)
			} else {
				scheduledTime, err = time.Parse("15:04", when)
				if err != nil {
					return fmt.Errorf("invalid time format: %s", when)
				}
				if scheduledTime.Before(time.Now()) {
					scheduledTime = scheduledTime.AddDate(0, 0, 1)
				}
			}
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

// Helper functions
func isClaudeWindow(content string) bool {
	claudeIndicators := []string{
		"claude", "Claude", "anthropic", "Assistant:", "Human:",
		"I'm Claude", "claude-3", "I'm an AI assistant", "Claude Code", "claude-code",
	}

	contentLower := strings.ToLower(content)
	for _, indicator := range claudeIndicators {
		if strings.Contains(contentLower, strings.ToLower(indicator)) {
			return true
		}
	}
	return false
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

// SetVersionInfo sets the version information for the CLI
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d

	// Update the root command with version info
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}
