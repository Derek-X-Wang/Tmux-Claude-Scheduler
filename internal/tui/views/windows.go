package views

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/discovery"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/types"
	"github.com/derekxwang/tcs/internal/utils"
)

// TmuxInterface defines the interface for tmux operations needed by Windows view
type TmuxInterface interface {
	IsRunning() bool
	ListSessions() ([]tmux.SessionInfo, error)
	CapturePane(target string, lines int) (string, error)
}

// Windows represents the window management view
type Windows struct {
	db              *gorm.DB
	windowDiscovery *discovery.WindowDiscovery
	tmuxClient      TmuxInterface

	// UI components
	windowsTable table.Model
	queueTable   table.Model

	// State
	width         int
	height        int
	activeTable   string // "windows", "queue"
	windows       []database.TmuxWindow
	sessionQueues map[string][]WindowQueueInfo

	// Key bindings
	keyMap WindowsKeyMap

	// Styles
	titleStyle    lipgloss.Style
	selectedStyle lipgloss.Style
	inactiveStyle lipgloss.Style
}

// WindowQueueInfo holds queue information grouped by session
type WindowQueueInfo struct {
	Window        database.TmuxWindow
	PendingCount  int
	QueuePriority int
}

// WindowsKeyMap defines key bindings for the windows view
type WindowsKeyMap struct {
	ScanWindows    key.Binding
	RefreshWindows key.Binding
	SwitchTable    key.Binding
	ToggleActive   key.Binding
	ChangePriority key.Binding
	ForceRescan    key.Binding
}

// DefaultWindowsKeyMap returns the default windows key bindings
func DefaultWindowsKeyMap() WindowsKeyMap {
	return WindowsKeyMap{
		ScanWindows: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "scan windows"),
		),
		RefreshWindows: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		SwitchTable: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch table"),
		),
		ToggleActive: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "toggle active"),
		),
		ChangePriority: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "change priority"),
		),
		ForceRescan: key.NewBinding(
			key.WithKeys("F"),
			key.WithHelp("F", "force rescan"),
		),
	}
}

// NewWindows creates a new windows view
func NewWindows(db *gorm.DB, windowDiscovery *discovery.WindowDiscovery, tmuxClient TmuxInterface) *Windows {
	// Create windows table
	windowsColumns := []table.Column{
		{Title: "Session", Width: 12},
		{Title: "Window", Width: 8},
		{Title: "Name", Width: 15},
		{Title: "Target", Width: 15},
		{Title: "Claude", Width: 8},
		{Title: "Priority", Width: 8},
		{Title: "Active", Width: 8},
		{Title: "Last Seen", Width: 16},
	}

	windowsTable := table.New(
		table.WithColumns(windowsColumns),
		table.WithFocused(true),
		table.WithHeight(12),
	)

	// Create queue table
	queueColumns := []table.Column{
		{Title: "Session", Width: 15},
		{Title: "Target", Width: 15},
		{Title: "Priority", Width: 8},
		{Title: "Pending", Width: 8},
		{Title: "Claude", Width: 8},
		{Title: "Status", Width: 10},
	}

	queueTable := table.New(
		table.WithColumns(queueColumns),
		table.WithFocused(false),
		table.WithHeight(10),
	)

	w := &Windows{
		db:              db,
		windowDiscovery: windowDiscovery,
		tmuxClient:      tmuxClient,
		windowsTable:    windowsTable,
		queueTable:      queueTable,
		activeTable:     "windows",
		sessionQueues:   make(map[string][]WindowQueueInfo),
		keyMap:          DefaultWindowsKeyMap(),
	}

	w.initStyles()
	return w
}

// initStyles initializes the windows view styles
func (w *Windows) initStyles() {
	w.titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		MarginBottom(1)

	w.selectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("57")).
		Bold(true)

	w.inactiveStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))
}

// Init initializes the windows view
func (w *Windows) Init() tea.Cmd {
	return w.refreshData()
}

// Update handles messages for the windows view
func (w *Windows) Update(msg tea.Msg) (*Windows, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
		w.updateTableSizes()

	case tea.KeyMsg:
		return w.handleKeys(msg)

	case types.RefreshDataMsg:
		if msg.Type == "all" || msg.Type == "windows" {
			if msg.Data != nil {
				// Handle window data refresh in main thread
				if windows, ok := msg.Data.([]database.TmuxWindow); ok {
					w.refreshWindowsWithData(windows)
					w.refreshQueuesWithData(windows)
				}
			} else {
				// Trigger new data fetch
				cmds = append(cmds, w.refreshData())
			}
		}

	case types.SuccessMsg:
		// After successful operations, trigger a refresh
		if msg.Title == "Force Rescan Complete" || msg.Title == "Window Updated" {
			cmds = append(cmds, w.refreshData())
		}
	}

	// Update tables
	if w.activeTable == "windows" {
		w.windowsTable, _ = w.windowsTable.Update(msg)
	} else if w.activeTable == "queue" {
		w.queueTable, _ = w.queueTable.Update(msg)
	}

	return w, tea.Batch(cmds...)
}

// handleKeys handles key presses
func (w *Windows) handleKeys(msg tea.KeyMsg) (*Windows, tea.Cmd) {
	var cmds []tea.Cmd

	switch {
	case key.Matches(msg, w.keyMap.ScanWindows):
		cmds = append(cmds, w.scanWindows())

	case key.Matches(msg, w.keyMap.RefreshWindows):
		cmds = append(cmds, w.refreshData())

	case key.Matches(msg, w.keyMap.SwitchTable):
		if w.activeTable == "windows" {
			w.activeTable = "queue"
			w.windowsTable.Blur()
			w.queueTable.Focus()
		} else {
			w.activeTable = "windows"
			w.queueTable.Blur()
			w.windowsTable.Focus()
		}

	case key.Matches(msg, w.keyMap.ToggleActive):
		if w.activeTable == "windows" && len(w.windows) > 0 {
			selected := w.windowsTable.Cursor()
			if selected < len(w.windows) {
				cmds = append(cmds, w.toggleWindowActive(w.windows[selected]))
			}
		}

	case key.Matches(msg, w.keyMap.ForceRescan):
		cmds = append(cmds, w.forceRescan())
	}

	return w, tea.Batch(cmds...)
}

// View renders the windows view
func (w *Windows) View() string {
	if w.width == 0 {
		return "Loading windows..."
	}

	var content []string

	// Windows table
	content = append(content, w.renderWindowsTable())

	// Queue table (grouped by session as requested)
	content = append(content, w.renderQueueTable())

	// Stats and help
	content = append(content, w.renderStats())
	content = append(content, w.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, content...)
}

// renderWindowsTable renders the windows table
func (w *Windows) renderWindowsTable() string {
	title := "Discovered Windows"
	if w.activeTable == "windows" {
		title = w.selectedStyle.Render("► " + title)
	} else {
		title = w.inactiveStyle.Render("  " + title)
	}

	return fmt.Sprintf("%s\n%s",
		w.titleStyle.Render(title),
		w.windowsTable.View())
}

// renderQueueTable renders the queue table (grouped by tmux session)
func (w *Windows) renderQueueTable() string {
	title := "Message Queues (by Tmux Session)"
	if w.activeTable == "queue" {
		title = w.selectedStyle.Render("► " + title)
	} else {
		title = w.inactiveStyle.Render("  " + title)
	}

	return fmt.Sprintf("\n%s\n%s",
		w.titleStyle.Render(title),
		w.queueTable.View())
}

// renderStats renders window and queue statistics
func (w *Windows) renderStats() string {
	totalWindows := len(w.windows)
	claudeWindows := 0
	activeWindows := 0
	totalPending := 0

	for _, window := range w.windows {
		if window.HasClaude {
			claudeWindows++
		}
		if window.Active {
			activeWindows++
		}
	}

	for _, queues := range w.sessionQueues {
		for _, queue := range queues {
			totalPending += queue.PendingCount
		}
	}

	stats := fmt.Sprintf(
		"\nWindows: %d total, %d active, %d with Claude | Messages: %d pending",
		totalWindows, activeWindows, claudeWindows, totalPending,
	)

	return w.inactiveStyle.Render(stats)
}

// renderHelp renders the help text
func (w *Windows) renderHelp() string {
	help := "\ns: Scan (discovery service)  F: Force Rescan (manual)  a: Toggle Active  tab: Switch Table  r: Refresh"
	return w.inactiveStyle.Render(help)
}

// SetSize sets the windows view size
func (w *Windows) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.updateTableSizes()
}

// Refresh refreshes the windows data
func (w *Windows) Refresh() tea.Cmd {
	return w.refreshData()
}

// updateTableSizes updates table sizes based on current dimensions
func (w *Windows) updateTableSizes() {
	tableWidth := w.width - 4

	// Update windows table
	w.windowsTable.SetWidth(tableWidth)
	w.windowsTable.SetHeight(min(12, w.height/3))

	// Update queue table
	w.queueTable.SetWidth(tableWidth)
	w.queueTable.SetHeight(min(10, w.height/3))
}

// refreshData refreshes all windows data
func (w *Windows) refreshData() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		// Add panic recovery
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in windows.refreshData: %v", r)
			}
		}()

		// Get ALL active windows, not just those with Claude
		windows, err := database.GetAllActiveTmuxWindows(w.db)
		if err != nil {
			return types.ErrorMsg{Title: "Refresh failed", Message: err.Error()}
		}

		// Return the data in the message so UI updates happen in main thread
		return types.RefreshDataMsg{
			Type: "windows",
			Data: windows,
		}
	})
}

// refreshWindowsWithData refreshes windows data using provided windows slice
func (w *Windows) refreshWindowsWithData(windows []database.TmuxWindow) {
	w.windows = windows
	var rows []table.Row

	for _, window := range windows {
		claudeStatus := "No"
		if window.HasClaude {
			claudeStatus = "Yes"
		}

		activeStatus := "No"
		if window.Active {
			activeStatus = "Yes"
		}

		lastSeen := window.LastSeen.Format("01-02 15:04")

		rows = append(rows, table.Row{
			window.SessionName,
			strconv.Itoa(window.WindowIndex),
			window.WindowName,
			window.Target,
			claudeStatus,
			strconv.Itoa(window.Priority),
			activeStatus,
			lastSeen,
		})
	}

	w.windowsTable.SetRows(rows)
}

// refreshQueuesWithData refreshes queue data using provided windows slice
func (w *Windows) refreshQueuesWithData(windows []database.TmuxWindow) {
	// Group by session and calculate pending messages
	w.sessionQueues = make(map[string][]WindowQueueInfo)
	var rows []table.Row

	for _, window := range windows {
		// Get pending message count for this window
		var pendingCount int64
		w.db.Model(&database.Message{}).
			Where("window_id = ? AND status = ?", window.ID, database.MessageStatusPending).
			Count(&pendingCount)

		// Get queue priority
		queue, err := database.GetOrCreateWindowMessageQueue(w.db, window.ID)
		queuePriority := 5 // default
		if err == nil {
			queuePriority = queue.Priority
		}

		queueInfo := WindowQueueInfo{
			Window:        window,
			PendingCount:  int(pendingCount),
			QueuePriority: queuePriority,
		}

		w.sessionQueues[window.SessionName] = append(w.sessionQueues[window.SessionName], queueInfo)

		// Add row for table
		claudeStatus := "No"
		if window.HasClaude {
			claudeStatus = "Yes"
		}

		status := "Active"
		if !window.Active {
			status = "Inactive"
		}

		rows = append(rows, table.Row{
			window.SessionName,
			window.Target,
			strconv.Itoa(queuePriority),
			strconv.Itoa(int(pendingCount)),
			claudeStatus,
			status,
		})
	}

	w.queueTable.SetRows(rows)
}

// scanWindows triggers a window scan with auto-refresh
func (w *Windows) scanWindows() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		if w.windowDiscovery == nil {
			return types.ErrorMsg{
				Title:   "Window Discovery Not Available",
				Message: "Window discovery service is not initialized. Use Force Rescan (F) instead.",
			}
		}

		if !w.windowDiscovery.IsRunning() {
			return types.ErrorMsg{
				Title:   "Window Discovery Not Running",
				Message: "Window discovery service is not running. Use Force Rescan (F) for manual scan.",
			}
		}

		if err := w.windowDiscovery.ForceRescan(); err != nil {
			return types.ErrorMsg{
				Title:   "Scan Failed",
				Message: fmt.Sprintf("Window discovery error: %v. Try Force Rescan (F) for manual scan.", err),
			}
		}

		// Sleep for 2 seconds to allow scan to complete
		time.Sleep(2 * time.Second)

		// Trigger a refresh after scan
		return types.RefreshDataMsg{
			Type: "windows",
			Data: nil,
		}
	})
}

// ForceRescan forces a complete rescan (public for testing)
func (w *Windows) ForceRescan() tea.Cmd {
	return w.forceRescan()
}

// forceRescan forces a complete rescan
func (w *Windows) forceRescan() tea.Cmd {
	return tea.Cmd(func() (result tea.Msg) {
		// Double-layer panic recovery - protect the entire command
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Panic in forceRescan tea.Cmd wrapper: %v\n", r)
				result = types.ErrorMsg{
					Title:   "Force Rescan Failed",
					Message: fmt.Sprintf("Command wrapper panic: %v", r),
				}
			}
		}()

		// Call the actual implementation
		return w.PerformForceRescan()
	})
}

// PerformForceRescan does the actual force rescan work with proper panic recovery
// Made public for testing purposes
func (w *Windows) PerformForceRescan() (result tea.Msg) {
	defer func() {
		if r := recover(); r != nil {
			// Log the panic for debugging and return error message
			fmt.Printf("Force rescan panic recovered: %v\n", r)
			result = types.ErrorMsg{
				Title:   "Force Rescan Failed",
				Message: fmt.Sprintf("Panic occurred during force rescan: %v", r),
			}
		}
	}()

	if w.tmuxClient == nil {
		return types.ErrorMsg{Title: "Error", Message: "Tmux client not available"}
	}

	if !w.tmuxClient.IsRunning() {
		return types.ErrorMsg{Title: "Error", Message: "Tmux server is not running"}
	}

	// Perform manual discovery
	sessions, err := w.tmuxClient.ListSessions()
	if err != nil {
		return types.ErrorMsg{Title: "Rescan failed", Message: err.Error()}
	}

	windowCount := 0
	claudeCount := 0

	for _, session := range sessions {
		for _, window := range session.Windows {
			// Detect Claude
			hasClaude := false
			content, err := w.tmuxClient.CapturePane(window.Target, 50)
			if err == nil {
				hasClaude = utils.IsClaudeWindow(content)
			}

			// Create or update window
			_, err = database.CreateOrUpdateTmuxWindow(
				w.db,
				window.SessionName,
				window.WindowIndex,
				window.WindowName,
				hasClaude,
			)
			if err != nil {
				continue
			}

			windowCount++
			if hasClaude {
				claudeCount++
			}
		}
	}

	// Sleep briefly to ensure database writes are complete
	time.Sleep(500 * time.Millisecond)

	return types.SuccessMsg{
		Title: "Force Rescan Complete",
		Message: fmt.Sprintf("Found %d windows (%d with Claude) across %d sessions. Data refreshed.",
			windowCount, claudeCount, len(sessions)),
	}
}

// toggleWindowActive toggles a window's active state
func (w *Windows) toggleWindowActive(window database.TmuxWindow) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		newActive := !window.Active

		err := w.db.Model(&window).Update("active", newActive).Error
		if err != nil {
			return types.ErrorMsg{
				Title:   "Failed to update window",
				Message: err.Error(),
			}
		}

		// Invalidate cache since window active state changed
		database.InvalidateActiveTmuxWindowsCache()

		status := "activated"
		if !newActive {
			status = "deactivated"
		}

		return types.SuccessMsg{
			Title:   "Window Updated",
			Message: fmt.Sprintf("Window %s %s", window.Target, status),
		}
	})
}
