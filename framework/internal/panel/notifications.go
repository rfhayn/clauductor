package panel

// The Notification hook carries a notification_type. Each documented type is mapped
// here explicitly to what it does to a session, whether it belongs in "Needs you",
// and how severe it is. An unknown type is shown (feed, lane chip, a counter) and is
// NEVER treated as waiting: the v0 panel set "waiting" for every Notification, so an
// informational one (auth_success, agent_completed) read as a blocked lane.
//
// Source: https://code.claude.com/docs/en/hooks (Notification), the 12 values
// documented for Claude Code 2.1.284.

// Severity of a notification or alert.
const (
	SevBlock   = "block"   // blocked on you: the lane cannot continue without you
	SevWarn    = "warn"    // will not continue by itself, but is not a question
	SevInfo    = "info"    // nothing to do
	SevUnknown = "unknown" // an undocumented type: shown, never acted on
)

// NotifKind is what one notification type means.
type NotifKind struct {
	Known bool
	// Waiting sets the session to waiting and keeps the note until answered.
	Waiting bool
	// NeedsYou puts the note in "Needs you".
	NeedsYou bool
	// Done marks the turn finished (the session is idle, your move). Shown apart
	// from "blocked", and never an interruption on its own.
	Done bool
	// SetsIdle sets the session's hook status to idle.
	SetsIdle bool
	// Clears answers a previous waiting note (the dialog was completed).
	Clears   bool
	Severity string
	Label    string
}

// notificationTable maps all 12 documented notification types.
var notificationTable = map[string]NotifKind{
	"permission_prompt":          {Known: true, Waiting: true, NeedsYou: true, Severity: SevBlock, Label: "Permission"},
	"idle_prompt":                {Known: true, Done: true, SetsIdle: true, Severity: SevInfo, Label: "Idle, your move"},
	"auth_success":               {Known: true, Severity: SevInfo, Label: "Signed in"},
	"elicitation_dialog":         {Known: true, Waiting: true, NeedsYou: true, Severity: SevBlock, Label: "MCP server asks for input"},
	"elicitation_url_dialog":     {Known: true, Waiting: true, NeedsYou: true, Severity: SevBlock, Label: "MCP server asks you to open a URL"},
	"elicitation_complete":       {Known: true, Clears: true, Severity: SevInfo, Label: "MCP input complete"},
	"elicitation_response":       {Known: true, Clears: true, Severity: SevInfo, Label: "MCP input answered"},
	"agent_needs_input":          {Known: true, Waiting: true, NeedsYou: true, Severity: SevBlock, Label: "Agent needs input"},
	"agent_completed":            {Known: true, Done: true, Severity: SevInfo, Label: "Agent completed"},
	"quota_auto_resume_fired":    {Known: true, Clears: true, Severity: SevInfo, Label: "Resumed after the quota reset"},
	"quota_auto_resume_stale":    {Known: true, NeedsYou: true, Severity: SevWarn, Label: "Will not auto-resume (the reset is stale)"},
	"quota_auto_resume_disabled": {Known: true, NeedsYou: true, Severity: SevWarn, Label: "Will not auto-resume (auto-resume is off)"},
}

// ClassifyNotification returns the meaning of a notification type. An unknown type
// is shown but never waiting and never in "Needs you".
func ClassifyNotification(typ string) NotifKind {
	if k, ok := notificationTable[typ]; ok {
		return k
	}
	label := "Unknown notification"
	if typ != "" {
		label += " " + oneLine(typ)
	}
	return NotifKind{Severity: SevUnknown, Label: label}
}

// NotificationTypes lists the documented types (tests check all are mapped).
func NotificationTypes() []string {
	return []string{"permission_prompt", "idle_prompt", "auth_success", "elicitation_dialog", "elicitation_url_dialog",
		"elicitation_complete", "elicitation_response", "agent_needs_input", "agent_completed",
		"quota_auto_resume_fired", "quota_auto_resume_stale", "quota_auto_resume_disabled"}
}

// knownHookEvent is the allow-list of hook_event_name values the reducer applies:
// exactly the events the panel subscribes to. Anything else is counted and dropped.
func knownHookEvent(name string) bool {
	for _, e := range HookEvents {
		if e == name {
			return true
		}
	}
	return false
}

// waitingForKind maps `claude agents --json` waitingFor text to its documented enum:
// permission prompt, input needed, sandbox request, worker request, dialog open.
// Unrecognised text is "other" and is shown as sent.
func waitingForKind(s string) string {
	l := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if r == '_' || r == '-' {
			r = ' '
		}
		l += string(r)
	}
	for _, k := range []struct{ prefix, kind string }{
		{"permission", "permission"}, {"input", "input"}, {"sandbox", "sandbox"}, {"worker", "worker"}, {"dialog", "dialog"},
	} {
		if len(l) >= len(k.prefix) && l[:len(k.prefix)] == k.prefix {
			return k.kind
		}
	}
	if s == "" {
		return ""
	}
	return "other"
}
