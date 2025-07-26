package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/derekxwang/tcs/internal/types"
)

// UsageBar represents a usage progress bar component
type UsageBar struct {
	width       int
	height      int
	showLabels  bool
	showDetails bool

	// Styles
	barStyle     lipgloss.Style
	filledStyle  lipgloss.Style
	emptyStyle   lipgloss.Style
	labelStyle   lipgloss.Style
	detailStyle  lipgloss.Style
	warningStyle lipgloss.Style
	dangerStyle  lipgloss.Style
}

// UsageBarOptions holds configuration options for the usage bar
type UsageBarOptions struct {
	Width       int
	Height      int
	ShowLabels  bool
	ShowDetails bool
	Theme       string
}

// NewUsageBar creates a new usage bar component
func NewUsageBar(opts UsageBarOptions) *UsageBar {
	ub := &UsageBar{
		width:       opts.Width,
		height:      opts.Height,
		showLabels:  opts.ShowLabels,
		showDetails: opts.ShowDetails,
	}

	if ub.width == 0 {
		ub.width = 50
	}
	if ub.height == 0 {
		ub.height = 3
	}

	ub.initStyles(opts.Theme)
	return ub
}

// initStyles initializes the component styles
func (ub *UsageBar) initStyles(theme string) {
	switch theme {
	case "dark":
		ub.initDarkTheme()
	case "light":
		ub.initLightTheme()
	default:
		ub.initDefaultTheme()
	}
}

// initDefaultTheme initializes the default theme
func (ub *UsageBar) initDefaultTheme() {
	ub.barStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1)

	ub.filledStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("10")) // Green

	ub.emptyStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("0")) // Black

	ub.labelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true)

	ub.detailStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	ub.warningStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("11")) // Yellow

	ub.dangerStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("9")) // Red
}

// initDarkTheme initializes the dark theme
func (ub *UsageBar) initDarkTheme() {
	ub.barStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("241")).
		Padding(0, 1)

	ub.filledStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("34")) // Dark green

	ub.emptyStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("235")) // Dark gray

	ub.labelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Bold(true)

	ub.detailStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	ub.warningStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("214")) // Orange

	ub.dangerStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("196")) // Bright red
}

// initLightTheme initializes the light theme
func (ub *UsageBar) initLightTheme() {
	ub.barStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	ub.filledStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("28")) // Light green

	ub.emptyStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("251")) // Light gray

	ub.labelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Bold(true)

	ub.detailStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	ub.warningStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("220")) // Light yellow

	ub.dangerStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("203")) // Light red
}

// Render renders the usage bar with the given usage statistics
func (ub *UsageBar) Render(stats types.UsageStats) string {
	var sections []string

	// Main progress bar
	sections = append(sections, ub.renderProgressBar(stats))

	// Labels and details if enabled
	if ub.showLabels {
		sections = append(sections, ub.renderLabels(stats))
	}

	if ub.showDetails {
		sections = append(sections, ub.renderDetails(stats))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderProgressBar renders the main progress bar
func (ub *UsageBar) renderProgressBar(stats types.UsageStats) string {
	// Calculate fill percentage
	percentage := stats.UsagePercentage
	if percentage > 1.0 {
		percentage = 1.0
	}

	// Calculate bar dimensions
	innerWidth := ub.width - 4 // Account for border and padding
	filledWidth := int(float64(innerWidth) * percentage)
	emptyWidth := innerWidth - filledWidth

	// Choose color based on usage level
	fillStyle := ub.filledStyle
	if percentage > 0.9 {
		fillStyle = ub.dangerStyle
	} else if percentage > 0.7 {
		fillStyle = ub.warningStyle
	}

	// Create the bar content
	filledPart := fillStyle.Render(strings.Repeat(" ", filledWidth))
	emptyPart := ub.emptyStyle.Render(strings.Repeat(" ", emptyWidth))
	barContent := filledPart + emptyPart

	// Add percentage overlay in the center
	percentageText := fmt.Sprintf("%.1f%%", percentage*100)
	if len(percentageText) <= innerWidth {
		// Calculate position to center the text
		textPos := (innerWidth - len(percentageText)) / 2
		if textPos >= 0 && textPos+len(percentageText) <= innerWidth {
			// Overlay the percentage text
			barRunes := []rune(barContent)
			textRunes := []rune(percentageText)

			// Apply overlay with appropriate styling
			for i, r := range textRunes {
				if textPos+i < len(barRunes) {
					// Determine if we're over filled or empty area
					if textPos+i < filledWidth {
						barRunes[textPos+i] = r
					} else {
						barRunes[textPos+i] = r
					}
				}
			}
			barContent = string(barRunes)
		}
	}

	return ub.barStyle.Width(ub.width).Render(barContent)
}

// renderLabels renders usage labels
func (ub *UsageBar) renderLabels(stats types.UsageStats) string {
	leftLabel := fmt.Sprintf("Messages: %d/%d", stats.MessagesUsed, stats.MessagesLimit)

	var rightLabel string
	if stats.CurrentWindow != nil {
		rightLabel = fmt.Sprintf("Time: %s", stats.TimeRemaining.Round(time.Minute))
	} else {
		rightLabel = "No active window"
	}

	// Calculate spacing
	totalLabelWidth := len(leftLabel) + len(rightLabel)
	spacing := ub.width - totalLabelWidth
	if spacing < 1 {
		spacing = 1
	}

	labelLine := ub.labelStyle.Render(leftLabel) +
		strings.Repeat(" ", spacing) +
		ub.labelStyle.Render(rightLabel)

	return labelLine
}

// renderDetails renders detailed usage information
func (ub *UsageBar) renderDetails(stats types.UsageStats) string {
	var details []string

	// Window information
	if stats.CurrentWindow != nil {
		windowInfo := fmt.Sprintf("Window: %s - %s",
			stats.WindowStartTime.Format("15:04"),
			stats.WindowEndTime.Format("15:04"))
		details = append(details, windowInfo)
	}

	// Status information
	var statusIcon, statusText string
	statusColor := lipgloss.Color("10") // Green

	if !stats.CanSendMessage {
		statusIcon = "✗"
		statusText = "Cannot send messages"
		statusColor = lipgloss.Color("9") // Red
	} else if stats.UsagePercentage > 0.9 {
		statusIcon = "⚠"
		statusText = "Critical usage level"
		statusColor = lipgloss.Color("9") // Red
	} else if stats.UsagePercentage > 0.7 {
		statusIcon = "⚠"
		statusText = "High usage level"
		statusColor = lipgloss.Color("11") // Yellow
	} else {
		statusIcon = "✓"
		statusText = "Normal usage level"
	}

	statusLine := lipgloss.NewStyle().Foreground(statusColor).Render(
		fmt.Sprintf("%s %s", statusIcon, statusText))
	details = append(details, statusLine)

	// Token information if available
	if stats.TokensUsed > 0 {
		tokenInfo := fmt.Sprintf("Tokens: %d/%d", stats.TokensUsed, stats.TokensLimit)
		details = append(details, ub.detailStyle.Render(tokenInfo))
	}

	return ub.detailStyle.Render(strings.Join(details, " • "))
}

// SetSize updates the component size
func (ub *UsageBar) SetSize(width, height int) {
	ub.width = width
	ub.height = height
}

// SetTheme updates the component theme
func (ub *UsageBar) SetTheme(theme string) {
	ub.initStyles(theme)
}

// SetShowLabels enables or disables label display
func (ub *UsageBar) SetShowLabels(show bool) {
	ub.showLabels = show
}

// SetShowDetails enables or disables detail display
func (ub *UsageBar) SetShowDetails(show bool) {
	ub.showDetails = show
}

// CompactUsageBar creates a compact single-line usage bar
func CompactUsageBar(stats types.UsageStats, width int) string {
	if width < 10 {
		return "Usage: N/A"
	}

	// Simple text-based progress bar
	percentage := stats.UsagePercentage
	if percentage > 1.0 {
		percentage = 1.0
	}

	barWidth := width - 20 // Reserve space for labels
	if barWidth < 5 {
		barWidth = 5
	}

	filledWidth := int(float64(barWidth) * percentage)
	emptyWidth := barWidth - filledWidth

	// Choose characters and colors based on usage level
	var fillChar, emptyChar string
	var fillColor lipgloss.Color

	if percentage > 0.9 {
		fillChar = "█"
		fillColor = lipgloss.Color("9") // Red
	} else if percentage > 0.7 {
		fillChar = "█"
		fillColor = lipgloss.Color("11") // Yellow
	} else {
		fillChar = "█"
		fillColor = lipgloss.Color("10") // Green
	}
	emptyChar = "░"

	filledPart := lipgloss.NewStyle().Foreground(fillColor).Render(strings.Repeat(fillChar, filledWidth))
	emptyPart := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(strings.Repeat(emptyChar, emptyWidth))

	// Add percentage and status
	percentageText := fmt.Sprintf("%.0f%%", percentage*100)
	statusIcon := "✓"
	if !stats.CanSendMessage {
		statusIcon = "✗"
	} else if percentage > 0.8 {
		statusIcon = "⚠"
	}

	return fmt.Sprintf("%s %s%s %s", statusIcon, filledPart, emptyPart, percentageText)
}

// MiniUsageIndicator creates a minimal usage indicator
func MiniUsageIndicator(stats types.UsageStats) string {
	percentage := stats.UsagePercentage
	if percentage > 1.0 {
		percentage = 1.0
	}

	var indicator string
	var color lipgloss.Color

	if !stats.CanSendMessage {
		indicator = "●"
		color = lipgloss.Color("9") // Red
	} else if percentage > 0.9 {
		indicator = "●"
		color = lipgloss.Color("9") // Red
	} else if percentage > 0.7 {
		indicator = "●"
		color = lipgloss.Color("11") // Yellow
	} else if percentage > 0.4 {
		indicator = "●"
		color = lipgloss.Color("10") // Green
	} else {
		indicator = "○"
		color = lipgloss.Color("8") // Gray
	}

	return lipgloss.NewStyle().Foreground(color).Render(indicator)
}

// UsageBarWithHistory creates a usage bar with historical data points
func UsageBarWithHistory(currentStats types.UsageStats, history []float64, width int) string {
	if width < 20 {
		return CompactUsageBar(currentStats, width)
	}

	// Main usage bar
	mainBar := CompactUsageBar(currentStats, width-15)

	// History sparkline
	sparklineWidth := 10
	if len(history) < sparklineWidth {
		sparklineWidth = len(history)
	}

	if sparklineWidth == 0 {
		return mainBar
	}

	// Create simple sparkline
	sparkline := createSparkline(history[len(history)-sparklineWidth:], sparklineWidth)

	return fmt.Sprintf("%s │ %s", mainBar, sparkline)
}

// createSparkline creates a simple text-based sparkline
func createSparkline(data []float64, width int) string {
	if len(data) == 0 || width == 0 {
		return ""
	}

	// Find min/max for normalization
	min, max := data[0], data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Avoid division by zero
	if max == min {
		return strings.Repeat("▄", width)
	}

	// Sparkline characters (from low to high)
	chars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

	var sparkline strings.Builder
	for i := 0; i < width && i < len(data); i++ {
		// Normalize value to 0-1 range
		normalized := (data[i] - min) / (max - min)

		// Map to character index
		charIndex := int(normalized * float64(len(chars)-1))
		if charIndex >= len(chars) {
			charIndex = len(chars) - 1
		}

		// Color based on value
		var color lipgloss.Color
		if normalized > 0.8 {
			color = lipgloss.Color("9") // Red
		} else if normalized > 0.6 {
			color = lipgloss.Color("11") // Yellow
		} else {
			color = lipgloss.Color("10") // Green
		}

		char := lipgloss.NewStyle().Foreground(color).Render(chars[charIndex])
		sparkline.WriteString(char)
	}

	return sparkline.String()
}
