package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/derekxwang/tcs/internal/types"
)

// MessageTable represents a table component for displaying messages
type MessageTable struct {
	table      table.Model
	messages   []types.MessageDisplayInfo
	width      int
	height     int
	showHeader bool
	showFooter bool

	// Styles
	headerStyle   lipgloss.Style
	footerStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	pendingStyle  lipgloss.Style
	sentStyle     lipgloss.Style
	failedStyle   lipgloss.Style
}

// MessageTableOptions holds configuration options for the message table
type MessageTableOptions struct {
	Width      int
	Height     int
	ShowHeader bool
	ShowFooter bool
	Focused    bool
	Theme      string
}

// MessageTableKeyMap defines key bindings for the message table
type MessageTableKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Select  key.Binding
	Delete  key.Binding
	SendNow key.Binding
	Cancel  key.Binding
	Refresh key.Binding
}

// DefaultMessageTableKeyMap returns the default message table key bindings
func DefaultMessageTableKeyMap() MessageTableKeyMap {
	return MessageTableKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d", "delete"),
			key.WithHelp("d", "delete"),
		),
		SendNow: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "send now"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "cancel"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// NewMessageTable creates a new message table component
func NewMessageTable(opts MessageTableOptions) *MessageTable {
	// Define table columns
	columns := []table.Column{
		{Title: "ID", Width: 4},
		{Title: "Session", Width: 12},
		{Title: "Content", Width: 30},
		{Title: "Priority", Width: 8},
		{Title: "Status", Width: 8},
		{Title: "Scheduled", Width: 16},
		{Title: "Until Send", Width: 12},
	}

	// Create table
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(opts.Focused),
		table.WithHeight(opts.Height),
	)

	mt := &MessageTable{
		table:      t,
		width:      opts.Width,
		height:     opts.Height,
		showHeader: opts.ShowHeader,
		showFooter: opts.ShowFooter,
	}

	mt.initStyles(opts.Theme)
	return mt
}

// initStyles initializes the component styles
func (mt *MessageTable) initStyles(theme string) {
	switch theme {
	case "dark":
		mt.initDarkTheme()
	case "light":
		mt.initLightTheme()
	default:
		mt.initDefaultTheme()
	}
}

// initDefaultTheme initializes the default theme
func (mt *MessageTable) initDefaultTheme() {
	mt.headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("57")).
		Padding(0, 1)

	mt.footerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 1)

	mt.selectedStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("57"))

	mt.pendingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")) // Yellow

	mt.sentStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")) // Green

	mt.failedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")) // Red
}

// initDarkTheme initializes the dark theme
func (mt *MessageTable) initDarkTheme() {
	mt.headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("240")).
		Padding(0, 1)

	mt.footerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Padding(0, 1)

	mt.selectedStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("240"))

	mt.pendingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")) // Orange

	mt.sentStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("34")) // Dark green

	mt.failedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")) // Bright red
}

// initLightTheme initializes the light theme
func (mt *MessageTable) initLightTheme() {
	mt.headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("252")).
		Padding(0, 1)

	mt.footerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)

	mt.selectedStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("240"))

	mt.pendingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("130")) // Dark orange

	mt.sentStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("28")) // Dark green

	mt.failedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("124")) // Dark red
}

// Update handles messages for the message table
func (mt *MessageTable) Update(msg tea.Msg) (*MessageTable, tea.Cmd) {
	var cmd tea.Cmd
	mt.table, cmd = mt.table.Update(msg)
	return mt, cmd
}

// View renders the message table
func (mt *MessageTable) View() string {
	var sections []string

	// Header
	if mt.showHeader {
		header := fmt.Sprintf("Messages (%d)", len(mt.messages))
		sections = append(sections, mt.headerStyle.Width(mt.width).Render(header))
	}

	// Table
	sections = append(sections, mt.table.View())

	// Footer with summary
	if mt.showFooter {
		sections = append(sections, mt.renderFooter())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderFooter renders the table footer with statistics
func (mt *MessageTable) renderFooter() string {
	if len(mt.messages) == 0 {
		return mt.footerStyle.Width(mt.width).Render("No messages")
	}

	// Count messages by status
	var pending, sent, failed int
	var nextSend *time.Time

	for _, msg := range mt.messages {
		switch msg.Status {
		case types.MessageStatusPending:
			pending++
			if nextSend == nil || msg.ScheduledTime.Before(*nextSend) {
				nextSend = &msg.ScheduledTime
			}
		case types.MessageStatusSent:
			sent++
		case types.MessageStatusFailed:
			failed++
		}
	}

	// Build footer text
	var footerParts []string

	if pending > 0 {
		footerParts = append(footerParts, mt.pendingStyle.Render(fmt.Sprintf("Pending: %d", pending)))
	}
	if sent > 0 {
		footerParts = append(footerParts, mt.sentStyle.Render(fmt.Sprintf("Sent: %d", sent)))
	}
	if failed > 0 {
		footerParts = append(footerParts, mt.failedStyle.Render(fmt.Sprintf("Failed: %d", failed)))
	}

	footerText := strings.Join(footerParts, " • ")

	// Add next send time if available
	if nextSend != nil && pending > 0 {
		timeUntil := time.Until(*nextSend)
		if timeUntil > 0 {
			nextText := fmt.Sprintf("Next: %s", timeUntil.Round(time.Second))
			footerText += " • " + nextText
		} else {
			footerText += " • Next: now"
		}
	}

	return mt.footerStyle.Width(mt.width).Render(footerText)
}

// SetMessages updates the messages displayed in the table
func (mt *MessageTable) SetMessages(messages []types.MessageDisplayInfo) {
	mt.messages = messages

	// Convert messages to table rows
	var rows []table.Row

	for _, msg := range messages {
		// Truncate content for display
		content := msg.Content
		if len(content) > 27 {
			content = content[:27] + "..."
		}

		// Format time until send
		timeUntilStr := "-"
		if msg.Status == types.MessageStatusPending {
			timeUntil := time.Until(msg.ScheduledTime)
			if timeUntil > 0 {
				timeUntilStr = timeUntil.Round(time.Second).String()
			} else {
				timeUntilStr = "now"
			}
		}

		// Create row with appropriate styling
		row := table.Row{
			strconv.Itoa(int(msg.ID)),
			msg.SessionName,
			content,
			strconv.Itoa(msg.Priority),
			mt.formatStatus(msg.Status),
			msg.ScheduledTime.Format("01-02 15:04"),
			timeUntilStr,
		}

		rows = append(rows, row)
	}

	mt.table.SetRows(rows)
}

// formatStatus formats the message status with appropriate styling
func (mt *MessageTable) formatStatus(status string) string {
	switch status {
	case types.MessageStatusPending:
		return mt.pendingStyle.Render("pending")
	case types.MessageStatusSent:
		return mt.sentStyle.Render("sent")
	case types.MessageStatusFailed:
		return mt.failedStyle.Render("failed")
	default:
		return status
	}
}

// GetSelectedMessage returns the currently selected message
func (mt *MessageTable) GetSelectedMessage() *types.MessageDisplayInfo {
	if len(mt.messages) == 0 {
		return nil
	}

	cursor := mt.table.Cursor()
	if cursor >= 0 && cursor < len(mt.messages) {
		return &mt.messages[cursor]
	}

	return nil
}

// GetSelectedIndex returns the index of the currently selected message
func (mt *MessageTable) GetSelectedIndex() int {
	return mt.table.Cursor()
}

// SetSize updates the table size
func (mt *MessageTable) SetSize(width, height int) {
	mt.width = width
	mt.height = height

	// Update table size
	mt.table.SetWidth(width)
	mt.table.SetHeight(height)

	// Adjust column widths based on available space
	columns := mt.table.Columns()
	if len(columns) > 0 && width > 50 {
		// Calculate available width for content column
		fixedWidth := 4 + 12 + 8 + 8 + 16 + 12 + 7 // Other columns + padding
		contentWidth := width - fixedWidth
		if contentWidth < 15 {
			contentWidth = 15
		}

		columns[2].Width = contentWidth // Content column
		mt.table.SetColumns(columns)
	}
}

// SetFocused sets the table focus state
func (mt *MessageTable) SetFocused(focused bool) {
	if focused {
		mt.table.Focus()
	} else {
		mt.table.Blur()
	}
}

// SetTheme updates the component theme
func (mt *MessageTable) SetTheme(theme string) {
	mt.initStyles(theme)
}

// FilterByStatus filters messages by status
func (mt *MessageTable) FilterByStatus(status string) {
	var filtered []types.MessageDisplayInfo

	for _, msg := range mt.messages {
		if status == "" || msg.Status == status {
			filtered = append(filtered, msg)
		}
	}

	mt.SetMessages(filtered)
}

// FilterBySession filters messages by session name
func (mt *MessageTable) FilterBySession(sessionName string) {
	var filtered []types.MessageDisplayInfo

	for _, msg := range mt.messages {
		if sessionName == "" || strings.Contains(strings.ToLower(msg.SessionName), strings.ToLower(sessionName)) {
			filtered = append(filtered, msg)
		}
	}

	mt.SetMessages(filtered)
}

// SortByPriority sorts messages by priority (high to low)
func (mt *MessageTable) SortByPriority() {
	// Create a copy and sort
	sorted := make([]types.MessageDisplayInfo, len(mt.messages))
	copy(sorted, mt.messages)

	// Simple bubble sort by priority (descending)
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j].Priority < sorted[j+1].Priority {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	mt.SetMessages(sorted)
}

// SortByScheduledTime sorts messages by scheduled time (earliest first)
func (mt *MessageTable) SortByScheduledTime() {
	// Create a copy and sort
	sorted := make([]types.MessageDisplayInfo, len(mt.messages))
	copy(sorted, mt.messages)

	// Simple bubble sort by scheduled time (ascending)
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j].ScheduledTime.After(sorted[j+1].ScheduledTime) {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	mt.SetMessages(sorted)
}

// GetStats returns statistics about the messages
func (mt *MessageTable) GetStats() map[string]int {
	stats := map[string]int{
		"total":   len(mt.messages),
		"pending": 0,
		"sent":    0,
		"failed":  0,
		"overdue": 0,
	}

	now := time.Now()
	for _, msg := range mt.messages {
		switch msg.Status {
		case types.MessageStatusPending:
			stats["pending"]++
			if msg.ScheduledTime.Before(now) {
				stats["overdue"]++
			}
		case types.MessageStatusSent:
			stats["sent"]++
		case types.MessageStatusFailed:
			stats["failed"]++
		}
	}

	return stats
}

// CompactMessageList creates a compact list view of messages
func CompactMessageList(messages []types.MessageDisplayInfo, width int, maxItems int) string {
	if len(messages) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No messages")
	}

	var lines []string
	displayCount := len(messages)
	if maxItems > 0 && displayCount > maxItems {
		displayCount = maxItems
	}

	for i := 0; i < displayCount; i++ {
		msg := messages[i]

		// Format line
		content := msg.Content
		if len(content) > 30 {
			content = content[:30] + "..."
		}

		// Choose color based on status
		var style lipgloss.Style
		switch msg.Status {
		case types.MessageStatusPending:
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		case types.MessageStatusSent:
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		case types.MessageStatusFailed:
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		default:
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
		}

		line := fmt.Sprintf("• %s [%s] %s", msg.SessionName, msg.Status, content)
		lines = append(lines, style.Render(line))
	}

	// Add "and X more" if truncated
	if len(messages) > displayCount {
		more := fmt.Sprintf("  ... and %d more", len(messages)-displayCount)
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(more))
	}

	return strings.Join(lines, "\n")
}

// MessageSummaryLine creates a one-line summary of messages
func MessageSummaryLine(messages []types.MessageDisplayInfo) string {
	if len(messages) == 0 {
		return "No messages"
	}

	stats := map[string]int{
		"pending": 0,
		"sent":    0,
		"failed":  0,
	}

	for _, msg := range messages {
		switch msg.Status {
		case types.MessageStatusPending:
			stats["pending"]++
		case types.MessageStatusSent:
			stats["sent"]++
		case types.MessageStatusFailed:
			stats["failed"]++
		}
	}

	var parts []string

	if stats["pending"] > 0 {
		part := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(
			fmt.Sprintf("%d pending", stats["pending"]))
		parts = append(parts, part)
	}

	if stats["sent"] > 0 {
		part := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(
			fmt.Sprintf("%d sent", stats["sent"]))
		parts = append(parts, part)
	}

	if stats["failed"] > 0 {
		part := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(
			fmt.Sprintf("%d failed", stats["failed"]))
		parts = append(parts, part)
	}

	total := fmt.Sprintf("Total: %d", len(messages))
	return fmt.Sprintf("%s (%s)", total, strings.Join(parts, ", "))
}
