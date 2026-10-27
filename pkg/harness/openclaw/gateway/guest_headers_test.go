package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSocketProviderHeadersAreCopiedAndNotExposedOnError(t *testing.T) {
	seen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("E2b-Traffic-Access-Token")
		http.Error(w, "private-provider-secret", http.StatusForbidden)
	}))
	defer server.Close()
	headers := http.Header{"E2b-Traffic-Access-Token": []string{"private-provider-secret"}}
	dial, err := NewWebSocketDialer(WebSocketOptions{URL: strings.Replace(server.URL, "http://", "ws://", 1), Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	headers.Set("E2b-Traffic-Access-Token", "changed-after-construction")
	_, err = dial(context.Background())
	if err == nil || strings.Contains(err.Error(), "private-provider-secret") {
		t.Fatalf("handshake error = %v", err)
	}
	if got := <-seen; got != "private-provider-secret" {
		t.Fatalf("provider credential was not snapshotted: %q", got)
	}
}
