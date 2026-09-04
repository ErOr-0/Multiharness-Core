// Package git supplies cooperative workspace exclusion and baseline-relative
// repository evidence. Snapshotting never stages, resets, stashes, commits, or
// runs external diff/textconv/fsmonitor programs. It reads tracked and untracked
// non-ignored files, storing symlink targets rather than dereferencing them.
// Diffs retain Git's relative before/after comparison-tree prefixes verbatim;
// ChangedFiles contains the repository-relative paths without those prefixes.
// Content and mode fingerprints also detect changes Git cannot express in a
// patch, such as permission-bit changes other than the executable bit.
//
// Dirty paths are protected at whole-file granularity. Any later edit to those
// paths, the index, or HEAD stops the workflow. On preservation violations or
// inspection failure, a private recovery directory retains the starting files
// and index-entry manifest; it is not automatically restored or removed.
//
// Limits fail closed. Non-Git/bare directories, subdirectory targets, nested
// repositories, submodules, unmerged indexes, sparse/skip-worktree and
// assume-unchanged entries, special files, and non-UTF-8 paths are unsupported.
// Ignored build artifacts are outside the snapshot boundary. New ignored files
// therefore are not claimed as workflow changes. The lock covers cooperating
// processes sharing a Git common directory on supported Unix systems; it is
// not a sandbox against arbitrary commands or concurrent human edits.
package git
