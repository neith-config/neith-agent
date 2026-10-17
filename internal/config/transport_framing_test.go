package config

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHubSSEClientPreservesMultilineDataSeparators(t *testing.T) {
	t.Parallel()

	c := NewHubSSEClient("http://hub", "tok", &http.Client{Transport: sseRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})}, noOpLogger{}, noOpSleep, fixedJitter)
	c.newScanner = func(io.Reader) lineScanner {
		return &fakeScanner{lines: []string{
			`data: {"revision":"` + testSnapshotRevision + `","config":{"KEY":"hello`,
			`data: world"}}`,
			"",
		}}
	}

	var received string
	_, err := c.connectEvents(context.Background(), func(data string) bool {
		received = data
		return true
	})
	if err != nil {
		t.Fatalf("connectEvents: %v", err)
	}
	want := `{"revision":"` + testSnapshotRevision + `","config":{"KEY":"hello` + "\n" + `world"}}`
	if received != want {
		t.Fatalf("multiline event = %q, want %q", received, want)
	}
}
