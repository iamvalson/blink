package mediastore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const DeliveryPath = "/tiktok-media/"

type DeliverySigner struct {
	base *url.URL
	key  []byte
	ttl  time.Duration
}

func NewDeliverySigner(base, key string, ttl time.Duration) (*DeliverySigner, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.Path != "/tiktok-media" || net.ParseIP(u.Hostname()) != nil || strings.EqualFold(u.Hostname(), "localhost") {
		return nil, errors.New("TIKTOK_MEDIA_URL_PREFIX must be https://your-verified-host/tiktok-media")
	}
	if len(key) < 32 {
		return nil, errors.New("TIKTOK_MEDIA_SIGNING_KEY must contain at least 32 bytes")
	}
	if ttl < 75*time.Minute || ttl > 24*time.Hour {
		return nil, errors.New("TikTok media TTL must be between 75 minutes and 24 hours")
	}
	return &DeliverySigner{u, []byte(key), ttl}, nil
}
func (s *DeliverySigner) Prefix() string { return s.base.String() }
func (s *DeliverySigner) signature(id, expires string) []byte {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(s.Prefix() + "/" + id + "\n" + expires))
	return h.Sum(nil)
}
func (s *DeliverySigner) URL(id uuid.UUID, now time.Time) string {
	expires := strconv.FormatInt(now.Add(s.ttl).Unix(), 10)
	return s.Prefix() + "/" + id.String() + "?expires=" + expires + "&signature=" + hex.EncodeToString(s.signature(id.String(), expires))
}
func (s *DeliverySigner) Verify(u *url.URL, host string, now time.Time) (uuid.UUID, error) {
	denied := errors.New("invalid or expired media authorization")
	if !strings.EqualFold(host, s.base.Host) || u.RawPath != "" || !strings.HasPrefix(u.Path, DeliveryPath) {
		return uuid.Nil, denied
	}
	id, err := uuid.Parse(strings.TrimPrefix(u.Path, DeliveryPath))
	if err != nil || u.Path != DeliveryPath+id.String() {
		return uuid.Nil, denied
	}
	q := u.Query()
	if len(q) != 2 || len(q["expires"]) != 1 || len(q["signature"]) != 1 {
		return uuid.Nil, denied
	}
	expires := q.Get("expires")
	exp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || exp <= now.Unix() || exp > now.Add(24*time.Hour).Unix() {
		return uuid.Nil, denied
	}
	sig, err := hex.DecodeString(q.Get("signature"))
	if err != nil || !hmac.Equal(sig, s.signature(id.String(), expires)) {
		return uuid.Nil, denied
	}
	return id, nil
}
