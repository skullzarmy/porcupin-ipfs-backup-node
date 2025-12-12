package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"porcupin/backend/core"
	"porcupin/backend/db"
)

// RouterConfig holds configuration for creating a router with middleware
type RouterConfig struct {
	Token           string
	TokenHash       string
	AllowPublic     bool
	RateLimiter     *RateLimiter
	EnableLogging   bool
}

// NewRouter creates a new chi router with all routes configured.
// If handlers is nil, new handlers will be created with the provided parameters.
func NewRouter(database *db.Database, service *core.BackupService, dataDir, version string) *chi.Mux {
	handlers := NewHandlers(database, service, dataDir, version)
	return NewRouterWithHandlers(handlers)
}

// NewRouterWithHandlers creates a new chi router using the provided handlers.
// This allows the caller to control the handlers instance (e.g., to set IPFS node).
// This version creates a router WITHOUT auth/rate-limit/IP-filter middleware (for testing).
func NewRouterWithHandlers(handlers *Handlers) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware (applied to all routes)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	// Mount routes
	mountRoutes(r, handlers)

	return r
}

// NewRouterWithConfig creates a new chi router with full middleware stack.
// Use this for the actual server.
func NewRouterWithConfig(handlers *Handlers, cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware (applied to all routes) - ORDER MATTERS
	// 1. Recoverer (outermost - catches panics)
	r.Use(middleware.Recoverer)
	
	// 2. RealIP (extracts real IP from headers)
	r.Use(middleware.RealIP)

	// 3. CORS (must be before auth to handle OPTIONS preflight)
	r.Use(CORSMiddleware)

	// 4. Logging (if enabled)
	if cfg.EnableLogging {
		r.Use(func(next http.Handler) http.Handler {
			return LoggingMiddleware(next)
		})
	}

	// 5. Rate Limiting (if configured)
	if cfg.RateLimiter != nil {
		r.Use(RateLimitMiddleware(cfg.RateLimiter))
	}

	// 6. IP Filtering
	r.Use(IPFilterMiddleware(cfg.AllowPublic))

	// 7. Authentication
	r.Use(AuthMiddleware(cfg.Token, cfg.TokenHash))

	// Mount routes AFTER middleware
	mountRoutes(r, handlers)

	return r
}

// mountRoutes adds all API routes to the router
func mountRoutes(r *chi.Mux, handlers *Handlers) {
	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// System endpoints
		r.Get("/health", handlers.GetHealth)   // No auth required (handled in AuthMiddleware)
		r.Get("/version", handlers.GetVersion)
		r.Get("/status", handlers.GetStatus)

		// Statistics
		r.Get("/stats", handlers.GetStats)
		r.Get("/activity", handlers.GetActivity)

		// Wallets CRUD
		r.Get("/wallets", handlers.GetWallets)
		r.Post("/wallets", handlers.AddWallet)
		r.Get("/wallets/{address}", handlers.GetWallet)
		r.Put("/wallets/{address}", handlers.UpdateWallet)
		r.Delete("/wallets/{address}", handlers.DeleteWallet)
		r.Post("/wallets/{address}/sync", handlers.SyncWallet)

		// NFTs
		r.Get("/nfts", handlers.GetNFTs)

		// Assets
		r.Get("/assets", handlers.GetAssets)
		r.Get("/assets/failed", handlers.GetFailedAssets)
		r.Post("/assets/retry-failed", handlers.RetryAllFailed)
		r.Delete("/assets/failed", handlers.ClearFailed)
		r.Post("/assets/{id}/retry", handlers.RetryAsset)
		r.Delete("/assets/{id}", handlers.DeleteAsset)

		// Control
		r.Post("/sync", handlers.TriggerSync)
		r.Post("/pause", handlers.PauseService)
		r.Post("/resume", handlers.ResumeService)
		r.Post("/gc", handlers.RunGC)

		// Discovery
		r.Get("/discover", handlers.DiscoverServers)
	})
}
