# Choose a workspace folder

A workspace is the folder selected with `/workspace` inside your shared Docker
mount, or `--workdir` when using the development CLI. It can be a plain folder, one project,
a project subfolder, or a parent containing several projects with separate Git
repositories. Multiharness does not initialize Git in your folder.

```text
My Suite/                 <- select this folder
  api/.git/
  api/main.go
  web/.git/
  web/package.json
  tools/                  <- Git is optional here too
```

Agents work from the selected folder and follow the relevant projects' local
instructions. Changes are reported as `api/main.go`, `web/package.json`, etc.
The planning, implementation, validation, review and repair workflow is the same
across projects. Select the folder containing only the projects relevant to the
task: Docker shares that entire folder, including its subfolders, with agents.

## Change tracking

- All included files are snapshotted before planning. File contents, modes,
  symlink targets and deletions are compared against that starting snapshot.
  Planning, validation and review must leave the captured state unchanged.
- For every discovered Git repository, existing staged, unstaged and untracked
  files are protected at whole-file granularity. Resolve those changes before
  asking Multiharness to edit the same files. Index, HEAD or Git layout changes
  stop the run; agents must not initialize, move or remove repositories.
- Plain files have no Git clean/dirty distinction. They are editable against
  their captured baseline. Snapshots provide change evidence, not a durable
  backup or automatic undo; keep your normal backups/version control.
- `.gitignore` rules work in plain folders too. Each repository also uses its
  own Git ignore rules. Ignored folders (including any repositories inside them)
  are excluded. A changed ignore rule cannot hide an already captured file.
  Add appropriate ignore rules for dependencies and build output, such as
  `node_modules/`, `bin/`, `obj/` and `dist/`, when your projects need them.
- Git remains a bundled inspection/diff utility in Docker. Plain-folder listing
  uses an empty Git index in a private temporary directory; it creates no Git
  metadata in your workspace. Native installations still need the Git executable.

Folder locks prevent overlapping parent/child workflows. Sibling folders may run
independently unless they share Git metadata. These cooperative locks do not stop
human edits or unrelated tools. Linked worktrees also share their Git common-dir
lock. When using Docker, their referenced Git metadata must be inside the mounted
folder and resolve at its container path; missing external metadata fails clearly.

When selecting a subfolder of a repository, only files beneath the selected folder
are included in change evidence. If ancestor Git metadata is accessible, its
index/HEAD are still protected. A Docker mount of only that subfolder cannot see
Git metadata outside the mount and is treated as a plain folder.

## Checks and limits

Configured validation commands run from the selected folder. Use project-aware
arguments, for example `npm --prefix web test`, `go -C api test ./...`, or an
explicit trusted script that checks all affected projects. No checks are invented
or run by default.

Workspace snapshots have no default cap on file count, individual file size,
total file content, Git command/diff output or combined repository metadata.
The existing `git.max_files`, `git.max_file_bytes`, `git.max_snapshot_bytes` and
`git.max_output_bytes` settings use `0` for unlimited; positive values opt into
limits on the combined workspace. Explicit limits still fail closed, and
incomplete evidence cannot be approved.

Previously saved configurations retain their positive limits. To remove them
in the interactive prompt, run:

```text
/set git-max-files 0
/set git-max-file-bytes 0
/set git-max-snapshot-bytes 0
/set git-max-output-bytes 0
/save
```

Snapshots and evidence are held in memory, so resource use grows with the selected
folder. Inspection deadlines and cancellation still apply.

Submodules, unmerged/sparse indexes, assume-unchanged entries, special files and
unsafe paths remain unsupported. Native Windows execution remains disabled;
Windows users run the Linux Docker image. The updated Docker preview and current
source support folders; the older v0.1.0-alpha.3 native release does not.
