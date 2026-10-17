package cli

import (
	"strconv"
	"testing"
	"time"
)

func TestParsePollIntervalRejectsDurationAndJitterOverflow(t *testing.T) {
	t.Parallel()

	maxSafeSeconds := int64((time.Duration(1<<63-1) / 5 * 4) / time.Second)
	if _, err := parsePollInterval(strconv.FormatInt(maxSafeSeconds, 10)); err != nil {
		t.Fatalf("expected maximum safe interval to be accepted: %v", err)
	}
	if _, err := parsePollInterval(strconv.FormatInt(maxSafeSeconds+1, 10)); err == nil {
		t.Fatalf("expected interval above safe duration/jitter bound to be rejected")
	}
}
