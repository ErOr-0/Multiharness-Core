package screen

import (
	"context"
	"fmt"
	"os"
	"strings"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/transport/cli/term"
)

// FailureDetails opens a temporary terminal view only when requested. The
// alternate screen restores the compact task display when the user closes it.
// LineReader reads one bounded line; the failure view waits on it to close.
type LineReader interface {
	ReadLine(context.Context, int) (string, error)
}

func (v *View) FailureDetails(ctx context.Context, input LineReader, failures []activity.Event, count uint64) (err error) {
	if count == 0 {
		return v.Notice("No tool failures were reported by the last task.", false)
	}
	_, tty := term.Size(v.Writer)
	modal := tty && os.Getenv("TERM") != "dumb"
	if reader, ok := input.(interface {
		ReadFailureDetails(context.Context, *View, []activity.Event, uint64) error
	}); ok && modal {
		return reader.ReadFailureDetails(ctx, v, failures, count)
	}
	body := v.failureText(failures, count)
	if modal {
		if err = term.Write(v.Writer, "\x1b[?1049h"); err != nil {
			return err
		}
		defer func() {
			if restore := term.Write(v.Writer, "\x1b[?1049l"); restore != nil {
				err = restore
			}
		}()
	}
	if modal {
		body += "  Press Enter to close and return to your task prompt.\n"
	}
	if err = v.Print(body); err != nil || !modal {
		return err
	}
	_, err = input.ReadLine(ctx, 16)
	return err
}

func (v *View) failureText(failures []activity.Event, count uint64) string {
	var body strings.Builder
	fmt.Fprintf(&body, "\n  %s\n\n", v.Paint("▼ Tool failure details", "1;33"))
	if count > uint64(len(failures)) {
		fmt.Fprintf(&body, "  Showing the latest %d of %d reported events.\n\n", len(failures), count)
	}
	details := NewFailurePager(failures, count)
	for i, failure := range details.events {
		label := failure.Summary
		if label == "" {
			label = "tool failed"
		}
		if failure.Stage != "" {
			label = failure.Stage + " · " + label
		}
		fmt.Fprintf(&body, "  %s\n", v.Paint(fmt.Sprintf("%d. %s · %s", i+1, failure.Agent, label), "1;33"))
		body.WriteString(v.resultBody(details.overviews[i]))
		if failure.Output != "" {
			body.WriteString("\n  COMMAND OUTPUT\n")
			body.WriteString(v.resultBody(failure.Output))
		}
		body.WriteByte('\n')
	}
	return body.String()
}
