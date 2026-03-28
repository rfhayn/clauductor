package hud

import "github.com/charmbracelet/lipgloss"

// Color palette — consistent across all panels.
var (
	colorGreen   = lipgloss.Color("#22c55e")
	colorYellow  = lipgloss.Color("#eab308")
	colorRed     = lipgloss.Color("#ef4444")
	colorDim     = lipgloss.Color("#6b7280")
	colorCyan    = lipgloss.Color("#06b6d4")
	colorWhite   = lipgloss.Color("#e5e7eb")
	colorBright  = lipgloss.Color("#f9fafb")
	colorBorder  = lipgloss.Color("#374151")
	colorTitle   = lipgloss.Color("#60a5fa")
	colorAccent  = lipgloss.Color("#a78bfa")
	colorBg      = lipgloss.Color("#111827")
	colorPanelBg = lipgloss.Color("#1f2937")
)

// Panel styles
var (
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitle).
			PaddingRight(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBright).
			Background(lipgloss.Color("#1e3a5f")).
			Padding(0, 1).
			Align(lipgloss.Center)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Align(lipgloss.Center)

	// Worker status indicators
	statusActive  = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	statusBlocked = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	statusIdle    = lipgloss.NewStyle().Foreground(colorDim)

	// General text styles
	dimText    = lipgloss.NewStyle().Foreground(colorDim)
	brightText = lipgloss.NewStyle().Foreground(colorBright)
	cyanText   = lipgloss.NewStyle().Foreground(colorCyan)
	accentText = lipgloss.NewStyle().Foreground(colorAccent)
	greenText  = lipgloss.NewStyle().Foreground(colorGreen)

	// Progress bar characters
	progressFull  = lipgloss.NewStyle().Foreground(colorGreen)
	progressEmpty = lipgloss.NewStyle().Foreground(colorBorder)
)
