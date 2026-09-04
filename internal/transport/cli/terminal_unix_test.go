//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTerminalReaderIsBoundedAndCancellationAware(t *testing.T) {
	for _, input := range []string{"yes\n", "yes", "", strings.Repeat("x", 65) + "\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.WriteString(input)
		w.Close()
		answer, err := (&terminalConfirmation{file: r}).ReadConfirmation(t.Context())
		r.Close()
		if input == "yes\n" && (answer != "yes" || err != nil) {
			t.Fatal("complete line lost")
		}
		if (input == "yes" || input == "") && !errors.Is(err, io.EOF) {
			t.Fatal("EOF became consent")
		}
		if len(answer) > 64 {
			t.Fatal("unbounded input")
		}
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = (&terminalConfirmation{file: r}).ReadConfirmation(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatal("terminal wait ignored deadline")
	}
}
