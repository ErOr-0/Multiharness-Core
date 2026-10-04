// Package screen renders what a person reads at the interactive prompt: the
// welcome and settings summary, readiness checks, results, notices and the
// tool-failure pager. A View only formats and writes; it reads input solely
// to close the failure view and never starts agents or changes settings.
package screen
