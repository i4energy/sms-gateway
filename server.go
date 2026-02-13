package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"i4.energy/across/smsgw/modem"
)

// Server handles incoming HTTP requests for interacting with the
// configured modem instance
type Server struct {
	Logger *slog.Logger
	Modem  *modem.Modem
}

// ServeHTTP implements the http.Handler interface for the Server struct
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sms", s.handleSMS)
	mux.ServeHTTP(w, r)
}

func (s *Server) sendError(w http.ResponseWriter, message string, statusCode int) {
	if message == "" {
		w.WriteHeader(statusCode)
		return
	}

	type ErrorResponse struct {
		Message string `json:"message"`
	}
	resp := ErrorResponse{Message: message}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)

}

// handleSMS processes incoming HTTP POST requests to send SMS messages
func (s *Server) handleSMS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "", http.StatusMethodNotAllowed)
		return
	}

	type SMSRequest struct {
		To      string `json:"to"`
		Message string `json:"message"`
	}

	var req SMSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.To == "" || req.Message == "" {
		s.sendError(w, "both 'to' and 'message' fields are required", http.StatusBadRequest)
		return
	}

	if err := s.Modem.SendSMS(r.Context(), req.To, req.Message); err != nil {
		s.Logger.Error("Failed to send SMS", "error", err, "to", req.To)
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.Logger.Info("SMS sent successfully", "to", req.To, "message_length", len(req.Message))
	w.WriteHeader(http.StatusOK)
}
