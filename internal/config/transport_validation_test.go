package config

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHubSSEClientRejectsAccumulatedEventAbovePayloadLimit(t *testing.T) {
	t.Parallel()

	const chunkSize = 1024
	lines := make([]string, 0, expectedMaxSnapshotPayloadBytes/chunkSize+2)
	remaining := expectedMaxSnapshotPayloadBytes + 1
	for remaining > 0 {
		chunk := chunkSize
		if remaining < chunk {
			chunk = remaining
		}
		lines = append(lines, "data: "+strings.Repeat("x", chunk))
		remaining -= chunk
	}
	lines = append(lines, "")

	c := NewHubSSEClient("http://hub", "tok", &http.Client{Transport: sseRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})}, noOpLogger{}, noOpSleep, fixedJitter)
	c.newScanner = func(io.Reader) lineScanner { return &fakeScanner{lines: lines} }

	_, err := c.connectEvents(context.Background(), func(string) bool { return true })
	if err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatalf("expected accumulated payload error, got %v", err)
	}
}
