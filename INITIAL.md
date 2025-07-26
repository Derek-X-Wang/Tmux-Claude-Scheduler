## FEATURE:

- I want to make golang cli tool.
- it can monitor the claude subscription usage
- it can schedule command and send command to tmux windows via send-keys
- it can manage multiple sessions and allow to setup schedule message for different tmux windows
- basically, I would like to use it to maximize my claude subscription usage.
- claude subscription reset usage every 5 hours. And I cannot stay up until midnight to make sure I don't waste that usage.
- A new session is only count when start sending the first message. and then it will start counting 5 hours.
- so I want be able to schedule messages for different app. I can add message and schedule time for it to send to tmux windows.
- I want this cli to support message queue for different tmux session so I can work with multiple projects
- beside of mannully exact time scheduling, I also want to add a smart mode which we can keep message queues and give them different priority. 
- cli will auto send if there is available usage.

## EXAMPLES:

- `examples/Claude-Code-Usage-Monitor` - this python cli shows how to monitor the usage. and it has a good cli ui.
- `examples/Tmux-Orchestrator` - this shows how to send key to tmux window and how to manage as orchestrator.

## DOCUMENTATION:

There are many tui frameworks for golang. You can decide which one fit better. or use others

TUI: https://github.com/charmbracelet/bubbles
TUI: https://github.com/rivo/tview
TUI: https://github.com/gizak/termui

## OTHER CONSIDERATIONS:

- Include critical implementation details in the PRPs
- Use context7 mcp to fetch documents if needed
- Ultrathink on the architecture
- I prefer monorepo style if applicable