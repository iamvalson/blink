package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
	"github.com/rs/zerolog/log"
)

type AccountsHandler struct {
	service *service.AccountsService
}

func NewAccountsHandler(svc *service.AccountsService) *AccountsHandler {
	return &AccountsHandler{service: svc}
}

type listAccountsResponse struct {
	Accounts []*model.SocialAccount `json:"accounts"`
}

func (h *AccountsHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	accounts, err := h.service.ListAccounts(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to list accounts")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if accounts == nil {
		accounts = []*model.SocialAccount{} // Avoid null JSON response
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(listAccountsResponse{Accounts: accounts}); err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to encode account list")
	}
}

func (h *AccountsHandler) DisconnectAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	platform := chi.URLParam(r, "platform")
	if platform == "" {
		http.Error(w, "Platform is required", http.StatusBadRequest)
		return
	}

	if err := h.service.DisconnectAccount(r.Context(), userID, platform); err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("platform", platform).Msg("Failed to disconnect account")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

