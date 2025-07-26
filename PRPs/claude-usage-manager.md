name: "Claude Usage Manager CLI - Go TUI for Maximizing Claude Subscription"
description: |

## Purpose
Build a Go CLI tool with TUI interface that monitors Claude subscription usage, schedules messages to tmux windows, and manages multiple sessions to maximize Claude subscription utilization during 5-hour usage windows.

## Core Principles
1. **Context is King**: Include ALL necessary documentation, examples, and caveats
2. **Validation Loops**: Provide executable tests/lints the AI can run and fix
3. **Information Dense**: Use keywords and patterns from the codebase
4. **Progressive Success**: Start simple, validate, then enhance
5. **Global rules**: Be sure to follow all rules in CLAUDE.md

---

## Goal
Build a Golang CLI tool that:
- Monitors Claude subscription usage (5-hour windows from first message)
- Schedules commands to send to tmux windows via send-keys
- Manages multiple sessions with message queues
- Provides smart auto-scheduling based on available usage
- Offers a beautiful TUI interface for monitoring and control

## Why
- **Maximize Claude subscription value**: Users can't stay up until midnight to use reset usage
- **Automate workflow**: Schedule messages for different projects to work continuously
- **Multi-project support**: Work on multiple projects simultaneously without manual coordination
- **Usage optimization**: Smart scheduling ensures no usage is wasted

## What
A Go CLI application with:
- Real-time usage monitoring with TUI dashboard
- Message scheduling (exact time and smart priority-based)
- Tmux integration for sending messages to Claude instances
- SQLite database for persistence
- Configuration management
- Usage prediction based on historical data

### Success Criteria
- [ ] Monitor Claude usage with 5-hour window tracking
- [ ] Send scheduled messages to specific tmux windows
- [ ] Support multiple concurrent sessions with separate queues
- [ ] Provide smart scheduling based on usage availability
- [ ] Display real-time usage in beautiful TUI
- [ ] Persist data across restarts
- [ ] Handle errors gracefully (tmux disconnection, etc.)

## All Needed Context

### Documentation & References
```yaml
# MUST READ - Include these in your context window
- url: https://github.com/charmbracelet/bubbletea
  why: Bubble Tea TUI framework documentation, Model-Update-View pattern
  
- url: https://github.com/charmbracelet/bubbles
  why: Pre-built TUI components (tables, progress bars, viewports)

- url: https://github.com/charmbracelet/lipgloss
  why: Styling library for TUI components
  
- url: https://github.com/jubnzv/go-tmux
  why: Comprehensive tmux integration library for Go
  
- url: https://gorm.io/docs/index.html
  why: GORM documentation for SQLite integration
  section: Connecting to Database, Models, Query, Performance

- url: https://github.com/glebarez/sqlite
  why: Pure-Go SQLite driver (no CGO required)
  
- url: https://github.com/robfig/cron/v3
  why: Cron scheduling library for exact-time scheduling
  
- file: examples/Claude-Code-Usage-Monitor/src/claude_monitor/monitoring/session_monitor.py
  why: Reference implementation for session monitoring logic
  
- file: examples/Tmux-Orchestrator/send-claude-message.sh
  why: Reference for sending messages to tmux (0.5s delay critical)
  
- file: examples/Tmux-Orchestrator/tmux_utils.py
  why: Tmux interaction patterns and window management

- url: https://github.com/spf13/viper
  why: Configuration management library
  
- url: https://pkg.go.dev/github.com/robfig/cron/v3
  why: Cron v3 documentation for scheduling
```

### Current Codebase tree
```bash
/Users/derekxwang/Development/tools/tcs/
├── INITIAL.md
├── PRPs/
│   ├── EXAMPLE_multi_agent_prp.md
│   ├── prp_base.md
│   └── templates/
│       └── prp_base.md
└── examples/
    ├── Claude-Code-Usage-Monitor/   # Python TUI for monitoring
    └── Tmux-Orchestrator/          # Bash/Python tmux integration
```

### Desired Codebase tree with files to be added
```bash
/Users/derekxwang/Development/tools/tcs/
├── go.mod                      # Go module definition
├── go.sum                      # Go dependencies
├── main.go                     # Entry point
├── config.yaml                 # Default configuration
├── README.md                   # Documentation
├── Makefile                    # Build and test commands
├── cmd/
│   └── root.go                # Cobra CLI setup
├── internal/
│   ├── config/
│   │   └── config.go          # Viper configuration management
│   ├── database/
│   │   ├── models.go          # GORM models
│   │   └── db.go              # Database connection and setup
│   ├── monitor/
│   │   ├── usage.go           # Usage tracking logic
│   │   └── session.go         # Session management
│   ├── scheduler/
│   │   ├── scheduler.go       # Main scheduler logic
│   │   ├── cron.go            # Exact-time scheduling
│   │   └── smart.go           # Smart priority-based scheduling
│   ├── tmux/
│   │   ├── client.go          # Tmux client wrapper
│   │   └── message.go         # Message sending logic
│   └── tui/
│       ├── app.go             # Main Bubble Tea app
│       ├── models.go          # TUI data models
│       ├── views/
│       │   ├── dashboard.go   # Main dashboard view
│       │   ├── sessions.go    # Session management view
│       │   └── scheduler.go   # Scheduler configuration view
│       └── components/
│           ├── usage_bar.go   # Usage progress bars
│           └── message_table.go # Message queue table
└── tests/
    ├── scheduler_test.go
    ├── tmux_test.go
    └── monitor_test.go
```

### Known Gotchas & Library Quirks
```go
// CRITICAL: Bubble Tea requires specific initialization pattern
// Example: Must return tea.Cmd from Init() and Update()

// CRITICAL: Tmux message sending requires 0.5s delay between message and Enter
// Example: send message, sleep 500ms, send Enter key

// CRITICAL: SQLite with pure-Go driver for easy deployment
// Example: Use github.com/glebarez/sqlite instead of standard driver

// CRITICAL: GORM auto-migration only adds columns, doesn't remove
// Example: Handle schema changes carefully in production

// CRITICAL: Cron v3 uses 5-field syntax by default (no seconds)
// Example: Use cron.New(cron.WithSeconds()) for 6-field syntax

// CRITICAL: Claude sessions start counting from FIRST message
// Example: Track session start time when first message is sent
```

## Implementation Blueprint

### Data models and structure

```go
// internal/database/models.go
package database

import (
    "time"
    "gorm.io/gorm"
)

// Session represents a Claude chat session
type Session struct {
    gorm.Model
    Name           string    `gorm:"uniqueIndex"`
    TmuxTarget     string    // e.g., "project:0"
    StartTime      *time.Time
    EndTime        *time.Time
    MessageCount   int
    TokensUsed     int
    Active         bool
}

// Message represents a scheduled message
type Message struct {
    gorm.Model
    SessionID      uint
    Session        Session
    Content        string
    ScheduledTime  time.Time
    Priority       int       // 1-10, higher = more important
    Status         string    // pending, sent, failed
    Error          string
    SentAt         *time.Time
}

// UsageWindow tracks 5-hour usage windows
type UsageWindow struct {
    gorm.Model
    StartTime      time.Time
    EndTime        time.Time
    TotalMessages  int
    TotalTokens    int
    SessionCount   int
}

// Configuration for the app
type Config struct {
    RefreshRate    int      // TUI refresh rate in ms
    MaxSessions    int      // Max concurrent sessions
    DefaultPriority int     // Default message priority
    DatabasePath   string   // SQLite file path
    LogLevel       string   // debug, info, warn, error
}
```

### List of tasks to be completed

```yaml
Task 1: Initialize Go module and install dependencies
CREATE go.mod:
  - module github.com/user/claude-usage-manager
  - require all necessary dependencies
  
CREATE Makefile:
  - build, test, run, clean targets
  - lint and format commands

Task 2: Set up database layer with GORM
CREATE internal/database/db.go:
  - Initialize SQLite connection with pure-Go driver
  - Auto-migrate models
  - Connection pool configuration
  
CREATE internal/database/models.go:
  - Define all GORM models with proper indexes
  - Add validation tags

Task 3: Implement tmux integration
CREATE internal/tmux/client.go:
  - Wrapper around go-tmux library
  - List sessions/windows functionality
  - Error handling for disconnected sessions

CREATE internal/tmux/message.go:
  - SendMessage function with 500ms delay
  - Verify window exists before sending
  - Return confirmation of delivery

Task 4: Build usage monitoring system
CREATE internal/monitor/usage.go:
  - Track 5-hour windows from first message
  - Calculate remaining time/usage
  - Historical data analysis

CREATE internal/monitor/session.go:
  - Monitor active sessions
  - Detect session start (first message)
  - Update usage statistics

Task 5: Implement scheduling system
CREATE internal/scheduler/scheduler.go:
  - Main scheduler loop
  - Process message queue
  - Handle failures and retries

CREATE internal/scheduler/cron.go:
  - Exact-time scheduling with robfig/cron
  - Parse cron expressions
  - Schedule message delivery

CREATE internal/scheduler/smart.go:
  - Priority queue implementation
  - Usage-based scheduling algorithm
  - Load balancing across sessions

Task 6: Build TUI with Bubble Tea
CREATE internal/tui/app.go:
  - Main Bubble Tea application
  - Model with all app state
  - Update function for events
  - View function for rendering

CREATE internal/tui/views/dashboard.go:
  - Main dashboard with usage overview
  - Real-time updates
  - Session status display

CREATE internal/tui/views/sessions.go:
  - Session management interface
  - Add/remove sessions
  - Configure tmux targets

CREATE internal/tui/views/scheduler.go:
  - Message scheduling interface
  - Queue visualization
  - Priority adjustment

Task 7: Create CLI with Cobra
CREATE cmd/root.go:
  - Main command setup
  - Global flags (config, debug)
  - Subcommands structure

CREATE main.go:
  - Entry point
  - Initialize and execute root command

Task 8: Add configuration management
CREATE internal/config/config.go:
  - Viper setup
  - Load from file and environment
  - Default values

CREATE config.yaml:
  - Default configuration values
  - Documentation for each option

Task 9: Implement tests
CREATE tests/*_test.go:
  - Unit tests for each component
  - Mock tmux for testing
  - Scheduler reliability tests

Task 10: Documentation and polish
CREATE README.md:
  - Installation instructions
  - Usage examples
  - Configuration guide
```

### Per task pseudocode

```go
// Task 3: Tmux integration
// internal/tmux/message.go
package tmux

import (
    "time"
    "github.com/jubnzv/go-tmux"
)

func SendMessage(target, message string) error {
    // PATTERN: Parse target (session:window)
    session, window := parseTarget(target)
    
    // GOTCHA: Verify window exists first
    if !windowExists(session, window) {
        return ErrWindowNotFound
    }
    
    // CRITICAL: Send message with proper delay
    server := tmux.NewServer()
    if err := server.SendKeys(session, window, message); err != nil {
        return err
    }
    
    // CRITICAL: 500ms delay before Enter
    time.Sleep(500 * time.Millisecond)
    
    // Send Enter key
    return server.SendKeys(session, window, "Enter")
}

// Task 5: Smart scheduling
// internal/scheduler/smart.go
package scheduler

import (
    "container/heap"
    "time"
)

func (s *SmartScheduler) processQueue() {
    for {
        // Check available usage
        available := s.monitor.GetAvailableUsage()
        if available <= 0 {
            time.Sleep(1 * time.Minute)
            continue
        }
        
        // Get highest priority message
        if s.queue.Len() > 0 {
            msg := heap.Pop(&s.queue).(*Message)
            
            // Find least recently used session
            session := s.findLRUSession()
            
            // Send message
            if err := s.tmux.SendMessage(session.TmuxTarget, msg.Content); err != nil {
                msg.Status = "failed"
                msg.Error = err.Error()
            } else {
                msg.Status = "sent"
                msg.SentAt = timePtr(time.Now())
                session.MessageCount++
            }
            
            s.db.Save(&msg)
            s.db.Save(&session)
        }
        
        time.Sleep(10 * time.Second)
    }
}

// Task 6: TUI Dashboard
// internal/tui/app.go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/table"
)

type model struct {
    sessions      []Session
    messages      []Message
    usageWindows  []UsageWindow
    messageTable  table.Model
    width, height int
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "tab":
            m.activeView = (m.activeView + 1) % 3
        }
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
    case tickMsg:
        // Update data from database
        m.sessions = m.db.GetActiveSessions()
        m.messages = m.db.GetPendingMessages()
        return m, tick()
    }
    
    // Update active component
    switch m.activeView {
    case 0:
        m.messageTable, cmd = m.messageTable.Update(msg)
    }
    
    return m, cmd
}
```

### Integration Points
```yaml
DATABASE:
  - auto-migration: Run on startup to create/update schema
  - indexes: 
    - Session.Name (unique)
    - Message.ScheduledTime
    - Message.Status
  
CONFIG:
  - location: $HOME/.config/claude-usage-manager/config.yaml
  - env prefix: CUM_ (e.g., CUM_DATABASE_PATH)
  - defaults: Embedded in binary
  
TMUX:
  - verification: Check tmux server running on startup
  - recovery: Handle disconnected sessions gracefully
  - discovery: Auto-discover Claude sessions
```

## Validation Loop

### Level 1: Syntax & Style
```bash
# Install dependencies
go mod download

# Run these FIRST - fix any errors before proceeding
go fmt ./...                    # Format code
golangci-lint run              # Comprehensive linting
go vet ./...                   # Go vet checks

# Expected: No errors. If errors, READ the error and fix.
```

### Level 2: Unit Tests
```go
// CREATE tests/scheduler_test.go
package tests

import (
    "testing"
    "time"
)

func TestSmartScheduler(t *testing.T) {
    // Test priority queue ordering
    scheduler := NewTestScheduler()
    
    // Add messages with different priorities
    scheduler.AddMessage("low priority", 1)
    scheduler.AddMessage("high priority", 10)
    
    // Verify high priority processed first
    next := scheduler.GetNext()
    assert.Equal(t, "high priority", next.Content)
}

func TestTmuxMessageDelay(t *testing.T) {
    // Test 500ms delay is respected
    start := time.Now()
    err := SendMessage("test:0", "test message")
    duration := time.Since(start)
    
    assert.NoError(t, err)
    assert.True(t, duration >= 500*time.Millisecond)
}

func TestUsageWindowTracking(t *testing.T) {
    // Test 5-hour window calculation
    monitor := NewUsageMonitor()
    
    // Simulate first message
    monitor.RecordMessage(time.Now())
    
    // Check remaining time
    remaining := monitor.GetRemainingTime()
    assert.True(t, remaining <= 5*time.Hour)
}
```

```bash
# Run and iterate until passing:
go test ./tests/... -v
# If failing: Read error, understand root cause, fix code, re-run
```

### Level 3: Integration Test
```bash
# Build the binary
go build -o claude-usage-manager

# Test CLI commands
./claude-usage-manager session add --name "project1" --target "project:0"
./claude-usage-manager message add --session "project1" --content "Test message" --priority 5

# Start TUI
./claude-usage-manager tui

# Expected: TUI starts showing sessions and messages
# If error: Check logs at ~/.config/claude-usage-manager/logs/
```

### Level 4: Tmux Integration Test
```bash
# Create test tmux session
tmux new-session -d -s test-session

# Send test message
./claude-usage-manager send --target "test-session:0" --message "Hello Claude"

# Verify message arrived
tmux capture-pane -t test-session:0 -p

# Cleanup
tmux kill-session -t test-session
```

## Final Validation Checklist
- [ ] All tests pass: `go test ./... -v`
- [ ] No linting errors: `golangci-lint run`
- [ ] No formatting issues: `go fmt ./...`
- [ ] TUI runs smoothly: `./claude-usage-manager tui`
- [ ] Messages delivered to tmux with proper delay
- [ ] Database persists across restarts
- [ ] Configuration loads from file and environment
- [ ] Error handling for disconnected tmux sessions
- [ ] Usage tracking accurate for 5-hour windows
- [ ] Smart scheduling prioritizes correctly

---

## Anti-Patterns to Avoid
- ❌ Don't use CGO-based SQLite driver (use pure-Go)
- ❌ Don't send Enter immediately after message (500ms delay required)
- ❌ Don't assume tmux windows exist (always verify first)
- ❌ Don't block the TUI update loop (use goroutines)
- ❌ Don't ignore GORM errors (always check err != nil)
- ❌ Don't hardcode paths (use config management)
- ❌ Don't store sensitive data in config files

## Implementation Notes

### TUI Design Principles
- Use Bubble Tea's Model-Update-View pattern consistently
- Keep Update function pure (no side effects)
- Use commands (tea.Cmd) for async operations
- Style with Lip Gloss for consistent appearance

### Database Best Practices
- Use transactions for multi-table updates
- Index frequently queried fields
- Soft delete (GORM's DeletedAt) for audit trail
- Regular VACUUM for SQLite optimization

### Scheduling Algorithm
1. Monitor all active sessions for usage
2. When usage available:
   - Pop highest priority message from queue
   - Select least recently used session
   - Send message and update tracking
3. For exact-time scheduling:
   - Use cron expressions
   - Override smart scheduling when triggered
4. Handle failures:
   - Retry with exponential backoff
   - Mark as failed after 3 attempts

## Confidence Score: 9/10

This PRP provides comprehensive context for implementing a Claude Usage Manager in Go. The architecture leverages proven libraries (Bubble Tea, GORM, go-tmux) with clear patterns from the example projects. The validation loops ensure quality at each step. The only uncertainty is exact Claude API behavior, but the monitoring approach from the Python example should translate well.