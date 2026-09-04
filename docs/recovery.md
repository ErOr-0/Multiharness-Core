# Interrupted and failed runs

There is no durable run database, checkpoint resume, or automatic rollback.
OpenCode session reuse is limited to repairs within one running workflow.
Rerunning the CLI starts a new task/run with a fresh repository baseline.

For billing/quota failures, the result contains `failure.provider.kind` equal to
`billing_exhausted` and a safe operator message. No automatic billing retry or
account switch occurs. The running interactive CLI can offer a confirmed switch
to the alternate agent for the failed role; it retains the original baseline and
inspects partial edits before continuing. If declined or unavailable, resolve the
configured provider's credit/usage issue, then follow
the inspection steps below; failed implementation/repair may leave partial files.
Restoring credits does not resume the stopped run. See
[provider-failures.md](provider-failures.md) for transient failure policy and limits.

## Stop and inspect

1. Prefer Ctrl+C or SIGTERM. The process adapter cancels supported Unix process
   groups and the workflow releases its lease. A cancelled result is not approval.
2. Confirm the run and its children have actually stopped before editing or
   starting another workflow. A killed/crashed parent may not have run cleanup;
   detached processes or external services need separate inspection.
3. Inspect the saved JSON status, failure stage/code, last validation/review, and
   repository evidence. Preserve the result securely if investigation requires it.
4. Review the live repository using read-only Git commands such as `git status`,
   `git diff`, and `git diff --cached`. These may contain sensitive content.
   Compare untracked files separately. Do not assume a failed run changed nothing.
5. Decide file-by-file what to retain, repair, or restore. Do not run a broad
   reset/clean command or overwrite unrelated work to restart the workflow.

The cooperative lock is a kernel-held advisory lock on `multiharness.lock` inside
the common Git directory. The file may remain after a normal run; its existence
does not mean the lock is held. **Do not delete the lock file** to bypass an active
workflow: another process could then acquire a different inode and run concurrently.
Investigate the owner and stop the process cleanly.

## Preservation failures

If a protected path changes or current-state inspection fails, the workspace
adapter attempts to retain the original snapshot in a private
`multiharness-recovery-*` directory. Its exact path is returned as
`repository.recovery_directory`. This is an error-recovery artifact, not a
routine project backup. Successful runs do not retain one.

The directory contains baseline files under `files/` and a `manifest.json` with
baseline state, index-entry metadata, HEAD reference, and baseline-missing paths.
Inspect the manifest and compare files before copying anything. Baseline-missing
paths represent original deletions; they are not files to recreate. A copy can be
partial if recovery itself failed, so inspect the reported error too.

The manifest is **not a complete Git backup**: it does not promise a copy of every
index-stage blob or object, ignored file, external side effect, or later human edit.
Do not rebuild the index or reset HEAD automatically from it. Protect retained
artifacts as source code; remove only the exact recovery directory after verifying
that its contents are no longer needed.

## When no result or recovery directory exists

SIGKILL, a machine crash, output failure or early interruption can leave no final
JSON. The normal baseline is in memory, so a recovery artifact is not guaranteed.
Use the repository's own history and any operator-managed safeguards. Never claim
the previous task was approved from an agent's completion message alone.

Before rerunning, reconcile remaining changes yourself. They will be considered
pre-existing user changes and protected by the new run; this can intentionally
prevent another agent from editing partially completed files. Resolve that overlap
explicitly (for example, retain and commit reviewed work yourself) or start from a
fresh disposable checkout. There is no automatic “resume anyway” bypass.
