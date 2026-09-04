package process

import (
	"io"
	"sync"
)

// tailBuffer retains only the most recent bytes written to it.
type tailBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{data: make([]byte, 0, limit), limit: limit}
}

func (buffer *tailBuffer) Write(data []byte) (int, error) {
	written := len(data)
	if written == 0 {
		return 0, nil
	}

	if written >= buffer.limit {
		buffer.truncated = buffer.truncated || len(buffer.data) > 0 || written > buffer.limit
		buffer.data = append(buffer.data[:0], data[written-buffer.limit:]...)
		return written, nil
	}

	overflow := len(buffer.data) + written - buffer.limit
	if overflow > 0 {
		buffer.truncated = true
		copy(buffer.data, buffer.data[overflow:])
		buffer.data = buffer.data[:len(buffer.data)-overflow]
	}
	buffer.data = append(buffer.data, data...)
	return written, nil
}

func (buffer *tailBuffer) String() string {
	return string(buffer.data)
}

func (buffer *tailBuffer) Truncated() bool {
	return buffer.truncated
}

// outputWriter serializes stdout and stderr forwarding through a shared mutex,
// preventing concurrent writes when both streams use the same external sink.
type outputWriter struct {
	mu      *sync.Mutex
	capture *tailBuffer
	sink    io.Writer
	err     error
}

func (writer *outputWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	_, _ = writer.capture.Write(data)
	if writer.sink == nil {
		return len(data), nil
	}

	written, err := writer.sink.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil && writer.err == nil {
		writer.err = err
	}
	return written, err
}

func (writer *outputWriter) Error() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.err
}

func populateOutput(result *Result, stdout, stderr *tailBuffer) {
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.StdoutTruncated = stdout.Truncated()
	result.StderrTruncated = stderr.Truncated()
}
