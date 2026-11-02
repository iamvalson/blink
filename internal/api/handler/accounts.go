package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
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

func (h *AccountsHandler) GetTikTokCreatorInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	accountID := chi.URLParam(r, "accountID")
	if accountID == "" {
		http.Error(w, "Account ID is required", http.StatusBadRequest)
		return
	}

	data, err := h.service.GetTikTokCreatorInfo(r.Context(), userID, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrSocialAccountNotFound) {
			http.Error(w, "Account not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrNotTikTokAccount) {
			http.Error(w, "Account is not a TikTok account", http.StatusBadRequest)
			return
		}
		if errors.Is(err, connectors.ErrInvalidRefreshToken) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":         "Account reconnect required: refresh token is invalid or revoked",
				"code":          "ACCOUNT_RECONNECT_REQUIRED",
				"platform":      "tiktok",
				"reconnect_url": "/auth/tiktok",
			})
			return
		}

		log.Error().Err(err).Str("user_id", userID).Str("account_id", accountID).Msg("Failed to get TikTok creator info")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if data == nil {
		http.Error(w, "TikTok creator info unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to encode TikTok creator info")
	}
}
