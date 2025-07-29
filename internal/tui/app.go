package tui

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
	"gorm.io/gorm"

	"github.com/derekxwang/tcs/internal/config"
	"github.com/derekxwang/tcs/internal/database"
	"github.com/derekxwang/tcs/internal/discovery"
	"github.com/derekxwang/tcs/internal/monitor"
	"github.com/derekxwang/tcs/internal/scheduler"
	"github.com/derekxwang/tcs/internal/tmux"
	"github.com/derekxwang/tcs/internal/tui/views"
)

// App represents the main TUI application
type App struct {
	// Core components
	db              *gorm.DB
	tmuxClient      *tmux.Client
	usageMonitor    *monitor.UsageMonitor
	windowDiscovery *discovery.WindowDiscovery
	scheduler       *scheduler.Scheduler

	// TUI state
	currentView ViewType
	width       int
	height      int

	// Views
	dashboard     *views.Dashboard
	windows       *views.Windows
	messages      *views.Messages
	schedulerView *views.Scheduler

	// Key bindings
	keyMap KeyMap

	// Update ticker
	ticker *time.Ticker

	// Context for cleanup
	ctx    context.Context
	cancel context.CancelFunc
}

// ViewType represents different views in the TUI
type ViewType int

const (
	DashboardView ViewType = iota
	WindowsView
	MessagesView
	SchedulerView
	HelpView
)

// KeyMap defines key bindings for the TUI
type KeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	Tab       key.Binding
	Enter     key.Binding
	Escape    key.Binding
	Help      key.Binding
	Quit      key.Binding
	Dashboard key.Binding
	Windows   key.Binding
	Messages  key.Binding
	Scheduler key.Binding
	Refresh   key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "move left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "move right"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next section"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Dashboard: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "dashboard"),
		),
		Windows: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "windows"),
		),
		Messages: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "messages"),
		),
		Scheduler: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "scheduler"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// TickMsg represents a tick message for periodic updates
type TickMsg time.Time

// RefreshMsg represents a manual refresh request
type RefreshMsg struct{}

// ViewChangeMsg represents a view change request
type ViewChangeMsg ViewType

// NewApp creates a new TUI application
func NewApp(db *gorm.DB, tmuxClient *tmux.Client, usageMonitor *monitor.UsageMonitor,
	windowDiscovery *discovery.WindowDiscovery, schedulerInstance *scheduler.Scheduler) *App {

	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		db:              db,
		tmuxClient:      tmuxClient,
		usageMonitor:    usageMonitor,
		windowDiscovery: windowDiscovery,
		scheduler:       schedulerInstance,
		currentView:     DashboardView,
		keyMap:          DefaultKeyMap(),
		ctx:             ctx,
		cancel:          cancel,
	}

	// Initialize views
	app.dashboard = views.NewDashboard(db, usageMonitor, windowDiscovery, tmuxClient)
	app.windows = views.NewWindows(db, windowDiscovery, tmuxClient)
	app.messages = views.NewMessages(db, schedulerInstance, tmuxClient)
	app.schedulerView = views.NewScheduler(db, schedulerInstance, windowDiscovery)

	return app
}

// Init initializes the TUI application
func (a *App) Init() tea.Cmd {
	// Start refresh ticker
	cfg := config.Get()
	refreshRate := cfg.TUI.RefreshRate
	if refreshRate > 0 {
		a.ticker = time.NewTicker(refreshRate)

		// Start ticker goroutine
		go func() {
			for {
				select {
				case <-a.ctx.Done():
					return
				case t := <-a.ticker.C:
					// Send tick message to the program
					go func() {
						// Note: In a real implementation, you'd send this through the program
						// For now, we'll handle periodic updates in Update()
						_ = t
					}()
				}
			}
		}()
	}

	return tea.Batch(
		a.dashboard.Init(),
		a.windows.Init(),
		a.messages.Init(),
		a.schedulerView.Init(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return TickMsg(t)
		}),
	)
}

// Update handles messages and updates the application state
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

		// Update views with new size
		a.dashboard.SetSize(msg.Width, msg.Height-4) // Leave space for header/footer
		a.windows.SetSize(msg.Width, msg.Height-4)
		a.messages.SetSize(msg.Width, msg.Height-4)
		a.schedulerView.SetSize(msg.Width, msg.Height-4)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, a.keyMap.Quit):
			a.cleanup()
			return a, tea.Quit

		case key.Matches(msg, a.keyMap.Dashboard):
			a.currentView = DashboardView
			return a, nil

		case key.Matches(msg, a.keyMap.Windows):
			a.currentView = WindowsView
			return a, nil

		case key.Matches(msg, a.keyMap.Messages):
			a.currentView = MessagesView
			return a, nil

		case key.Matches(msg, a.keyMap.Scheduler):
			a.currentView = SchedulerView
			return a, nil

		case key.Matches(msg, a.keyMap.Refresh):
			cmds = append(cmds, func() tea.Msg {
				return RefreshMsg{}
			})
		}

	case TickMsg:
		// Periodic refresh
		cmds = append(cmds, func() tea.Msg {
			return RefreshMsg{}
		})
		cmds = append(cmds, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return TickMsg(t)
		}))

	case RefreshMsg:
		// Refresh all views
		cmds = append(cmds, a.dashboard.Refresh())
		cmds = append(cmds, a.windows.Refresh())
		cmds = append(cmds, a.messages.Refresh())
		cmds = append(cmds, a.schedulerView.Refresh())

	case ViewChangeMsg:
		a.currentView = ViewType(msg)
		return a, nil
	}

	// Update current view
	var cmd tea.Cmd
	switch a.currentView {
	case DashboardView:
		a.dashboard, cmd = a.dashboard.Update(msg)
		cmds = append(cmds, cmd)
	case WindowsView:
		a.windows, cmd = a.windows.Update(msg)
		cmds = append(cmds, cmd)
	case MessagesView:
		a.messages, cmd = a.messages.Update(msg)
		cmds = append(cmds, cmd)
	case SchedulerView:
		a.schedulerView, cmd = a.schedulerView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

// View renders the current view
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		// Fallback size if terminal dimensions aren't detected
		a.width = 120
		a.height = 30
		// Update views with fallback size
		a.dashboard.SetSize(a.width, a.height-4)
		a.windows.SetSize(a.width, a.height-4)
		a.messages.SetSize(a.width, a.height-4)
		a.schedulerView.SetSize(a.width, a.height-4)
	}

	// Header
	header := a.renderHeader()

	// Main content
	var content string
	switch a.currentView {
	case DashboardView:
		content = a.dashboard.View()
	case WindowsView:
		content = a.windows.View()
	case MessagesView:
		content = a.messages.View()
	case SchedulerView:
		content = a.schedulerView.View()
	default:
		content = "Unknown view"
	}

	// Footer
	footer := a.renderFooter()

	return fmt.Sprintf("%s\n%s\n%s", header, content, footer)
}

// renderHeader renders the application header
func (a *App) renderHeader() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("57")).
		Padding(0, 1).
		Width(a.width)

	var viewName string
	switch a.currentView {
	case DashboardView:
		viewName = "Dashboard"
	case WindowsView:
		viewName = "Windows"
	case MessagesView:
		viewName = "Messages"
	case SchedulerView:
		viewName = "Scheduler"
	default:
		viewName = "Unknown"
	}

	title := fmt.Sprintf("TCS (Tmux Claude Scheduler) - %s", viewName)
	return headerStyle.Render(title)
}

// renderFooter renders the application footer with key bindings
func (a *App) renderFooter() string {
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Background(lipgloss.Color("0")).
		Padding(0, 1).
		Width(a.width)

	help := "1: Dashboard  2: Windows  3: Messages  4: Scheduler  r: Refresh  ?: Help  q: Quit"
	return footerStyle.Render(help)
}

// cleanup performs cleanup when the app is shutting down
func (a *App) cleanup() {
	if a.ticker != nil {
		a.ticker.Stop()
	}
	if a.cancel != nil {
		a.cancel()
	}
}

// logRedirection holds the original log output for restoration
var originalLogOutput io.Writer

// redirectLogsToFile redirects all log output to a file during TUI mode
func redirectLogsToFile() (*os.File, error) {
	// Save original log output
	originalLogOutput = log.Writer()
	
	// Create logs directory if it doesn't exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	
	tcsDir := filepath.Join(homeDir, ".tcs")
	if err := os.MkdirAll(tcsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .tcs directory: %w", err)
	}
	
	// Create log file
	logFile := filepath.Join(tcsDir, "tui.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	
	// Redirect log output to file
	log.SetOutput(file)
	
	// Write a separator to indicate new TUI session
	log.Printf("=== TUI Session Started at %s ===", time.Now().Format(time.RFC3339))
	
	return file, nil
}

// restoreLogOutput restores the original log output
func restoreLogOutput() {
	if originalLogOutput != nil {
		log.SetOutput(originalLogOutput)
	}
}

// cleanupTerminal restores the terminal to its normal state
func cleanupTerminal() {
	// Comprehensive terminal cleanup sequences
	sequences := []string{
		"\033[?1000l",  // Disable X10 mouse mode
		"\033[?1002l",  // Disable button event tracking
		"\033[?1003l",  // Disable any-event tracking
		"\033[?1006l",  // Disable SGR extended mode
		"\033[?1015l",  // Disable urxvt mouse mode
		"\033[?1005l",  // Disable UTF-8 mouse mode
		"\033[?47l",    // Disable alternate screen (backup method)
		"\033[?1049l",  // Exit alternate screen (primary method)
		"\033[?2004l",  // Disable bracketed paste mode
		"\033[?25h",    // Show cursor
		"\033[0m",      // Reset all text attributes
		"\033[H",       // Move cursor to home position
		"\033[2J",      // Clear entire screen
	}
	
	// Apply all sequences
	for _, seq := range sequences {
		fmt.Print(seq)
	}
	
	// Force flush to all outputs
	os.Stdout.Sync()
	os.Stderr.Sync()
	
	// Nuclear option: reset terminal state via stty (ignore errors)
	cmd := exec.Command("stty", "sane")
	cmd.Run() // Ignore any errors from stty
}

// Run starts the TUI application
func Run() error {
	// Redirect logs to file to prevent TUI interference
	logFile, err := redirectLogsToFile()
	if err != nil {
		return fmt.Errorf("failed to redirect logs: %w", err)
	}
	defer func() {
		// Recover from any panics and restore terminal state
		if r := recover(); r != nil {
			// Restore terminal to normal state
			cleanupTerminal()
			
			restoreLogOutput()
			if logFile != nil {
				logFile.Close()
			}
			
			log.Printf("TUI crashed with panic: %v", r)
			fmt.Fprintf(os.Stderr, "\nTUI crashed. Check ~/.tcs/tui.log for details.\n")
			fmt.Fprintf(os.Stderr, "Terminal state has been restored.\n")
			panic(r) // Re-panic to show stack trace
		}
		
		restoreLogOutput()
		if logFile != nil {
			logFile.Close()
		}
	}()

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

	// Create window discovery service
	windowDiscovery := discovery.NewWindowDiscovery(database.GetDB(), tmuxClient, nil)
	if err := windowDiscovery.Start(); err != nil {
		return fmt.Errorf("failed to start window discovery: %w", err)
	}
	defer func() {
		if err := windowDiscovery.Stop(); err != nil {
			log.Printf("Warning: failed to stop window discovery: %v", err)
		}
	}()

	schedulerInstance := scheduler.NewScheduler(
		database.GetDB(),
		tmuxClient,
		usageMonitor,
		nil, // No callback needed for TUI
	)
	if err := schedulerInstance.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize scheduler: %w", err)
	}

	// Start the scheduler to process messages
	if err := schedulerInstance.Start(); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Create and run TUI app
	app := NewApp(database.GetDB(), tmuxClient, usageMonitor, windowDiscovery, schedulerInstance)

	// Try to get terminal size manually and set it
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		app.width = width
		app.height = height
		// Update views with initial size
		app.dashboard.SetSize(width, height-4)
		app.windows.SetSize(width, height-4)
		app.messages.SetSize(width, height-4)
		app.schedulerView.SetSize(width, height-4)
	}

	p := tea.NewProgram(app, 
		tea.WithAltScreen(),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stderr),
	)
	if _, err := p.Run(); err != nil {
		cleanupTerminal()
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	// Clean up terminal on normal exit
	cleanupTerminal()
	
	return nil
}
