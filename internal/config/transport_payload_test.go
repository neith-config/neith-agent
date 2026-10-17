package config

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const (
	testSnapshotRevision            = "018f0000-0000-7000-8000-000000000001"
	expectedMaxSnapshotPayloadBytes = 1 << 20
)

func TestHubHTTPClientRejectsSnapshotAbovePayloadLimit(t *testing.T) {
	t.Parallel()

	body := snapshotEnvelopeJSON(strings.Repeat("x", expectedMaxSnapshotPayloadBytes))
	client := &http.Client{Transport: sseRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	_, err := NewHubHTTPClient("http://hub", "tok", client).FetchSnapshot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatalf("expected bounded payload error, got %v", err)
	}
}

func TestHubSSEClientAcceptsEventAboveScannerDefaultLimit(t *testing.T) {
	t.Parallel()

	body := "data: " + snapshotEnvelopeJSON(strings.Repeat("x", 70*1024)) + "\n\n"
	client := &http.Client{Transport: sseRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
	c := NewHubSSEClient("http://hub", "tok", client, noOpLogger{}, noOpSleep, fixedJitter)

	var received Snapshot
	_, err := c.connectEvents(context.Background(), func(data string) bool {
		snapshot, parseErr := parseSnapshotEnvelope([]byte(data))
		if parseErr != nil {
			t.Fatalf("parse received event: %v", parseErr)
		}
		received = snapshot
		return true
	})
	if err != nil {
		t.Fatalf("connectEvents: %v", err)
	}
	if len(received.Config["KEY"]) != 70*1024 {
		t.Fatalf("received payload length = %d, want %d", len(received.Config["KEY"]), 70*1024)
	}
}

func snapshotEnvelopeJSON(value string) string {
	return `{"revision":"` + testSnapshotRevision + `","config":{"KEY":"` + value + `"}}`
}
