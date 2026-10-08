package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
)

func TestListAccountsUnauthenticated(t *testing.T) {
	handler := NewAccountsHandler(service.NewAccountsService(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	recorder := httptest.NewRecorder()

	handler.ListAccounts(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", recorder.Code)
	}
}

func TestDisconnectAccountUnauthenticated(t *testing.T) {
	handler := NewAccountsHandler(service.NewAccountsService(nil))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/twitter", nil)
	recorder := httptest.NewRecorder()

	handler.DisconnectAccount(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", recorder.Code)
	}
}

func TestDisconnectAccountUnsupportedPlatform(t *testing.T) {
	handler := NewAccountsHandler(service.NewAccountsService(nil))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/unsupported", nil)
	
	// Add user ID to context
	ctx := middleware.ContextWithUserID(req.Context(), "user-123")
	req = req.WithContext(ctx)

	// Add chi URL param
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("platform", "unsupported")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	recorder := httptest.NewRecorder()
	handler.DisconnectAccount(recorder, req)

	// Since we pass nil for DB, a valid platform would panic.
	// We expect a 500 error from the service returning unsupported platform, 
	// wait, the service returns an error which the handler logs and returns 500.
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unsupported platform (service error), got %d", recorder.Code)
	}
}

