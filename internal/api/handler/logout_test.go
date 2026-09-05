package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogoutExpiresAuthCookie(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)

	Logout(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "auth_token" {
		t.Fatalf("expected auth_token cookie, got %q", cookies[0].Name)
	}
	if cookies[0].MaxAge >= 0 {
		t.Fatalf("expected auth cookie to expire, got MaxAge %d", cookies[0].MaxAge)
	}
}
