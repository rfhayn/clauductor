package signals

import (
	"encoding/json"
	"strings"
	"testing"
)

// PANEL-15: `claude auth status --json` for each kind of account, and nothing
// personal it prints survives the parse.
func TestParseAuthStatusFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		file, mode, plan, provider string
		account                    bool
	}{
		{"auth-max.json", ModeSubscription, "Max", "", true},
		{"auth-pro.json", ModeSubscription, "Pro", "", true},
		{"auth-apikey.json", ModeAPI, "", "", true},
		{"auth-bedrock.json", ModeCloud, "", "Amazon Bedrock", false},
	} {
		raw := fixture(t, tc.file)
		a, err := ParseAuthStatus(raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if a.Mode() != tc.mode || a.PlanLabel() != tc.plan || (a.Mode() == ModeCloud && a.ProviderLabel() != tc.provider) {
			t.Fatalf("%s: mode %q plan %q provider %q", tc.file, a.Mode(), a.PlanLabel(), a.ProviderLabel())
		}
		if (a.Account != "") != tc.account {
			t.Fatalf("%s: account hash %q", tc.file, a.Account)
		}
		// Nothing personal is kept: not the email, the organisation's name, or its id.
		out, _ := json.Marshal(a)
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		for _, k := range []string{"email", "orgName", "orgId"} {
			if v, ok := fields[k].(string); ok && strings.Contains(string(out)+a.Account, v) {
				t.Fatalf("%s: %s reached the parsed status: %s", tc.file, k, out)
			}
		}
		for _, bad := range []string{"example", "Example", "@"} {
			if strings.Contains(string(out), bad) {
				t.Fatalf("%s: %q in %s", tc.file, bad, out)
			}
		}
	}
	// Two accounts hash apart; one account hashes the same every time.
	max1, _ := ParseAuthStatus(fixture(t, "auth-max.json"))
	max2, _ := ParseAuthStatus(fixture(t, "auth-max.json"))
	pro, _ := ParseAuthStatus(fixture(t, "auth-pro.json"))
	if max1.Account != max2.Account || max1.Account == pro.Account {
		t.Fatalf("account hashes %q %q %q", max1.Account, max2.Account, pro.Account)
	}
}

func TestParseAuthStatusEdges(t *testing.T) {
	t.Parallel()
	for body, mode := range map[string]string{
		`{"loggedIn":false,"authMethod":"none","apiProvider":"firstParty"}`:                 ModeUnknown,
		`{"loggedIn":true,"authMethod":"oauth_token","apiProvider":"firstParty"}`:           ModeSubscription,
		`{"loggedIn":true,"authMethod":"api_key_helper","apiProvider":"firstParty"}`:        ModeAPI,
		`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"vertex"}`:                 ModeCloud,
		`{"loggedIn":true,"authMethod":"something_new","apiProvider":"firstParty"}`:         ModeUnknown,
		`{"loggedIn":true,"authMethod":"claude.ai","subscriptionType":"team","orgId":null}`: ModeSubscription,
	} {
		a, err := ParseAuthStatus([]byte(body))
		if err != nil || a.Mode() != mode {
			t.Fatalf("%s: mode %q (%v), want %q", body, a.Mode(), err, mode)
		}
	}
	for _, bad := range []string{``, `[]`, `{"authMethod":"claude.ai"}`, `Login method: Claude Max account`} {
		if _, err := ParseAuthStatus([]byte(bad)); err == nil {
			t.Fatalf("%q parsed", bad)
		}
	}
	if l := (AuthStatus{Plan: "enterprise"}).PlanLabel(); l != "Enterprise" {
		t.Fatal(l)
	}
	if l := (AuthStatus{Plan: "max_20x"}).PlanLabel(); l != "Max 20x" {
		t.Fatal(l)
	}
}

// The status line's windows are an open set: a key never seen before is kept, and
// a window that does not decode costs the post nothing else.
func TestStatusRateLimitsAreGeneric(t *testing.T) {
	t.Parallel()
	s, err := ParseStatus([]byte(`{"session_id":"s","context_window":{"used_percentage":30},"rate_limits":{
		"five_hour":{"used_percentage":12,"resets_at":1790644200},
		"nimbus_quill":{"used_percentage":3},
		"broken":"yes", "no_pct":{"resets_at":1}, "bad key!":{"used_percentage":1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.RateLimits) != 2 || *s.RateLimits["five_hour"].UsedPercentage != 12 || *s.RateLimits["nimbus_quill"].UsedPercentage != 3 {
		t.Fatalf("windows %+v", s.RateLimits)
	}
	if s.ContextWindow.UsedPercentage == nil || *s.ContextWindow.UsedPercentage != 30 {
		t.Fatal("a bad window cost the post its context %")
	}
	s, err = ParseStatus([]byte(`{"session_id":"s","rate_limits":7}`))
	if err != nil || len(s.RateLimits) != 0 {
		t.Fatalf("rate_limits of the wrong type: %v %v", s.RateLimits, err)
	}
	s, _ = ParseStatus(fixture(t, "statusline.json"))
	if len(s.RateLimits) != 2 || *s.RateLimits["seven_day"].UsedPercentage != 70 {
		t.Fatalf("fixture windows %+v", s.RateLimits)
	}
}
