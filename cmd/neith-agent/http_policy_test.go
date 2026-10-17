package main

import (
	"net/http"
	"testing"
)

func TestHTTPClientPoliciesSeparatePollingAndSSELifetime(t *testing.T) {
	polling := newPollingHTTPClient()
	if polling.Timeout != DefaultConfigFetchTimeout {
		t.Fatalf("polling client timeout = %v, want %v", polling.Timeout, DefaultConfigFetchTimeout)
	}

	streaming := newStreamingHTTPClient()
	if streaming.Timeout != 0 {
		t.Fatalf("streaming client timeout = %v, want no total response timeout", streaming.Timeout)
	}
	transport, ok := streaming.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("streaming client transport = %T, want *http.Transport", streaming.Transport)
	}
	if transport.ResponseHeaderTimeout != DefaultConfigFetchTimeout {
		t.Fatalf("streaming response-header timeout = %v, want %v", transport.ResponseHeaderTimeout, DefaultConfigFetchTimeout)
	}
	if transport.TLSHandshakeTimeout != DefaultConfigFetchTimeout {
		t.Fatalf("streaming TLS-handshake timeout = %v, want %v", transport.TLSHandshakeTimeout, DefaultConfigFetchTimeout)
	}
}
