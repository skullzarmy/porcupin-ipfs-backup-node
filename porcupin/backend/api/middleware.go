package api

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Middleware keys for context
type contextKey string

const (
	// ContextKeyClientIP is the context key for the client IP address
	ContextKeyClientIP contextKey = "clientIP"
)

// CORSMiddleware adds CORS headers to allow cross-origin requests.
// Required for browser-based clients and future web UIs.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// AuthMiddleware creates middleware that validates API tokens.
// Health endpoint is exempt from authentication.
// If plainToken is provided, uses constant-time comparison.
// If tokenHash is provided (and plainToken is empty), uses bcrypt comparison.
func AuthMiddleware(plainToken, tokenHash string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health endpoint is always accessible (for load balancers, monitoring)
			if r.URL.Path == "/api/v1/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				WriteUnauthorized(w, "Missing authorization header")
				return
			}

			// Expect "Bearer <token>" format
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				WriteUnauthorized(w, "Invalid authorization format, expected 'Bearer <token>'")
				return
			}

			providedToken := parts[1]

			// Validate token
			var valid bool
			if plainToken != "" {
				// Plain token from env var - use constant-time comparison
				valid = ValidateToken(providedToken, plainToken)
			} else if tokenHash != "" {
				// Token hash from file - use bcrypt comparison
				valid = ValidateTokenAgainstHash(providedToken, tokenHash)
			} else {
				// No token configured - deny all
				valid = false
			}

			if !valid {
				// Log failed auth attempts (without the token)
				clientIP := getClientIP(r)
				log.Printf("AUTH FAILED: unauthorized request from %s to %s", clientIP, r.URL.Path)
				WriteUnauthorized(w, "Invalid token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// IPFilterMiddleware creates middleware that restricts access to private IP addresses.
// If allowPublic is true, all IPs are allowed.
func IPFilterMiddleware(allowPublic bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Store client IP in context for logging
			clientIP := getClientIP(r)
			ctx := context.WithValue(r.Context(), ContextKeyClientIP, clientIP)
			r = r.WithContext(ctx)

			// If public access is allowed, skip IP filtering
			if allowPublic {
				next.ServeHTTP(w, r)
				return
			}

			// Parse the client IP
			ip := net.ParseIP(clientIP)
			if ip == nil {
				log.Printf("IP FILTER: could not parse IP: %s", clientIP)
				WriteForbidden(w, "Could not determine client IP")
				return
			}

			// Check if IP is private/local
			if !isPrivateIP(ip) {
				log.Printf("IP FILTER: rejected public IP: %s", clientIP)
				WriteForbidden(w, "Access denied: public IPs not allowed. Use --allow-public to enable.")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isPrivateIP checks if an IP address is in a private/local range
func isPrivateIP(ip net.IP) bool {
	// Loopback
	if ip.IsLoopback() {
		return true
	}

	// Link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// IPv4 private ranges
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		return false
	}

	// IPv6 private ranges
	// fc00::/7 (Unique Local Addresses)
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return true
	}

	return false
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	mu          sync.Mutex
	perIP       map[string]*rateBucket
	global      *rateBucket
	perIPRate   int           // requests per second per IP
	globalRate  int           // requests per second global
	cleanupTick time.Duration // how often to clean up old entries
}

// rateBucket is a simple token bucket
type rateBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(perIPRate, globalRate int) *RateLimiter {
	rl := &RateLimiter{
		perIP:       make(map[string]*rateBucket),
		global:      newBucket(globalRate),
		perIPRate:   perIPRate,
		globalRate:  globalRate,
		cleanupTick: 5 * time.Minute,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

func newBucket(rate int) *rateBucket {
	return &rateBucket{
		tokens:     float64(rate),
		maxTokens:  float64(rate),
		refillRate: float64(rate),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request from the given IP should be allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Check global limit first
	rl.global.refill(now)
	if rl.global.tokens < 1 {
		return false
	}

	// Check per-IP limit
	bucket, exists := rl.perIP[ip]
	if !exists {
		bucket = newBucket(rl.perIPRate)
		rl.perIP[ip] = bucket
	}
	bucket.refill(now)
	if bucket.tokens < 1 {
		return false
	}

	// Consume tokens
	rl.global.tokens--
	bucket.tokens--
	return true
}

func (b *rateBucket) refill(now time.Time) {
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupTick)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, bucket := range rl.perIP {
			// Remove buckets that haven't been used in 10 minutes
			if now.Sub(bucket.lastRefill) > 10*time.Minute {
				delete(rl.perIP, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware creates middleware that rate limits requests
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := getClientIP(r)

			if !limiter.Allow(clientIP) {
				log.Printf("RATE LIMIT: exceeded for %s", clientIP)
				WriteRateLimited(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware logs all requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		clientIP := getClientIP(r)

		log.Printf("API: %s %s %s %d %v",
			clientIP,
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			duration,
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// JSONContentTypeMiddleware sets Content-Type to application/json
func JSONContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}
