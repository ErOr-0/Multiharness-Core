package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"multiharness-core/internal/adapter/agent/activity"
)

// failureDetails opens a temporary terminal view only when requested. The
// alternate screen restores the compact task display when the user closes it.
func (v *interactiveView) failureDetails(ctx context.Context, input LineInput, failures []activity.Event, count uint64) (err error) {
	if count == 0 {
		return v.notice("No tool failures were reported by the last task.", false)
	}
	_, tty := terminalSize(v.writer)
	modal := tty && os.Getenv("TERM") != "dumb"
	if reader, ok := input.(interface {
		readFailureDetails(context.Context, *interactiveView, []activity.Event, uint64) error
	}); ok && modal {
		return reader.readFailureDetails(ctx, v, failures, count)
	}
	body := v.failureText(failures, count)
	if modal {
		if err = interactiveWrite(v.writer, "\x1b[?1049h"); err != nil {
			return err
		}
		defer func() {
			if restore := interactiveWrite(v.writer, "\x1b[?1049l"); restore != nil {
				err = restore
			}
		}()
	}
	if modal {
		body += "  Press Enter to close and return to your task prompt.\n"
	}
	if err = v.write(body); err != nil || !modal {
		return err
	}
	_, err = input.ReadLine(ctx, 16)
	return err
}

func (v *interactiveView) failureText(failures []activity.Event, count uint64) string {
	var body strings.Builder
	fmt.Fprintf(&body, "\n  %s\n\n", v.paint("▼ Tool failure details", "1;33"))
	if count > uint64(len(failures)) {
		fmt.Fprintf(&body, "  Showing the latest %d of %d reported events.\n\n", len(failures), count)
	}
	details := newFailurePager(failures, count)
	for i, failure := range details.events {
		label := failure.Summary
		if label == "" {
			label = "tool failed"
		}
		if failure.Stage != "" {
			label = failure.Stage + " · " + label
		}
		fmt.Fprintf(&body, "  %s\n", v.paint(fmt.Sprintf("%d. %s · %s", i+1, failure.Agent, label), "1;33"))
		body.WriteString(v.resultBody(details.overviews[i]))
		if failure.Output != "" {
			body.WriteString("\n  COMMAND OUTPUT\n")
			body.WriteString(v.resultBody(failure.Output))
		}
		body.WriteByte('\n')
	}
	return body.String()
}
