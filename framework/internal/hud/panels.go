package hud

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Version is set by the cmd package at startup.
var Version = "0.1.0"

// renderHeader returns the top title bar.
func renderHeader(width int) string {
	title := "Clauductor"
	ver := "v" + Version

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
func renderFooter(width int, showHelp bool) string {
	var keys string
	if showHelp {
		keys = " Press any key to close help "
	} else {
		keys = " ↑↓ scroll  1-9 sessions  tab focus  r refresh  q quit  ? help "
	}
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
func renderWorkers(data HUDData, width int, focused bool) string {
	var b strings.Builder

	// Calculate dynamic name width from actual data (min 10, max 24)
	nameW := 10
	for _, w := range data.Workers {
		if len(w.Name) > nameW {
			nameW = len(w.Name)
		}
	}
	if nameW > 24 {
		nameW = 24
	}

	// Milestone column: find max width (min 6, max 12)
	msW := 6
	for _, w := range data.Workers {
		if len(w.Milestone) > msW {
			msW = len(w.Milestone)
		}
	}
	if msW > 12 {
		msW = 12
	}

	for _, w := range data.Workers {
		dot := statusDot(w.Status)

		wName := w.Name
		if len(wName) > nameW {
			wName = wName[:nameW-1] + "…"
		}
		name := lipgloss.NewStyle().
			Width(nameW).
			Foreground(colorBright).
			Bold(true).
			Render(wName)

		milestone := cyanText.Width(msW).Render(truncate(w.Milestone, msW))
		wtype := accentText.Width(6).Render(strings.ToUpper(w.Type))

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
	return wrapPanel("WORKERS", content, width, focused)
}

// renderLocks renders the file locks panel.
func renderLocks(data HUDData, width int, focused bool, scrollOffset int) string {
	var b strings.Builder

	if len(data.Locks) == 0 {
		b.WriteString(dimText.Render("  No active file locks"))
		content := strings.TrimRight(b.String(), "\n")
		return wrapPanel("FILE LOCKS", content, width, focused)
	}

	visible := 5
	start := scrollOffset
	if start > len(data.Locks)-visible {
		start = len(data.Locks) - visible
	}
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(data.Locks) {
		end = len(data.Locks)
	}

	// Find longest path for alignment (across visible locks)
	maxPath := 0
	for i := start; i < end; i++ {
		if len(data.Locks[i].FilePath) > maxPath {
			maxPath = len(data.Locks[i].FilePath)
		}
	}
	if maxPath > 30 {
		maxPath = 30
	}

	for i := start; i < end; i++ {
		l := data.Locks[i]
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

		lockedTime := ""
		if !l.LockedAt.IsZero() {
			lockedTime = " locked " + l.LockedAt.Format("15:04")
		}

		line := cyanText.Render(path) +
			dimText.Render(connector) +
			brightText.Render(owner) +
			dimText.Render(lockedTime)
		b.WriteString(line + "\n")
	}

	// Show scroll indicator if there are more locks than visible
	if len(data.Locks) > visible {
		indicator := fmt.Sprintf("  [%d-%d of %d]", start+1, end, len(data.Locks))
		b.WriteString(dimText.Render(indicator))
	}

	content := strings.TrimRight(b.String(), "\n")
	return wrapPanel("FILE LOCKS", content, width, focused)
}

// renderMilestones renders the milestones panel.
func renderMilestones(data HUDData, width int, focused bool) string {
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
	return wrapPanel("MILESTONES", content, width, focused)
}

// renderActivity renders the recent activity feed.
func renderActivity(data HUDData, width int, focused bool, scrollOffset int) string {
	var b strings.Builder

	visible := 10
	start := scrollOffset
	if start > len(data.Events)-visible {
		start = len(data.Events) - visible
	}
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(data.Events) {
		end = len(data.Events)
	}

	// Dynamic worker column width based on actual data
	workerW := 10
	for _, e := range data.Events {
		if len(e.WorkerID) > workerW {
			workerW = len(e.WorkerID)
		}
	}
	if workerW > 24 {
		workerW = 24
	}

	// Prefix: "HH:MM  " (7) + worker (workerW) + " " (1)
	prefixWidth := 8 + workerW
	contentWidth := width - 4 // panel borders + padding
	detailMax := contentWidth - prefixWidth
	if detailMax < 10 {
		detailMax = 10
	}

	for i := start; i < end; i++ {
		e := data.Events[i]
		ts := dimText.Render(e.Timestamp.Format("15:04"))
		worker := accentText.Width(workerW).Render(truncate(e.WorkerID, workerW))

		detail := e.Detail
		if len(detail) > detailMax {
			detail = detail[:detailMax-1] + "…"
		}

		b.WriteString(fmt.Sprintf("%s  %s %s\n", ts, worker, brightText.Render(detail)))
	}

	// Show scroll indicator if there are more events
	if len(data.Events) > visible {
		indicator := fmt.Sprintf("  [%d-%d of %d]", start+1, end, len(data.Events))
		b.WriteString(dimText.Render(indicator))
	}

	content := strings.TrimRight(b.String(), "\n")
	return wrapPanel("RECENT ACTIVITY", content, width, focused)
}

// --- helpers ---

func wrapPanel(title, content string, width int, focused bool) string {
	// Inner content width: total width minus border (2) minus padding (2)
	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	titleRendered := panelTitleStyle.Width(innerWidth).Render(title)
	contentRendered := lipgloss.NewStyle().Width(innerWidth).Render(content)

	style := panelStyle.Width(width - 2) // -2 for border
	if focused {
		style = style.BorderForeground(colorTitle)
	}
	return style.Render(lipgloss.JoinVertical(lipgloss.Left,
		titleRendered,
		contentRendered,
	))
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return s[:max-1] + "…"
}
