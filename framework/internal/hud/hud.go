package hud

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tickMsg triggers a data refresh.
type tickMsg time.Time

// panel identifies a focusable panel in the HUD.
type panel int

const (
	panelNone       panel = iota
	panelWorkers
	panelLocks
	panelMilestones
	panelActivity
)

const panelCount = 4

// Model is the Bubble Tea model for the Clauductor HUD.
type Model struct {
	data       HUDData
	source     DataSource
	width      int
	height     int
	err        error
	quitting   bool

	// Navigation state
	focusPanel   panel  // currently focused panel (0 = none)
	scrollOffset int    // scroll offset for activity feed
	showHelp     bool   // whether help overlay is visible
}

// New creates a new HUD model with the given data source.
func New(source DataSource) Model {
	return Model{
		source: source,
		width:  80,
		height: 24,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchData(),
		tickEvery(2*time.Second),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If help overlay is showing, any key closes it
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}

		case "down", "j":
			maxScroll := len(m.data.Events) - 6
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}

		case "tab":
			next := int(m.focusPanel) + 1
			if next > panelCount {
				next = 1
			}
			m.focusPanel = panel(next)

		case "esc":
			m.focusPanel = panelNone
			m.showHelp = false

		case "?":
			m.showHelp = true

		case "r":
			return m, m.fetchData()

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '0')
			return m, switchToPane(idx)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, tea.Batch(
			m.fetchData(),
			tickEvery(2*time.Second),
		)

	case HUDData:
		m.data = msg
		m.err = nil

	case errMsg:
		m.err = msg.err
	}

	return m, nil
}

// switchToPane switches to the Nth tmux pane.
func switchToPane(index int) tea.Cmd {
	c := exec.Command("tmux", "select-window", "-t", fmt.Sprintf(":%d", index))
	return tea.ExecProcess(c, func(err error) tea.Msg {
		// Ignore errors (e.g., pane doesn't exist)
		return nil
	})
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	w := m.width
	if w < 40 {
		w = 40
	}

	// Header
	header := renderHeader(w)

	// Workers panel — full width
	workers := renderWorkers(m.data, w, m.focusPanel == panelWorkers)

	// Locks and milestones — both full width, stacked vertically
	locks := renderLocks(m.data, w, m.focusPanel == panelLocks)
	milestones := renderMilestones(m.data, w, m.focusPanel == panelMilestones)
	middle := lipgloss.JoinVertical(lipgloss.Left, locks, milestones)

	// Activity feed — full width
	activity := renderActivity(m.data, w, m.focusPanel == panelActivity, m.scrollOffset)

	// Footer
	footer := renderFooter(w, m.showHelp)

	view := lipgloss.JoinVertical(lipgloss.Left,
		header, workers, middle, activity, footer)

	// Help overlay
	if m.showHelp {
		overlay := renderHelpOverlay(w, m.height)
		return overlay
	}

	return view
}

// fetchData returns a Cmd that fetches data from the source.
func (m Model) fetchData() tea.Cmd {
	return func() tea.Msg {
		data, err := m.source.Fetch()
		if err != nil {
			return errMsg{err}
		}
		return data
	}
}

type errMsg struct {
	err error
}

// tickEvery returns a Cmd that sends a tickMsg after the given duration.
func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// renderHelpOverlay renders a centered help overlay with all keybindings.
func renderHelpOverlay(width, height int) string {
	help := `
  CLAUDUCTOR HUD — KEYBOARD SHORTCUTS

  Navigation
  ----------
  up / k        Scroll activity feed up
  down / j      Scroll activity feed down
  tab           Cycle focus between panels
  esc           Unfocus panel

  Sessions
  --------
  1-9           Switch to worker session N (tmux pane)

  Actions
  -------
  r             Force refresh data
  ?             Toggle this help overlay
  q / Ctrl+C    Quit HUD

  Press any key to close this help.
`

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorTitle).
		Foreground(colorBright).
		Padding(1, 2).
		Width(50).
		Render(help)

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		box)
}

// Run starts the HUD program.
func Run(source DataSource) error {
	p := tea.NewProgram(
		New(source),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}
