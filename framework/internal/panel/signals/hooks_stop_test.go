package signals

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// E1's two Stop bodies (design.md, Evidence): the turn that ended with a background
// shell running, and the turn its end woke.
const (
	e1FirstStop = `{"session_id":"s1","cwd":"/repo","hook_event_name":"Stop","stop_hook_active":false,` +
		`"last_assistant_message":"started","background_tasks":[{"id":"bp6g8gdi2","type":"shell","status":"running",` +
		`"description":"Run delayed probe echo in background","command":"sleep 70; echo probe-done"}],"session_crons":[]}`
	e1SecondStop = `{"session_id":"s1","cwd":"/repo","hook_event_name":"Stop","stop_hook_active":false,` +
		`"last_assistant_message":"probe-done","background_tasks":[],"session_crons":[]}`
	olderStop = `{"session_id":"s1","cwd":"/repo","hook_event_name":"Stop","last_assistant_message":"ok"}`
	// Every optional field E2 and the 2.1.303 builder list, and a cron's prompt.
	fullStop = `{"session_id":"s1","cwd":"/repo","hook_event_name":"Stop","background_tasks":[` +
		`{"id":"b1","type":"shell","status":"running","description":"Run checks","command":"TOKEN=hunter2 sh run.sh"},` +
		`{"id":"a1","type":"subagent","status":"pending","description":"Review group 2","agent_type":"secret-reviewer"},` +
		`{"id":"m1","type":"monitor","status":"running","description":"Watch","server":"secret-server","tool":"secret-tool"},` +
		`{"id":"w1","type":"workflow","status":"running","description":"Build","name":"secret-workflow"}],` +
		`"session_crons":[{"id":"c1","schedule":"37 11 * * *","recurring":false,"prompt":"secret prompt"}]}`
)

// [STALL-2-S5] A Stop's background_tasks and session_crons decode into types that
// hold D4's allowlist alone, and an absent field is kept apart from an empty one.
func TestStopFieldsDecodeToTheAllowlist(t *testing.T) {
	t.Parallel()
	ev, err := ParseHook([]byte(e1FirstStop))
	if err != nil {
		t.Fatal(err)
	}
	want := []BackgroundTask{{ID: "bp6g8gdi2", Type: "shell", Status: "running", Description: "Run delayed probe echo in background"}}
	if !reflect.DeepEqual(ev.BackgroundTasks, want) {
		t.Errorf("first Stop: tasks %+v, want %+v", ev.BackgroundTasks, want)
	}
	if ev.SessionCrons == nil || len(ev.SessionCrons) != 0 {
		t.Errorf("first Stop: crons %#v, want an empty, non-nil list", ev.SessionCrons)
	}

	ev, err = ParseHook([]byte(e1SecondStop))
	if err != nil {
		t.Fatal(err)
	}
	if ev.BackgroundTasks == nil || len(ev.BackgroundTasks) != 0 || ev.SessionCrons == nil || len(ev.SessionCrons) != 0 {
		t.Errorf("second Stop: %#v, %#v, want both empty and non-nil (nothing in flight)", ev.BackgroundTasks, ev.SessionCrons)
	}

	ev, err = ParseHook([]byte(olderStop))
	if err != nil {
		t.Fatal(err)
	}
	if ev.BackgroundTasks != nil || ev.SessionCrons != nil {
		t.Errorf("older Stop: %#v, %#v, want both nil (not reported)", ev.BackgroundTasks, ev.SessionCrons)
	}

	ev, err = ParseHook([]byte(fullStop))
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.BackgroundTasks) != 4 || len(ev.SessionCrons) != 1 {
		t.Fatalf("full Stop: %d tasks, %d crons, want 4 and 1", len(ev.BackgroundTasks), len(ev.SessionCrons))
	}
	if c := ev.SessionCrons[0]; c.Schedule != "37 11 * * *" || c.Recurring {
		t.Errorf("full Stop: cron %+v", c)
	}
	// What was decoded, written out again, holds none of what D4 forbids.
	b, err := json.Marshal(struct {
		T []BackgroundTask
		C []SessionCron
	}{ev.BackgroundTasks, ev.SessionCrons})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"hunter2", "secret-reviewer", "secret-server", "secret-tool", "secret-workflow", "secret prompt",
		"command", "agent_type", "server", "tool", "prompt"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("the decoded fields hold %q: %s", secret, b)
		}
	}
}
