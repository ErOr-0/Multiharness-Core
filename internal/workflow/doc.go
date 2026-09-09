// Package workflow defines the vendor-neutral contracts and state machine for
// coordinating planning, implementation, validation, review, and repair.
//
// NewService constructs the plain-Go application entry point. Service.Run
// accepts a typed store.TaskInput, validates contracts at stage boundaries,
// and returns a store.TaskOutput. It executes ordinary Go methods with the
// caller's context and emits ordered lifecycle events through EventSink.
// No framework runtime, flow registration, or separate step executor is needed.
//
// A workflow follows these transitions:
//
//	intake -> planning -> implementation -> validation -> review
//	                                                      |
//	                         approved <--------------------+ approved
//	                                                      |
//	                         repair -> validation -> review+ blocking findings
//	                                                      |
//	                         repair_limit_reached <--------+ no repair available
//
// Planning can instead return an explicit answer-only plan. That branch ends
// with answered without acquiring a lease or scanning the workspace. It never
// calls implementation, validation, review, or repair. Planning runs with
// provider-enforced read-only permissions; answered is not coding approval.
//
// Cancellation moves any non-terminal stage to cancelled. A non-cancellation
// error moves it to failed and records a WorkflowStage and FailureCode.
//
// MaxRepairAttempts counts completed calls to Implementer.ApplyReview. The
// initial implementation does not consume this allowance. Consequently, zero
// permits the initial review but no repair, while N permits at most N repairs
// and N+1 reviews. A rejected review must contain at least one blocking finding.
// Non-blocking suggestions alone never trigger a repair.
// Read-only planning/review provider retries are a separate, opt-in allowance.
// All agent launches count toward a per-run invocation limit; this is not a
// monetary budget. Billing/access failures and mutating calls are never retried.
//
// Intake validates the request. After planning chooses implementation, the
// implementation stage acquires a lease and records the baseline before edits.
// Every coding round compares against that original
// baseline. Independently inspected changed files replace agent-reported
// claims; review and final results carry the same repository evidence.
// Validation and review must not change the inspected checkout.
// Missing evidence, changed protected user work, and workspace failures stop
// the run without approval. The lease is released on every terminal path.
// Concrete adapters define supported repositories and preservation granularity.
//
// This package owns workflow use-case policy and consumer-side ports. Shared
// workflow state and contracts live in internal/store, split by responsibility.
// Filesystem access, agent CLIs, Git inspection, persistence, and delivery
// handlers are outer adapters injected through the ports declared here. The
// workflow package never imports those adapters.
package workflow
