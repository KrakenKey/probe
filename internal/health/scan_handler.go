package health

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/krakenkey/probe/internal/scanner"
)

type scanRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type scanHandler struct {
	secret []byte
}

func newScanHandler(secret string) *scanHandler {
	return &scanHandler{secret: []byte(secret)}
}

func (h *scanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !h.authenticate(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host is required"})
		return
	}
	if req.Port < 1 || req.Port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "port must be 1-65535"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	ep := scanner.EndpointResult{
		Host: req.Host,
		Port: req.Port,
		SNI:  req.Host,
	}

	result := scanner.ScanEndpoint(ctx, ep, 10*time.Second)

	writeJSON(w, http.StatusOK, result)
}

func (h *scanHandler) authenticate(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if len(auth) <= 7 || auth[:7] != "Bearer " {
		return false
	}
	token := []byte(auth[7:])
	return subtle.ConstantTimeCompare(token, h.secret) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
