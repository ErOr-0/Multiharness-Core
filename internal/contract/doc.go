// Package contract owns the serializable state and invariants exchanged across
// the Multiharness workflow: tasks, plans, handoff projections, evidence and
// outcomes. Its files are separated by domain responsibility. It performs no
// I/O; persistence and filesystem adapters belong outside this package.
package contract
