package types

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/derekxwang/tcs/internal/database"
)

// TUIModel represents the base interface for all TUI models
type TUIModel interface {
	tea.Model
	SetSize(width, height int)
	Refresh() tea.Cmd
}

// UsageStats represents current usage statistics for display
type UsageStats struct {
	MessagesUsed    int                   `json:"messages_used"`
	MessagesLimit   int                   `json:"messages_limit"`
	TokensUsed      int                   `json:"tokens_used"`
	TokensLimit     int                   `json:"tokens_limit"`
	UsagePercentage float64               `json:"usage_percentage"`
	TimeRemaining   time.Duration         `json:"time_remaining"`
	WindowStartTime time.Time             `json:"window_start_time"`
	WindowEndTime   time.Time             `json:"window_end_time"`
	CanSendMessage  bool                  `json:"can_send_message"`
	CurrentWindow   *database.UsageWindow `json:"current_window"`
}

// SessionDisplayInfo represents session information for display
type SessionDisplayInfo struct {
	ID            uint       `json:"id"`
	Name          string     `json:"name"`
	TmuxTarget    string     `json:"tmux_target"`
	Priority      int        `json:"priority"`
	Active        bool       `json:"active"`
	MessageCount  int        `json:"message_count"`
	TokensUsed    int        `json:"tokens_used"`
	LastActivity  *time.Time `json:"last_activity"`
	StartTime     *time.Time `json:"start_time"`
	EndTime       *time.Time `json:"end_time"`
	Status        string     `json:"status"` // "active", "inactive", "error"
	TmuxConnected bool       `json:"tmux_connected"`
}

// MessageDisplayInfo represents message information for display
type MessageDisplayInfo struct {
	ID            uint          `json:"id"`
	SessionName   string        `json:"session_name"`
	Content       string        `json:"content"`
	Priority      int           `json:"priority"`
	Status        string        `json:"status"` // "pending", "sent", "failed"
	ScheduledTime time.Time     `json:"scheduled_time"`
	SentTime      *time.Time    `json:"sent_time"`
	CreatedAt     time.Time     `json:"created_at"`
	Error         string        `json:"error,omitempty"`
	TimeUntilSend time.Duration `json:"time_until_send"`
}

// TmuxSessionInfo represents tmux session information for display
type TmuxSessionInfo struct {
	Name      string           `json:"name"`
	Attached  bool             `json:"attached"`
	Windows   []TmuxWindowInfo `json:"windows"`
	HasClaude bool             `json:"has_claude"`
	Connected bool             `json:"connected"`
}

// TmuxWindowInfo represents tmux window information for display
type TmuxWindowInfo struct {
	SessionName string `json:"session_name"`
	WindowIndex int    `json:"window_index"`
	WindowName  string `json:"window_name"`
	Active      bool   `json:"active"`
	Target      string `json:"target"`
	HasClaude   bool   `json:"has_claude"`
}

// SchedulerStats represents scheduler statistics for display
type SchedulerStats struct {
	SmartSchedulerEnabled bool          `json:"smart_scheduler_enabled"`
	CronSchedulerEnabled  bool          `json:"cron_scheduler_enabled"`
	PendingMessages       int           `json:"pending_messages"`
	ProcessingMessages    int           `json:"processing_messages"`
	SentMessages          int           `json:"sent_messages"`
	FailedMessages        int           `json:"failed_messages"`
	NextScheduledMessage  *time.Time    `json:"next_scheduled_message"`
	LastProcessedMessage  *time.Time    `json:"last_processed_message"`
	MessagesPerHour       float64       `json:"messages_per_hour"`
	AverageProcessingTime time.Duration `json:"average_processing_time"`
}

// DatabaseStats represents database statistics for display
type DatabaseStats struct {
	TotalSessions     int   `json:"total_sessions"`
	ActiveSessions    int   `json:"active_sessions"`
	TotalMessages     int   `json:"total_messages"`
	PendingMessages   int   `json:"pending_messages"`
	SentMessages      int   `json:"sent_messages"`
	FailedMessages    int   `json:"failed_messages"`
	TotalUsageWindows int   `json:"total_usage_windows"`
	DatabaseSize      int64 `json:"database_size"`
}

// SystemInfo represents system information for display
type SystemInfo struct {
	TmuxRunning       bool              `json:"tmux_running"`
	TmuxSessions      []TmuxSessionInfo `json:"tmux_sessions"`
	DatabaseConnected bool              `json:"database_connected"`
	DatabasePath      string            `json:"database_path"`
	ConfigPath        string            `json:"config_path"`
	Uptime            time.Duration     `json:"uptime"`
	MemoryUsage       uint64            `json:"memory_usage"`
	LastRefresh       time.Time         `json:"last_refresh"`
}

// ApplicationState represents the overall application state
type ApplicationState struct {
	Usage      UsageStats           `json:"usage"`
	Sessions   []SessionDisplayInfo `json:"sessions"`
	Messages   []MessageDisplayInfo `json:"messages"`
	Scheduler  SchedulerStats       `json:"scheduler"`
	Database   DatabaseStats        `json:"database"`
	System     SystemInfo           `json:"system"`
	LastUpdate time.Time            `json:"last_update"`
}

// Message types for TUI communication
type RefreshDataMsg struct {
	Type string      `json:"type"` // "usage", "sessions", "messages", "scheduler", "system", "all"
	Data interface{} `json:"data"`
}

type ErrorMsg struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

type SuccessMsg struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

type StatusMsg struct {
	Message string `json:"message"`
	Level   string `json:"level"` // "info", "warn", "error", "success"
}

// Constants for message statuses
const (
	MessageStatusPending = "pending"
	MessageStatusSent    = "sent"
	MessageStatusFailed  = "failed"

	// Session statuses
	SessionStatusActive   = "active"
	SessionStatusInactive = "inactive"
	SessionStatusError    = "error"

	// Log levels
	LogLevelInfo    = "info"
	LogLevelWarn    = "warn"
	LogLevelError   = "error"
	LogLevelSuccess = "success"
)
