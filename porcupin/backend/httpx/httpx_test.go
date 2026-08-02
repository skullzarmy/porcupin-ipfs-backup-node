package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"porcupin/backend/version"
)

func TestUserAgentFormat(t *testing.T) {
	ua := UserAgent()
	if !strings.HasPrefix(ua, "Porcupin/") {
		t.Errorf("User-Agent should start with %q, got %q", "Porcupin/", ua)
	}
	if !strings.Contains(ua, version.Version) {
		t.Errorf("User-Agent %q should contain version %q", ua, version.Version)
	}
	if !strings.Contains(ua, "+https://github.com/skullzarmy/porcupin-ipfs-backup-node") {
		t.Errorf("User-Agent %q should contain the project URL contact hint", ua)
	}
}

func TestClientSetsUserAgent(t *testing.T) {
	gotCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCh <- r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	resp, err := Client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if got := <-gotCh; got != UserAgent() {
		t.Errorf("expected User-Agent %q, got %q", UserAgent(), got)
	}
}

func TestTransportPreservesExplicitUserAgent(t *testing.T) {
	gotCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCh <- r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "Custom/1.0")

	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if got := <-gotCh; got != "Custom/1.0" {
		t.Errorf("explicit User-Agent should be preserved, got %q", got)
	}
}
