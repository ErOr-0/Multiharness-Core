package term

import (
	"errors"
	"io"
)

// ErrInputTooLong reports a line that exceeded the caller's byte limit.
var ErrInputTooLong = errors.New("input exceeds the configured byte limit")

// ErrOutput wraps every failed or short terminal write.
var ErrOutput = errors.New("terminal output failed")

// Write writes value completely or returns an error wrapping ErrOutput.
func Write(w io.Writer, value string) error {
	n, err := io.WriteString(w, value)
	if err == nil && n != len(value) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errors.Join(ErrOutput, err)
	}
	return err
}
