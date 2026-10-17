package agent

import (
	"testing"
	"time"
)

func TestPollIntervalDurationRejectsOverflowAndJitterUnsafeValues(t *testing.T) {
	t.Parallel()

	maxSafeSeconds := int((time.Duration(1<<63-1) / 5 * 4) / time.Second)
	if got, ok := pollIntervalDuration(maxSafeSeconds); !ok || got <= 0 {
		t.Fatalf("expected maximum safe interval to be accepted, got %v, ok=%v", got, ok)
	}
	if _, ok := pollIntervalDuration(maxSafeSeconds + 1); ok {
		t.Fatal("expected interval above safe duration/jitter bound to be rejected")
	}
}
