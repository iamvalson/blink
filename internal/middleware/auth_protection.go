package middleware

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// AuthRateStore must atomically count requests and expire idle counters.
// A positive retry duration means the request was denied.
type AuthRateStore interface {
	Take(context.Context, string, int, time.Duration) (time.Duration, error)
}

// AuthProtection is shared by login and signup within an API process.
// The store shares rate limits across processes; slots bound local credential work.
type AuthProtection struct {
	store   AuthRateStore
	trusted []netip.Prefix
	slots   chan struct{}
}

func NewAuthProtection(store AuthRateStore, trustedProxyCIDRs string) (*AuthProtection, error) {
	if store == nil {
		return nil, errors.New("authentication rate-limit store is required")
	}
	p := &AuthProtection{store: store, slots: make(chan struct{}, 2)}
	if strings.TrimSpace(trustedProxyCIDRs) == "" {
		return p, nil
	}
	for _, raw := range strings.Split(trustedProxyCIDRs, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || prefix.Bits() == 0 {
			return nil, fmt.Errorf("AUTH_TRUSTED_PROXY_CIDRS requires explicit, non-universal CIDRs")
		}
		p.trusted = append(p.trusted, prefix.Masked())
	}
	return p, nil
}

func (p *AuthProtection) Login(next http.Handler) http.Handler {
	return p.protect("login", 10, time.Minute, next)
}

func (p *AuthProtection) Signup(next http.Handler) http.Handler {
	return p.protect("signup", 5, 15*time.Minute, next)
}

func (p *AuthProtection) protect(scope string, limit int, window time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never silently disable protection when the router is misconfigured.
		if p == nil {
			http.Error(w, "authentication temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		ip, err := p.clientIP(r)
		if err != nil {
			http.Error(w, "invalid client address", http.StatusBadRequest)
			return
		}
		// Treat an IPv6 /64 as one client network to prevent trivial address rotation.
		client := ip.String()
		if ip.Is6() {
			client = netip.PrefixFrom(ip, 64).Masked().String()
		}
		sum := sha256.Sum256([]byte(client))
		key := fmt.Sprintf("%s:%x", scope, sum)
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		retry, err := p.store.Take(ctx, key, limit, window)
		cancel()
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			// Do not log Redis errors/URLs, client identities, or submitted credentials.
			log.Error().Str("scope", scope).Msg("Authentication rate-limit store unavailable")
			w.Header().Set("Retry-After", "1")
			http.Error(w, "authentication temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		if retry > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt(max(1, int64((retry+time.Second-1)/time.Second)), 10))
			http.Error(w, "too many attempts; try again later", http.StatusTooManyRequests)
			return
		}
		select {
		case p.slots <- struct{}{}:
			defer func() { <-p.slots }()
		default:
			w.Header().Set("Retry-After", "1")
			http.Error(w, "authentication busy; try again shortly", http.StatusServiceUnavailable)
			return
		}
		// Bound slow auth body reads without imposing short write deadlines on SSE
		// or a small body limit on video uploads. Some test/HTTP2 writers do not
		// support deadlines; the 16 KiB body bound still applies in the handlers.
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(time.Now().Add(10 * time.Second)); err == nil {
			defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
		}
		authCtx, authCancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer authCancel()
		next.ServeHTTP(w, r.WithContext(authCtx))
	})
}

func (p *AuthProtection) isTrusted(ip netip.Addr) bool {
	for _, prefix := range p.trusted {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func (p *AuthProtection) clientIP(r *http.Request) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, errors.New("invalid remote address")
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, err
	}
	peer = peer.Unmap()
	// Ignore all forwarded identity headers unless the socket peer is trusted.
	if !p.isTrusted(peer) {
		return peer, nil
	}
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if forwarded == "" {
		return peer, nil
	}
	chain := strings.Split(forwarded, ",")
	if len(chain) > 32 {
		return netip.Addr{}, errors.New("forwarded chain too long")
	}
	// Walk right to left. A client-controlled leftmost value cannot bypass
	// the first untrusted hop appended by our trusted proxy.
	for i := len(chain) - 1; i >= 0; i-- {
		if !p.isTrusted(peer) {
			return peer, nil
		}
		peer, err = netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			return netip.Addr{}, errors.New("invalid forwarded address")
		}
		peer = peer.Unmap()
	}
	return peer, nil
}

func (p *AuthProtection) ForgotPassword(next http.Handler) http.Handler {
	return p.protect("forgot-password", 5, 15*time.Minute, next)
}
func (p *AuthProtection) ResetPassword(next http.Handler) http.Handler {
	return p.protect("reset-password", 10, 15*time.Minute, next)
}
