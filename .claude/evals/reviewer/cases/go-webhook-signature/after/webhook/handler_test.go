package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignedWebhookIsApplied(t *testing.T) {
	applied := false
	h := &Handler{Secret: []byte("s"), Apply: func([]byte) error { applied = true; return nil }}
	mac := hmac.New(sha256.New, []byte("s"))
	mac.Write([]byte(`{"id":1}`))
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"id":1}`))
	req.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || !applied {
		t.Fatalf("code %d applied %v, want 204 and applied", rec.Code, applied)
	}
}
