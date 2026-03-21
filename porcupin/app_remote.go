package main

import (
	"context"
	"log/slog"
	"time"

	"porcupin/backend/api"
)

// RemoteServerConfig holds configuration for connecting to a remote server
type RemoteServerConfig struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Token  string `json:"token"`
	UseTLS bool   `json:"useTLS"`
}

// RemoteHealthResponse holds the health check response from a remote server
type RemoteHealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}

// RemoteProxyRequest holds a generic HTTP request to proxy to a remote server
type RemoteProxyRequest struct {
	Host    string            `json:"host"`
	Port    int               `json:"port"`
	Token   string            `json:"token"`
	UseTLS  bool              `json:"useTLS"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// RemoteProxyResponse holds the response from a proxied request
type RemoteProxyResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// DiscoverServers scans the local network for Porcupin servers via mDNS
func (a *App) DiscoverServers() ([]api.DiscoveredServer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return api.DiscoverServers(ctx, 5*time.Second)
}

// TestRemoteConnection tests connectivity to a remote Porcupin server
func (a *App) TestRemoteConnection(cfg RemoteServerConfig) (*RemoteHealthResponse, error) {
	slog.Info("TestRemoteConnection: connecting", "host", cfg.Host, "port", cfg.Port, "tls", cfg.UseTLS)
	
	client := api.NewRemoteClient(cfg.Host, cfg.Port, cfg.Token, cfg.UseTLS)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	health, err := client.Health(ctx)
	if err != nil {
		slog.Error("TestRemoteConnection failed", "error", err)
		return nil, err
	}
	
	slog.Info("TestRemoteConnection: success", "version", health.Version)
	return &RemoteHealthResponse{
		Status:    health.Status,
		Version:   health.Version,
		Timestamp: health.Timestamp,
	}, nil
}

// RemoteProxy proxies an HTTP request to a remote Porcupin server
// This allows the frontend to make any API call to a remote server via Go
func (a *App) RemoteProxy(req RemoteProxyRequest) (*RemoteProxyResponse, error) {
	slog.Info("RemoteProxy: proxying request", "method", req.Method, "path", req.Path, "host", req.Host, "port", req.Port)
	
	client := api.NewRemoteClient(req.Host, req.Port, req.Token, req.UseTLS)
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	proxyReq := api.ProxyRequest{
		Method:  req.Method,
		Path:    req.Path,
		Headers: req.Headers,
		Body:    req.Body,
	}
	
	resp, err := client.Proxy(ctx, proxyReq)
	if err != nil {
		slog.Error("RemoteProxy failed", "error", err)
		return nil, err
	}
	
	slog.Info("RemoteProxy: response received", "status_code", resp.StatusCode)
	return &RemoteProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       resp.Body,
	}, nil
}
