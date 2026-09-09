package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type ProcessRunner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

// Run terminates the command tree on a reported provider error, including a
// pre-session error and an exit-zero error. Only the private child context is
// cancelled: a provider failure must not become a user-cancelled workflow.
func Run(ctx context.Context, runner ProcessRunner, command process.Command) (process.Result, error) {
	if ctx == nil {
		return runner.Run(ctx, command)
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	report := &failureReport{cancel: cancel}
	out := &lineObserver{report: report}
	errOut := &lineObserver{report: report, stderr: true}
	command.Stdout = join(out, command.Stdout)
	command.Stderr = join(errOut, command.Stderr)
	result, err := runner.Run(child, command)
	out.finish()
	errOut.finish()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if failure := report.get(); failure != nil {
		return result, failure
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return result, err
	}
	if err != nil {
		if failure := Text(result.Stderr); failure != nil {
			failure.Source = "stderr"
			return result, failure
		}
	}
	return result, err
}

func join(observer, existing io.Writer) io.Writer {
	if existing == nil {
		return observer
	}
	return io.MultiWriter(observer, existing)
}

type failureReport struct {
	mu      sync.Mutex
	failure *store.ProviderFailure
	cancel  context.CancelFunc
}

func (r *failureReport) set(f *store.ProviderFailure) {
	if f == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// A billing failure can never be downgraded to a transient error in the same
	// output batch, even if the provider emits further diagnostics during exit.
	if r.failure == nil || (r.failure.Transient() && !f.Transient()) || f.Kind == store.ProviderBillingExhausted {
		r.failure = f
	}
	r.cancel()
}
func (r *failureReport) get() *store.ProviderFailure {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failure
}

const maxLineBytes = 1 << 20

type lineObserver struct {
	report     *failureReport
	stderr     bool
	buffer     []byte
	discarding bool
}

func (o *lineObserver) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		end := len(data)
		if index >= 0 {
			end = index
		}
		part := data[:end]
		if !o.discarding {
			if len(o.buffer)+len(part) > maxLineBytes {
				o.discarding = true
				o.buffer = nil
			} else {
				o.buffer = append(o.buffer, part...)
			}
		}
		if index < 0 {
			break
		}
		if !o.discarding {
			o.inspect(o.buffer)
		}
		o.buffer = nil
		o.discarding = false
		data = data[index+1:]
	}
	return n, nil
}
func (o *lineObserver) finish() {
	if !o.discarding {
		o.inspect(o.buffer)
	}
	o.buffer = nil
}
func (o *lineObserver) inspect(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	// Validate the discriminator without interpreting arbitrary tool payloads.
	// Only terminal errors belong to the strict provider-error schema below.
	kind, ambiguous := envelopeType(line)
	if ambiguous {
		o.report.set(&store.ProviderFailure{Kind: store.ProviderUnknown, Reason: "malformed_error_event", Attempts: 1})
		return
	}
	var event struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error"`
	}
	if (kind == "error" || kind == "turn.failed") && json.Unmarshal(line, &event) == nil {
		if structured.ValidateObject(line, "error", "message") != nil {
			o.report.set(&store.ProviderFailure{Kind: store.ProviderUnknown, Attempts: 1})
			return
		}
		data := event.Error
		if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			data = line
		}
		f := Classify(data, time.Now())
		if f == nil {
			f = &store.ProviderFailure{Kind: store.ProviderUnknown, Attempts: 1}
		}
		f.Source = event.Type
		o.report.set(f)
		return
	}
	if o.stderr {
		text := string(line)
		if strings.HasPrefix(text, "Error:") || strings.HasPrefix(text, "ERROR:") {
			if f := Text(text); f != nil {
				f.Source = "stderr"
				o.report.set(f)
			}
		}
	}
}

// Decode values as opaque JSON rather than recursively applying error-contract
// rules to tool output. Duplicate/aliased type keys still cannot hide an error.
func envelopeType(line []byte) (kind string, ambiguous bool) {
	if !json.Valid(line) {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	opening, _ := decoder.Token()
	if opening != json.Delim('{') {
		return "", false
	}
	seen := false
	for decoder.More() {
		token, _ := decoder.Token()
		key, ok := token.(string)
		if !ok {
			return "", false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return "", false
		}
		if !strings.EqualFold(key, "type") {
			continue
		}
		if seen || key != "type" {
			return "", true
		}
		seen = true
		if json.Unmarshal(value, &kind) != nil {
			return "", true
		}
	}
	return kind, false
}
