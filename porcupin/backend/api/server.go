package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"porcupin/backend/core"
	"porcupin/backend/db"
	"porcupin/backend/ipfs"
)

// ServerConfig holds the configuration for the API server
type ServerConfig struct {
	// Port is the port to listen on
	Port int

	// BindAddress is the address to bind to (e.g., "0.0.0.0" or "127.0.0.1")
	BindAddress string

	// Token is the API authentication token (plain text, from env var or flag)
	// If set, this takes precedence over TokenHash
	Token string

	// TokenHash is the bcrypt hash of the token (from file)
	// Used when Token is empty
	TokenHash string

	// AllowPublic allows connections from public IP addresses
	AllowPublic bool

	// DataDir is the path to the data directory
	DataDir string

	// Version is the application version
	Version string

	// PerIPRateLimit is the rate limit per IP per second
	PerIPRateLimit int

	// GlobalRateLimit is the global rate limit per second
	GlobalRateLimit int

	// TLSCert is the path to the TLS certificate file
	TLSCert string

	// TLSKey is the path to the TLS private key file
	TLSKey string
}

// DefaultServerConfig returns a ServerConfig with secure defaults
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port:            8085,
		BindAddress:     "0.0.0.0",
		Token:           "",
		AllowPublic:     false,
		DataDir:         "",
		Version:         "unknown",
		PerIPRateLimit:  10,  // 10 requests per second per IP
		GlobalRateLimit: 100, // 100 requests per second global
		TLSCert:         "",
		TLSKey:          "",
	}
}

// Server represents the API server
type Server struct {
	config      ServerConfig
	httpServer  *http.Server
	database    *db.Database
	service     *core.BackupService
	ipfs        *ipfs.Node
	rateLimiter *RateLimiter
	handlers    *Handlers
	mdns        *MDNSServer
	listenAddr  string
	mu          sync.RWMutex
}

// NewServer creates a new API server
func NewServer(config ServerConfig, database *db.Database, service *core.BackupService) *Server {
	return &Server{
		config:      config,
		database:    database,
		service:     service,
		rateLimiter: NewRateLimiter(config.PerIPRateLimit, config.GlobalRateLimit),
	}
}

// SetIPFS sets the IPFS node for the server
func (s *Server) SetIPFS(node *ipfs.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipfs = node
	if s.handlers != nil {
		s.handlers.SetIPFS(node)
	}
}

// Start starts the API server (blocking)
func (s *Server) Start(ctx context.Context) error {
	// Print startup warnings
	s.printStartupWarnings()

	// Create handlers and set IPFS node if available
	s.handlers = NewHandlers(s.database, s.service, s.config.DataDir, s.config.Version)
	if s.ipfs != nil {
		s.handlers.SetIPFS(s.ipfs)
	}

	// Create chi router with handlers and full middleware stack
	routerCfg := RouterConfig{
		Token:         s.config.Token,
		TokenHash:     s.config.TokenHash,
		AllowPublic:   s.config.AllowPublic,
		RateLimiter:   s.rateLimiter,
		EnableLogging: true,
	}
	router := NewRouterWithConfig(s.handlers, routerCfg)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.Port)
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Configure TLS if certificates provided
	if s.config.TLSCert != "" && s.config.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(s.config.TLSCert, s.config.TLSKey)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificates: %w", err)
		}
		s.httpServer.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	// Start listening
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Store listen address for GetListenAddress (thread-safe)
	s.mu.Lock()
	s.listenAddr = listener.Addr().String()
	s.mu.Unlock()

	protocol := "http"
	if s.httpServer.TLSConfig != nil {
		protocol = "https"
		listener = tls.NewListener(listener, s.httpServer.TLSConfig)
	}

	slog.Info("API server listening", "protocol", protocol, "address", addr)

	// Start mDNS announcement
	useTLS := s.httpServer.TLSConfig != nil
	s.mdns = NewMDNSServer(s.config.Port, s.config.Version, useTLS)
	if err := s.mdns.Start(); err != nil {
		slog.Warn("mDNS announcement failed", "error", err)
		// Non-fatal - server still works without mDNS
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		// Stop mDNS in parallel (non-blocking)
		if s.mdns != nil {
			go s.mdns.Stop()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("API server shutdown error", "error", err)
		}
	}()

	// Serve (blocks until shutdown)
	if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Stop gracefully stops the API server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// printStartupWarnings prints security warnings at startup
func (s *Server) printStartupWarnings() {
	slog.Info("PORCUPIN API SERVER starting")

	// Warning 1: No TLS
	if s.config.TLSCert == "" || s.config.TLSKey == "" {
		slog.Warn("No TLS configured - traffic is unencrypted. For internet exposure, use a reverse proxy with TLS")
	}

	// Warning 2: Public access
	if s.config.AllowPublic {
		slog.Warn("--allow-public is enabled: public IP addresses can connect to this server")
	}

	// Security reminders
	slog.Info("Security reminders: token stored at ~/.porcupin/.api-token (mode 0600), use PORCUPIN_API_TOKEN env var for automation, designed for LAN use only")

	protocol := "http"
	if s.config.TLSCert != "" && s.config.TLSKey != "" {
		protocol = "https"
	}
	slog.Info("API server bind config",
		"protocol", protocol,
		"bind_address", s.config.BindAddress,
		"port", s.config.Port,
		"allow_public", s.config.AllowPublic)
}

// GetListenAddress returns the address the server is listening on
func (s *Server) GetListenAddress() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listenAddr
}
