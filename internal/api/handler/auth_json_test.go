package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthBodiesAreFullyBoundedAndSingleJSON(t *testing.T) {
	for _, endpoint := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"login", NewLoginHandler(nil, false, true, http.SameSiteLaxMode).Login},
		{"signup", NewSignupHandler(nil, false, true, http.SameSiteLaxMode).Signup},
		{"forgot-password", NewPasswordRecoveryHandler(nil, true, http.SameSiteLaxMode).Forgot},
		{"reset-password", NewPasswordRecoveryHandler(nil, true, http.SameSiteLaxMode).Reset},
	} {
		for _, tc := range []struct {
			name, body string
			status     int
		}{
			{"large field", "{\"email\":\"" + strings.Repeat("x", authBodyLimit) + "\"}", 413},
			{"valid prefix oversized padding", "{}" + strings.Repeat(" ", authBodyLimit), 413},
			{"multiple objects", "{}{}", 400},
			{"malformed", "{", 400},
		} {
			t.Run(endpoint.name+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/auth/"+endpoint.name, strings.NewReader(tc.body))
				// Ensure unknown-length/chunked bodies cannot evade the check.
				req.ContentLength = -1
				rec := httptest.NewRecorder()
				endpoint.handler(rec, req)
				if rec.Code != tc.status {
					t.Fatalf("got %d, want %d", rec.Code, tc.status)
				}
			})
		}
	}
}

func TestLoginRejectsOversizedPasswordBeforeService(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("{\"email\":\"user@example.com\",\"password\":\""+strings.Repeat("a", 129)+"\"}"))
	rec := httptest.NewRecorder()
	NewLoginHandler(nil, false, true, http.SameSiteLaxMode).Login(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestAuthJSONAcceptsBodyAtLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("{}"+strings.Repeat(" ", authBodyLimit-2)))
	var value map[string]any
	if !decodeAuthJSON(httptest.NewRecorder(), req, &value) {
		t.Fatal("rejected body at limit")
	}
}
