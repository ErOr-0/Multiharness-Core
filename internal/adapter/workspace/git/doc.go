// Package git supplies cooperative workspace exclusion and baseline-relative
// repository evidence. Snapshotting never stages, resets, stashes, commits, or
// runs external diff/textconv/fsmonitor programs. It reads plain folder files and
// tracked/untracked non-ignored files in discovered repositories, storing symlink
// targets rather than dereferencing them. Git is optional in the user's folder;
// the Git executable supplies ignore rules and diffs using private temporary data.
// Diffs retain Git's relative before/after comparison-tree prefixes verbatim;
// ChangedFiles contains the repository-relative paths without those prefixes.
// Content and mode fingerprints also detect changes Git cannot express in a
// patch, such as permission-bit changes other than the executable bit.
//
// Dirty paths are protected at whole-file granularity. Any later edit to those
// paths, any repository's index/HEAD, or Git layout stops the workflow. Plain
// files without Git remain editable against the captured baseline.
// On preservation violations or
// inspection failure, a private recovery directory retains the starting files
// and index-entry manifest; it is not automatically restored or removed.
//
// Limits cover the whole selected folder and fail closed. Submodules,
// unmerged indexes, sparse/skip-worktree and
// assume-unchanged entries, special files, and non-UTF-8 paths are unsupported.
// Ignored build artifacts are outside the snapshot boundary. New ignored files
// therefore are not claimed as workflow changes. Locks cover cooperating
// processes using overlapping folders or a shared Git common directory; this is
// not a sandbox against arbitrary commands or concurrent human edits.
package git
