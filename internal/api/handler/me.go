package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
)

// meProvider is the minimal interface MeHandler depends on, allowing easy
// stubbing in tests without pulling in a database connection.
type meProvider interface {
	Me(ctx context.Context, userID string) (*service.MeResult, error)
}

// MeHandler handles GET /auth/me.
type MeHandler struct {
	me meProvider
}

func NewMeHandler(me *service.MeService) *MeHandler {
	return &MeHandler{me: me}
}

// meResponse is the JSON shape returned to the caller.
// PasswordHash is never included.
type meResponse struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// Me returns the authenticated user's public profile.
// The endpoint requires a valid auth_token cookie (enforced by RequireAuth).
func (h *MeHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		// Should never reach here if RequireAuth is applied, but be defensive.
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	result, err := h.me.Me(r.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			http.Error(w, "user not found", http.StatusNotFound)
		default:
			http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(meResponse{
		UserID:      result.ID,
		Email:       result.Email,
		DisplayName: result.DisplayName,
	})
}
