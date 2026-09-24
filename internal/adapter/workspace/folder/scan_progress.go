package folder

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type ScanProgress struct {
	Phase string
	Files int
}

type scanProgressKey struct{}

// The workspace timeout bounds lack of IO progress. A healthy large scan is
// bounded by the caller's task deadline, rather than a small fixed scan budget.
type scanWatch struct {
	mu               sync.Mutex
	last, reported   time.Time
	timeout          time.Duration
	timer            *time.Timer
	cancel           context.CancelFunc
	stopped, expired bool
	progress         ScanProgress
	pass             int
	observe          func(ScanProgress)
}

func startScan(parent context.Context, timeout time.Duration, observe func(ScanProgress)) (context.Context, *scanWatch) {
	ctx, cancel := context.WithCancel(parent)
	w := &scanWatch{last: time.Now(), timeout: timeout, cancel: cancel, observe: observe}
	w.mu.Lock()
	w.timer = time.AfterFunc(timeout, w.check)
	w.mu.Unlock()
	return context.WithValue(ctx, scanProgressKey{}, w), w
}

func (w *scanWatch) check() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	if remaining := w.timeout - time.Since(w.last); remaining > 0 {
		w.timer.Reset(remaining)
		return
	}
	w.expired = true
	w.cancel()
}

func (w *scanWatch) close() {
	w.mu.Lock()
	w.stopped = true
	w.timer.Stop()
	w.mu.Unlock()
	w.cancel()
	if w.observe != nil {
		w.observe(ScanProgress{})
	}
}

func (w *scanWatch) nextPass(pass int) {
	w.mu.Lock()
	w.pass = pass
	w.progress = ScanProgress{}
	w.reported = time.Time{}
	w.mu.Unlock()
}

func scanAdvanced(ctx context.Context, phase string, files int) {
	w, ok := ctx.Value(scanProgressKey{}).(*scanWatch)
	if !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	phase = fmt.Sprintf("%s (pass %d/2)", phase, w.pass)
	w.last = time.Now()
	if phase != w.progress.Phase {
		w.progress = ScanProgress{Phase: phase}
	}
	w.progress.Files += files
	if w.observe != nil && (w.reported.IsZero() || time.Since(w.reported) >= time.Second) {
		w.reported = time.Now()
		w.observe(w.progress)
	}
}

func (w *scanWatch) failure(root string, parent context.Context, err error) error {
	if err == nil {
		return nil
	}
	if parent.Err() != nil {
		return fmt.Errorf("workspace inspection %q: %w", root, parent.Err())
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.expired {
		return fmt.Errorf("workspace inspection %q stalled for %s during %s after %d files; increase /set workspace-timeout or check the mounted drive: %w", root, w.timeout, w.progress.Phase, w.progress.Files, context.DeadlineExceeded)
	}
	return fmt.Errorf("workspace inspection %q during %s after %d files: %w", root, w.progress.Phase, w.progress.Files, err)
}
