// Package progress turns workflow events and agent activity into the stderr
// stream for one task run: allowlisted JSON or key=value log records for
// scripts, or the friendly live display on a terminal. Sink is the
// workflow.EventSink the composition root hands to a run.
package progress
