package views

import (
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/config"
	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/discovery"
	"github.com/derekxwang/tcs/internal/monitor"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/types"
)

// Dashboard represents the main dashboard view
type Dashboard struct {
	db              *gorm.DB
	usageMonitor    *monitor.UsageMonitor
	windowDiscovery *discovery.WindowDiscovery
	tmuxClient      *tmux.Client

	// UI components
	usageProgress progress.Model

	// State
	width      int
	height     int
	state      *types.ApplicationState
	lastUpdate time.Time

	// Styles
	titleStyle   lipgloss.Style
	sectionStyle lipgloss.Style
	valueStyle   lipgloss.Style
	errorStyle   lipgloss.Style
	successStyle lipgloss.Style
	cardStyle    lipgloss.Style
}

// NewDashboard creates a new dashboard view
func NewDashboard(db *gorm.DB, usageMonitor *monitor.UsageMonitor,
	windowDiscovery *discovery.WindowDiscovery, tmuxClient *tmux.Client) *Dashboard {

	// Create progress bar for usage
	usageProgress := progress.New(progress.WithDefaultGradient())

	d := &Dashboard{
		db:              db,
		usageMonitor:    usageMonitor,
		windowDiscovery: windowDiscovery,
		tmuxClient:      tmuxClient,
		usageProgress:   usageProgress,
		state:           &types.ApplicationState{},
		lastUpdate:      time.Now(),
	}

	d.initStyles()
	return d
}

// initStyles initializes the dashboard styles
func (d *Dashboard) initStyles() {
	d.titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		MarginBottom(1)

	d.sectionStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		MarginTop(1).
		MarginBottom(1)

	d.valueStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("10"))

	d.errorStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("9"))

	d.successStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("10"))

	d.cardStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1).
		MarginRight(2).
		MarginBottom(1)
}

// Init initializes the dashboard
func (d *Dashboard) Init() tea.Cmd {
	return d.refreshData()
}

// Update handles messages for the dashboard
func (d *Dashboard) Update(msg tea.Msg) (*Dashboard, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		d.usageProgress.Width = min(msg.Width-10, 60)

	case types.RefreshDataMsg:
		if msg.Type == "all" || msg.Type == "usage" || msg.Type == "dashboard" {
			if msg.Data != nil {
				// Handle dashboard data refresh in main thread
				if dashboardData, ok := msg.Data.(map[string]interface{}); ok {
					d.updateStateWithData(dashboardData)
				}
			} else {
				// Trigger new data fetch
				cmds = append(cmds, d.refreshData())
			}
		}
	}

	// Update progress bar
	progressModel, cmd := d.usageProgress.Update(msg)
	if model, ok := progressModel.(progress.Model); ok {
		d.usageProgress = model
	}
	cmds = append(cmds, cmd)

	return d, tea.Batch(cmds...)
}

// IsFormActive returns true if the form is currently active (dashboard has no forms)
func (d *Dashboard) IsFormActive() bool {
	return false
}

// View renders the dashboard
func (d *Dashboard) View() string {
	if d.width == 0 {
		return "Loading dashboard..."
	}

	// Main content
	var sections []string

	// Usage overview section
	sections = append(sections, d.renderUsageOverview())

	// System status section
	sections = append(sections, d.renderSystemStatus())

	// Quick stats section
	sections = append(sections, d.renderQuickStats())

	// Recent activity section
	sections = append(sections, d.renderRecentActivity())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// SetSize sets the dashboard size
func (d *Dashboard) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.usageProgress.Width = min(width-10, 60)
}

// Refresh refreshes the dashboard data
func (d *Dashboard) Refresh() tea.Cmd {
	return d.refreshData()
}

// refreshData refreshes all dashboard data
func (d *Dashboard) refreshData() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		// Collect all data in goroutine without modifying UI state
		usageStats := d.collectUsageStats()
		windowStats := d.collectWindowStats()
		systemStats := d.collectSystemStats()
		schedulerStats := d.collectSchedulerStats()

		// Create dashboard data to pass to main thread (using individual components)
		dashboardData := map[string]interface{}{
			"usage":     usageStats,
			"windows":   windowStats,
			"system":    systemStats,
			"scheduler": schedulerStats,
		}

		return types.RefreshDataMsg{
			Type: "dashboard",
			Data: dashboardData,
		}
	})
}

// collectUsageStats collects usage statistics (thread-safe data collection)
func (d *Dashboard) collectUsageStats() types.UsageStats {
	if d.usageMonitor == nil {
		return types.UsageStats{
			MessagesUsed:    0,
			MessagesLimit:   config.Get().Usage.MaxMessages,
			UsagePercentage: 0,
			CanSendMessage:  false,
		}
	}

	stats, err := d.usageMonitor.GetCurrentStats()
	if err != nil {
		return types.UsageStats{
			MessagesUsed:    0,
			MessagesLimit:   config.Get().Usage.MaxMessages,
			UsagePercentage: 0,
			CanSendMessage:  false,
		}
	}

	return types.UsageStats{
		MessagesUsed:    stats.MessagesUsed,
		MessagesLimit:   stats.MessageLimit, // Use dynamic limit instead of config
		TokensUsed:      stats.TokensUsed,
		TokensLimit:     stats.TokenLimit, // Use dynamic limit instead of config
		CostUsed:        stats.CostUsed,
		CostLimit:       stats.CostLimit,
		UsagePercentage: stats.UsagePercentage,
		TimeRemaining:   stats.TimeRemaining,
		WindowStartTime: stats.WindowStartTime,
		WindowEndTime:   stats.WindowEndTime,
		CanSendMessage:  stats.CanSendMessage,
		CurrentWindow:   stats.CurrentWindow,
		DynamicLimits:   stats.DynamicLimits,
		WindowsActive:   stats.WindowsActive,
	}
}

// collectWindowStats collects window statistics (thread-safe data collection)
func (d *Dashboard) collectWindowStats() map[string]interface{} {
	// Get window statistics
	windows, err := database.GetActiveTmuxWindows(d.db)
	if err != nil {
		return map[string]interface{}{
			"sessions":       []types.SessionDisplayInfo{},
			"totalSessions":  0,
			"activeSessions": 0,
		}
	}

	// Group windows by session for display
	sessionGroups := make(map[string][]database.TmuxWindow)
	for _, window := range windows {
		sessionGroups[window.SessionName] = append(sessionGroups[window.SessionName], window)
	}

	// Convert to display format
	sessions := make([]types.SessionDisplayInfo, 0, len(sessionGroups))
	activeSessions := 0

	for sessionName, sessionWindows := range sessionGroups {
		// Calculate aggregated stats for this session
		totalPending := 0
		hasActiveWindows := false
		var lastActivity *time.Time

		for _, window := range sessionWindows {
			if window.Active {
				hasActiveWindows = true
			}

			// Count pending messages for this window
			var pendingCount int64
			d.db.Model(&database.Message{}).
				Where("window_id = ? AND status = ?", window.ID, database.MessageStatusPending).
				Count(&pendingCount)
			totalPending += int(pendingCount)

			// Track most recent activity
			if window.LastActivity != nil {
				if lastActivity == nil || window.LastActivity.After(*lastActivity) {
					lastActivity = window.LastActivity
				}
			}
		}

		if hasActiveWindows {
			activeSessions++
		}

		status := types.SessionStatusInactive
		if hasActiveWindows {
			status = types.SessionStatusActive
		}

		// Create display info for this session group
		displayInfo := types.SessionDisplayInfo{
			ID:           0, // Not applicable for window groups
			Name:         sessionName,
			TmuxTarget:   fmt.Sprintf("%s:*", sessionName), // Indicates multiple windows
			Priority:     5,                                // Default, could calculate average
			Active:       hasActiveWindows,
			MessageCount: totalPending,
			TokensUsed:   0, // Not tracked at window level
			LastActivity: lastActivity,
			StartTime:    nil, // Not applicable
			EndTime:      nil, // Not applicable
			Status:       status,
		}

		sessions = append(sessions, displayInfo)
	}

	return map[string]interface{}{
		"sessions":       sessions,
		"totalSessions":  len(sessionGroups),
		"activeSessions": activeSessions,
	}
}

// collectSystemStats collects system statistics (thread-safe data collection)
func (d *Dashboard) collectSystemStats() types.SystemInfo {
	// Tmux status
	tmuxRunning := d.tmuxClient.IsRunning()
	var tmuxSessions []types.TmuxSessionInfo

	if tmuxRunning {
		if sessions, err := d.tmuxClient.ListSessions(); err == nil {
			tmuxSessions = make([]types.TmuxSessionInfo, len(sessions))
			for i, session := range sessions {
				windows := make([]types.TmuxWindowInfo, len(session.Windows))
				for j, window := range session.Windows {
					windows[j] = types.TmuxWindowInfo{
						SessionName: window.SessionName,
						WindowIndex: window.WindowIndex,
						WindowName:  window.WindowName,
						Active:      window.Active,
						Target:      window.Target,
					}
				}
				tmuxSessions[i] = types.TmuxSessionInfo{
					Name:      session.Name,
					Attached:  session.Attached,
					Windows:   windows,
					Connected: true,
				}
			}
		}
	}

	return types.SystemInfo{
		TmuxRunning:       tmuxRunning,
		TmuxSessions:      tmuxSessions,
		DatabaseConnected: d.db != nil,
		DatabasePath:      config.Get().Database.Path,
		LastRefresh:       time.Now(),
	}
}

// collectSchedulerStats collects scheduler statistics (thread-safe data collection)
func (d *Dashboard) collectSchedulerStats() map[string]interface{} {
	cfg := config.Get()

	var pending, sent, failed int64
	// Get message counts from database
	if d.db != nil {
		d.db.Model(&database.Message{}).Where("status = ?", "pending").Count(&pending)
		d.db.Model(&database.Message{}).Where("status = ?", "sent").Count(&sent)
		d.db.Model(&database.Message{}).Where("status = ?", "failed").Count(&failed)
	}

	return map[string]interface{}{
		"schedulerStats": types.SchedulerStats{
			SmartSchedulerEnabled: cfg.Scheduler.SmartEnabled,
			CronSchedulerEnabled:  cfg.Scheduler.CronEnabled,
			PendingMessages:       int(pending),
			SentMessages:          int(sent),
			FailedMessages:        int(failed),
		},
		"databaseStats": types.DatabaseStats{
			PendingMessages: int(pending),
			SentMessages:    int(sent),
			FailedMessages:  int(failed),
			TotalMessages:   int(pending + sent + failed),
		},
	}
}

// updateStateWithData updates the dashboard state with collected data (called from main thread)
func (d *Dashboard) updateStateWithData(dashboardData map[string]interface{}) {
	// Update usage stats
	if usageStats, ok := dashboardData["usage"].(types.UsageStats); ok {
		d.state.Usage = usageStats
	}

	// Update window stats
	if windowStats, ok := dashboardData["windows"].(map[string]interface{}); ok {
		if sessions, ok := windowStats["sessions"].([]types.SessionDisplayInfo); ok {
			d.state.Sessions = sessions
		}
		if totalSessions, ok := windowStats["totalSessions"].(int); ok {
			d.state.Database.TotalSessions = totalSessions
		}
		if activeSessions, ok := windowStats["activeSessions"].(int); ok {
			d.state.Database.ActiveSessions = activeSessions
		}
	}

	// Update system stats
	if systemStats, ok := dashboardData["system"].(types.SystemInfo); ok {
		d.state.System = systemStats
	}

	// Update scheduler stats
	if schedulerData, ok := dashboardData["scheduler"].(map[string]interface{}); ok {
		if schedulerStats, ok := schedulerData["schedulerStats"].(types.SchedulerStats); ok {
			d.state.Scheduler = schedulerStats
		}
		if dbStats, ok := schedulerData["databaseStats"].(types.DatabaseStats); ok {
			d.state.Database.PendingMessages = dbStats.PendingMessages
			d.state.Database.SentMessages = dbStats.SentMessages
			d.state.Database.FailedMessages = dbStats.FailedMessages
			d.state.Database.TotalMessages = dbStats.TotalMessages
		}
	}

	// Update last refresh time
	d.lastUpdate = time.Now()
}

// renderUsageOverview renders the usage overview section matching Claude Monitor format
func (d *Dashboard) renderUsageOverview() string {
	usage := d.state.Usage

	// Main title
	var title string
	if usage.DynamicLimits {
		title = "📊 Session-Based Dynamic Limits\nBased on your historical usage patterns when hitting limits (P90)"
	} else {
		title = "📊 Usage Overview"
	}

	// Calculate individual percentages for color coding
	messagePercentage := 0.0
	tokenPercentage := 0.0
	costPercentage := 0.0

	if usage.MessagesLimit > 0 {
		messagePercentage = float64(usage.MessagesUsed) / float64(usage.MessagesLimit)
	}
	if usage.TokensLimit > 0 {
		tokenPercentage = float64(usage.TokensUsed) / float64(usage.TokensLimit)
	}
	if usage.CostLimit > 0 {
		costPercentage = usage.CostUsed / usage.CostLimit
	}

	// Helper function for color coding and icons
	getUsageColor := func(percentage float64) (string, string) {
		if percentage >= 0.75 {
			return "🔴", "9" // Red
		} else if percentage >= 0.5 {
			return "🟡", "11" // Yellow
		}
		return "🟢", "10" // Green
	}

	// Helper function for clamping float64 values to 1.0
	clampToOne := func(value float64) float64 {
		if value > 1.0 {
			return 1.0
		}
		return value
	}

	// Cost usage line (most important)
	costIcon, costColor := getUsageColor(costPercentage)
	costStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(costColor))
	costBar := d.usageProgress.ViewAs(clampToOne(costPercentage))
	costLine := fmt.Sprintf("💰 Cost Usage:           %s [%s] %.1f%%    $%.2f / $%.2f",
		costIcon, costBar, costPercentage*100, usage.CostUsed, usage.CostLimit)

	// Token usage line
	tokenIcon, tokenColor := getUsageColor(tokenPercentage)
	tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(tokenColor))
	tokenBar := d.usageProgress.ViewAs(clampToOne(tokenPercentage))
	tokenLine := fmt.Sprintf("📊 Token Usage:          %s [%s] %.1f%%    %s / %s",
		tokenIcon, tokenBar, tokenPercentage*100,
		d.formatNumber(usage.TokensUsed), d.formatNumber(usage.TokensLimit))

	// Message usage line
	messageIcon, messageColor := getUsageColor(messagePercentage)
	messageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(messageColor))
	messageBar := d.usageProgress.ViewAs(clampToOne(messagePercentage))
	messageLine := fmt.Sprintf("📨 Messages Usage:       %s [%s] %.1f%%    %d / %d",
		messageIcon, messageBar, messagePercentage*100, usage.MessagesUsed, usage.MessagesLimit)

	// Time remaining
	timeUsed := 1.0 - (float64(usage.TimeRemaining.Seconds()) / (5 * time.Hour).Seconds())
	timeIcon, timeColor := getUsageColor(timeUsed)
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(timeColor))
	timeBar := d.usageProgress.ViewAs(clampToOne(timeUsed))
	timeLine := fmt.Sprintf("⏱️  Time to Reset:       %s [%s] %s",
		timeIcon, timeBar, usage.TimeRemaining.Round(time.Minute).String())

	// Status message
	var statusColor lipgloss.Color = "10" // Green
	var statusText = "✓ Can send messages"
	if !usage.CanSendMessage {
		statusColor = "9" // Red
		statusText = "✗ Cannot send messages"
	}
	statusStyle := lipgloss.NewStyle().Bold(true).Foreground(statusColor)

	content := fmt.Sprintf(
		"%s\n────────────────────────────────────────────────────────────\n%s\n\n%s\n\n%s\n────────────────────────────────────────────────────────────\n%s\n\nWindow: %s - %s\nStatus: %s",
		d.sectionStyle.Render(title),
		costStyle.Render(costLine),
		tokenStyle.Render(tokenLine),
		messageStyle.Render(messageLine),
		timeStyle.Render(timeLine),
		d.valueStyle.Render(usage.WindowStartTime.Format("15:04")),
		d.valueStyle.Render(usage.WindowEndTime.Format("15:04")),
		statusStyle.Render(statusText),
	)

	return d.cardStyle.Width(d.width - 4).Render(content)
}

// formatNumber formats large numbers with commas for readability
func (d *Dashboard) formatNumber(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%s", humanizeNumber(n))
	}
	return strconv.Itoa(n)
}

// humanizeNumber converts large numbers to human readable format
func humanizeNumber(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%s", addCommas(n))
	}
	return strconv.Itoa(n)
}

// addCommas adds commas to numbers for readability
func addCommas(n int) string {
	str := strconv.Itoa(n)
	length := len(str)
	if length <= 3 {
		return str
	}

	result := ""
	for i, char := range str {
		if i > 0 && (length-i)%3 == 0 {
			result += ","
		}
		result += string(char)
	}
	return result
}

// renderSystemStatus renders the system status section
func (d *Dashboard) renderSystemStatus() string {
	system := d.state.System

	// Tmux status
	tmuxStatus := "✗ Disconnected"
	tmuxColor := lipgloss.Color("9")
	if system.TmuxRunning {
		tmuxStatus = fmt.Sprintf("✓ Connected (%d sessions)", len(system.TmuxSessions))
		tmuxColor = lipgloss.Color("10")
	}

	// Database status
	dbStatus := "✗ Disconnected"
	dbColor := lipgloss.Color("9")
	if system.DatabaseConnected {
		dbStatus = "✓ Connected"
		dbColor = lipgloss.Color("10")
	}

	content := fmt.Sprintf(
		"%s\n\nTmux: %s\nDatabase: %s\nConfig: %s\nLast Refresh: %s",
		d.sectionStyle.Render("🔧 System Status"),
		lipgloss.NewStyle().Foreground(tmuxColor).Render(tmuxStatus),
		lipgloss.NewStyle().Foreground(dbColor).Render(dbStatus),
		d.valueStyle.Render(system.ConfigPath),
		d.valueStyle.Render(system.LastRefresh.Format("15:04:05")),
	)

	return d.cardStyle.Width(d.width/2 - 2).Render(content)
}

// renderQuickStats renders the quick stats section
func (d *Dashboard) renderQuickStats() string {
	db := d.state.Database
	scheduler := d.state.Scheduler

	content := fmt.Sprintf(
		"%s\n\nSessions: %s\nMessages: %s\nPending: %s\nSchedulers: %s",
		d.sectionStyle.Render("📈 Quick Stats"),
		d.valueStyle.Render(fmt.Sprintf("%d total, %d active", db.TotalSessions, db.ActiveSessions)),
		d.valueStyle.Render(fmt.Sprintf("%d total (%d sent, %d failed)",
			db.TotalMessages, db.SentMessages, db.FailedMessages)),
		d.valueStyle.Render(strconv.Itoa(scheduler.PendingMessages)),
		d.renderSchedulerStatus(scheduler),
	)

	return d.cardStyle.Width(d.width/2 - 2).Render(content)
}

// renderRecentActivity renders the recent activity section
func (d *Dashboard) renderRecentActivity() string {
	content := fmt.Sprintf(
		"%s\n\n%s\n%s\n%s",
		d.sectionStyle.Render("⚡ Recent Activity"),
		"• Dashboard refreshed at "+d.lastUpdate.Format("15:04:05"),
		"• Monitoring "+strconv.Itoa(d.state.Database.ActiveSessions)+" active sessions",
		"• Processing "+strconv.Itoa(d.state.Scheduler.PendingMessages)+" pending messages",
	)

	return d.cardStyle.Width(d.width - 4).Render(content)
}

// renderSchedulerStatus renders scheduler status
func (d *Dashboard) renderSchedulerStatus(scheduler types.SchedulerStats) string {
	var status []string

	if scheduler.SmartSchedulerEnabled {
		status = append(status, "Smart")
	}
	if scheduler.CronSchedulerEnabled {
		status = append(status, "Cron")
	}

	if len(status) == 0 {
		return d.errorStyle.Render("None")
	}

	return d.successStyle.Render(fmt.Sprintf("%s enabled",
		lipgloss.JoinHorizontal(lipgloss.Left, status...)))
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
