package folder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestScanProgressExtendsIdleBudgetAndStallsRemainBounded(t *testing.T) {
	ctx, watch := startScan(t.Context(), time.Hour, nil)
	defer watch.close()
	watch.nextPass(1)
	watch.mu.Lock()
	watch.last = time.Now().Add(-2 * time.Hour)
	watch.mu.Unlock()
	scanAdvanced(ctx, "listing files", 25000)
	watch.check()
	if ctx.Err() != nil {
		t.Fatal("healthy scan was cancelled")
	}
	watch.mu.Lock()
	watch.last = time.Now().Add(-2 * time.Hour)
	watch.mu.Unlock()
	watch.check()
	if ctx.Err() == nil {
		t.Fatal("stalled scan did not cancel")
	}
	err := watch.failure("/workspace", t.Context(), ctx.Err())
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "25000 files") || !strings.Contains(err.Error(), "workspace-timeout") {
		t.Fatal(err)
	}
}

func TestScanProgressCannotExtendParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()
	<-parent.Done()
	ctx, watch := startScan(parent, time.Hour, nil)
	defer watch.close()
	scanAdvanced(ctx, "reading files", 1)
	if ctx.Err() == nil || !errors.Is(watch.failure("/workspace", parent, ctx.Err()), context.DeadlineExceeded) {
		t.Fatal("lost task deadline")
	}
}

func TestScanProgressObserverIsSerializedAndCleared(t *testing.T) {
	var events []ScanProgress
	ctx, watch := startScan(t.Context(), time.Hour, func(p ScanProgress) { events = append(events, p) })
	watch.nextPass(1)
	scanAdvanced(ctx, "listing files", 1)
	watch.nextPass(2)
	scanAdvanced(ctx, "reading files", 1)
	watch.close()
	if len(events) != 3 || events[0].Phase != "listing files (pass 1/2)" || events[1].Phase != "reading files (pass 2/2)" || events[2].Phase != "" {
		t.Fatal(events)
	}
}
