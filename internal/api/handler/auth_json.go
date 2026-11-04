package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
)

const authBodyLimit = 16 * 1024

// Read the entire bounded body before decoding, so a valid JSON prefix followed
// by oversized padding or another JSON object cannot bypass the limit.
func decodeAuthJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, authBodyLimit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		var networkError net.Error
		switch {
		case errors.As(err, &tooLarge):
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		case errors.As(err, &networkError) && networkError.Timeout():
			http.Error(w, "request body timed out", http.StatusRequestTimeout)
		default:
			http.Error(w, "invalid request body", http.StatusBadRequest)
		}
		return false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}
