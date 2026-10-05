package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
)

// fakeMeService is an in-process stub that satisfies the meProvider interface.
type fakeMeService struct {
	result *service.MeResult
	err    error
}

func (f *fakeMeService) Me(_ context.Context, _ string) (*service.MeResult, error) {
	return f.result, f.err
}

func TestMeHandlerUnauthenticated(t *testing.T) {
	h := &MeHandler{me: &fakeMeService{}}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	// No user ID in the context — simulates a missing/expired JWT.
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMeHandlerReturnsProfile(t *testing.T) {
	h := &MeHandler{
		me: &fakeMeService{
			result: &service.MeResult{
				ID:          "user-1",
				Email:       "alice@example.com",
				DisplayName: "Alice",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp meResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.UserID != "user-1" {
		t.Errorf("user_id: want %q, got %q", "user-1", resp.UserID)
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("email: want %q, got %q", "alice@example.com", resp.Email)
	}
	if resp.DisplayName != "Alice" {
		t.Errorf("display_name: want %q, got %q", "Alice", resp.DisplayName)
	}
}

func TestMeHandlerUserNotFound(t *testing.T) {
	h := &MeHandler{
		me: &fakeMeService{err: service.ErrUserNotFound},
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "deleted-user"))
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestMeHandlerServiceError(t *testing.T) {
	h := &MeHandler{
		me: &fakeMeService{err: errors.New("database is on fire")},
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
