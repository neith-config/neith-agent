package config

import (
	"context"
	"testing"
	"time"
)

func TestStubClientRunTicker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &StubClient{}
	client.config = client.buildSnapshot()
	ticks := make(chan time.Time, 1)
	updated := make(chan struct{})
	done := make(chan struct{})
	go func() {
		runStubClient(ctx, client, ticks, func() { close(updated) })
		close(done)
	}()

	ticks <- time.Time{}
	<-updated
	cancel()
	<-done

	config, err := client.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config["NEITH_VERSION"] != "1" {
		t.Fatalf("updated version = %q, want 1", config["NEITH_VERSION"])
	}
}
