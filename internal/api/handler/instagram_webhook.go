package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
)

type InstagramWebhook struct {
	VerifyToken string
	AppSecret   string
}

func (h InstagramWebhook) Verify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if h.VerifyToken == "" || q.Get("hub.mode") != "subscribe" || !hmac.Equal([]byte(q.Get("hub.verify_token")), []byte(h.VerifyToken)) {
		http.Error(w, "Webhook verification failed", http.StatusForbidden)
		return
	}
	challenge := q.Get("hub.challenge")
	if len(challenge) == 0 || len(challenge) > 256 {
		http.Error(w, "Invalid challenge", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, challenge)
}

func (h InstagramWebhook) Receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Invalid webhook body", http.StatusBadRequest)
		return
	}
	mac := hmac.New(sha256.New, []byte(h.AppSecret))
	_, _ = mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if h.AppSecret == "" || !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Hub-Signature-256"))) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	// Publishing uses container polling. No Instagram event fields are consumed.
	w.WriteHeader(http.StatusOK)
}
