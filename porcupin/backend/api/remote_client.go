package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// RemoteClient is an HTTP client for connecting to a remote Porcupin server
type RemoteClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}

// ProxyRequest represents a generic HTTP request to proxy to the remote server
type ProxyRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// ProxyResponse represents the response from a proxied request
type ProxyResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// NewRemoteClient creates a new client for connecting to a remote Porcupin server
func NewRemoteClient(host string, port int, token string, useTLS bool) *RemoteClient {
	protocol := "http"
	if useTLS {
		protocol = "https"
	}
	
	return &RemoteClient{
		baseURL: fmt.Sprintf("%s://%s:%d", protocol, host, port),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Health checks the server health
func (c *RemoteClient) Health(ctx context.Context) (*HealthResponse, error) {
	url := c.baseURL + "/api/v1/health"
	log.Printf("RemoteClient: GET %s", url)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("RemoteClient: Request failed: %v", err)
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()
	
	log.Printf("RemoteClient: Response status: %d", resp.StatusCode)
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		log.Printf("RemoteClient: Error response: %s", string(body))
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}
	
	var health HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	return &health, nil
}

// Proxy sends a generic HTTP request to the remote server and returns the response
func (c *RemoteClient) Proxy(ctx context.Context, proxyReq ProxyRequest) (*ProxyResponse, error) {
	url := c.baseURL + proxyReq.Path
	log.Printf("RemoteClient.Proxy: %s %s", proxyReq.Method, url)
	
	var bodyReader io.Reader
	if proxyReq.Body != "" {
		bodyReader = bytes.NewBufferString(proxyReq.Body)
	}
	
	req, err := http.NewRequestWithContext(ctx, proxyReq.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set default headers
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	
	// Set any custom headers from the request
	for k, v := range proxyReq.Headers {
		req.Header.Set(k, v)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("RemoteClient.Proxy: Request failed: %v", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	log.Printf("RemoteClient.Proxy: Response status: %d, body length: %d", resp.StatusCode, len(body))
	log.Printf("RemoteClient.Proxy: Response body: %s", string(body))
	
	// Collect response headers
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	
	return &ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       string(body),
	}, nil
}
