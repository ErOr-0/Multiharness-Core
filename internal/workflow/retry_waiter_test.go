package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTimerWaiterRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (timerWaiter{}).Wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if err := (timerWaiter{}).Wait(t.Context(), time.Nanosecond); err != nil {
		t.Fatal(err)
	}
}
