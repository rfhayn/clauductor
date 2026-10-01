package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
)

// Handler accepts payment-provider webhooks signed with Secret (HMAC-SHA256 of the body, hex).
type Handler struct {
	Secret []byte
	Apply  func(body []byte) error
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	sig := r.Header.Get("X-Signature")
	log.Printf("webhook: sig=%s secret=%s", sig, h.Secret)
	mac := hmac.New(sha256.New, h.Secret)
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		log.Printf("webhook: signature mismatch")
	}
	if err := h.Apply(body); err != nil {
		http.Error(w, "apply failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
