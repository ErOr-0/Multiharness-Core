package workflow

import (
	"multiharness-core/internal/contract"
)

// Canonical evidence versus agent handoff evidence.
//
// RepositoryEvidence, ValidationReport, Review and ImplementationResult in
// workflow state remain the complete machine-owned record for verification
// and final audit output. Agent prompts receive only bounded, role-specific
// projections built in internal/contract (ImplementationHandoff, ReviewChunk,
// ValidationHandoff, RepairHandoff). Provider and role sessions stay isolated:
// team handoffs carry explicit context, never another role's session ID.
//
// handoffDiag builds metadata-only diagnostics for lifecycle events: byte
// counts and chunk counts, never prompt contents. Per-prompt byte sizes stay
// with the adapter layer (and future optional Langfuse observations) because
// the role ports return results, not prompt receipts.
func (state *runState) handoffDiag() contract.HandoffDiagnostics {
	var diag contract.HandoffDiagnostics
	if state.repository != nil {
		diag.PreExistingFileCount = len(state.repository.PreExistingFiles)
		diag.RawDiffBytes = len(state.repository.Diff)
	}
	if state.validation != nil {
		diag.ValidationBytesRetained = contract.ProjectValidation(*state.validation, contract.MaxValidationHandoffBytes).BytesRetained
	}
	return diag
}
