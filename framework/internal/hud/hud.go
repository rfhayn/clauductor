package hud

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tickMsg triggers a data refresh.
type tickMsg time.Time

// Model is the Bubble Tea model for the Clauductor HUD.
type Model struct {
	data       HUDData
	source     DataSource
	width      int
	height     int
	err        error
	quitting   bool
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
		tickEvery(time.Second),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, tea.Batch(
			m.fetchData(),
			tickEvery(time.Second),
		)

	case HUDData:
		m.data = msg
		m.err = nil

	case errMsg:
		m.err = msg.err
	}

	return m, nil
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
	workers := renderWorkers(m.data, w)

	// Middle row: locks + milestones side by side
	halfW := w / 2
	if halfW < 30 {
		// Too narrow for side-by-side — stack vertically
		locks := renderLocks(m.data, w)
		milestones := renderMilestones(m.data, w)
		middle := lipgloss.JoinVertical(lipgloss.Left, locks, milestones)

		activity := renderActivity(m.data, w)
		footer := renderFooter(w)

		return lipgloss.JoinVertical(lipgloss.Left,
			header, workers, middle, activity, footer)
	}

	locks := renderLocks(m.data, halfW)
	milestones := renderMilestones(m.data, w-halfW)
	middle := lipgloss.JoinHorizontal(lipgloss.Top, locks, milestones)

	// Activity feed — full width
	activity := renderActivity(m.data, w)

	// Footer
	footer := renderFooter(w)

	return lipgloss.JoinVertical(lipgloss.Left,
		header, workers, middle, activity, footer)
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

// Run starts the HUD program.
func Run(source DataSource) error {
	p := tea.NewProgram(
		New(source),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}
