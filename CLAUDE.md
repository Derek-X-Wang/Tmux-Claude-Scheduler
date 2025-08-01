# TCS (Tmux Claude Scheduler) - Project Knowledge Base

## Project Overview

TCS is a sophisticated window-based scheduler for managing Claude conversations across multiple tmux windows. It automatically discovers Claude instances, manages message queues per window, and provides intelligent scheduling to maximize Claude subscription usage within dynamic 5-hour windows that start from your first message.

## Development Commands

For development, testing, and maintenance of TCS, use the following `make` commands:

```bash
# Run tests
make test

# Build the project  
make build

# Run linter
make lint

# Run all checks (test + build + lint)
make all
```

## Architecture Revolution: Session-Based → Window-Based

### Major Architectural Change (2025)

TCS underwent a significant architectural transformation from session-based to window-based management:

**Previous (Session-Based)**:
- Manually managed Claude sessions
- One message queue per session
- Required explicit session creation: `tcs session add`

**Current (Window-Based)**:
- Automatic window discovery
- One message queue per tmux window
- Auto-detects Claude instances
- Message queues grouped by session for display

### Why Window-Based Architecture?

1. **Automatic Discovery**: No manual session management needed
2. **Finer Granularity**: Each tmux window has its own queue
3. **Better Organization**: Queues grouped by session for display
4. **Flexible Targeting**: Can retarget messages between windows
5. **Natural Workflow**: Matches how users actually organize tmux

## Core Components

### 1. Window Discovery System (`internal/discovery/`)
- **WindowDiscovery Service**: Continuously scans tmux every 30 seconds
- **Claude Detection**: Uses multiple indicators to detect Claude instances
- **Database Persistence**: Stores window information with metadata
- **Cleanup**: Removes stale windows that no longer exist

### 2. Window-Based Database Models (`internal/database/`)

#### TmuxWindow
```go
type TmuxWindow struct {
    gorm.Model
    SessionName  string    // tmux session name
    WindowIndex  int       // window index (0, 1, 2...)
    WindowName   string    // window title
    Target       string    // "session:window" format
    HasClaude    bool      // detected Claude instance
    Priority     int       // queue priority (1-10)
    Active       bool      // enabled for scheduling
    LastSeen     time.Time // last discovery time
    LastActivity *time.Time // last message activity
}
```

#### WindowMessageQueue
```go
type WindowMessageQueue struct {
    gorm.Model
    WindowID     uint       // references TmuxWindow
    Window       TmuxWindow
    Priority     int        // queue priority
    Active       bool       // queue enabled
    LastActivity *time.Time
    CreatedAt    time.Time
}
```

#### Message (Updated)
```go
type Message struct {
    gorm.Model
    WindowID      uint       // references TmuxWindow (new)
    Window        TmuxWindow // foreign key relationship
    SessionID     *uint      // legacy field for migration
    Content       string     // message content
    ScheduledTime time.Time  // when to send
    Priority      int        // message priority
    Status        string     // pending, sent, failed
    // ... other fields
}
```

### 3. CLI Commands (Updated)

#### Window Management
```bash
# Scan for Claude windows
tcs windows scan

# List discovered windows
tcs windows list

# Force rescan
tcs windows scan --force
```

#### Queue Management
```bash
# List message queues (grouped by session)
tcs queue list

# Show queue status with pending counts
tcs queue status
```

#### Message Management
```bash
# Schedule message to specific window
tcs message add project:0 "Hello Claude!" --priority 5 --when now

# List all messages
tcs message list

# Edit existing message
tcs message edit <message-id>

# Delete message
tcs message delete <message-id>
```

### 4. TUI Interface (Updated)

#### Views
1. **Dashboard**: Usage stats, system status, window counts
2. **Windows**: Discovered windows, queue management by session
3. **Messages**: All messages with editing capabilities
4. **Scheduler**: Processing queue and scheduler controls

#### Message Editing Features
- Edit message content
- Change target window
- Adjust priority (1-10)
- Reschedule time
- Form-based interface with validation

### 5. Scheduler (Updated)
- **Window-Based Queues**: Each window has its own priority queue
- **Smart Scheduling**: Selects best window based on priority and activity
- **Target Validation**: Ensures window exists before scheduling
- **Activity Tracking**: Updates window last activity on message send

## Target Format

All operations use the `"session:window"` format:
- `"project:0"` - Window 0 in session "project"
- `"research:2"` - Window 2 in session "research"
- `"claude-work:1"` - Window 1 in session "claude-work"

## Claude Detection Indicators

The system detects Claude instances by scanning for these indicators:
- "claude", "Claude"
- "anthropic", "Assistant:"
- "Human:", "I'm Claude"
- "claude-3", "Claude Code"
- "I'm an AI assistant"

## Database Migration

The system supports migration from session-based to window-based:

### Migration Functions
```go
// Migrates existing session data to window format
func MigrateSessionsToWindows(db *gorm.DB) error

// Updates messages to use WindowID instead of SessionID  
func MigrateMessagesToWindows(db *gorm.DB) error
```

### Legacy Support
- Old `SessionID` field maintained for backward compatibility
- Automatic migration on first run
- Data integrity preserved during transition

## Claude Usage Tracking - Dynamic 5-Hour Windows

### How Claude's Usage Limits Work

**IMPORTANT**: Claude does NOT use fixed reset times like "12 PM every day." Instead:

1. **Dynamic Window Start**: Your 5-hour usage window begins when you send your **first message** to Claude
2. **Window Duration**: Each window lasts exactly 5 hours from that first message
3. **Example**: If you send your first message at 7:00 AM, your usage window runs from 7:00 AM to 12:00 PM (noon)
4. **Reset Time**: The "reset time" is always `first_message_time + 5 hours`

### TCS Implementation

TCS correctly implements this dynamic behavior:

- **Automatic Detection**: Reads your actual Claude usage data from `~/.claude` directory
- **Dynamic Windows**: Creates usage windows that match Claude's actual session timing
- **Real-time Tracking**: Updates window boundaries based on your actual usage patterns
- **No Manual Configuration**: No need to set reset hours - TCS discovers them automatically

### Configuration Options
```yaml
# Window discovery settings
discovery:
  scan_interval: "30s"      # How often to scan for windows
  claude_detection: true    # Enable Claude detection
  cleanup_interval: "5m"    # Remove stale windows

# Usage tracking (dynamic windows)
usage:
  max_messages: 1000        # Message limit per 5-hour window
  max_tokens: 100000        # Token limit (if available)
  window_duration: "5h"     # Always 5 hours for Claude
  monitoring_interval: "30s" # How often to check usage

# Queue management
queues:
  default_priority: 5       # Default queue priority
  group_by_session: true    # Group display by session
  auto_create: true         # Auto-create queues for new windows
```

## Best Practices

### Window Organization
1. **Descriptive Names**: Use meaningful tmux window names
2. **Session Grouping**: Related work in same session
3. **Priority Assignment**: 
   - 8-10: Critical work windows
   - 5-7: Regular work
   - 1-4: Low priority/experimental

### Message Management
1. **Specific Targets**: Always use exact window targets
2. **Priority Usage**: Higher priority for urgent messages
3. **Time Scheduling**: Use relative times for flexibility
4. **Content Clarity**: Clear, actionable message content

### Queue Management
1. **Regular Scanning**: Let auto-discovery handle window detection
2. **Priority Tuning**: Adjust window priorities based on importance
3. **Activity Monitoring**: Check last activity times
4. **Queue Cleanup**: Remove inactive windows as needed

## Troubleshooting

### Common Issues

#### "Window target not found"
```bash
# Check available windows
tcs windows list

# Rescan for new windows
tcs windows scan

# Verify tmux target exists
tmux list-windows -t session
```

#### Messages not sending
```bash
# Check window active status
tcs windows list

# Check queue status
tcs queue status

# Verify Claude is running in target window
tcs windows scan --force
```

#### Discovery not working
```bash
# Check tmux is running
tmux list-sessions

# Manual discovery
tcs windows scan --force

# Check discovery logs in TUI dashboard
```

## Development Notes

### Key Files
- `internal/discovery/window_discovery.go` - Main discovery logic
- `internal/database/models.go` - Window-based data models
- `internal/scheduler/scheduler.go` - Window-aware scheduling
- `internal/tui/views/windows.go` - Windows TUI view
- `internal/tui/views/messages.go` - Message editing TUI

### Testing
- Tests updated for window-based architecture
- Uses `TmuxWindow` entities instead of sessions
- Target format: `"session:window"`

### Migration Checklist
- [x] Database models updated
- [x] Discovery system implemented
- [x] Scheduler refactored for windows
- [x] CLI commands updated
- [x] TUI interface redesigned
- [x] Tests fixed for new architecture
- [x] Documentation updated

## Future Enhancements

### Planned Features
1. **Window Templates**: Pre-configured window setups
2. **Bulk Operations**: Batch message scheduling
3. **Advanced Filters**: Complex query capabilities
4. **Integration APIs**: REST API for external tools
5. **Window Synchronization**: Cross-session coordination

### Performance Optimizations
1. **Selective Scanning**: Only scan changed windows
2. **Caching**: Cache window states between scans
3. **Batch Updates**: Group database operations
4. **Background Processing**: Async discovery and cleanup

## Conclusion

The window-based architecture represents a major evolution in TCS, providing:
- **Automatic Management**: No manual setup required
- **Natural Organization**: Matches tmux usage patterns
- **Flexible Scheduling**: Per-window message queues
- **Better UX**: Session-grouped display with window-level control
- **Future-Proof**: Foundation for advanced features

This architecture enables TCS to be truly "set and forget" - users just work in tmux, and TCS automatically discovers and manages their Claude windows.