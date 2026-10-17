package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHubHTTPClient(t *testing.T) {
	t.Parallel()
	c := NewHubHTTPClient("http://hub", "tok", &http.Client{})
	if c.hubURL != "http://hub" || c.token != "tok" {
		t.Fatal("fields not stored")
	}
}

func TestHubHTTPClientFetchSuccess(t *testing.T) {
	t.Parallel()
	want := map[string]string{"KEY": "VAL", "OTHER": "123"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/snapshot" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("unexpected auth header %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := NewHubHTTPClient(srv.URL, "tok", srv.Client())
	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("expected %s=%s, got %s=%s", k, v, k, got[k])
		}
	}
}

func TestHubHTTPClientFetchNonOKStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewHubHTTPClient(srv.URL, "tok", srv.Client())
	_, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 status")
	}
}

func TestHubHTTPClientFetchMalformedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewHubHTTPClient(srv.URL, "tok", srv.Client())
	_, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestHubHTTPClientFetchContextCancelled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // block until cancelled
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := NewHubHTTPClient(srv.URL, "tok", srv.Client())

	done := make(chan error, 1)
	go func() {
		_, err := c.Fetch(ctx)
		done <- err
	}()

	cancel()

	if err := <-done; err == nil {
		t.Fatal("expected error on context cancel")
	}
}

func TestHubHTTPClientFetchNetworkError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close() // close before request

	c := NewHubHTTPClient(url, "tok", &http.Client{})
	_, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error when server is closed")
	}
}

func TestHubHTTPClientFetchReadAllError(t *testing.T) {
	old := readAll
	defer func() { readAll = old }()
	readAll = func(_ io.Reader) ([]byte, error) {
		return nil, fmt.Errorf("injected read error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"K":"V"}`))
	}))
	defer srv.Close()

	c := NewHubHTTPClient(srv.URL, "tok", srv.Client())
	_, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error from readAll injection")
	}
}

func TestHubHTTPClientFetchBadURL(t *testing.T) {
	t.Parallel()
	// URL with a control character makes http.NewRequestWithContext fail.
	c := NewHubHTTPClient("http://\x00invalid", "tok", &http.Client{})
	_, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
