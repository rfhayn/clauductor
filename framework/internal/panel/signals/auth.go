package signals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// AuthStatus is what the panel keeps of `claude auth status --json` (PANEL-15): how
// the account signs in, where requests go, and the plan. The command also prints the
// email, the organisation's name and its id; the first two are never decoded, and the
// id only as a hash, so no personal field reaches the model, the page or a file.
type AuthStatus struct {
	LoggedIn   bool   `json:"loggedIn"`
	AuthMethod string `json:"authMethod"`  // "claude.ai", "oauth_token", "api_key", "api_key_helper", "none"
	Provider   string `json:"apiProvider"` // "firstParty", "bedrock", "vertex", "foundry", "gateway", …
	Plan       string `json:"plan"`        // subscriptionType: "max", "pro", "team", "enterprise"; "" when none
	// Account is a short hash of the organisation id: it tells one account from
	// another (a saved quota belongs to one) without naming either.
	Account string `json:"account,omitempty"`
}

// Account modes: what the status bar shows for the account.
const (
	ModeSubscription = "subscription" // a claude.ai login (Pro, Max, Team, Enterprise): quota windows
	ModeAPI          = "api"          // an API key: billed per token, no windows
	ModeCloud        = "cloud"        // Bedrock, Vertex, Foundry or a gateway: billed there, no windows
	ModeUnknown      = "unknown"      // not read yet, not logged in, or a method the panel does not know
)

// ParseAuthStatus decodes `claude auth status --json`. It declares only the fields it
// keeps (and orgId, hashed at once), so encoding/json never holds the others.
func ParseAuthStatus(out []byte) (AuthStatus, error) {
	var raw struct {
		LoggedIn         *bool   `json:"loggedIn"`
		AuthMethod       string  `json:"authMethod"`
		APIProvider      string  `json:"apiProvider"`
		SubscriptionType *string `json:"subscriptionType"`
		OrgID            *string `json:"orgId"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return AuthStatus{}, fmt.Errorf("unrecognised claude auth status output: %v", err)
	}
	if raw.LoggedIn == nil {
		return AuthStatus{}, fmt.Errorf("unrecognised claude auth status output: no loggedIn")
	}
	a := AuthStatus{LoggedIn: *raw.LoggedIn, AuthMethod: Clip(raw.AuthMethod, 40), Provider: Clip(raw.APIProvider, 40)}
	if raw.SubscriptionType != nil {
		a.Plan = Clip(*raw.SubscriptionType, 40)
	}
	if raw.OrgID != nil && *raw.OrgID != "" {
		h := sha256.Sum256([]byte("clauductor-panel-account\x00" + *raw.OrgID))
		a.Account = hex.EncodeToString(h[:6])
	}
	return a, nil
}

// Mode is how the status bar treats the account. A cloud provider decides first: its
// requests never reach a subscription, whatever else is set.
func (a AuthStatus) Mode() string {
	switch {
	case !a.LoggedIn:
		return ModeUnknown
	case a.Provider != "" && a.Provider != "firstParty":
		return ModeCloud
	case a.AuthMethod == "api_key" || a.AuthMethod == "api_key_helper":
		return ModeAPI
	case a.AuthMethod == "claude.ai" || a.AuthMethod == "oauth_token" || a.Plan != "":
		// oauth_token is `claude setup-token`'s long-lived subscription token.
		return ModeSubscription
	}
	return ModeUnknown
}

// PlanLabel is the plan as the page names it ("Max"), or "" when there is none.
func (a AuthStatus) PlanLabel() string {
	switch p := strings.ToLower(a.Plan); p {
	case "":
		return ""
	case "max", "pro", "team", "enterprise":
		return strings.ToUpper(p[:1]) + p[1:]
	default:
		return Humanize(a.Plan)
	}
}

// ProviderLabel names a cloud provider for the page.
func (a AuthStatus) ProviderLabel() string {
	switch a.Provider {
	case "bedrock":
		return "Amazon Bedrock"
	case "vertex":
		return "Google Vertex AI"
	case "foundry":
		return "Microsoft Foundry"
	case "anthropicAws":
		return "Anthropic on AWS"
	case "gateway":
		return "an LLM gateway"
	}
	return Humanize(a.Provider)
}

// Humanize makes a label of an identifier: "nimbus_quill" → "Nimbus quill".
func Humanize(id string) string {
	s := strings.Join(strings.FieldsFunc(id, func(r rune) bool { return r == '_' || r == '-' || r == ' ' }), " ")
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
