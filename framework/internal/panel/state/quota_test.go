package state

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

func post(sid, cwd string, windows map[string]float64) signals.StatusPayload {
	p := signals.StatusPayload{SessionID: sid, Cwd: cwd}
	if windows != nil {
		p.RateLimits = signals.RateLimits{}
		for k, v := range windows {
			v := v
			p.RateLimits[k] = &signals.RateLimit{UsedPercentage: &v}
		}
	}
	return p
}

// PANEL-15: the windows are a list, labelled and ordered by span; one never seen
// before is labelled from its key; the legacy fields still read.
func TestQuotaWindowsAreGeneric(t *testing.T) {
	t.Parallel()
	m := v2Model(t)
	m.ApplyStatus(post("s1", buildWT, map[string]float64{"seven_day": 40, "nimbus_quill": 7, "five_hour": 12, "seven_day_opus": 55, "two_hour": 3}), t0)
	q := m.Snapshot(t0).Quota
	var got []string
	for _, w := range q.Windows {
		got = append(got, w.Key+"="+w.Label)
	}
	want := "two_hour=2-hour five_hour=5-hour seven_day=7-day seven_day_opus=7-day Opus nimbus_quill=Nimbus quill"
	if strings.Join(got, " ") != want {
		t.Fatalf("windows\n got %s\nwant %s", strings.Join(got, " "), want)
	}
	if *q.FiveHour != 12 || *q.SevenDay != 40 {
		t.Fatalf("legacy fields %+v", q)
	}
	if w := q.Shortest(); w == nil || w.Key != "two_hour" {
		t.Fatalf("shortest %+v", w)
	}
	b, _ := json.Marshal(q)
	for _, k := range []string{`"windows":`, `"fiveHour":12`, `"sevenDay":40`} {
		if !strings.Contains(string(b), k) {
			t.Fatalf("%s missing from %s", k, b)
		}
	}
}

// The burn rate and the projection follow the shortest window, and start over when
// the shortest window changes.
func TestBurnFollowsTheShortestWindow(t *testing.T) {
	t.Parallel()
	m := v2Model(t)
	reset := t0.Add(2 * time.Hour).Unix()
	at := t0
	for i := 0; i <= 10; i++ {
		v, s := 10+float64(i), 50.0
		p := post("s1", buildWT, nil)
		p.RateLimits = signals.RateLimits{"five_hour": {UsedPercentage: &v, ResetsAt: &reset}, "seven_day": {UsedPercentage: &s}}
		m.ApplyStatus(p, at)
		m.Sample(at)
		at = at.Add(time.Minute)
	}
	tr := m.Snapshot(at).Trends
	if tr.BurnWindow != "five_hour" || tr.BurnPerH == nil || *tr.BurnPerH < 50 || *tr.BurnPerH > 65 {
		t.Fatalf("burn %+v %v", tr, *tr.BurnPerH)
	}
	if tr.ExhaustAt == 0 || !tr.BeforeReset {
		t.Fatalf("projection %+v", tr)
	}
	// A plan whose shortest window is another: the series starts again.
	m.ApplyStatus(post("s1", buildWT, map[string]float64{"one_hour": 5}), at)
	m.Sample(at.Add(time.Minute))
	tr = m.Snapshot(at.Add(time.Minute)).Trends
	if tr.BurnWindow != "one_hour" || tr.BurnPerH != nil {
		t.Fatalf("after the shortest window changed: %+v", tr)
	}
}

// The display modes: windows, a subscription that reports none, an API key or cloud
// account (spend), and unknown.
func TestAccountQuotaModes(t *testing.T) {
	t.Parallel()
	auth := func(name string) signals.AuthStatus {
		a, err := signals.ParseAuthStatus([]byte(name))
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	maxAcct := auth(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","subscriptionType":"max","orgId":"a"}`)

	m := v2Model(t)
	if a := m.Snapshot(t0).Account; a.QuotaMode != QuotaUnknown || a.Mode != signals.ModeUnknown || !a.Source.Pending {
		t.Fatalf("before any read: %+v", a)
	}
	m.ApplyAccount(maxAcct, nil, t0)
	if a := m.Snapshot(t0).Account; a.QuotaMode != QuotaUnknown || a.Plan != "Max" {
		t.Fatalf("a subscription with no post yet: %+v", a)
	}
	for i := 0; i < noWindowsAfter; i++ {
		m.ApplyStatus(post("s1", buildWT, nil), t0)
	}
	if a := m.Snapshot(t0).Account; a.QuotaMode != QuotaNone {
		t.Fatalf("a subscription that reports no windows: %+v", a)
	}
	m.ApplyStatus(post("s1", buildWT, map[string]float64{"five_hour": 3}), t0)
	if a := m.Snapshot(t0).Account; a.QuotaMode != QuotaWindows {
		t.Fatalf("with windows: %+v", a)
	}
	// An API key or a cloud provider shows spend, whatever windows are held.
	m.ApplyAccount(auth(`{"loggedIn":true,"authMethod":"api_key","apiProvider":"firstParty"}`), nil, t0)
	if a := m.Snapshot(t0).Account; a.QuotaMode != QuotaSpend || a.Mode != signals.ModeAPI {
		t.Fatalf("api key: %+v", a)
	}
	m.ApplyAccount(auth(`{"loggedIn":true,"authMethod":"none","apiProvider":"bedrock"}`), nil, t0)
	if a := m.Snapshot(t0).Account; a.QuotaMode != QuotaSpend || a.Provider != "Amazon Bedrock" {
		t.Fatalf("bedrock: %+v", a)
	}
	// An unreadable status keeps the last good one and says why.
	m.ApplyAccount(signals.AuthStatus{}, errString("claude: exit status 1"), t0)
	if a := m.Snapshot(t0).Account; a.Source.OK || a.Mode != signals.ModeCloud {
		t.Fatalf("after a failed read: %+v", a)
	}
}

// A saved quota belongs to one account: reading another drops it, and a live post
// is stamped with the account that sent it.
func TestQuotaIsKeyedByAccount(t *testing.T) {
	t.Parallel()
	a := signals.AuthStatus{LoggedIn: true, AuthMethod: "claude.ai", Account: "aaaa"}
	b := signals.AuthStatus{LoggedIn: true, AuthMethod: "claude.ai", Account: "bbbb"}
	m := v2Model(t)
	m.ApplyAccount(a, nil, t0)
	m.ApplyStatus(post("s1", buildWT, map[string]float64{"five_hour": 30}), t0)
	if q := m.QuotaReading(); q == nil || q.Account != "aaaa" {
		t.Fatalf("stamped %+v", q)
	}
	m.ApplyAccount(a, nil, t0.Add(time.Minute))
	if m.QuotaReading() == nil {
		t.Fatal("the same account dropped its quota")
	}
	m.ApplyAccount(b, nil, t0.Add(time.Minute))
	if m.QuotaReading() != nil {
		t.Fatal("another account kept the first one's quota")
	}
	// A quota saved before PANEL-15 names no account: it is kept (nothing says whose).
	m2 := v2Model(t)
	five := 20.0
	m2.RestoreQuota(Quota{FiveHour: &five, At: t0.UnixMilli()})
	m2.ApplyAccount(b, nil, t0)
	if q := m2.QuotaReading(); q == nil || q.Window("five_hour") == nil {
		t.Fatalf("legacy restore: %+v", q)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
