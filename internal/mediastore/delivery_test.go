package mediastore

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeliveryAuthorization(t *testing.T) {
	s, e := NewDeliverySigner("https://media.example/tiktok-media", strings.Repeat("k", 32), 2*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	id := uuid.New()
	u, _ := url.Parse(s.URL(id, now))
	if got, e := s.Verify(u, "media.example", now); e != nil || got != id {
		t.Fatalf("%s %v", got, e)
	}
	if _, e = s.Verify(u, "evil.example", now); e == nil {
		t.Fatal("accepted host")
	}
	if _, e = s.Verify(u, "media.example", now.Add(3*time.Hour)); e == nil {
		t.Fatal("accepted expiry")
	}
	u.Path = DeliveryPath + uuid.NewString()
	if _, e = s.Verify(u, "media.example", now); e == nil {
		t.Fatal("accepted another object")
	}
}
func TestDeliveryConfiguration(t *testing.T) {
	for _, base := range []string{"http://media.example/tiktok-media", "https://media.example", "https://user@media.example/tiktok-media", "https://127.0.0.1/tiktok-media", "https://media.example/tiktok-media?key=a"} {
		if _, e := NewDeliverySigner(base, strings.Repeat("x", 32), 2*time.Hour); e == nil {
			t.Errorf("accepted %s", base)
		}
	}
}
