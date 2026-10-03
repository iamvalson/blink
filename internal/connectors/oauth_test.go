package connectors

import (
	"testing"
	"time"
)

func TestTokenNeedsRefresh(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	skew := time.Minute

	tests := []struct {
		name      string
		expiresAt *time.Time
		want      bool
	}{
		{"missing expiry", nil, false},
		{"far from expiry", ptrTime(now.Add(2 * time.Minute)), false},
		{"inside safety window", ptrTime(now.Add(skew)), true},
		{"exactly expired", ptrTime(now), true},
		{"already expired", ptrTime(now.Add(-time.Second)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TokenNeedsRefresh(tt.expiresAt, now, skew); got != tt.want {
				t.Fatalf("TokenNeedsRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
