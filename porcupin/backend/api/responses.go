package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// Response wraps successful API responses
type Response struct {
	Data interface{} `json:"data"`
	Meta *Meta       `json:"meta,omitempty"`
}

// Meta contains response metadata
type Meta struct {
	Timestamp string `json:"timestamp"`
}

// ErrorResponse wraps error API responses
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes
const (
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeBadRequest     = "BAD_REQUEST"
	ErrCodeConflict       = "CONFLICT"
	ErrCodeRateLimited    = "RATE_LIMITED"
	ErrCodeInternalError  = "INTERNAL_ERROR"
	ErrCodeServiceUnavail = "SERVICE_UNAVAILABLE"
)

// WriteJSON writes a successful JSON response with metadata
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Data: data,
		Meta: &Meta{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Can't do much at this point, response already started
		return
	}
}

// WriteJSONRaw writes a JSON response without the wrapper (for health endpoint etc)
func WriteJSONRaw(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes an error JSON response
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// Helper functions for common errors

// WriteUnauthorized writes a 401 Unauthorized response
func WriteUnauthorized(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Invalid or missing API token"
	}
	WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// WriteForbidden writes a 403 Forbidden response
func WriteForbidden(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Access denied"
	}
	WriteError(w, http.StatusForbidden, ErrCodeForbidden, message)
}

// WriteNotFound writes a 404 Not Found response
func WriteNotFound(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Resource not found"
	}
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, message)
}

// WriteBadRequest writes a 400 Bad Request response
func WriteBadRequest(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Invalid request"
	}
	WriteError(w, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// WriteConflict writes a 409 Conflict response
func WriteConflict(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Resource already exists"
	}
	WriteError(w, http.StatusConflict, ErrCodeConflict, message)
}

// WriteRateLimited writes a 429 Too Many Requests response
func WriteRateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "1")
	WriteError(w, http.StatusTooManyRequests, ErrCodeRateLimited, "Rate limit exceeded")
}

// WriteInternalError writes a 500 Internal Server Error response
func WriteInternalError(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Internal server error"
	}
	WriteError(w, http.StatusInternalServerError, ErrCodeInternalError, message)
}

// WriteServiceUnavailable writes a 503 Service Unavailable response
func WriteServiceUnavailable(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Service unavailable"
	}
	WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavail, message)
}

// WriteAccepted writes a 202 Accepted response for async operations
func WriteAccepted(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusAccepted, data)
}

// WriteCreated writes a 201 Created response
func WriteCreated(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusCreated, data)
}

// WriteNoContent writes a 204 No Content response
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
