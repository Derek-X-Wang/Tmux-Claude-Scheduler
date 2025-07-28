# TCS (Tmux Claude Scheduler)

A Go CLI tool that maximizes Claude subscription usage by monitoring 5-hour usage windows, scheduling messages to tmux windows, and managing multiple Claude windows with smart priority-based scheduling.

**TCS** stands for **T**mux **C**laude **S**cheduler - a lightweight, efficient tool designed to help you make the most of your Claude subscription by automatically discovering and managing Claude instances across all your tmux windows.

## Features

- **Real Claude Usage Tracking**: Reads actual Claude usage data from `~/.claude` directory and aligns with Claude's 11 AM reset schedule
- **5-Hour Usage Window Monitoring**: Automatically tracks Claude's official 5-hour usage windows with accurate timing
- **One-Command Setup**: `tcs init` command handles complete setup in one go
- **Smart Message Scheduling**: Priority-based message queue with intelligent scheduling
- **Tmux Integration**: Send messages to Claude running in tmux sessions with proper timing
- **Window-Based Architecture**: Automatically discover and manage Claude instances in tmux windows
- **Multiple Window Queues**: Each tmux window gets its own message queue with priority
- **Beautiful TUI Dashboard**: Interactive terminal UI for monitoring and control
- **Cron Scheduling**: Schedule messages for specific times
- **Auto-discovery**: Automatically discover tmux windows containing Claude
- **Message Queue Management**: Grouped by session for easy organization
- **Message Editing**: Edit message content, priority, and schedule time through TUI
- **Real-time Usage Monitoring**: Live tracking of actual message usage and time until reset
- **Session Health Checks**: Monitor tmux session connectivity and Claude presence

## Installation

### Prerequisites

- Go 1.19 or higher
- tmux installed and running
- SQLite (bundled, no installation needed)

### Build from Source

```bash
git clone https://github.com/derekxwang/tcs.git
cd tcs
make build
```

This will create the binary at `./bin/tcs`.

### Install

```bash
make install
```

This installs the binary to `/usr/local/bin/tcs`.

## Quick Start

### Option 1: One-Command Setup (Recommended)

1. Start tmux and open Claude in tmux windows:
   ```bash
   tmux new-session -s project
   # Open Claude in your browser or terminal client
   # Create additional windows as needed: Ctrl+B, C
   ```

2. Initialize TCS with complete setup:
   ```bash
   tcs init
   ```
   
   This single command will:
   - Generate default configuration file
   - Initialize the database
   - Scan and discover your tmux windows with Claude detection
   - Set up real-time Claude usage monitoring (aligned with 11 AM reset)
   - Show current status and provide next steps

3. You're ready! Schedule messages, view status, or launch the TUI:
   ```bash
   tcs message add project:0 "Hello Claude!" --priority 5 --when now
   tcs status
   tcs tui
   ```

### Option 2: Manual Setup

If you prefer manual setup or need more control:

1. Start tmux and open Claude in tmux windows
2. Generate configuration: `tcs config init`
3. Discover Claude windows: `tcs window scan`
4. View status: `tcs status`
5. Launch TUI dashboard: `tcs tui`

## Usage

### CLI Commands

#### Initial Setup

```bash
# One-command setup (recommended for new users)
tcs init

# This will:
# - Generate default configuration file
# - Initialize database
# - Scan and discover tmux windows with Claude detection
# - Set up real-time Claude usage monitoring
# - Show current status and next steps
```

#### Window Management

```bash
# Scan for Claude windows
tcs window scan

# List discovered windows
tcs window list

# List message queues (grouped by session)
tcs queue list

# Queue status with pending message counts
tcs queue status
```

#### Message Scheduling

```bash
# Schedule a message to a specific window
tcs message add <window-target> <content> [--priority 1-10] [--when time]

# Time formats for --when:
# - "now" (immediate)
# - "+5m" (in 5 minutes)
# - "+2h" (in 2 hours)
# - "14:30" (at 2:30 PM)

# Examples
tcs message add project:0 "What is quantum computing?" --priority 8 --when now
tcs message add research:1 "Analyze this paper" --priority 5 --when +30m
tcs message add project:0 "Daily summary" --priority 3 --when 17:00

# List all messages
tcs message list

# Edit a message
tcs message edit <message-id>

# Delete a message
tcs message delete <message-id>
```

#### Direct Message Sending

```bash
# Send a message immediately to a tmux target
tcs send <tmux-target> <message>

# Example
tcs send claude:0 "Quick question about Go"
```

#### Status and Monitoring

```bash
# Show current usage and system status
tcs status

# Output includes:
# - Current usage window (messages used/limit, time remaining)
# - Active sessions and their status
# - Pending messages in queue
# - Tmux connectivity status
```

#### Configuration

```bash
# Generate default configuration
tcs config init

# Show current configuration
tcs config show
```

### TUI Dashboard

Launch the interactive terminal UI:

```bash
tcs tui
```

#### TUI Features

- **Dashboard View** (Press `1`):
  - Real-time usage statistics with progress bars
  - System status overview
  - Quick stats for windows and messages
  - Recent activity log

- **Windows View** (Press `2`):
  - View discovered tmux windows
  - See which windows have Claude detected
  - Toggle window active status
  - Force window rescan
  - View message queues grouped by session

- **Messages View** (Press `3`):
  - View all scheduled messages
  - Edit message content, target, priority, and schedule time
  - Create new messages
  - Delete messages
  - Messages grouped by session for easy navigation

- **Scheduler View** (Press `4`):
  - View message processing queue
  - Monitor scheduler status
  - Control scheduler operations

#### TUI Key Bindings

- `1` - Dashboard view
- `2` - Windows view
- `3` - Messages view
- `4` - Scheduler view
- `Tab` - Switch between sections
- `↑/↓` or `j/k` - Navigate
- `Enter` - Select/Edit
- `n` - New message
- `e` - Edit message
- `d` - Delete
- `a` - Toggle active
- `s` - Scan windows
- `F` - Force rescan
- `r` - Refresh
- `?` - Help
- `q` - Quit

## Configuration

The configuration file is located at `~/.config/tcs/config.yaml`.

### Default Configuration

```yaml
# Database configuration
database:
  path: "~/.config/tcs/tcs.db"
  log_level: "warn"
  max_idle_conns: 10
  max_open_conns: 100
  conn_max_life: "1h"

# Terminal User Interface configuration
tui:
  refresh_rate: "1s"
  theme: "default"  # options: default, dark, light
  show_debug_info: false

# Scheduler configuration
scheduler:
  smart_enabled: true        # Enable priority-based scheduling
  cron_enabled: true         # Enable time-based scheduling
  processing_interval: "10s" # How often to process queue
  max_concurrent_messages: 3
  retry_attempts: 3
  retry_delay: "30s"

# Usage monitoring configuration
usage:
  max_messages: 1000         # Claude subscription limit
  max_tokens: 100000         # Token limit (if applicable)
  window_duration: "5h"      # Usage window duration
  monitoring_interval: "30s"
  claude_reset_hour: 11      # Hour when Claude usage resets (0-23, default: 11 AM)

# Logging configuration
logging:
  level: "info"              # debug, info, warn, error
  format: "text"             # text or json
  file: ""                   # log file path (empty = stdout)

# Tmux integration configuration
tmux:
  discovery_interval: "30s"
  health_check_interval: "60s"
  message_delay: "500ms"     # Critical delay for reliable message delivery
```

## Architecture

### Core Components

1. **Database Layer** (SQLite/GORM)
   - TmuxWindows, WindowMessageQueues, Messages, UsageWindows models
   - Automatic migrations and legacy support
   - Window-based architecture with session grouping
   - Optimized queries with indexing

2. **Window Discovery System**
   - Automatic tmux window scanning
   - Claude detection using multiple indicators
   - Continuous monitoring with 30-second intervals
   - Window persistence and cleanup

3. **Tmux Integration**
   - Shell-based commands for reliability
   - 500ms delay for proper message delivery
   - Window validation and health checks
   - Target format: "session:window"

4. **Usage Monitor**
   - Real Claude data integration via `~/.claude` directory
   - 5-hour window tracking aligned with Claude's 11 AM reset schedule
   - Live usage statistics from actual Claude usage data
   - Message and token counting with real-time sync

5. **Smart Scheduler**
   - Window-based priority queues
   - Priority queue using heap algorithm
   - Cron-based scheduling for specific times
   - Automatic retry with backoff

6. **Message Management**
   - Window-specific message queues
   - Message editing capabilities
   - Queue retargeting support
   - Session-grouped display

7. **TUI System** (Bubble Tea)
   - Window-based interface design
   - Message editing forms
   - Real-time updates
   - Keyboard navigation

## Advanced Usage

### Scheduling Strategies

1. **Priority-Based Scheduling**:
   - Higher priority (1-10) messages are sent first
   - Same priority messages use FIFO ordering

2. **Time-Based Scheduling**:
   - Schedule messages for specific times
   - Automatic adjustment for past times (schedules for next day)

3. **Smart Scheduling**:
   - Automatically selects best session based on priority and recent usage
   - Distributes load across multiple sessions

### Window Management Best Practices

1. **Priority Assignment**:
   - 8-10: Critical/primary work windows
   - 5-7: Regular work windows
   - 1-4: Low-priority/research windows

2. **Multiple Windows**:
   - Use different windows for different types of work
   - Each window maintains its own message queue
   - Queues are grouped by session for organization
   - Helps distribute usage across the 5-hour window
   - Enables parallel conversations

3. **Window Organization**:
   - Group related work in the same tmux session
   - Use descriptive window names
   - TCS will automatically discover and track windows

### Automation Examples

#### Bash Script Integration

```bash
#!/bin/bash
# Send daily summary at 5 PM

SUMMARY="Please provide a summary of our work today"
tcs message add project:0 "$SUMMARY" --priority 7 --when 17:00
```

#### Cron Job for Regular Messages

```cron
# Send morning briefing at 9 AM every weekday
0 9 * * 1-5 /usr/local/bin/tcs message add project:0 "Good morning! What should we focus on today?" --priority 8 --when now
```

## Development

### Project Structure

```
tcs/
├── cmd/                    # CLI commands
│   └── root.go
├── internal/
│   ├── claude/            # Claude data integration (reads ~/.claude)
│   ├── config/            # Configuration management
│   ├── database/          # Database models and operations
│   ├── discovery/         # Window discovery system
│   ├── monitor/           # Usage monitoring with real Claude data
│   ├── scheduler/         # Message scheduling logic
│   ├── tmux/              # Tmux integration
│   ├── tui/               # Terminal UI components
│   │   ├── components/    # Reusable UI components
│   │   └── views/         # Main view implementations
│   └── types/             # Shared type definitions
├── tests/                 # Test files
├── Makefile              # Build automation
└── config.yaml           # Default configuration
```

### Running Tests

```bash
# Run all tests
make test

# Run specific test suites
go test ./tests/... -run TestScheduler
go test ./tests/... -run TestUsageMonitor
go test ./tests/... -run TestTmux
```

### Building for Different Platforms

```bash
# Build for current platform
make build

# Build for specific platforms
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

## Troubleshooting

### Common Issues

1. **"tmux server is not running"**
   - Start tmux: `tmux new-session -d`
   - Verify tmux is running: `tmux ls`
   - Run `tcs init` again after starting tmux

2. **"Window target not found" or "record not found" errors**
   - Run `tcs init` to set up everything properly
   - Or manually: Check discovered windows: `tcs window list`
   - Scan for new windows: `tcs window scan`
   - Verify tmux target: `tmux list-windows`
   - Use format "session:window" (e.g., "project:0")

3. **"Cannot send messages" (usage limit)**
   - Check current usage: `tcs status`
   - Wait for the 5-hour window to reset (shows time remaining)
   - TCS now reads real Claude usage data for accurate limits

4. **Incorrect usage statistics**
   - Ensure `~/.claude` directory exists and contains usage data
   - TCS reads actual Claude usage from JSONL files
   - Run `tcs init` to refresh usage monitoring setup

5. **TUI won't start**
   - Ensure you're in a proper terminal (not a pipe)
   - Try with a different terminal emulator
   - Check terminal capabilities: `echo $TERM`

### Debug Mode

Enable debug logging for troubleshooting:

```bash
# Set log level in config
tcs config set logging.level debug

# Or use verbose flag
tcs -v status
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## Acknowledgments

- Inspired by the need to maximize Claude subscription usage
- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI
- Uses [Cobra](https://github.com/spf13/cobra) for CLI structure
- Database operations powered by [GORM](https://gorm.io)