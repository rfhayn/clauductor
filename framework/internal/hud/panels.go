package hud

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderHeader returns the top title bar.
func renderHeader(width int) string {
	title := "Clauductor"
	ver := "v0.1.0"

	// Build: ┌─ Clauductor ─────────── v0.1.0 ─┐
	inner := width - 2 // account for border chars
	if inner < 20 {
		inner = 20
	}

	left := "─ " + title + " "
	right := " " + ver + " ─"
	fill := inner - len(left) - len(right)
	if fill < 1 {
		fill = 1
	}

	line := "┌" + left + strings.Repeat("─", fill) + right + "┐"

	return lipgloss.NewStyle().
		Foreground(colorTitle).
		Bold(true).
		Width(width).
		Render(line)
}

// renderFooter returns the bottom key-bindings bar.
func renderFooter(width int) string {
	keys := " ↑↓ scroll  1-4 focus panel  q quit "
	inner := width - 2
	if inner < 20 {
		inner = 20
	}

	fill := inner - len(keys)
	left := fill / 2
	right := fill - left
	if left < 1 {
		left = 1
	}
	if right < 1 {
		right = 1
	}

	line := "└" + strings.Repeat("─", left) + keys + strings.Repeat("─", right) + "┘"

	return lipgloss.NewStyle().
		Foreground(colorBorder).
		Width(width).
		Render(line)
}

// renderWorkers renders the workers panel.
func renderWorkers(data HUDData, width int) string {
	var b strings.Builder

	for _, w := range data.Workers {
		dot := statusDot(w.Status)
		name := lipgloss.NewStyle().
			Width(12).
			Foreground(colorBright).
			Bold(true).
			Render(w.Name)

		milestone := cyanText.Width(6).Render(w.Milestone)
		wtype := accentText.Width(9).Render(strings.ToUpper(w.Type))

		files := dimText.Render("(no locks)")
		if len(w.LockedFiles) > 0 {
			f := w.LockedFiles[0]
			if len(f) > 22 {
				f = "..." + f[len(f)-19:]
			}
			files = dimText.Width(24).Render(f)
		}

		dur := formatDuration(w.Duration)
		statusLabel := renderStatusLabel(w.Status)

		line := fmt.Sprintf("%s %s %s %s %s %s %s",
			dot, name, milestone, wtype, files, dur, statusLabel)
		b.WriteString(line + "\n")
	}

	content := strings.TrimRight(b.String(), "\n")
	return wrapPanel("WORKERS", content, width)
}

// renderLocks renders the file locks panel.
func renderLocks(data HUDData, width int) string {
	var b strings.Builder

	if len(data.Locks) == 0 {
		b.WriteString(dimText.Render("  No active file locks"))
	}

	// Find longest path for alignment
	maxPath := 0
	for _, l := range data.Locks {
		if len(l.FilePath) > maxPath {
			maxPath = len(l.FilePath)
		}
	}
	if maxPath > 30 {
		maxPath = 30
	}

	for _, l := range data.Locks {
		path := l.FilePath
		if len(path) > 30 {
			path = "..." + path[len(path)-27:]
		}

		padding := maxPath - len(path)
		if padding < 1 {
			padding = 1
		}
		connector := " " + strings.Repeat("─", padding) + " "

		owner := fmt.Sprintf("%s (%s)", l.WorkerID, l.Milestone)

		line := cyanText.Render(path) +
			dimText.Render(connector) +
			brightText.Render(owner)
		b.WriteString(line + "\n")
	}

	content := strings.TrimRight(b.String(), "\n")
	return wrapPanel("FILE LOCKS", content, width)
}

// renderMilestones renders the milestones panel.
func renderMilestones(data HUDData, width int) string {
	var b strings.Builder

	for _, m := range data.Milestones {
		id := cyanText.Width(6).Render(m.ID)
		title := lipgloss.NewStyle().
			Foreground(colorBright).
			Width(20).
			Render(m.Title)

		bar := renderProgressBar(m.Progress, 14)

		var pctStr string
		if m.Progress < 0 {
			pctStr = accentText.Width(6).Render("spike")
		} else if m.Status == "planned" {
			pctStr = dimText.Width(6).Render("")
		} else {
			pctStr = brightText.Width(6).Render(fmt.Sprintf("%3d%%", m.Progress))
		}

		var assignee string
		if m.AssignedTo == "" {
			assignee = dimText.Render("unclaimed")
		} else {
			assignee = greenText.Render(m.AssignedTo)
		}

		var statusIcon string
		switch m.Status {
		case "planned":
			statusIcon = dimText.Render("○")
		case "active":
			statusIcon = greenText.Render("●")
		case "complete":
			statusIcon = greenText.Render("✓")
		}

		line := fmt.Sprintf("%s %s %s %s %s %s",
			statusIcon, id, title, bar, pctStr, assignee)
		b.WriteString(line + "\n")
	}

	content := strings.TrimRight(b.String(), "\n")
	return wrapPanel("MILESTONES", content, width)
}

// renderActivity renders the recent activity feed.
func renderActivity(data HUDData, width int) string {
	var b strings.Builder

	max := 6
	if len(data.Events) < max {
		max = len(data.Events)
	}

	for i := 0; i < max; i++ {
		e := data.Events[i]
		ts := dimText.Render(e.Timestamp.Format("15:04"))
		worker := accentText.Width(10).Render(e.WorkerID)
		detail := brightText.Render(e.Detail)
		b.WriteString(fmt.Sprintf("%s  %s %s\n", ts, worker, detail))
	}

	content := strings.TrimRight(b.String(), "\n")
	return wrapPanel("RECENT ACTIVITY", content, width)
}

// --- helpers ---

func wrapPanel(title, content string, width int) string {
	titleRendered := panelTitleStyle.Render(title)

	style := panelStyle.Width(width - 2) // -2 for border
	return lipgloss.JoinVertical(lipgloss.Left,
		style.Render(lipgloss.JoinVertical(lipgloss.Left,
			titleRendered,
			content,
		)),
	)
}

func statusDot(status string) string {
	switch status {
	case "active":
		return statusActive.Render("●")
	case "blocked":
		return statusBlocked.Render("●")
	case "idle":
		return statusIdle.Render("○")
	default:
		return dimText.Render("○")
	}
}

func renderStatusLabel(status string) string {
	switch status {
	case "active":
		return statusActive.Render("active")
	case "blocked":
		return statusBlocked.Render("blocked")
	case "idle":
		return statusIdle.Render("idle")
	default:
		return dimText.Render(status)
	}
}

func renderProgressBar(pct int, barWidth int) string {
	if pct < 0 {
		// Spike — no measurable progress
		bar := strings.Repeat("░", barWidth)
		return accentText.Render(bar)
	}

	filled := pct * barWidth / 100
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	bar := progressFull.Render(strings.Repeat("█", filled)) +
		progressEmpty.Render(strings.Repeat("░", empty))
	return bar
}

func formatDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := fmt.Sprintf("%dm", m)
	return dimText.Width(6).Render(s)
}
