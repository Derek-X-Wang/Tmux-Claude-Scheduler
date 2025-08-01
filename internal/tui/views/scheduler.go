package views

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/scheduler"
	"github.com/derekxwang/tcs/internal/types"
)

// Scheduler represents the scheduler management view
type Scheduler struct {
	db        *gorm.DB
	scheduler *scheduler.Scheduler
	// sessionMonitor removed - using window-based architecture

	// UI components
	messagesTable table.Model
	queueTable    table.Model

	// Form inputs for new message
	sessionInput  textinput.Model
	contentInput  textinput.Model
	priorityInput textinput.Model
	whenInput     textinput.Model

	// State
	width       int
	height      int
	activeTable string // "messages", "queue", "form"
	showForm    bool
	messages    []types.MessageDisplayInfo
	queueItems  []types.MessageDisplayInfo

	// Key bindings
	keyMap SchedulerKeyMap

	// Styles
	titleStyle    lipgloss.Style
	selectedStyle lipgloss.Style
	inactiveStyle lipgloss.Style
	formStyle     lipgloss.Style
	statusStyle   lipgloss.Style
}

// SchedulerKeyMap defines key bindings for the scheduler view
type SchedulerKeyMap struct {
	NewMessage      key.Binding
	DeleteMessage   key.Binding
	SendNow         key.Binding
	CancelMessage   key.Binding
	RefreshQueue    key.Binding
	SwitchTable     key.Binding
	ShowForm        key.Binding
	CancelForm      key.Binding
	SubmitForm      key.Binding
	PauseScheduler  key.Binding
	ResumeScheduler key.Binding
}

// DefaultSchedulerKeyMap returns the default scheduler key bindings
func DefaultSchedulerKeyMap() SchedulerKeyMap {
	return SchedulerKeyMap{
		NewMessage: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new message"),
		),
		DeleteMessage: key.NewBinding(
			key.WithKeys("d", "delete"),
			key.WithHelp("d", "delete message"),
		),
		SendNow: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "send now"),
		),
		CancelMessage: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "cancel message"),
		),
		RefreshQueue: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		SwitchTable: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch table"),
		),
		ShowForm: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "show form"),
		),
		CancelForm: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		SubmitForm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		PauseScheduler: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause scheduler"),
		),
		ResumeScheduler: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "resume scheduler"),
		),
	}
}

// NewScheduler creates a new scheduler view
func NewScheduler(db *gorm.DB, schedulerInstance *scheduler.Scheduler, windowDiscovery interface{}) *Scheduler {
	// Create messages table
	messagesColumns := []table.Column{
		{Title: "ID", Width: 4},
		{Title: "Session", Width: 12},
		{Title: "Content", Width: 30},
		{Title: "Priority", Width: 8},
		{Title: "Status", Width: 8},
		{Title: "Scheduled", Width: 16},
		{Title: "Time Until", Width: 12},
	}

	messagesTable := table.New(
		table.WithColumns(messagesColumns),
		table.WithFocused(true),
		table.WithHeight(12),
	)

	// Create queue table (active/processing messages)
	queueColumns := []table.Column{
		{Title: "ID", Width: 4},
		{Title: "Session", Width: 12},
		{Title: "Content", Width: 25},
		{Title: "Priority", Width: 8},
		{Title: "Scheduled", Width: 16},
		{Title: "Status", Width: 10},
	}

	queueTable := table.New(
		table.WithColumns(queueColumns),
		table.WithFocused(false),
		table.WithHeight(8),
	)

	// Create form inputs
	sessionInput := textinput.New()
	sessionInput.Placeholder = "session-name"
	sessionInput.Focus()
	sessionInput.CharLimit = 50
	sessionInput.Width = 30

	contentInput := textinput.New()
	contentInput.Placeholder = "Message content"
	contentInput.CharLimit = 500
	contentInput.Width = 50

	priorityInput := textinput.New()
	priorityInput.Placeholder = "1-10 (default: 5)"
	priorityInput.CharLimit = 2
	priorityInput.Width = 20

	whenInput := textinput.New()
	whenInput.Placeholder = "now, +5m, 14:30, etc."
	whenInput.CharLimit = 20
	whenInput.Width = 30

	s := &Scheduler{
		db:        db,
		scheduler: schedulerInstance,
		// sessionMonitor removed
		messagesTable: messagesTable,
		queueTable:    queueTable,
		sessionInput:  sessionInput,
		contentInput:  contentInput,
		priorityInput: priorityInput,
		whenInput:     whenInput,
		activeTable:   "messages",
		keyMap:        DefaultSchedulerKeyMap(),
	}

	s.initStyles()
	return s
}

// initStyles initializes the scheduler view styles
func (s *Scheduler) initStyles() {
	s.titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		MarginBottom(1)

	s.selectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("57")).
		Bold(true)

	s.inactiveStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	s.formStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1).
		Width(60)

	s.statusStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Bold(true)
}

// Init initializes the scheduler view
func (s *Scheduler) Init() tea.Cmd {
	return s.refreshData()
}

// Update handles messages for the scheduler view
func (s *Scheduler) Update(msg tea.Msg) (*Scheduler, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s.updateTableSizes()

	case tea.KeyMsg:
		if s.showForm {
			return s.handleFormKeys(msg)
		}
		return s.handleTableKeys(msg)

	case types.RefreshDataMsg:
		if msg.Type == "all" || msg.Type == "scheduler" || msg.Type == "messages" {
			if msg.Data != nil {
				// Handle scheduler data refresh in main thread
				if dbMessages, ok := msg.Data.([]struct {
					ID            uint
					WindowID      uint
					Content       string
					Priority      int
					Status        string
					ScheduledTime time.Time
					SentTime      *time.Time
					CreatedAt     time.Time
					Error         string
					WindowTarget  string
				}); ok {
					s.refreshMessagesWithData(dbMessages)
					s.refreshQueueWithData()
				}
			} else {
				// Trigger new data fetch
				cmds = append(cmds, s.refreshData())
			}
		}

	case types.SuccessMsg:
		// After successful operations, trigger a refresh
		if msg.Title == "Message Scheduled" || msg.Title == "Message Deleted" || msg.Title == "Message Updated" || msg.Title == "Message Canceled" {
			cmds = append(cmds, s.refreshData())
		}
	}

	// Update tables
	if s.activeTable == "messages" {
		s.messagesTable, _ = s.messagesTable.Update(msg)
	} else if s.activeTable == "queue" {
		s.queueTable, _ = s.queueTable.Update(msg)
	}

	return s, tea.Batch(cmds...)
}

// IsFormActive returns true if the form is currently active
func (s *Scheduler) IsFormActive() bool {
	return s.showForm
}

// handleFormKeys handles key presses when form is shown
func (s *Scheduler) handleFormKeys(msg tea.KeyMsg) (*Scheduler, tea.Cmd) {
	var cmds []tea.Cmd

	switch {
	case key.Matches(msg, s.keyMap.CancelForm):
		s.showForm = false
		s.clearForm()
		return s, nil

	case key.Matches(msg, s.keyMap.SubmitForm):
		return s.handleFormSubmit()

	case msg.String() == "tab":
		// Cycle through form inputs
		if s.sessionInput.Focused() {
			s.sessionInput.Blur()
			s.contentInput.Focus()
		} else if s.contentInput.Focused() {
			s.contentInput.Blur()
			s.priorityInput.Focus()
		} else if s.priorityInput.Focused() {
			s.priorityInput.Blur()
			s.whenInput.Focus()
		} else {
			s.whenInput.Blur()
			s.sessionInput.Focus()
		}
		return s, nil
	}

	// Update focused input
	if s.sessionInput.Focused() {
		s.sessionInput, _ = s.sessionInput.Update(msg)
	} else if s.contentInput.Focused() {
		s.contentInput, _ = s.contentInput.Update(msg)
	} else if s.priorityInput.Focused() {
		s.priorityInput, _ = s.priorityInput.Update(msg)
	} else if s.whenInput.Focused() {
		s.whenInput, _ = s.whenInput.Update(msg)
	}

	return s, tea.Batch(cmds...)
}

// handleTableKeys handles key presses when tables are active
func (s *Scheduler) handleTableKeys(msg tea.KeyMsg) (*Scheduler, tea.Cmd) {
	var cmds []tea.Cmd

	switch {
	case key.Matches(msg, s.keyMap.NewMessage):
		s.showForm = true
		s.sessionInput.Focus()
		return s, nil

	case key.Matches(msg, s.keyMap.DeleteMessage):
		if s.activeTable == "messages" && len(s.messages) > 0 {
			selected := s.messagesTable.Cursor()
			if selected < len(s.messages) {
				cmds = append(cmds, s.deleteMessage(s.messages[selected].ID))
			}
		}

	case key.Matches(msg, s.keyMap.SendNow):
		if s.activeTable == "messages" && len(s.messages) > 0 {
			selected := s.messagesTable.Cursor()
			if selected < len(s.messages) && s.messages[selected].Status == "pending" {
				cmds = append(cmds, s.sendMessageNow(s.messages[selected].ID))
			}
		}

	case key.Matches(msg, s.keyMap.CancelMessage):
		if s.activeTable == "messages" && len(s.messages) > 0 {
			selected := s.messagesTable.Cursor()
			if selected < len(s.messages) && s.messages[selected].Status == "pending" {
				cmds = append(cmds, s.cancelMessage(s.messages[selected].ID))
			}
		}

	case key.Matches(msg, s.keyMap.SwitchTable):
		if s.activeTable == "messages" {
			s.activeTable = "queue"
			s.messagesTable.Blur()
			s.queueTable.Focus()
		} else {
			s.activeTable = "messages"
			s.queueTable.Blur()
			s.messagesTable.Focus()
		}

	case key.Matches(msg, s.keyMap.RefreshQueue):
		cmds = append(cmds, s.refreshData())
	}

	return s, tea.Batch(cmds...)
}

// View renders the scheduler view
func (s *Scheduler) View() string {
	if s.width == 0 {
		return "Loading scheduler..."
	}

	if s.showForm {
		return s.renderForm()
	}

	var content []string

	// Scheduler status
	content = append(content, s.renderSchedulerStatus())

	// Messages table
	content = append(content, s.renderMessagesTable())

	// Queue table
	content = append(content, s.renderQueueTable())

	// Help text
	content = append(content, s.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, content...)
}

// renderSchedulerStatus renders the scheduler status section
func (s *Scheduler) renderSchedulerStatus() string {
	status := "🔄 Scheduler Status: "

	// Get basic status info
	smartEnabled := "Smart: ✓"
	cronEnabled := "Cron: ✓"

	// TODO: Get actual scheduler state from scheduler instance
	pendingCount := len(s.messages)
	processingCount := 0

	for _, msg := range s.messages {
		if msg.Status == "processing" {
			processingCount++
		}
	}

	statusLine := fmt.Sprintf("%s%s  %s  Pending: %d  Processing: %d",
		status, smartEnabled, cronEnabled, pendingCount, processingCount)

	return s.statusStyle.Render(statusLine) + "\n"
}

// renderMessagesTable renders the messages table
func (s *Scheduler) renderMessagesTable() string {
	title := "Scheduled Messages"
	if s.activeTable == "messages" {
		title = s.selectedStyle.Render("► " + title)
	} else {
		title = s.inactiveStyle.Render("  " + title)
	}

	return fmt.Sprintf("%s\n%s",
		s.titleStyle.Render(title),
		s.messagesTable.View())
}

// renderQueueTable renders the queue table
func (s *Scheduler) renderQueueTable() string {
	title := "Processing Queue"
	if s.activeTable == "queue" {
		title = s.selectedStyle.Render("► " + title)
	} else {
		title = s.inactiveStyle.Render("  " + title)
	}

	return fmt.Sprintf("\n%s\n%s",
		s.titleStyle.Render(title),
		s.queueTable.View())
}

// renderForm renders the new message form
func (s *Scheduler) renderForm() string {
	form := fmt.Sprintf(
		"Schedule New Message\n\n"+
			"Session:\n%s\n\n"+
			"Content:\n%s\n\n"+
			"Priority (1-10):\n%s\n\n"+
			"When:\n%s\n\n"+
			"Press Enter to schedule, Esc to cancel, Tab to navigate",
		s.sessionInput.View(),
		s.contentInput.View(),
		s.priorityInput.View(),
		s.whenInput.View(),
	)

	return s.formStyle.Render(form)
}

// renderHelp renders the help text
func (s *Scheduler) renderHelp() string {
	help := "\nn: New Message  d: Delete  s: Send Now  c: Cancel  tab: Switch Table  r: Refresh  p/P: Pause/Resume"
	return s.inactiveStyle.Render(help)
}

// SetSize sets the scheduler view size
func (s *Scheduler) SetSize(width, height int) {
	s.width = width
	s.height = height
	s.updateTableSizes()
}

// Refresh refreshes the scheduler data
func (s *Scheduler) Refresh() tea.Cmd {
	return s.refreshData()
}

// updateTableSizes updates table sizes based on current dimensions
func (s *Scheduler) updateTableSizes() {
	tableWidth := s.width - 4

	// Update messages table
	columns := s.messagesTable.Columns()
	if len(columns) > 0 {
		// Adjust content column based on available width
		contentWidth := max(20, tableWidth-70) // Reserve space for other columns

		columns[2].Width = contentWidth // Content
		s.messagesTable.SetColumns(columns)
	}

	s.messagesTable.SetWidth(tableWidth)
	s.messagesTable.SetHeight(min(12, s.height/2))

	// Update queue table
	queueColumns := s.queueTable.Columns()
	if len(queueColumns) > 0 {
		contentWidth := max(15, tableWidth-65)

		queueColumns[2].Width = contentWidth // Content
		s.queueTable.SetColumns(queueColumns)
	}

	s.queueTable.SetWidth(tableWidth)
	s.queueTable.SetHeight(min(8, s.height/3))
}

// refreshData refreshes all scheduler data
func (s *Scheduler) refreshData() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		if s.db == nil {
			return types.ErrorMsg{Title: "Database Error", Message: "Database not available"}
		}

		// Query all messages from the database (do DB operations in goroutine)
		var dbMessages []struct {
			ID            uint
			WindowID      uint
			Content       string
			Priority      int
			Status        string
			ScheduledTime time.Time
			SentTime      *time.Time
			CreatedAt     time.Time
			Error         string
			WindowTarget  string
		}

		// Query messages with window information
		err := s.db.Table("messages").
			Select("messages.*, tmux_windows.target as window_target").
			Joins("left join tmux_windows on tmux_windows.id = messages.window_id").
			Order("scheduled_time desc").
			Limit(50).
			Find(&dbMessages).Error
		if err != nil {
			return types.ErrorMsg{Title: "Refresh failed", Message: err.Error()}
		}

		// Return the data in the message so UI updates happen in main thread
		return types.RefreshDataMsg{
			Type: "scheduler",
			Data: dbMessages,
		}
	})
}

// refreshMessagesWithData refreshes messages data using provided message data (called from main thread)
func (s *Scheduler) refreshMessagesWithData(dbMessages []struct {
	ID            uint
	WindowID      uint
	Content       string
	Priority      int
	Status        string
	ScheduledTime time.Time
	SentTime      *time.Time
	CreatedAt     time.Time
	Error         string
	WindowTarget  string
}) {
	s.messages = make([]types.MessageDisplayInfo, len(dbMessages))
	var rows []table.Row

	for i, msg := range dbMessages {
		timeUntilSend := time.Until(msg.ScheduledTime)
		if timeUntilSend < 0 {
			timeUntilSend = 0
		}

		s.messages[i] = types.MessageDisplayInfo{
			ID:            msg.ID,
			SessionName:   msg.WindowTarget,
			Content:       msg.Content,
			Priority:      msg.Priority,
			Status:        msg.Status,
			ScheduledTime: msg.ScheduledTime,
			SentTime:      msg.SentTime,
			CreatedAt:     msg.CreatedAt,
			Error:         msg.Error,
			TimeUntilSend: timeUntilSend,
		}

		// Truncate content for display
		displayContent := msg.Content
		if len(displayContent) > 27 {
			displayContent = displayContent[:27] + "..."
		}

		timeUntilStr := "-"
		if msg.Status == "pending" {
			if timeUntilSend > 0 {
				timeUntilStr = timeUntilSend.Round(time.Second).String()
			} else {
				timeUntilStr = "now"
			}
		}

		rows = append(rows, table.Row{
			strconv.Itoa(int(msg.ID)),
			msg.WindowTarget,
			displayContent,
			strconv.Itoa(msg.Priority),
			msg.Status,
			msg.ScheduledTime.Format("01-02 15:04"),
			timeUntilStr,
		})
	}

	s.messagesTable.SetRows(rows)
}

// refreshQueueWithData refreshes the processing queue data (called from main thread)
func (s *Scheduler) refreshQueueWithData() {
	// Filter messages that are currently being processed or in queue
	var queueItems []types.MessageDisplayInfo
	var rows []table.Row

	for _, msg := range s.messages {
		if msg.Status == "pending" || msg.Status == "processing" {
			queueItems = append(queueItems, msg)

			displayContent := msg.Content
			if len(displayContent) > 22 {
				displayContent = displayContent[:22] + "..."
			}

			rows = append(rows, table.Row{
				strconv.Itoa(int(msg.ID)),
				msg.SessionName,
				displayContent,
				strconv.Itoa(msg.Priority),
				msg.ScheduledTime.Format("01-02 15:04"),
				msg.Status,
			})
		}
	}

	s.queueItems = queueItems
	s.queueTable.SetRows(rows)
}

// handleFormSubmit handles form submission
func (s *Scheduler) handleFormSubmit() (*Scheduler, tea.Cmd) {
	sessionName := s.sessionInput.Value()
	content := s.contentInput.Value()
	priorityStr := s.priorityInput.Value()
	when := s.whenInput.Value()

	// Validate inputs
	if sessionName == "" || content == "" {
		return s, nil // TODO: Show error
	}

	priority := 5 // Default priority
	if priorityStr != "" {
		if p, err := strconv.Atoi(priorityStr); err == nil && p >= 1 && p <= 10 {
			priority = p
		}
	}

	if when == "" {
		when = "now"
	}

	// Clear form and hide it
	s.showForm = false
	s.clearForm()

	return s, s.scheduleMessage(sessionName, content, priority, when)
}

// clearForm clears all form inputs
func (s *Scheduler) clearForm() {
	s.sessionInput.SetValue("")
	s.contentInput.SetValue("")
	s.priorityInput.SetValue("")
	s.whenInput.SetValue("")
	s.sessionInput.Focus()
}

// scheduleMessage schedules a new message
func (s *Scheduler) scheduleMessage(sessionName, content string, priority int, when string) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		if s.scheduler == nil {
			return types.ErrorMsg{Title: "Error", Message: "Scheduler not available"}
		}

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
					return types.ErrorMsg{Title: "Invalid time format", Message: err.Error()}
				}
				scheduledTime = roundScheduledTimeToMinute(time.Now().Add(duration))
			} else {
				// Try to parse as time (e.g., "14:30")
				scheduledTime, err = time.Parse("15:04", when)
				if err != nil {
					return types.ErrorMsg{Title: "Invalid time format", Message: err.Error()}
				}
				// If time is in the past, schedule for tomorrow
				if scheduledTime.Before(time.Now()) {
					scheduledTime = scheduledTime.AddDate(0, 0, 1)
				}
			}
		}

		_, err = s.scheduler.ScheduleMessage(sessionName, content, scheduledTime, priority)
		if err != nil {
			return types.ErrorMsg{Title: "Failed to schedule message", Message: err.Error()}
		}

		// Data will refresh via SuccessMsg handling

		return types.SuccessMsg{
			Title:   "Message Scheduled",
			Message: fmt.Sprintf("Scheduled message for session '%s'", sessionName),
		}
	})
}

// deleteMessage deletes a message
func (s *Scheduler) deleteMessage(id uint) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		if s.db == nil {
			return types.ErrorMsg{Title: "Error", Message: "Database not available"}
		}

		err := s.db.Delete(&types.MessageDisplayInfo{}, id).Error
		if err != nil {
			return types.ErrorMsg{Title: "Failed to delete message", Message: err.Error()}
		}

		// Data will refresh via SuccessMsg handling

		return types.SuccessMsg{
			Title:   "Message Deleted",
			Message: fmt.Sprintf("Deleted message with ID %d", id),
		}
	})
}

// sendMessageNow sends a message immediately
func (s *Scheduler) sendMessageNow(id uint) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		// Update message to be sent now
		if s.db != nil {
			s.db.Model(&types.MessageDisplayInfo{}).
				Where("id = ?", id).
				Update("scheduled_time", time.Now())
		}

		// Data will refresh via SuccessMsg handling

		return types.SuccessMsg{
			Title:   "Message Updated",
			Message: fmt.Sprintf("Message %d scheduled to send now", id),
		}
	})
}

// cancelMessage cancels a message
func (s *Scheduler) cancelMessage(id uint) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		if s.db != nil {
			s.db.Model(&types.MessageDisplayInfo{}).
				Where("id = ?", id).
				Update("status", "canceled")
		}

		// Data will refresh via SuccessMsg handling

		return types.SuccessMsg{
			Title:   "Message Canceled",
			Message: fmt.Sprintf("Canceled message with ID %d", id),
		}
	})
}

// roundScheduledTimeToMinute rounds a time down to the nearest minute (ignores seconds)
func roundScheduledTimeToMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute)
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
