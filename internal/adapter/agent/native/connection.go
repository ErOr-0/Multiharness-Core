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

const frameLimit = 4 << 20

// The stdout sink never waits on terminal input. Bounds cover both oversized
// frames and a peer flooding notifications while a human is deciding.
type frames struct {
	mu    sync.Mutex
	buf   []byte
	ch    chan object
	err   error
	bytes int
	fail  context.CancelFunc
}

func (f *frames) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer func() {
		if f.err != nil && f.fail != nil {
			f.fail()
		}
	}()
	if f.err != nil {
		return 0, f.err
	}
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		end := len(p)
		if i >= 0 {
			end = i + 1
		}
		if len(f.buf)+end > frameLimit {
			f.err = errors.New("native protocol frame exceeds limit")
			return 0, f.err
		}
		f.buf = append(f.buf, p[:end]...)
		p = p[end:]
		if i < 0 {
			break
		}
		var m object
		if structured.ValidateJSON(f.buf) != nil || json.Unmarshal(f.buf, &m) != nil || m == nil {
			f.err = errors.New("invalid native protocol frame")
			return 0, f.err
		}
		if f.bytes+len(f.buf) > 8<<20 {
			f.err = errors.New("native protocol buffered data exceeds limit")
			return 0, f.err
		}
		f.bytes += messageSize(m)
		f.buf = f.buf[:0]
		select {
		case f.ch <- m:
		default:
			f.err = errors.New("native protocol queue exceeds limit")
			return 0, f.err
		}
	}
	return n, nil
}

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
	f := &frames{ch: make(chan object, 256), fail: cancel}
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
		return nil, io.EOF
	}
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
