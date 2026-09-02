package handler

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)


func HealthHandler(w http.ResponseWriter, r *http.Request) {
	// Set Content-Type, Set data, Encode to JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	respone := map[string]string {
		"status":"ok",
	}

	if err := json.NewEncoder(w).Encode(respone); err != nil {
		log.Error().Err(err).Msg("failed to encode health response")
	}
	log.Debug().Msg("Health check passed")
}