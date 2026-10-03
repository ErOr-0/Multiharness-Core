// Package native implements live, bidirectional native harness protocols.
package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type Runner interface {
	Run(context.Context, process.Command) (process.Result, error)
}
type object map[string]json.RawMessage

func obj(raw json.RawMessage) object { var v object; _ = json.Unmarshal(raw, &v); return v }
func str(raw json.RawMessage) string { var v string; _ = json.Unmarshal(raw, &v); return v }
func raw(v any) json.RawMessage      { b, _ := json.Marshal(v); return b }

type dict = map[string]any

const (
	frameLimit    = 4 << 20
	bufferedLimit = 64 << 20
	// sniffBytes of an oversized frame decide whether it can be skipped.
	sniffBytes = 4 << 10
)

// The stdout sink never waits on terminal input. A full queue applies
// backpressure to the harness (bounded by the connection context) instead of
// failing, so bursts such as transcript replays do not abort a run. Oversized
// progress notifications (a tool echoing a large file) are skipped; oversized
// frames that need an answer or carry a result still fail closed.
type frames struct {
	mu       sync.Mutex
	buf      []byte
	ch       chan object
	err      error
	bytes    int
	skipping bool
	skipped  int
	fail     context.CancelFunc
	done     <-chan struct{}
}

func (f *frames) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		m, consumed, err := f.frame(p)
		if err != nil {
			if f.fail != nil {
				f.fail()
			}
			return 0, err
		}
		p = p[consumed:]
		if m == nil {
			continue
		}
		select {
		case f.ch <- m:
		case <-f.done:
			return 0, errors.New("native protocol connection closed")
		}
	}
	return n, nil
}

// frame consumes bytes up to and including one newline and returns the
// completed frame, or nil while a frame is partial or being skipped.
func (f *frames) frame(p []byte) (object, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, 0, f.err
	}
	i := bytes.IndexByte(p, '\n')
	end := len(p)
	if i >= 0 {
		end = i + 1
	}
	if f.skipping {
		f.skipping = i < 0
		return nil, end, nil
	}
	if len(f.buf)+end > frameLimit {
		head := f.buf
		if len(head) < sniffBytes {
			head = append(head, p[:min(end, sniffBytes-len(head))]...)
		}
		if essentialFrame(head) {
			f.err = errors.New("native protocol frame exceeds limit")
			return nil, 0, f.err
		}
		f.skipped++
		f.buf = f.buf[:0]
		f.skipping = i < 0
		return nil, end, nil
	}
	f.buf = append(f.buf, p[:end]...)
	if i < 0 {
		return nil, end, nil
	}
	var m object
	if structured.ValidateJSON(f.buf) != nil || json.Unmarshal(f.buf, &m) != nil || m == nil {
		f.err = errors.New("invalid native protocol frame")
		return nil, 0, f.err
	}
	if f.bytes+len(f.buf) > bufferedLimit {
		f.err = errors.New("native protocol buffered data exceeds limit")
		return nil, 0, f.err
	}
	f.bytes += messageSize(m)
	f.buf = f.buf[:0]
	return m, end, nil
}

// essentialFrame reports whether a frame prefix may be a request or response
// (JSON-RPC "id" before "params"/"result") or a Claude control/result message.
// Unknown shapes are essential so nothing that needs an answer is dropped.
func essentialFrame(head []byte) bool {
	head = bytes.TrimLeft(head, " \t\r")
	if typ, ok := bytes.CutPrefix(head, []byte(`{"type":"`)); ok {
		name, _, _ := bytes.Cut(typ, []byte(`"`))
		switch string(name) {
		case "assistant", "user", "system", "stream_event":
			return false
		}
		return true
	}
	if !bytes.Contains(head, []byte(`"method":`)) {
		return true
	}
	payload := len(head)
	for _, key := range []string{`"params":`, `"result":`} {
		if i := bytes.Index(head, []byte(key)); i >= 0 {
			payload = min(payload, i)
		}
	}
	if payload == len(head) {
		return true
	}
	return bytes.Contains(head[:payload], []byte(`"id":`))
}

// stallLimit bounds how long a harness may stay silent while this side waits
// on it. A CLI that finishes without reporting it (or hangs) must not hold a
// task until the role deadline. Waiting on the user's approval is not silence.
var stallLimit = 10 * time.Minute

type connection struct {
	ctx        context.Context
	cancel     context.CancelFunc
	in         *os.File
	read       *os.File
	frames     *frames
	done       chan error
	ended      bool
	endErr     error
	queue      []object
	queueBytes int
	seq        int
}

func start(ctx context.Context, runner Runner, command process.Command) (*connection, error) {
	if ctx == nil {
		return nil, errors.New("native context is nil")
	}
	ctx, cancel := context.WithCancel(ctx)
	if command.Timeout > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, command.Timeout)
		previous := cancel
		cancel = func() { stop(); previous() }
	}
	r, w, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	f := &frames{ch: make(chan object, 256), fail: cancel, done: ctx.Done()}
	c := &connection{ctx: ctx, cancel: cancel, in: w, read: r, frames: f, done: make(chan error, 1)}
	command.Stdin = r
	command.Stdout = f
	go func() {
		result, err := provider.Run(ctx, runner, command)
		_ = r.Close()
		f.mu.Lock()
		if f.err != nil {
			err = f.err
		}
		if len(f.buf) > 0 && err == nil {
			err = errors.New("incomplete native protocol frame")
		}
		f.mu.Unlock()
		if err == nil && result.ExitCode != 0 {
			err = errors.New("native harness exited unsuccessfully")
		}
		c.done <- err
	}()
	return c, nil
}
func (c *connection) close() {
	c.cancel()
	_ = c.in.Close()
	_ = c.read.Close()
	if !c.ended {
		<-c.done
		c.ended = true
	}
}

// Print-mode providers flush their resumable session after sending the result.
// Close input and let them exit before tearing down the process tree.
func (c *connection) finish() error {
	_ = c.in.Close()
	if c.ended {
		return c.endErr
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-c.done:
		c.ended, c.endErr = true, err
		return err
	case <-c.ctx.Done():
		return c.cause()
	case <-timer.C:
		return errors.New("native harness did not finish saving its session")
	}
}
func (c *connection) send(v any) error {
	if err := c.cause(); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	// Closing the pipe on cancellation also interrupts a peer that stops reading.
	stop := context.AfterFunc(c.ctx, func() { _ = c.in.Close() })
	defer stop()
	_, err = c.in.Write(b)
	return err
}
func (c *connection) receive() (object, error) {
	if err := c.cause(); err != nil {
		return nil, err
	}
	select {
	case m := <-c.frames.ch:
		c.received(m)
		return m, nil
	default:
	}
	if c.ended {
		if c.endErr != nil {
			return nil, c.endErr
		}
		if skipped := c.skipped(); skipped > 0 {
			return nil, fmt.Errorf("native harness ended after %d oversized protocol frames were skipped: %w", skipped, io.EOF)
		}
		return nil, io.EOF
	}
	stalled := time.NewTimer(stallLimit)
	defer stalled.Stop()
	select {
	case <-c.ctx.Done():
		return nil, c.cause()
	case m := <-c.frames.ch:
		c.received(m)
		return m, nil
	case err := <-c.done:
		c.ended = true
		c.endErr = err
		return c.receive()
	case <-stalled.C:
		return nil, &store.ProviderFailure{Kind: store.ProviderStalled, Attempts: 1}
	}
}
func (c *connection) next() (object, error) {
	if err := c.cause(); err != nil {
		return nil, err
	}
	if len(c.queue) > 0 {
		m := c.queue[0]
		c.queueBytes -= messageSize(m)
		c.queue = c.queue[1:]
		return m, nil
	}
	return c.receive()
}
func (c *connection) skipped() int {
	c.frames.mu.Lock()
	defer c.frames.mu.Unlock()
	return c.frames.skipped
}

func (c *connection) enqueue(m object) error {
	if len(c.queue) >= 256 || c.queueBytes+messageSize(m) > 8<<20 {
		return errors.New("native pending events exceed limit")
	}
	c.queue = append(c.queue, m)
	c.queueBytes += messageSize(m)
	return nil
}
func (c *connection) call(method string, params any) (object, error) {
	c.seq++
	id := fmt.Sprintf("multiharness-%d", c.seq)
	if err := c.send(dict{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		m, err := c.receive()
		if err != nil {
			return nil, err
		}
		if str(m["id"]) == id && m["method"] == nil {
			if m["error"] != nil {
				return nil, fmt.Errorf("native %s request rejected", method)
			}
			return obj(m["result"]), nil
		}
		if err = c.enqueue(m); err != nil {
			return nil, err
		}
	}
}

// callDiscarding is call for methods that replay history (OpenCode
// session/load) as notifications before responding. Replayed notifications are
// context for the native session, not this invocation, so they are dropped
// instead of queued; peer requests are still queued for the caller.
func (c *connection) callDiscarding(method string, params any) (object, error) {
	c.seq++
	id := fmt.Sprintf("multiharness-%d", c.seq)
	if err := c.send(dict{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		m, err := c.receive()
		if err != nil {
			return nil, err
		}
		if str(m["id"]) == id && m["method"] == nil {
			if m["error"] != nil {
				return nil, fmt.Errorf("native %s request rejected", method)
			}
			return obj(m["result"]), nil
		}
		if m["id"] == nil {
			continue
		}
		if err = c.enqueue(m); err != nil {
			return nil, err
		}
	}
}

var errWithdrawn = errors.New("native approval withdrawn")

// Decide drains notifications while the terminal is waiting and cancels stale
// prompts before returning. The native protocol remains the policy authority.
func (c *connection) decide(a store.NativeApprover, request store.NativeApproval, withdrawn func(object) bool) (string, error) {
	if a == nil {
		return "", nil
	}
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	type decision struct {
		id  string
		err error
	}
	answer := make(chan decision, 1)
	go func() { id, err := a.ApproveNative(ctx, request); answer <- decision{id, err} }()
	finish := func(err error) (string, error) { cancel(); <-answer; return "", err }
	for _, m := range c.queue {
		if withdrawn(m) {
			return finish(errWithdrawn)
		}
	}
	for {
		select {
		case d := <-answer:
			if err := c.cause(); err != nil {
				return "", err
			}
			// Drain already-arrived invalidations before sending a decision.
			for {
				select {
				case m := <-c.frames.ch:
					c.received(m)
					if err := c.enqueue(m); err != nil {
						return "", err
					}
					if withdrawn(m) {
						return "", errWithdrawn
					}
				default:
					if d.err != nil {
						return "", d.err
					}
					if d.id == "" {
						return "", nil
					}
					for _, ch := range request.Choices {
						if ch.ID == d.id {
							return d.id, nil
						}
					}
					return "", errors.New("invalid native approval choice")
				}
			}
		case <-c.ctx.Done():
			return finish(c.cause())
		case err := <-c.done:
			c.ended = true
			c.endErr = err
			if err == nil {
				err = io.EOF
			}
			return finish(err)
		case m := <-c.frames.ch:
			c.received(m)
			if err := c.enqueue(m); err != nil {
				return finish(err)
			}
			if withdrawn(m) {
				return finish(errWithdrawn)
			}
		}
	}
}

func messageSize(m object) int {
	n := 0
	for k, v := range m {
		n += len(k) + len(v)
	}
	return n
}

func (c *connection) cause() error {
	c.frames.mu.Lock()
	err := c.frames.err
	c.frames.mu.Unlock()
	if err != nil {
		return err
	}
	return c.ctx.Err()
}
func (c *connection) received(m object) {
	c.frames.mu.Lock()
	c.frames.bytes -= messageSize(m)
	c.frames.mu.Unlock()
}
