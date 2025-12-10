package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"porcupin/backend/db"
)

// =============================================================================
// Token Tests (Security Critical)
// =============================================================================

func TestGenerateToken_Length(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if len(token) != TokenLength {
		t.Errorf("GenerateToken() length = %d, want %d", len(token), TokenLength)
	}
}

func TestGenerateToken_Prefix(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if !strings.HasPrefix(token, TokenPrefix) {
		t.Errorf("GenerateToken() = %q, want prefix %q", token, TokenPrefix)
	}
}

func TestGenerateToken_Randomness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken() error = %v", err)
		}
		if tokens[token] {
			t.Errorf("GenerateToken() produced duplicate token: %s", token)
		}
		tokens[token] = true
	}
}

func TestGenerateToken_Charset(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	// Skip the prefix
	randomPart := token[len(TokenPrefix):]

	for _, c := range randomPart {
		if !isAlphanumeric(c) {
			t.Errorf("GenerateToken() contains non-alphanumeric character: %c", c)
		}
	}
}

func TestValidateToken_WrongLength(t *testing.T) {
	token, _ := GenerateToken()

	// Too short
	if ValidateToken("prcpn_short", token) {
		t.Error("ValidateToken() returned true for short token")
	}

	// Too long
	longToken := token + "extra"
	if ValidateToken(longToken, token) {
		t.Error("ValidateToken() returned true for long token")
	}
}

func TestValidateTokenFormat(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "valid token",
			token: "prcpn_012345678901234567890123456789012345678901",
			want:  true,
		},
		{
			name:  "wrong prefix",
			token: "wrong_012345678901234567890123456789012345678901",
			want:  false,
		},
		{
			name:  "too short",
			token: "prcpn_short",
			want:  false,
		},
		{
			name:  "too long",
			token: "prcpn_0123456789012345678901234567890123456789012345",
			want:  false,
		},
		{
			name:  "empty",
			token: "",
			want:  false,
		},
		{
			name:  "special chars",
			token: "prcpn_0123456789!@#$%^&*()_+-=[]{}|;':\",./<>?",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateTokenFormat(tt.token); got != tt.want {
				t.Errorf("ValidateTokenFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenFile_WriteReadHash(t *testing.T) {
	tmpDir := t.TempDir()
	tf := NewTokenFile(tmpDir)

	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	hash, err := HashToken(token)
	if err != nil {
		t.Fatalf("HashToken() error = %v", err)
	}

	if err := tf.WriteHash(hash); err != nil {
		t.Fatalf("TokenFile.WriteHash() error = %v", err)
	}

	if !tf.Exists() {
		t.Error("TokenFile.Exists() = false after WriteHash()")
	}

	readHash, err := tf.ReadHash()
	if err != nil {
		t.Fatalf("TokenFile.ReadHash() error = %v", err)
	}

	if readHash != hash {
		t.Errorf("TokenFile.ReadHash() = %q, want %q", readHash, hash)
	}

	// Verify the hash validates against original token
	if !ValidateTokenAgainstHash(token, readHash) {
		t.Error("ValidateTokenAgainstHash() failed for stored hash")
	}
}

func TestTokenFile_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	tf := NewTokenFile(tmpDir)

	token, _ := GenerateToken()
	hash, _ := HashToken(token)
	tf.WriteHash(hash)

	if err := tf.Delete(); err != nil {
		t.Fatalf("TokenFile.Delete() error = %v", err)
	}

	if tf.Exists() {
		t.Error("TokenFile.Exists() = true after Delete()")
	}
}

func TestGetOrCreateToken(t *testing.T) {
	tmpDir := t.TempDir()

	token1, isNew1, err := GetOrCreateToken(tmpDir)
	if err != nil {
		t.Fatalf("GetOrCreateToken() error = %v", err)
	}
	if !isNew1 {
		t.Error("GetOrCreateToken() isNew = false on first call")
	}
	if !ValidateTokenFormat(token1) {
		t.Error("GetOrCreateToken() returned invalid token format")
	}

	// Second call should return empty token (can't retrieve) and isNew=false
	token2, isNew2, err := GetOrCreateToken(tmpDir)
	if err != nil {
		t.Fatalf("GetOrCreateToken() error = %v", err)
	}
	if isNew2 {
		t.Error("GetOrCreateToken() isNew = true on second call")
	}
	if token2 != "" {
		t.Error("GetOrCreateToken() should return empty token on second call (hash stored, not plain)")
	}

	// Verify the hash file exists and validates against original token
	hash, err := GetTokenHashFromFile(tmpDir)
	if err != nil {
		t.Fatalf("GetTokenHashFromFile() error = %v", err)
	}
	if !ValidateTokenAgainstHash(token1, hash) {
		t.Error("Stored hash doesn't validate against original token")
	}
}

func TestRegenerateToken(t *testing.T) {
	tmpDir := t.TempDir()

	token1, _, _ := GetOrCreateToken(tmpDir)

	token2, err := RegenerateToken(tmpDir)
	if err != nil {
		t.Fatalf("RegenerateToken() error = %v", err)
	}

	if token2 == token1 {
		t.Error("RegenerateToken() returned same token")
	}

	if !ValidateTokenFormat(token2) {
		t.Error("RegenerateToken() returned invalid token format")
	}

	// Verify new token validates against stored hash
	hash, err := GetTokenHashFromFile(tmpDir)
	if err != nil {
		t.Fatalf("GetTokenHashFromFile() error = %v", err)
	}
	if !ValidateTokenAgainstHash(token2, hash) {
		t.Error("RegenerateToken() hash doesn't validate against new token")
	}
	// Old token should NOT validate
	if ValidateTokenAgainstHash(token1, hash) {
		t.Error("Old token still validates after regeneration")
	}
}

// =============================================================================
// Middleware Tests
// =============================================================================

func TestAuthMiddleware_ValidToken(t *testing.T) {
	token, _ := GenerateToken()

	// Test with plain token mode
	handler := AuthMiddleware(token, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("AuthMiddleware() status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	token, _ := GenerateToken()
	wrongToken, _ := GenerateToken()

	handler := AuthMiddleware(token, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+wrongToken)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("AuthMiddleware() status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	token, _ := GenerateToken()

	handler := AuthMiddleware(token, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("AuthMiddleware() status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_HealthBypass(t *testing.T) {
	token, _ := GenerateToken()

	handler := AuthMiddleware(token, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("AuthMiddleware() health bypass status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_HashMode(t *testing.T) {
	token, _ := GenerateToken()
	hash, _ := HashToken(token)

	// Test with hash mode (empty plain token, hash provided)
	handler := AuthMiddleware("", hash)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("AuthMiddleware() hash mode status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Wrong token should fail
	wrongToken, _ := GenerateToken()
	req2 := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req2.Header.Set("Authorization", "Bearer "+wrongToken)
	rr2 := httptest.NewRecorder()

	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("AuthMiddleware() hash mode wrong token status = %d, want %d", rr2.Code, http.StatusUnauthorized)
	}
}

func TestIPFilterMiddleware_PrivateIP(t *testing.T) {
	handler := IPFilterMiddleware(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	privateIPs := []string{
		"127.0.0.1",
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.0.1",
		"192.168.255.255",
	}

	for _, ip := range privateIPs {
		t.Run(ip, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = ip + ":12345"
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("IPFilterMiddleware() rejected private IP %s, status = %d", ip, rr.Code)
			}
		})
	}
}

func TestIPFilterMiddleware_PublicIP(t *testing.T) {
	handler := IPFilterMiddleware(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"203.0.113.1",
		"172.32.0.1",
		"172.15.0.1",
		"192.169.0.1",
	}

	for _, ip := range publicIPs {
		t.Run(ip, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = ip + ":12345"
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("IPFilterMiddleware() allowed public IP %s, status = %d", ip, rr.Code)
			}
		})
	}
}

func TestIPFilterMiddleware_AllowPublicFlag(t *testing.T) {
	handler := IPFilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("IPFilterMiddleware(allowPublic=true) status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRateLimitMiddleware_Under(t *testing.T) {
	limiter := NewRateLimiter(10, 100)
	handler := RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("RateLimitMiddleware() request %d status = %d, want %d", i, rr.Code, http.StatusOK)
		}
	}
}

func TestRateLimitMiddleware_Over(t *testing.T) {
	limiter := NewRateLimiter(5, 100)
	handler := RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("RateLimitMiddleware() over limit status = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}

	if rr.Header().Get("Retry-After") == "" {
		t.Error("RateLimitMiddleware() missing Retry-After header")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.255.255", false},
		{"172.32.0.1", false},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"192.169.0.1", false},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}
			if got := isPrivateIP(ip); got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		want       string
	}{
		{
			name:       "RemoteAddr only",
			remoteAddr: "192.168.1.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For",
			remoteAddr: "127.0.0.1:12345",
			xff:        "192.168.1.100",
			want:       "192.168.1.100",
		},
		{
			name:       "X-Forwarded-For multiple",
			remoteAddr: "127.0.0.1:12345",
			xff:        "192.168.1.100, 10.0.0.1",
			want:       "192.168.1.100",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			xri:        "192.168.1.200",
			want:       "192.168.1.200",
		},
		{
			name:       "X-Forwarded-For takes precedence",
			remoteAddr: "127.0.0.1:12345",
			xff:        "192.168.1.100",
			xri:        "192.168.1.200",
			want:       "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			got := getClientIP(req)
			if got != tt.want {
				t.Errorf("getClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Handler Tests
// =============================================================================

func TestGetHealth(t *testing.T) {
	h := &Handlers{
		version: "1.0.0-test",
	}

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	h.GetHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetHealth() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("GetHealth() failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("GetHealth() status = %q, want %q", resp["status"], "ok")
	}
	if resp["version"] != "1.0.0-test" {
		t.Errorf("GetHealth() version = %q, want %q", resp["version"], "1.0.0-test")
	}
	if resp["timestamp"] == "" {
		t.Error("GetHealth() missing timestamp")
	}
}

func TestGetVersion(t *testing.T) {
	h := &Handlers{
		version: "2.0.0-test",
	}

	req := httptest.NewRequest("GET", "/api/v1/version", nil)
	rr := httptest.NewRecorder()

	h.GetVersion(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetVersion() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("GetVersion() failed to decode response: %v", err)
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("GetVersion() missing data field")
	}
	if data["version"] != "2.0.0-test" {
		t.Errorf("GetVersion() version = %q, want %q", data["version"], "2.0.0-test")
	}
}

func TestGetStatus_NoService(t *testing.T) {
	h := &Handlers{
		service: nil,
	}

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	rr := httptest.NewRecorder()

	h.GetStatus(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("GetStatus() status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestAddWallet_MissingAddress(t *testing.T) {
	h := &Handlers{}

	body := bytes.NewBufferString(`{"alias": "test"}`)
	req := httptest.NewRequest("POST", "/api/v1/wallets", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.AddWallet(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("AddWallet() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestAddWallet_InvalidJSON(t *testing.T) {
	h := &Handlers{}

	body := bytes.NewBufferString(`{invalid json`)
	req := httptest.NewRequest("POST", "/api/v1/wallets", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.AddWallet(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("AddWallet() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetWallet_MissingAddress(t *testing.T) {
	h := &Handlers{}

	req := httptest.NewRequest("GET", "/api/v1/wallets/", nil)
	rr := httptest.NewRecorder()

	h.GetWallet(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetWallet() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestDeleteWallet_MissingAddress(t *testing.T) {
	h := &Handlers{}

	req := httptest.NewRequest("DELETE", "/api/v1/wallets/", nil)
	rr := httptest.NewRecorder()

	h.DeleteWallet(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("DeleteWallet() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRetryAsset_InvalidID(t *testing.T) {
	h := &Handlers{}

	r := chi.NewRouter()
	r.Post("/api/v1/assets/{id}/retry", h.RetryAsset)

	req := httptest.NewRequest("POST", "/api/v1/assets/invalid/retry", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("RetryAsset() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestTriggerSync_NoService(t *testing.T) {
	h := &Handlers{
		service: nil,
	}

	req := httptest.NewRequest("POST", "/api/v1/sync", nil)
	rr := httptest.NewRecorder()

	h.TriggerSync(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("TriggerSync() status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestPauseService_NoService(t *testing.T) {
	h := &Handlers{
		service: nil,
	}

	req := httptest.NewRequest("POST", "/api/v1/pause", nil)
	rr := httptest.NewRecorder()

	h.PauseService(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("PauseService() status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestResumeService_NoService(t *testing.T) {
	h := &Handlers{
		service: nil,
	}

	req := httptest.NewRequest("POST", "/api/v1/resume", nil)
	rr := httptest.NewRecorder()

	h.ResumeService(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("ResumeService() status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestRunGC_NoIPFS(t *testing.T) {
	h := &Handlers{
		ipfs: nil,
	}

	req := httptest.NewRequest("POST", "/api/v1/gc", nil)
	rr := httptest.NewRecorder()

	h.RunGC(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("RunGC() status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestUpdateWallet_MissingAddress(t *testing.T) {
	h := &Handlers{}

	body := bytes.NewBufferString(`{"alias": "updated"}`)
	req := httptest.NewRequest("PUT", "/api/v1/wallets/", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.UpdateWallet(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("UpdateWallet() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// Response helper tests
func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusOK, map[string]string{"key": "value"})

	if rr.Code != http.StatusOK {
		t.Errorf("WriteJSON() status = %d, want %d", rr.Code, http.StatusOK)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("WriteJSON() Content-Type = %q, want %q", contentType, "application/json")
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteError(rr, http.StatusBadRequest, ErrCodeBadRequest, "test error")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("WriteError() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("WriteError() failed to decode response: %v", err)
	}

	if resp.Error.Code != ErrCodeBadRequest {
		t.Errorf("WriteError() code = %q, want %q", resp.Error.Code, ErrCodeBadRequest)
	}
	if resp.Error.Message != "test error" {
		t.Errorf("WriteError() message = %q, want %q", resp.Error.Message, "test error")
	}
}

func TestWriteBadRequest(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteBadRequest(rr, "bad request message")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("WriteBadRequest() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWriteNotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteNotFound(rr, "not found message")

	if rr.Code != http.StatusNotFound {
		t.Errorf("WriteNotFound() status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestWriteConflict(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteConflict(rr, "conflict message")

	if rr.Code != http.StatusConflict {
		t.Errorf("WriteConflict() status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestWriteInternalError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteInternalError(rr, "internal error message")

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("WriteInternalError() status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestWriteServiceUnavailable(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteServiceUnavailable(rr, "service unavailable message")

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("WriteServiceUnavailable() status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestWriteCreated(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteCreated(rr, map[string]string{"id": "123"})

	if rr.Code != http.StatusCreated {
		t.Errorf("WriteCreated() status = %d, want %d", rr.Code, http.StatusCreated)
	}
}

func TestWriteAccepted(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteAccepted(rr, map[string]string{"message": "accepted"})

	if rr.Code != http.StatusAccepted {
		t.Errorf("WriteAccepted() status = %d, want %d", rr.Code, http.StatusAccepted)
	}
}

func TestWriteNoContent(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteNoContent(rr)

	if rr.Code != http.StatusNoContent {
		t.Errorf("WriteNoContent() status = %d, want %d", rr.Code, http.StatusNoContent)
	}

	if rr.Body.Len() != 0 {
		t.Errorf("WriteNoContent() body length = %d, want 0", rr.Body.Len())
	}
}

// =============================================================================
// Test Database Setup Helper
// =============================================================================

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *db.Database {
	t.Helper()
	
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	
	if err := db.InitDB(gormDB); err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}
	
	return db.NewDatabase(gormDB)
}

// =============================================================================
// Handler Integration Tests (with real database)
// =============================================================================

func TestGetWallets_Empty(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	req := httptest.NewRequest("GET", "/api/v1/wallets", nil)
	rr := httptest.NewRecorder()

	h.GetWallets(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetWallets() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp Response
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	wallets, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("Data is not array: %T", resp.Data)
	}
	if len(wallets) != 0 {
		t.Errorf("Expected empty wallet list, got %d wallets", len(wallets))
	}
}

func TestAddWallet_Success(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	body := bytes.NewBufferString(`{"address": "tz1test123", "alias": "Test Wallet"}`)
	req := httptest.NewRequest("POST", "/api/v1/wallets", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.AddWallet(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("AddWallet() status = %d, want %d. Body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify wallet was actually saved
	wallet, err := database.GetWallet("tz1test123")
	if err != nil {
		t.Fatalf("Failed to get wallet from DB: %v", err)
	}
	if wallet == nil {
		t.Fatal("Wallet was not saved to database")
	}
	if wallet.Alias != "Test Wallet" {
		t.Errorf("Wallet alias = %q, want %q", wallet.Alias, "Test Wallet")
	}
}

func TestAddWallet_Duplicate(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	// Add first wallet
	wallet := &db.Wallet{Address: "tz1duplicate", Alias: "First"}
	if err := database.SaveWallet(wallet); err != nil {
		t.Fatalf("Failed to create initial wallet: %v", err)
	}

	// Try to add duplicate
	body := bytes.NewBufferString(`{"address": "tz1duplicate", "alias": "Second"}`)
	req := httptest.NewRequest("POST", "/api/v1/wallets", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.AddWallet(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("AddWallet(duplicate) status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestGetWallet_Found(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	// Create wallet
	wallet := &db.Wallet{Address: "tz1found", Alias: "Found Wallet", SyncOwned: true, SyncCreated: false}
	database.SaveWallet(wallet)

	// Set up chi context for URL params
	r := chi.NewRouter()
	r.Get("/api/v1/wallets/{address}", h.GetWallet)

	req := httptest.NewRequest("GET", "/api/v1/wallets/tz1found", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetWallet() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})

	if data["address"] != "tz1found" {
		t.Errorf("Wallet address = %v, want tz1found", data["address"])
	}
	if data["alias"] != "Found Wallet" {
		t.Errorf("Wallet alias = %v, want 'Found Wallet'", data["alias"])
	}
}

func TestGetWallet_NotFound(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	r := chi.NewRouter()
	r.Get("/api/v1/wallets/{address}", h.GetWallet)

	req := httptest.NewRequest("GET", "/api/v1/wallets/tz1nonexistent", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GetWallet(nonexistent) status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestUpdateWallet_Success(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	// Create wallet
	wallet := &db.Wallet{Address: "tz1update", Alias: "Old Name", SyncOwned: true, SyncCreated: true}
	database.SaveWallet(wallet)

	r := chi.NewRouter()
	r.Put("/api/v1/wallets/{address}", h.UpdateWallet)

	body := bytes.NewBufferString(`{"alias": "New Name", "sync_owned": false}`)
	req := httptest.NewRequest("PUT", "/api/v1/wallets/tz1update", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("UpdateWallet() status = %d, want %d. Body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// Verify changes persisted
	updated, _ := database.GetWallet("tz1update")
	if updated.Alias != "New Name" {
		t.Errorf("Wallet alias = %q, want %q", updated.Alias, "New Name")
	}
	if updated.SyncOwned != false {
		t.Errorf("Wallet SyncOwned = %v, want false", updated.SyncOwned)
	}
	// SyncCreated should be unchanged
	if updated.SyncCreated != true {
		t.Errorf("Wallet SyncCreated = %v, want true (unchanged)", updated.SyncCreated)
	}
}

func TestDeleteWallet_Success(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	// Create wallet
	wallet := &db.Wallet{Address: "tz1delete", Alias: "To Delete"}
	database.SaveWallet(wallet)

	r := chi.NewRouter()
	r.Delete("/api/v1/wallets/{address}", h.DeleteWallet)

	req := httptest.NewRequest("DELETE", "/api/v1/wallets/tz1delete", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("DeleteWallet() status = %d, want %d", rr.Code, http.StatusNoContent)
	}

	// Verify deletion
	deleted, _ := database.GetWallet("tz1delete")
	if deleted != nil {
		t.Error("Wallet was not deleted from database")
	}
}

func TestDeleteWallet_NotFound(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	r := chi.NewRouter()
	r.Delete("/api/v1/wallets/{address}", h.DeleteWallet)

	req := httptest.NewRequest("DELETE", "/api/v1/wallets/tz1nonexistent", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("DeleteWallet(nonexistent) status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGetStats_WithData(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	// Create test data
	wallet := &db.Wallet{Address: "tz1stats"}
	database.SaveWallet(wallet)

	nft := &db.NFT{TokenID: "1", ContractAddress: "KT1stats", WalletAddress: "tz1stats", Name: "Test NFT"}
	database.Create(nft)

	// Create some assets with different statuses
	database.Create(&db.Asset{URI: "ipfs://pending", NFTID: nft.ID, Status: db.StatusPending})
	database.Create(&db.Asset{URI: "ipfs://pinned", NFTID: nft.ID, Status: db.StatusPinned})
	database.Create(&db.Asset{URI: "ipfs://failed", NFTID: nft.ID, Status: db.StatusFailed})

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	rr := httptest.NewRecorder()

	h.GetStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetStats() status = %d, want %d. Body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})

	if int(data["wallets_count"].(float64)) != 1 {
		t.Errorf("wallets_count = %v, want 1", data["wallets_count"])
	}
	if int(data["total_nfts"].(float64)) != 1 {
		t.Errorf("total_nfts = %v, want 1", data["total_nfts"])
	}
	if int(data["total_assets"].(float64)) != 3 {
		t.Errorf("total_assets = %v, want 3", data["total_assets"])
	}
	if int(data["pinned_assets"].(float64)) != 1 {
		t.Errorf("pinned_assets = %v, want 1", data["pinned_assets"])
	}
	if int(data["pending_assets"].(float64)) != 1 {
		t.Errorf("pending_assets = %v, want 1", data["pending_assets"])
	}
	if int(data["failed_assets"].(float64)) != 1 {
		t.Errorf("failed_assets = %v, want 1", data["failed_assets"])
	}
}

func TestGetAssets_Paginated(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	// Create test NFT and assets
	nft := &db.NFT{TokenID: "1", ContractAddress: "KT1assets", WalletAddress: "tz1test"}
	database.Create(nft)

	for i := 0; i < 10; i++ {
		database.Create(&db.Asset{
			URI:    fmt.Sprintf("ipfs://asset%d", i),
			NFTID:  nft.ID,
			Status: db.StatusPinned,
		})
	}

	// Request page 1 with limit 3
	req := httptest.NewRequest("GET", "/api/v1/assets?page=1&limit=3", nil)
	rr := httptest.NewRecorder()

	h.GetAssets(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetAssets() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})

	if int(data["total"].(float64)) != 10 {
		t.Errorf("total = %v, want 10", data["total"])
	}
	if int(data["page"].(float64)) != 1 {
		t.Errorf("page = %v, want 1", data["page"])
	}
	if int(data["limit"].(float64)) != 3 {
		t.Errorf("limit = %v, want 3", data["limit"])
	}
	assets := data["assets"].([]interface{})
	if len(assets) != 3 {
		t.Errorf("returned %d assets, want 3", len(assets))
	}
}

func TestGetAssets_FilterByStatus(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	nft := &db.NFT{TokenID: "1", ContractAddress: "KT1filter", WalletAddress: "tz1test"}
	database.Create(nft)

	database.Create(&db.Asset{URI: "ipfs://pending1", NFTID: nft.ID, Status: db.StatusPending})
	database.Create(&db.Asset{URI: "ipfs://pending2", NFTID: nft.ID, Status: db.StatusPending})
	database.Create(&db.Asset{URI: "ipfs://pinned1", NFTID: nft.ID, Status: db.StatusPinned})

	req := httptest.NewRequest("GET", "/api/v1/assets?status=pending", nil)
	rr := httptest.NewRecorder()

	h.GetAssets(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetAssets(status=pending) status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})

	if int(data["total"].(float64)) != 2 {
		t.Errorf("total pending = %v, want 2", data["total"])
	}
}

func TestGetFailedAssets(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	nft := &db.NFT{TokenID: "1", ContractAddress: "KT1failed", WalletAddress: "tz1test"}
	database.Create(nft)

	database.Create(&db.Asset{URI: "ipfs://failed1", NFTID: nft.ID, Status: db.StatusFailed, ErrorMsg: "timeout"})
	database.Create(&db.Asset{URI: "ipfs://failed2", NFTID: nft.ID, Status: db.StatusFailedUnavailable, ErrorMsg: "not found"})
	database.Create(&db.Asset{URI: "ipfs://pinned1", NFTID: nft.ID, Status: db.StatusPinned})

	req := httptest.NewRequest("GET", "/api/v1/assets/failed", nil)
	rr := httptest.NewRecorder()

	h.GetFailedAssets(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetFailedAssets() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp Response
	json.NewDecoder(rr.Body).Decode(&resp)
	assets := resp.Data.([]interface{})

	if len(assets) != 2 {
		t.Errorf("got %d failed assets, want 2", len(assets))
	}
}

func TestRetryAsset_Success(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	nft := &db.NFT{TokenID: "1", ContractAddress: "KT1retry", WalletAddress: "tz1test"}
	database.Create(nft)

	asset := &db.Asset{URI: "ipfs://retry", NFTID: nft.ID, Status: db.StatusFailed, ErrorMsg: "previous error", RetryCount: 3}
	database.Create(asset)

	r := chi.NewRouter()
	r.Post("/api/v1/assets/{id}/retry", h.RetryAsset)

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/assets/%d/retry", asset.ID), nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("RetryAsset() status = %d, want %d", rr.Code, http.StatusAccepted)
	}

	// Verify asset was reset
	var updated db.Asset
	database.First(&updated, asset.ID)

	if updated.Status != db.StatusPending {
		t.Errorf("Asset status = %q, want %q", updated.Status, db.StatusPending)
	}
	if updated.ErrorMsg != "" {
		t.Errorf("Asset ErrorMsg = %q, want empty", updated.ErrorMsg)
	}
	if updated.RetryCount != 0 {
		t.Errorf("Asset RetryCount = %d, want 0", updated.RetryCount)
	}
}

func TestRetryAsset_NotFound(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	r := chi.NewRouter()
	r.Post("/api/v1/assets/{id}/retry", h.RetryAsset)

	req := httptest.NewRequest("POST", "/api/v1/assets/99999/retry", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("RetryAsset(nonexistent) status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGetActivity(t *testing.T) {
	database := setupTestDB(t)
	h := NewHandlers(database, nil, t.TempDir(), "test")

	nft := &db.NFT{TokenID: "1", ContractAddress: "KT1activity", WalletAddress: "tz1test", Name: "Activity NFT"}
	database.Create(nft)

	now := time.Now()
	database.Create(&db.Asset{URI: "ipfs://recent1", NFTID: nft.ID, Status: db.StatusPinned, PinnedAt: &now})
	database.Create(&db.Asset{URI: "ipfs://pending", NFTID: nft.ID, Status: db.StatusPending})

	req := httptest.NewRequest("GET", "/api/v1/activity?limit=10", nil)
	rr := httptest.NewRecorder()

	h.GetActivity(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetActivity() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp Response
	json.NewDecoder(rr.Body).Decode(&resp)
	activities := resp.Data.([]interface{})

	// Should only return pinned assets
	if len(activities) != 1 {
		t.Errorf("got %d activities, want 1 (only pinned)", len(activities))
	}
}

// =============================================================================
// Integration Tests (Full Server with Database)
// =============================================================================

func TestServerStartStop(t *testing.T) {
	token, _ := GenerateToken()
	database := setupTestDB(t)

	cfg := ServerConfig{
		Port:            18085,
		BindAddress:     "127.0.0.1",
		Token:           token,
		AllowPublic:     false,
		DataDir:         t.TempDir(),
		Version:         "test",
		PerIPRateLimit:  10,
		GlobalRateLimit: 100,
	}

	server := NewServer(cfg, database, nil)
	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Server.Start() error = %v", err)
		}
	default:
	}

	cancel()
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server shutdown error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not shut down within timeout")
	}
}

func TestFullAuthFlow(t *testing.T) {
	token, _ := GenerateToken()
	database := setupTestDB(t)

	cfg := ServerConfig{
		Port:            18086,
		BindAddress:     "127.0.0.1",
		Token:           token,
		AllowPublic:     false,
		DataDir:         t.TempDir(),
		Version:         "test",
		PerIPRateLimit:  10,
		GlobalRateLimit: 100,
	}

	server := NewServer(cfg, database, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	addr := server.GetListenAddress()
	if addr == "" {
		addr = "127.0.0.1:18086"
	}

	client := &http.Client{Timeout: 5 * time.Second}

	// Health without auth should work
	healthURL := "http://" + addr + "/api/v1/health"
	resp, err := client.Get(healthURL)
	if err != nil {
		t.Fatalf("Health request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Stats without auth should fail
	statsURL := "http://" + addr + "/api/v1/stats"
	resp, err = client.Get(statsURL)
	if err != nil {
		t.Fatalf("Stats request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Stats without auth status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Stats with valid auth should return 200
	req, _ := http.NewRequest("GET", statsURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Stats with auth request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Stats with valid auth status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Stats with invalid token should fail
	req, _ = http.NewRequest("GET", statsURL, nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Stats with bad auth request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Stats with invalid auth status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestFullWalletCRUD(t *testing.T) {
	token, _ := GenerateToken()
	database := setupTestDB(t)

	cfg := ServerConfig{
		Port:            18090,
		BindAddress:     "127.0.0.1",
		Token:           token,
		AllowPublic:     false,
		DataDir:         t.TempDir(),
		Version:         "test",
		PerIPRateLimit:  100,
		GlobalRateLimit: 1000,
	}

	server := NewServer(cfg, database, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	addr := "127.0.0.1:18090"
	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := "http://" + addr

	// CREATE wallet
	createBody := bytes.NewBufferString(`{"address": "tz1crud", "alias": "CRUD Test"}`)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/wallets", createBody)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Create wallet failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Create wallet status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	// READ wallet
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/wallets/tz1crud", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Get wallet failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Get wallet status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// UPDATE wallet
	updateBody := bytes.NewBufferString(`{"alias": "Updated Name"}`)
	req, _ = http.NewRequest("PUT", baseURL+"/api/v1/wallets/tz1crud", updateBody)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Update wallet failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Update wallet status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify update
	wallet, _ := database.GetWallet("tz1crud")
	if wallet.Alias != "Updated Name" {
		t.Errorf("Wallet alias after update = %q, want %q", wallet.Alias, "Updated Name")
	}

	// DELETE wallet
	req, _ = http.NewRequest("DELETE", baseURL+"/api/v1/wallets/tz1crud", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Delete wallet failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Delete wallet status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// Verify deletion
	wallet, _ = database.GetWallet("tz1crud")
	if wallet != nil {
		t.Error("Wallet still exists after delete")
	}
}

func TestConcurrentRequests(t *testing.T) {
	token, _ := GenerateToken()
	database := setupTestDB(t)

	cfg := ServerConfig{
		Port:            18087,
		BindAddress:     "127.0.0.1",
		Token:           token,
		AllowPublic:     false,
		DataDir:         t.TempDir(),
		Version:         "test",
		PerIPRateLimit:  100,
		GlobalRateLimit: 1000,
	}

	server := NewServer(cfg, database, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	addr := server.GetListenAddress()
	if addr == "" {
		addr = "127.0.0.1:18087"
	}

	numRequests := 20
	var wg sync.WaitGroup
	results := make(chan int, numRequests)
	client := &http.Client{Timeout: 5 * time.Second}

	healthURL := "http://" + addr + "/api/v1/health"

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(healthURL)
			if err != nil {
				results <- 0
				return
			}
			resp.Body.Close()
			results <- resp.StatusCode
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	for status := range results {
		if status == http.StatusOK {
			successCount++
		}
	}

	if successCount != numRequests {
		t.Errorf("Concurrent requests: %d/%d succeeded", successCount, numRequests)
	}
}

func TestGracefulShutdown(t *testing.T) {
	token, _ := GenerateToken()
	database := setupTestDB(t)

	cfg := ServerConfig{
		Port:            18088,
		BindAddress:     "127.0.0.1",
		Token:           token,
		AllowPublic:     false,
		DataDir:         t.TempDir(),
		Version:         "test",
		PerIPRateLimit:  10,
		GlobalRateLimit: 100,
	}

	server := NewServer(cfg, database, nil)

	ctx, cancel := context.WithCancel(context.Background())

	startedCh := make(chan struct{})
	doneCh := make(chan error, 1)
	go func() {
		close(startedCh)
		doneCh <- server.Start(ctx)
	}()

	<-startedCh
	time.Sleep(200 * time.Millisecond)

	shutdownStart := time.Now()
	cancel()

	select {
	case err := <-doneCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Graceful shutdown error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not shut down within timeout")
	}

	shutdownDuration := time.Since(shutdownStart)

	if shutdownDuration > 2*time.Second {
		t.Errorf("Shutdown took %v, expected < 2s", shutdownDuration)
	}
}

func TestServerConfigDefaults(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Port != 8085 {
		t.Errorf("Default port = %d, want %d", cfg.Port, 8085)
	}

	if cfg.BindAddress != "0.0.0.0" {
		t.Errorf("Default BindAddress = %q, want %q", cfg.BindAddress, "0.0.0.0")
	}

	if cfg.AllowPublic != false {
		t.Errorf("Default AllowPublic = %v, want %v", cfg.AllowPublic, false)
	}

	if cfg.PerIPRateLimit != 10 {
		t.Errorf("Default PerIPRateLimit = %d, want %d", cfg.PerIPRateLimit, 10)
	}

	if cfg.GlobalRateLimit != 100 {
		t.Errorf("Default GlobalRateLimit = %d, want %d", cfg.GlobalRateLimit, 100)
	}
}

func TestServerGetListenAddress(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Port = 18089
	cfg.BindAddress = "127.0.0.1"
	cfg.Token = "test-token"
	cfg.DataDir = t.TempDir()

	database := setupTestDB(t)
	server := NewServer(cfg, database, nil)

	if addr := server.GetListenAddress(); addr != "" {
		t.Errorf("GetListenAddress() before start = %q, want empty", addr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	addr := server.GetListenAddress()
	if addr == "" {
		t.Error("GetListenAddress() after start returned empty string")
	}
}