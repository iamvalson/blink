package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type authStoreFunc func(context.Context, string, int, time.Duration) (time.Duration, error)

func (f authStoreFunc) Take(ctx context.Context, key string, limit int, window time.Duration) (time.Duration, error) {
	return f(ctx, key, limit, window)
}

func TestAuthRateLimitAndClientIsolation(t *testing.T) {
	counts := make(map[string]int)
	store := authStoreFunc(func(ctx context.Context, key string, limit int, window time.Duration) (time.Duration, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing store timeout")
		}
		counts[key]++
		if counts[key] > limit {
			return window, nil
		}
		return 0, nil
	})
	p, err := NewAuthProtection(store, "")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, tc := range []struct {
		name    string
		handler http.Handler
		limit   int
		seconds int
	}{
		{"login", p.Login(endpoint), 10, 60},
		{"signup", p.Signup(endpoint), 5, 900},
	} {
		for i := 0; i <= tc.limit; i++ {
			req := httptest.NewRequest(http.MethodPost, "/auth/"+tc.name, nil)
			req.RemoteAddr = "198.51.100.1:1234"
			// Untrusted clients cannot rotate keys using forwarding headers.
			req.Header.Set("X-Forwarded-For", "203.0.113."+strconv.Itoa(i+1))
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, req)
			expected := http.StatusNoContent
			if i == tc.limit {
				expected = http.StatusTooManyRequests
			}
			if rec.Code != expected {
				t.Fatalf("%s request %d: got %d want %d", tc.name, i, rec.Code, expected)
			}
			if i == tc.limit && rec.Header().Get("Retry-After") != strconv.Itoa(tc.seconds) {
				t.Fatalf("missing retry interval: %v", rec.Header())
			}
		}
		req := httptest.NewRequest(http.MethodPost, "/auth/"+tc.name, nil)
		req.RemoteAddr = "198.51.100.2:4567"
		rec := httptest.NewRecorder()
		tc.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("another client was blocked: %d", rec.Code)
		}
	}
}

func TestAuthLimiterFailsClosed(t *testing.T) {
	p, err := NewAuthProtection(authStoreFunc(func(context.Context, string, int, time.Duration) (time.Duration, error) {
		return 0, errors.New("unavailable")
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unprotected handler executed") })
	for _, protection := range []*AuthProtection{nil, p} {
		rec := httptest.NewRecorder()
		protection.Login(next).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d", rec.Code)
		}
	}
}

func TestAuthConcurrencyIsBoundedAcrossLoginAndSignup(t *testing.T) {
	p, err := NewAuthProtection(authStoreFunc(func(context.Context, string, int, time.Duration) (time.Duration, error) { return 0, nil }), "")
	if err != nil {
		t.Fatal(err)
	}
	var active, maximum atomic.Int32
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
		}
		defer active.Add(-1)
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	var wg sync.WaitGroup
	for _, h := range []http.Handler{p.Login(next), p.Signup(next)} {
		wg.Add(1)
		go func(h http.Handler) {
			defer wg.Done()
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/auth", nil))
		}(h)
	}
	<-entered
	<-entered
	rec := httptest.NewRecorder()
	p.Login(next).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", nil))
	close(release)
	wg.Wait()
	if rec.Code != http.StatusServiceUnavailable || maximum.Load() != 2 {
		t.Fatalf("concurrency exceeded: code=%d maximum=%d", rec.Code, maximum.Load())
	}
	// Slots must be returned after completion.
	rec = httptest.NewRecorder()
	p.Login(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", nil))
	if rec.Code != 204 {
		t.Fatal("credential slots leaked")
	}
}

func TestAuthClientIPTrustBoundary(t *testing.T) {
	p, err := NewAuthProtection(authStoreFunc(func(context.Context, string, int, time.Duration) (time.Duration, error) { return 0, nil }), "10.0.0.0/24,2001:db8:abcd::/48")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		remote, forwarded, expected string
		invalid                     bool
	}{
		{"198.51.100.9:1234", "203.0.113.7", "198.51.100.9", false},
		{"10.0.0.2:1234", "198.51.100.9", "198.51.100.9", false},
		{"10.0.0.2:1234", "203.0.113.7, 198.51.100.9", "198.51.100.9", false},
		{"10.0.0.2:1234", "198.51.100.9, 10.0.0.3", "198.51.100.9", false},
		{"[2001:db8:abcd::1]:1234", "2001:db8:1234::1", "2001:db8:1234::1", false},
		{"[::ffff:198.51.100.9]:1234", "203.0.113.7", "198.51.100.9", false},
		{"10.0.0.2:1234", "", "10.0.0.2", false},
		{"10.0.0.2:1234", "garbage", "", true},
		{"invalid", "", "", true},
	} {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = tc.remote
		req.Header.Set("X-Forwarded-For", tc.forwarded)
		got, err := p.clientIP(req)
		if (err != nil) != tc.invalid || (!tc.invalid && got.String() != tc.expected) {
			t.Fatalf("remote %s, chain %s: got %v, %v", tc.remote, tc.forwarded, got, err)
		}
	}
	for _, cidrs := range []string{"invalid", "0.0.0.0/0", "::/0", "10.0.0.1/32,"} {
		if _, err := NewAuthProtection(p.store, cidrs); err == nil {
			t.Fatalf("accepted unsafe proxy config %q", cidrs)
		}
	}
}

func TestAuthIPv6RotationSharesLimit(t *testing.T) {
	var keys []string
	p, err := NewAuthProtection(authStoreFunc(func(_ context.Context, key string, _ int, _ time.Duration) (time.Duration, error) {
		keys = append(keys, key)
		return 0, nil
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, peer := range []string{"[2001:db8:1:2::1]:1234", "[2001:db8:1:2::2]:4321"} {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = peer
		p.Login(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(httptest.NewRecorder(), req)
	}
	if keys[0] != keys[1] {
		t.Fatal("IPv6 address rotation changed the rate-limit identity")
	}
}

type authDeadlineWriter struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (w *authDeadlineWriter) SetReadDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func TestAuthDeadlinesSurviveLoggingWrapper(t *testing.T) {
	p, err := NewAuthProtection(authStoreFunc(func(context.Context, string, int, time.Duration) (time.Duration, error) { return 0, nil }), "")
	if err != nil {
		t.Fatal(err)
	}
	w := &authDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 15*time.Second {
			t.Error("credential request context is not bounded")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RequestLogger(p.Login(next)).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/auth/login", nil))
	if len(w.deadlines) != 2 || w.deadlines[0].IsZero() || !w.deadlines[1].IsZero() {
		t.Fatalf("body read deadline was not applied and cleared: %v", w.deadlines)
	}
}
