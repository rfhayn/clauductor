package state

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// The account's quota (PANEL-15). The status line reports whatever windows the
// account's plan has, each a percentage of the plan's own limit, so nothing here is
// specific to Pro or Max: the windows are a list keyed as the status line keys them.

// QuotaWindow is one quota window.
type QuotaWindow struct {
	Key      string   `json:"key"`   // the status line's key: "five_hour", "seven_day", …
	Label    string   `json:"label"` // "5-hour", "7-day", or one made from the key
	Pct      *float64 `json:"pct"`
	ResetsAt *int64   `json:"resetsAt,omitempty"` // unix seconds
	// Expired: resets_at has passed, so Pct (from before the reset) is dropped and
	// the gauge says "reset" rather than a stale number.
	Expired bool `json:"expired,omitempty"`
}

// Quota is the latest account quota the status line reported.
type Quota struct {
	Windows     []QuotaWindow `json:"windows"`
	At          int64         `json:"at"`
	FromSession string        `json:"fromSession,omitempty"`
	// Account is the hash of the account that reported it (signals.AuthStatus): a
	// saved reading is dropped once the panel reads another account.
	Account string `json:"account,omitempty"`

	// The fixed fields before PANEL-15, kept one release so a page left open across
	// an upgrade (and an older quota.json) still reads. Derived from Windows.
	FiveHour        *float64 `json:"fiveHour"`
	SevenDay        *float64 `json:"sevenDay"`
	FiveHourResets  *int64   `json:"fiveHourResetsAt,omitempty"`
	SevenDayResets  *int64   `json:"sevenDayResetsAt,omitempty"`
	FiveHourExpired bool     `json:"fiveHourExpired,omitempty"`
	SevenDayExpired bool     `json:"sevenDayExpired,omitempty"`
}

// Window returns the window with this key, or nil.
func (q *Quota) Window(key string) *QuotaWindow {
	if q == nil {
		return nil
	}
	for i := range q.Windows {
		if q.Windows[i].Key == key {
			return &q.Windows[i]
		}
	}
	return nil
}

// Shortest is the window with the shortest span that has a number: the one whose
// burn rate says the most about the next hours. Nil when none has.
func (q *Quota) Shortest() *QuotaWindow {
	if q == nil {
		return nil
	}
	for i := range q.Windows { // sorted by span (sortWindows)
		if q.Windows[i].Pct != nil {
			return &q.Windows[i]
		}
	}
	return nil
}

// fillLegacy derives the pre-PANEL-15 fields from the windows.
func (q *Quota) fillLegacy() {
	q.FiveHour, q.FiveHourResets, q.FiveHourExpired = nil, nil, false
	q.SevenDay, q.SevenDayResets, q.SevenDayExpired = nil, nil, false
	if w := q.Window("five_hour"); w != nil {
		q.FiveHour, q.FiveHourResets, q.FiveHourExpired = w.Pct, w.ResetsAt, w.Expired
	}
	if w := q.Window("seven_day"); w != nil {
		q.SevenDay, q.SevenDayResets, q.SevenDayExpired = w.Pct, w.ResetsAt, w.Expired
	}
}

// fromLegacy fills Windows from a reading saved before PANEL-15.
func (q *Quota) fromLegacy() {
	if len(q.Windows) > 0 {
		return
	}
	if q.FiveHour != nil {
		q.Windows = append(q.Windows, QuotaWindow{Key: "five_hour", Pct: q.FiveHour, ResetsAt: q.FiveHourResets})
	}
	if q.SevenDay != nil {
		q.Windows = append(q.Windows, QuotaWindow{Key: "seven_day", Pct: q.SevenDay, ResetsAt: q.SevenDayResets})
	}
	sortWindows(q.Windows)
}

// The windows Claude Code reports today, or has; any other key is labelled from
// its own words.
var knownWindows = map[string]struct {
	label string
	span  time.Duration
}{
	"five_hour":        {"5-hour", 5 * time.Hour},
	"seven_day":        {"7-day", 7 * 24 * time.Hour},
	"seven_day_opus":   {"7-day Opus", 7 * 24 * time.Hour},
	"seven_day_sonnet": {"7-day Sonnet", 7 * 24 * time.Hour},
}

var spanRe = regexp.MustCompile(`^(\d+|one|two|three|four|five|six|seven|eight|nine|ten|twelve|fourteen|thirty)_(hour|day|week|month)s?(?:_(.+))?$`)

var spanWords = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
	"nine": 9, "ten": 10, "twelve": 12, "fourteen": 14, "thirty": 30}

var spanUnits = map[string]time.Duration{"hour": time.Hour, "day": 24 * time.Hour, "week": 7 * 24 * time.Hour, "month": 30 * 24 * time.Hour}

// windowMeta labels a window key and says how long its window is (0: unknown). A
// key shaped like the known ones ("two_hour", "thirty_day_opus") reads the same way.
func windowMeta(key string) (string, time.Duration) {
	if k, ok := knownWindows[key]; ok {
		return k.label, k.span
	}
	if m := spanRe.FindStringSubmatch(key); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			n = spanWords[m[1]]
		}
		label := fmt.Sprintf("%d-%s", n, m[2])
		if m[3] != "" {
			label += " " + signals.Humanize(m[3])
		}
		return label, time.Duration(n) * spanUnits[m[2]]
	}
	return signals.Humanize(key), 0
}

// sortWindows orders windows by span, shortest first, then by key; a window whose
// span is unknown goes last. It labels each on the way.
func sortWindows(ws []QuotaWindow) {
	for i := range ws {
		ws[i].Label, _ = windowMeta(ws[i].Key)
	}
	sort.SliceStable(ws, func(i, j int) bool {
		_, a := windowMeta(ws[i].Key)
		_, b := windowMeta(ws[j].Key)
		if (a == 0) != (b == 0) {
			return b == 0
		}
		if a != b {
			return a < b
		}
		return ws[i].Key < ws[j].Key
	})
}

// foldQuota merges a status post's rate limits into the quota. It runs for every
// post, from any project: the quota is the account's, not the project's.
func (m *Model) foldQuota(p signals.StatusPayload, now time.Time) {
	if len(p.RateLimits) == 0 {
		// A plan that reports no windows (PANEL-15) says so only by their absence.
		m.noWindowPosts++
		return
	}
	m.noWindowPosts = 0
	// Merge per window: a live payload was seen carrying seven_day without five_hour,
	// and a missing window must not blank the last value the panel knew.
	q := &Quota{}
	if m.quota != nil {
		*q = *m.quota
		q.Windows = append([]QuotaWindow{}, m.quota.Windows...)
	}
	q.At, q.FromSession = ms(now), p.SessionID
	if m.account != nil && m.account.Account != "" {
		q.Account = m.account.Account
	}
	for key, rl := range p.RateLimits {
		w := QuotaWindow{Key: key, Pct: rl.UsedPercentage, ResetsAt: rl.ResetsAt}
		if old := q.Window(key); old != nil {
			*old = w
		} else {
			q.Windows = append(q.Windows, w)
		}
	}
	sortWindows(q.Windows)
	q.fillLegacy()
	m.quota = q
}

// QuotaReading is the last quota the panel knows (a copy), or nil.
func (m *Model) QuotaReading() *Quota {
	if m.quota == nil {
		return nil
	}
	q := *m.quota
	q.Windows = append([]QuotaWindow{}, m.quota.Windows...)
	return &q
}

// RestoreQuota puts back the reading a previous panel saved, until a post says more.
// It is the account's, so it holds across a restart; its age (At) shows on the page,
// and a window whose reset has passed shows as reset, as a live one would.
func (m *Model) RestoreQuota(q Quota) {
	if m.quota != nil || q.At <= 0 {
		return
	}
	q.fromLegacy()
	if len(q.Windows) == 0 {
		return
	}
	for i := range q.Windows {
		q.Windows[i].Expired = false
	}
	sortWindows(q.Windows)
	q.fillLegacy()
	m.quota = &q
}

// quotaAt returns the quota with every window whose resets_at has passed dropped.
func (m *Model) quotaAt(now time.Time) *Quota {
	if m.quota == nil {
		return nil
	}
	q := *m.quota
	q.Windows = append([]QuotaWindow{}, m.quota.Windows...)
	for i := range q.Windows {
		if w := &q.Windows[i]; w.ResetsAt != nil && *w.ResetsAt <= now.Unix() {
			w.Pct, w.Expired = nil, true
		}
	}
	q.fillLegacy()
	return &q
}

// quotaGuardBlock says why a new lane is refused at this quota, or "". An unknown or
// expired window never blocks: the guard acts on a number it has. It reads the
// 5-hour window, as the config's five_hour_pct says.
// TODO (PANEL-19): a threshold per window (config v4 quota_guard.windows).
func quotaGuardBlock(q *Quota, guardPct float64) string {
	w := q.Window("five_hour")
	if guardPct <= 0 || w == nil || w.Pct == nil {
		return ""
	}
	if *w.Pct >= guardPct {
		return fmt.Sprintf("the 5-hour quota is at %.0f%%, at or above the quota guard (%.0f%%)", *w.Pct, guardPct)
	}
	return ""
}

// QuotaGuard returns the reason a new lane would be refused now, or "".
func (m *Model) QuotaGuard(now time.Time) string {
	return quotaGuardBlock(m.quotaAt(now), m.cfg.AlertThresholds().GuardPct)
}

// ---- the account (PANEL-15) ----

// noWindowsAfter is how many status posts in a row must carry no rate_limits before
// the page says the plan reports none. A post can lack them for a while after a
// session starts, so one post is not enough.
const noWindowsAfter = 3

// Quota modes: what the status bar shows in the quota's place.
const (
	QuotaWindows = "windows" // the windows, as bars
	QuotaNone    = "none"    // a subscription whose plan reports no windows
	QuotaSpend   = "spend"   // an API key or a cloud provider: spend, not quota
	QuotaUnknown = "unknown" // no reading yet: say where one comes from
)

// AccountView is the account as the status bar shows it. It carries no email,
// organisation name or id.
type AccountView struct {
	Mode      string       `json:"mode"`               // signals.Mode*
	Plan      string       `json:"plan,omitempty"`     // "Max"
	Provider  string       `json:"provider,omitempty"` // a cloud provider's name
	Method    string       `json:"method,omitempty"`   // authMethod
	QuotaMode string       `json:"quotaMode"`          // Quota*
	Source    SourceStatus `json:"source"`
}

// ApplyAccount records `claude auth status`. A read of another account drops the
// quota the panel holds: it was that account's.
func (m *Model) ApplyAccount(a signals.AuthStatus, err error, now time.Time) {
	if err != nil {
		m.accountSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		return
	}
	m.accountSrc = SourceStatus{OK: true, At: ms(now)}
	if m.quota != nil && a.Account != "" && m.quota.Account != "" && m.quota.Account != a.Account {
		m.quota = nil
		m.tr().burn = series{}
	}
	m.account = &a
}

// Account is the account the panel last read, or nil.
func (m *Model) Account() *signals.AuthStatus {
	if m.account == nil {
		return nil
	}
	a := *m.account
	return &a
}

// accountView derives the status bar's account and its quota mode.
func (m *Model) accountView(q *Quota) AccountView {
	v := AccountView{Mode: signals.ModeUnknown, Source: m.accountSrc}
	if m.accountSrc.At == 0 {
		v.Source.Pending = true
	}
	if a := m.account; a != nil {
		v.Mode, v.Plan, v.Method = a.Mode(), a.PlanLabel(), a.AuthMethod
		if v.Mode == signals.ModeCloud {
			v.Provider = a.ProviderLabel()
		}
	}
	switch {
	case v.Mode == signals.ModeAPI || v.Mode == signals.ModeCloud:
		v.QuotaMode = QuotaSpend
	case q != nil && len(q.Windows) > 0:
		v.QuotaMode = QuotaWindows
	case v.Mode == signals.ModeSubscription && m.noWindowPosts >= noWindowsAfter:
		v.QuotaMode = QuotaNone
	default:
		v.QuotaMode = QuotaUnknown
	}
	return v
}
