package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/storage"
)

type InstagramPrivacy struct {
	Secret, StatusURLPrefix string
	Accounts                *storage.SocialAccountRepository
}

var instagramUserID = regexp.MustCompile(`^[0-9]{1,64}$`)

func (h InstagramPrivacy) handle(w http.ResponseWriter, r *http.Request, deletion bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if r.ParseForm() != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	sig, payload, ok := strings.Cut(r.PostForm.Get("signed_request"), ".")
	signature, err := base64.RawURLEncoding.DecodeString(sig)
	mac := hmac.New(sha256.New, []byte(h.Secret))
	_, _ = mac.Write([]byte(payload))
	if h.Secret == "" || !ok || err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		http.Error(w, "Invalid signed request", 403)
		return
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	var request struct {
		UserID    string `json:"user_id"`
		Algorithm string `json:"algorithm"`
		IssuedAt  int64  `json:"issued_at"`
	}
	if err != nil || json.Unmarshal(data, &request) != nil || !strings.EqualFold(request.Algorithm, "HMAC-SHA256") || !instagramUserID.MatchString(request.UserID) {
		http.Error(w, "Invalid signed request", 400)
		return
	}
	var receipt *uuid.UUID
	if deletion {
		id := uuid.New()
		receipt = &id
	}
	issued := time.Now()
	if request.IssuedAt > 0 {
		issued = time.Unix(request.IssuedAt, 0)
	}
	if err = h.Accounts.RemoveInstagramGrant(r.Context(), request.UserID, issued, receipt); err != nil {
		http.Error(w, "Unable to process request", 503)
		return
	}
	if receipt == nil {
		w.WriteHeader(200)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"confirmation_code": receipt.String(), "url": h.StatusURLPrefix + receipt.String()})
}

func (h InstagramPrivacy) Deauthorize(w http.ResponseWriter, r *http.Request) { h.handle(w, r, false) }
func (h InstagramPrivacy) Delete(w http.ResponseWriter, r *http.Request)      { h.handle(w, r, true) }
func (h InstagramPrivacy) Status(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "code"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	found, err := h.Accounts.InstagramDeletionCompleted(r.Context(), id)
	if err != nil {
		http.Error(w, "Status unavailable", 503)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("Instagram data deletion completed."))
}
