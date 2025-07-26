package tui

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

// RefreshDataMsg represents a data refresh message
type RefreshDataMsg struct {
	Type string      `json:"type"` // "usage", "sessions", "messages", "scheduler", "system", "all"
	Data interface{} `json:"data"`
}

// ErrorMsg represents an error message
type ErrorMsg struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

// SuccessMsg represents a success message
type SuccessMsg struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// StatusMsg represents a status update message
type StatusMsg struct {
	Message string `json:"message"`
	Level   string `json:"level"` // "info", "warn", "error", "success"
}

// InputRequestMsg represents a request for user input
type InputRequestMsg struct {
	Prompt      string               `json:"prompt"`
	Placeholder string               `json:"placeholder"`
	Validator   func(string) error   `json:"-"`
	Callback    func(string) tea.Cmd `json:"-"`
}

// NavigationMsg represents navigation messages
type NavigationMsg struct {
	Direction string `json:"direction"` // "up", "down", "left", "right", "enter", "back"
	Target    string `json:"target,omitempty"`
}

// TableSelectionMsg represents table selection messages
type TableSelectionMsg struct {
	Table string      `json:"table"` // "sessions", "messages", "tmux"
	Index int         `json:"index"`
	Item  interface{} `json:"item"`
}

// FormSubmitMsg represents form submission messages
type FormSubmitMsg struct {
	Form string                 `json:"form"` // "new_session", "new_message", "edit_session"
	Data map[string]interface{} `json:"data"`
}

// ActionMsg represents action messages
type ActionMsg struct {
	Action string                 `json:"action"` // "delete", "activate", "deactivate", "send_now", "cancel"
	Target string                 `json:"target"` // "session", "message"
	ID     uint                   `json:"id"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// ViewUpdateMsg represents view update messages
type ViewUpdateMsg struct {
	View string      `json:"view"` // "dashboard", "sessions", "scheduler"
	Data interface{} `json:"data"`
}

// TimerMsg represents timer-based messages
type TimerMsg struct {
	Type      string      `json:"type"` // "refresh", "countdown", "alert"
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// ConfirmationMsg represents confirmation dialog messages
type ConfirmationMsg struct {
	Title     string  `json:"title"`
	Message   string  `json:"message"`
	OnConfirm tea.Cmd `json:"-"`
	OnCancel  tea.Cmd `json:"-"`
}

// HelpMsg represents help system messages
type HelpMsg struct {
	View    string `json:"view"` // "main", "sessions", "scheduler", "keybindings"
	Visible bool   `json:"visible"`
}

// ThemeMsg represents theme change messages
type ThemeMsg struct {
	Theme string `json:"theme"` // "default", "dark", "light", "high_contrast"
}

// ExportMsg represents data export messages
type ExportMsg struct {
	Type   string `json:"type"`   // "sessions", "messages", "usage", "all"
	Format string `json:"format"` // "json", "csv", "yaml"
	Path   string `json:"path"`
}

// ImportMsg represents data import messages
type ImportMsg struct {
	Type string `json:"type"` // "sessions", "messages", "config"
	Path string `json:"path"`
}

// Constants for message types and statuses
const (
	// Message statuses
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

	// View types
	ViewDashboard = "dashboard"
	ViewSessions  = "sessions"
	ViewScheduler = "scheduler"
	ViewHelp      = "help"

	// Table types
	TableSessions = "sessions"
	TableMessages = "messages"
	TableTmux     = "tmux"

	// Action types
	ActionDelete     = "delete"
	ActionActivate   = "activate"
	ActionDeactivate = "deactivate"
	ActionSendNow    = "send_now"
	ActionCancel     = "cancel"
	ActionEdit       = "edit"
	ActionClone      = "clone"

	// Form types
	FormNewSession  = "new_session"
	FormNewMessage  = "new_message"
	FormEditSession = "edit_session"
	FormEditMessage = "edit_message"

	// Export formats
	ExportFormatJSON = "json"
	ExportFormatCSV  = "csv"
	ExportFormatYAML = "yaml"

	// Theme types
	ThemeDefault      = "default"
	ThemeDark         = "dark"
	ThemeLight        = "light"
	ThemeHighContrast = "high_contrast"
)
