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

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/scheduler"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/types"
)

// Messages represents the message management view with editing capabilities
type Messages struct {
	db         *gorm.DB
	scheduler  *scheduler.Scheduler
	tmuxClient *tmux.Client

	// UI components
	messagesTable table.Model

	// Form inputs for new/edit message
	targetInput   textinput.Model
	contentInput  textinput.Model
	priorityInput textinput.Model
	whenInput     textinput.Model

	// State
	width         int
	height        int
	showForm      bool
	editMode      bool
	editingID     uint
	messages      []database.Message
	sessionGroups map[string][]database.Message

	// Key bindings
	keyMap MessagesKeyMap

	// Styles
	titleStyle    lipgloss.Style
	selectedStyle lipgloss.Style
	inactiveStyle lipgloss.Style
	formStyle     lipgloss.Style
	errorStyle    lipgloss.Style
}

// MessagesKeyMap defines key bindings for the messages view
type MessagesKeyMap struct {
	NewMessage      key.Binding
	EditMessage     key.Binding
	DeleteMessage   key.Binding
	RefreshMessages key.Binding
	ShowForm        key.Binding
	CancelForm      key.Binding
	SubmitForm      key.Binding
	NextInput       key.Binding
}

// DefaultMessagesKeyMap returns the default messages key bindings
func DefaultMessagesKeyMap() MessagesKeyMap {
	return MessagesKeyMap{
		NewMessage: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new message"),
		),
		EditMessage: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit message"),
		),
		DeleteMessage: key.NewBinding(
			key.WithKeys("d", "delete"),
			key.WithHelp("d", "delete message"),
		),
		RefreshMessages: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
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
		NextInput: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next input"),
		),
	}
}

// NewMessages creates a new messages view
func NewMessages(db *gorm.DB, schedulerInstance *scheduler.Scheduler, tmuxClient *tmux.Client) *Messages {
	// Create messages table - grouped by session as requested
	messagesColumns := []table.Column{
		{Title: "ID", Width: 6},
		{Title: "Session", Width: 12},
		{Title: "Target", Width: 12},
		{Title: "Priority", Width: 8},
		{Title: "Content", Width: 30},
		{Title: "Scheduled", Width: 16},
		{Title: "Status", Width: 10},
		{Title: "Retries", Width: 7},
	}

	messagesTable := table.New(
		table.WithColumns(messagesColumns),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	// Create form inputs for message editing
	targetInput := textinput.New()
	targetInput.Placeholder = "session:window (e.g., project:0)"
	targetInput.CharLimit = 50
	targetInput.Width = 40

	contentInput := textinput.New()
	contentInput.Placeholder = "Message content"
	contentInput.CharLimit = 200
	contentInput.Width = 60

	priorityInput := textinput.New()
	priorityInput.Placeholder = "1-10"
	priorityInput.CharLimit = 2
	priorityInput.Width = 10

	whenInput := textinput.New()
	whenInput.Placeholder = "now, +5m, 14:30, etc."
	whenInput.CharLimit = 20
	whenInput.Width = 20

	m := &Messages{
		db:            db,
		scheduler:     schedulerInstance,
		tmuxClient:    tmuxClient,
		messagesTable: messagesTable,
		targetInput:   targetInput,
		contentInput:  contentInput,
		priorityInput: priorityInput,
		whenInput:     whenInput,
		sessionGroups: make(map[string][]database.Message),
		keyMap:        DefaultMessagesKeyMap(),
	}

	m.initStyles()
	return m
}

// initStyles initializes the messages view styles
func (m *Messages) initStyles() {
	m.titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		MarginBottom(1)

	m.selectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("57")).
		Bold(true)

	m.inactiveStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	m.formStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1).
		Width(70)

	m.errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)
}

// Init initializes the messages view
func (m *Messages) Init() tea.Cmd {
	return m.refreshData()
}

// Update handles messages for the messages view
func (m *Messages) Update(msg tea.Msg) (*Messages, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateTableSizes()

	case tea.KeyMsg:
		if m.showForm {
			return m.handleFormKeys(msg)
		}
		return m.handleTableKeys(msg)

	case types.RefreshDataMsg:
		if msg.Type == "all" || msg.Type == "messages" {
			cmds = append(cmds, m.refreshData())
		}

	case types.SuccessMsg:
		// After successful operations, trigger a refresh
		if msg.Title == "Message Created" || msg.Title == "Message Updated" || msg.Title == "Message Deleted" {
			cmds = append(cmds, m.refreshData())
		}
	}

	// Update table
	m.messagesTable, _ = m.messagesTable.Update(msg)

	return m, tea.Batch(cmds...)
}

// IsFormActive returns true if the form is currently active
func (m *Messages) IsFormActive() bool {
	return m.showForm
}

// handleFormKeys handles key presses when form is shown
func (m *Messages) handleFormKeys(msg tea.KeyMsg) (*Messages, tea.Cmd) {
	var cmds []tea.Cmd

	switch {
	case key.Matches(msg, m.keyMap.CancelForm):
		m.showForm = false
		m.clearForm()
		return m, nil

	case key.Matches(msg, m.keyMap.SubmitForm):
		return m.handleFormSubmit()

	case key.Matches(msg, m.keyMap.NextInput):
		// Cycle through form inputs
		if m.targetInput.Focused() {
			m.targetInput.Blur()
			m.contentInput.Focus()
		} else if m.contentInput.Focused() {
			m.contentInput.Blur()
			m.priorityInput.Focus()
		} else if m.priorityInput.Focused() {
			m.priorityInput.Blur()
			m.whenInput.Focus()
		} else {
			m.whenInput.Blur()
			m.targetInput.Focus()
		}
		return m, nil
	}

	// Update focused input
	if m.targetInput.Focused() {
		m.targetInput, _ = m.targetInput.Update(msg)
	} else if m.contentInput.Focused() {
		m.contentInput, _ = m.contentInput.Update(msg)
	} else if m.priorityInput.Focused() {
		m.priorityInput, _ = m.priorityInput.Update(msg)
	} else if m.whenInput.Focused() {
		m.whenInput, _ = m.whenInput.Update(msg)
	}

	return m, tea.Batch(cmds...)
}

// handleTableKeys handles key presses when table is active
func (m *Messages) handleTableKeys(msg tea.KeyMsg) (*Messages, tea.Cmd) {
	var cmds []tea.Cmd

	switch {
	case key.Matches(msg, m.keyMap.NewMessage):
		m.showForm = true
		m.editMode = false
		m.editingID = 0
		m.clearForm()
		m.targetInput.Focus()
		return m, nil

	case key.Matches(msg, m.keyMap.EditMessage):
		if len(m.messages) > 0 {
			selected := m.messagesTable.Cursor()
			if selected < len(m.messages) {
				cmds = append(cmds, m.startEditMessage(m.messages[selected]))
			}
		}

	case key.Matches(msg, m.keyMap.DeleteMessage):
		if len(m.messages) > 0 {
			selected := m.messagesTable.Cursor()
			if selected < len(m.messages) {
				cmds = append(cmds, m.deleteMessage(m.messages[selected].ID))
			}
		}

	case key.Matches(msg, m.keyMap.RefreshMessages):
		cmds = append(cmds, m.refreshData())
	}

	return m, tea.Batch(cmds...)
}

// View renders the messages view
func (m *Messages) View() string {
	if m.width == 0 {
		return "Loading messages..."
	}

	if m.showForm {
		return m.renderForm()
	}

	var content []string

	// Messages table (grouped by session)
	content = append(content, m.renderMessagesTable())

	// Statistics
	content = append(content, m.renderStats())

	// Help text
	content = append(content, m.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, content...)
}

// renderMessagesTable renders the messages table
func (m *Messages) renderMessagesTable() string {
	title := "Scheduled Messages (grouped by session)"
	title = m.selectedStyle.Render("► " + title)

	return fmt.Sprintf("%s\n%s",
		m.titleStyle.Render(title),
		m.messagesTable.View())
}

// renderStats renders message statistics
func (m *Messages) renderStats() string {
	totalMessages := len(m.messages)
	pendingMessages := 0
	sessionCount := len(m.sessionGroups)

	for _, msg := range m.messages {
		if msg.Status == database.MessageStatusPending {
			pendingMessages++
		}
	}

	stats := fmt.Sprintf(
		"\nMessages: %d total, %d pending across %d sessions",
		totalMessages, pendingMessages, sessionCount,
	)

	return m.inactiveStyle.Render(stats)
}

// renderForm renders the new/edit message form
func (m *Messages) renderForm() string {
	formTitle := "Create New Message"
	if m.editMode {
		formTitle = fmt.Sprintf("Edit Message #%d", m.editingID)
	}

	form := fmt.Sprintf(
		"%s\n\n"+
			"Target Window:\n%s\n\n"+
			"Content:\n%s\n\n"+
			"Priority (1-10):\n%s\n\n"+
			"When (now, +5m, 14:30):\n%s\n\n"+
			"Press Enter to %s, Esc to cancel, Tab to cycle inputs",
		formTitle,
		m.targetInput.View(),
		m.contentInput.View(),
		m.priorityInput.View(),
		m.whenInput.View(),
		map[bool]string{true: "update", false: "create"}[m.editMode],
	)

	return m.formStyle.Render(form)
}

// renderHelp renders the help text
func (m *Messages) renderHelp() string {
	help := "\nn: New Message  e: Edit  d: Delete  r: Refresh"
	return m.inactiveStyle.Render(help)
}

// SetSize sets the messages view size
func (m *Messages) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.updateTableSizes()
}

// Refresh refreshes the messages data
func (m *Messages) Refresh() tea.Cmd {
	return m.refreshData()
}

// updateTableSizes updates table sizes based on current dimensions
func (m *Messages) updateTableSizes() {
	tableWidth := m.width - 4

	// Adjust content column width based on available space
	columns := m.messagesTable.Columns()
	if len(columns) > 4 {
		contentWidth := 20
		if tableWidth-60 > contentWidth {
			contentWidth = tableWidth - 60
		}
		columns[4].Width = contentWidth // Content column
		m.messagesTable.SetColumns(columns)
	}

	m.messagesTable.SetWidth(tableWidth)
	tableHeight := 15
	if (m.height*2)/3 < tableHeight {
		tableHeight = (m.height * 2) / 3
	}
	m.messagesTable.SetHeight(tableHeight)
}

// refreshData refreshes all messages data
func (m *Messages) refreshData() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		m.refreshMessages()

		return types.RefreshDataMsg{
			Type: "messages",
			Data: nil,
		}
	})
}

// refreshMessages refreshes messages data grouped by session
func (m *Messages) refreshMessages() {
	// Get all pending messages
	messages, err := database.GetPendingMessages(m.db, 0)
	if err != nil {
		return
	}

	// Group by session for display as requested
	m.sessionGroups = make(map[string][]database.Message)
	var rows []table.Row

	for _, msg := range messages {
		// Load window info
		err := m.db.Preload("Window").First(&msg, msg.ID).Error
		if err != nil {
			continue
		}

		sessionName := msg.Window.SessionName
		m.sessionGroups[sessionName] = append(m.sessionGroups[sessionName], msg)

		// Truncate content for display
		content := msg.Content
		if len(content) > 30 {
			content = content[:27] + "..."
		}

		// Format scheduled time
		scheduledTime := msg.ScheduledTime.Format("01-02 15:04")

		rows = append(rows, table.Row{
			strconv.Itoa(int(msg.ID)),
			sessionName,
			msg.Window.Target,
			strconv.Itoa(msg.Priority),
			content,
			scheduledTime,
			msg.Status,
			strconv.Itoa(msg.Retries),
		})
	}

	m.messages = messages
	m.messagesTable.SetRows(rows)
}

// startEditMessage starts editing a message
func (m *Messages) startEditMessage(message database.Message) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		// Check if message can be edited
		if message.Status == database.MessageStatusSent {
			return types.ErrorMsg{
				Title:   "Cannot Edit",
				Message: "Message has already been sent",
			}
		}

		// Load window info if not already loaded
		if message.Window.Target == "" {
			m.db.Preload("Window").First(&message, message.ID)
		}

		// Pre-fill form with existing values
		m.targetInput.SetValue(message.Window.Target)
		m.contentInput.SetValue(message.Content)
		m.priorityInput.SetValue(strconv.Itoa(message.Priority))

		// Format the scheduled time
		if message.ScheduledTime.After(time.Now()) {
			m.whenInput.SetValue(message.ScheduledTime.Format("15:04"))
		} else {
			m.whenInput.SetValue("now")
		}

		m.showForm = true
		m.editMode = true
		m.editingID = message.ID
		m.targetInput.Focus()

		return types.StatusMsg{
			Message: fmt.Sprintf("Editing message #%d", message.ID),
			Level:   "info",
		}
	})
}

// handleFormSubmit handles form submission for new/edit message
func (m *Messages) handleFormSubmit() (*Messages, tea.Cmd) {
	target := m.targetInput.Value()
	content := m.contentInput.Value()
	priorityStr := m.priorityInput.Value()
	when := m.whenInput.Value()

	// Validate inputs
	if target == "" || content == "" {
		return m, func() tea.Msg {
			return types.ErrorMsg{
				Title:   "Validation Error",
				Message: "Target and content are required",
			}
		}
	}

	priority := 5 // Default priority
	if priorityStr != "" {
		if p, err := strconv.Atoi(priorityStr); err == nil && p >= 1 && p <= 10 {
			priority = p
		}
	}

	// Parse schedule time
	scheduledTime := time.Now()
	if when != "" && when != "now" {
		if when[0] == '+' {
			if duration, err := time.ParseDuration(when[1:]); err == nil {
				scheduledTime = roundTimeToMinute(time.Now().Add(duration))
			}
		} else {
			if t, err := time.Parse("15:04", when); err == nil {
				// If time is in the past, schedule for tomorrow
				if t.Before(time.Now()) {
					scheduledTime = t.AddDate(0, 0, 1)
				} else {
					scheduledTime = t
				}
			}
		}
	}

	// Clear form and hide it
	m.showForm = false
	m.clearForm()

	if m.editMode {
		return m, m.updateMessage(m.editingID, target, content, priority, scheduledTime)
	} else {
		return m, m.createMessage(target, content, priority, scheduledTime)
	}
}

// createMessage creates a new message
func (m *Messages) createMessage(target, content string, priority int, scheduledTime time.Time) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		if m.scheduler == nil {
			return types.ErrorMsg{Title: "Error", Message: "Scheduler not available"}
		}

		message, err := m.scheduler.ScheduleMessage(target, content, scheduledTime, priority)
		if err != nil {
			return types.ErrorMsg{Title: "Failed to create message", Message: err.Error()}
		}

		// Data will refresh via SuccessMsg handling

		return types.SuccessMsg{
			Title:   "Message Created",
			Message: fmt.Sprintf("Created message #%d for target '%s'", message.ID, target),
		}
	})
}

// updateMessage updates an existing message
func (m *Messages) updateMessage(messageID uint, target, content string, priority int, scheduledTime time.Time) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		// Get existing message
		var message database.Message
		err := m.db.Preload("Window").First(&message, messageID).Error
		if err != nil {
			return types.ErrorMsg{Title: "Message not found", Message: err.Error()}
		}

		// Prepare updates
		updates := make(map[string]interface{})
		updates["content"] = content
		updates["priority"] = priority
		updates["scheduled_time"] = scheduledTime

		// Handle target change if needed
		if target != message.Window.Target {
			window, err := database.GetTmuxWindow(m.db, target)
			if err != nil {
				return types.ErrorMsg{Title: "Invalid target", Message: fmt.Sprintf("Target '%s' not found", target)}
			}
			updates["window_id"] = window.ID
		}

		// Apply updates
		err = m.db.Model(&message).Updates(updates).Error
		if err != nil {
			return types.ErrorMsg{Title: "Failed to update message", Message: err.Error()}
		}

		// Data will refresh via SuccessMsg handling

		return types.SuccessMsg{
			Title:   "Message Updated",
			Message: fmt.Sprintf("Updated message #%d", messageID),
		}
	})
}

// deleteMessage deletes a message
func (m *Messages) deleteMessage(messageID uint) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		// Check if message exists and can be deleted
		var message database.Message
		err := m.db.First(&message, messageID).Error
		if err != nil {
			return types.ErrorMsg{Title: "Message not found", Message: err.Error()}
		}

		if message.Status == database.MessageStatusSent {
			return types.ErrorMsg{
				Title:   "Cannot Delete",
				Message: "Message has already been sent",
			}
		}

		// Delete message
		err = m.db.Delete(&message).Error
		if err != nil {
			return types.ErrorMsg{Title: "Failed to delete message", Message: err.Error()}
		}

		// Data will refresh via SuccessMsg handling

		return types.SuccessMsg{
			Title:   "Message Deleted",
			Message: fmt.Sprintf("Deleted message #%d", messageID),
		}
	})
}

// clearForm clears all form inputs
func (m *Messages) clearForm() {
	m.targetInput.SetValue("")
	m.contentInput.SetValue("")
	m.priorityInput.SetValue("")
	m.whenInput.SetValue("")
	m.editMode = false
	m.editingID = 0
}

// roundTimeToMinute rounds a time down to the nearest minute (ignores seconds)
func roundTimeToMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute)
}

// Helper functions - remove these as they're duplicated in other files
