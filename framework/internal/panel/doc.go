// Package panel implements `clauductor panel`: a standalone, read-only, loopback-only
// web dashboard over the Claude Code sessions working in one project.
//
// It is deliberately independent of the rest of Clauductor. It needs no `clauductor
// install`, no template, no skills, no SQLite database and no file locks. Its only
// inputs are Claude Code's own signals (HTTP hooks, the status line's stdin,
// `claude agents --json`), git, gh, and the commands a project names in its
// .clauductor/panel.json.
//
// This package wires the others together and runs them:
//
//	clock    the one clock every package reads through
//	types    plain records the packages hand each other (no code, no imports)
//	lease    the on-disk queue lease lock-run holds (imports only clock and types)
//	signals  parsers of what the panel reads: hooks, status line, claude agents, git, gh
//	config   panel.json, and where the panel keeps its files
//	lanes    tmux lanes, the lane registry, restore
//	state    the pure reducer: the Model, its View, blocked, alerts, notifier decisions
//	install  hooks installer, launchd agent, token, config trust, the machine lock
//	web      the HTTP server, the hub, terminals, the embedded page
package panel
