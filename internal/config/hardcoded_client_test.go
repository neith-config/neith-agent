package config

import (
	"context"
	"testing"
)

func TestHardcodedClientFetch(t *testing.T) {
	t.Parallel()

	got, err := (HardcodedClient{}).Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got["NETH_TEST"] != "ON" {
		t.Fatalf("unexpected value: %#v", got)
	}
}
