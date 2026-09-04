# Dependency setup and failure handling

Multiharness checks dependencies when their stage uses them. A direct answer
requires Git and the planner; it does not require the implementer or reviewer.
Missing dependencies never authorize billing fallback or provider retries.

## Interactive installation

On macOS/Linux, a missing **default** `codex` or `opencode` command can prompt
for installation when a trusted npm executable is available on PATH. Only exact
`yes` (case-insensitive, ignoring surrounding spaces) authorizes it. Both stdin
and stderr must be terminals; CI, piped input, EOF and refusal cannot authorize it.

The prompt discloses the exact command, global npm changes, package installation
scripts running with the operator's permissions, and the restart requirement.
Install packages are fixed by this release:

- `@openai/codex@0.153.0`
- `opencode-ai@1.18.23`

The adapter runs npm directly, not through a shell, with an explicit public npm
registry, bounded retained output, and no task stdin or agent stream callbacks.
It uses a temporary directory outside the target checkout, not the target's npm
project configuration. This directory is removed afterward; npm's normal cache
and global installation remain. Raw npm diagnostics are not printed or classified
as provider failures.

No sudo, package-manager bootstrap, unattended upgrade, cache deletion, credential
creation, sign-in, or model substitution is performed. A per-user cooperative OS
lock serializes Multiharness installations across repositories; it does not lock
other applications' package-manager usage. The private lock file remains in the
user cache but is released on close/process exit.

The selected npm executable must be an absolute regular executable outside the
target, including its resolved symlink, and not world-writable. The operator's
package-manager installation, configuration, inherited environment, registry and
package scripts remain trusted. This is not a package execution sandbox or a
complete supply-chain verification system. Managed deployments should preinstall
and audit dependencies, configure explicit executable paths, and disable installs.

## Controls

| Option | Environment | Default |
| --- | --- | --- |
| `--install-mode prompt\|disabled` | `MULTIHARNESS_INSTALL_MODE` | `prompt` |
| `--install-timeout 5m` | `MULTIHARNESS_INSTALL_TIMEOUT` | `5m` |

JSON keys are `install_mode` and `install_timeout` in version-1 configuration.
Normal file → environment → flag precedence applies. Installer timeouts must be
positive and at most 30 minutes. Stage and whole-run deadlines also apply,
including while waiting for consent. Ordinary Make targets and CI disable
installation.

After even a successful install, **the current workflow stops without approval**.
Check PATH, run `codex` to sign in or `opencode` and `/connect` to configure your
provider, and rerun your task. No task is automatically replayed. An existing
implementation remains in the checkout and its independent evidence is preserved;
review that work before rerunning. This is not durable workflow resume.

## Failure matrix

| Condition | Behavior |
| --- | --- |
| Default CLI genuinely absent | Offer bounded installation if eligible; otherwise give setup instructions |
| Explicit path/custom executable absent | Preserve the pin; explain how to fix it, without replacing it |
| Missing/broken npm or unsafe repository-local npm | Give manual npm/Homebrew instructions; do not bootstrap npm |
| Missing Git | Explain OS installation (macOS: `xcode-select --install`); do not launch OS installers |
| Wrong permissions, bad interpreter, process exit 127 | Report failure; never assume this means a missing package |
| Installed Codex too old for cache/protocol | Select another compatible installed runtime; otherwise require manual update |
| Corrupt/unreadable Codex cache | Fail safely; never rewrite shared caches or credentials |
| Refusal, blank input, EOF, piped yes, CI | No installation and no automatic provider switch |
| Broken prompt/progress output | Stop before installation rather than accepting invisible consent |
| Another Multiharness installer holds the lock | Stop immediately; no duplicate installation or indefinite lock wait |
| Network/permission/disk/package-manager failure | Stop with safe setup guidance; no sudo or automatic retry |
| Timeout or Ctrl+C | Cancel installation through Unix process-tree handling; partial installation may remain |
| Installer exits zero but CLI still unavailable | Report PATH/setup failure; never claim the workflow succeeded |
| Install succeeds | Stop for authentication and a fresh run; compatibility is rechecked on the next invocation |
| Expired/missing authentication or inaccessible model | Existing provider classification gives account/model guidance; no reinstall |
| Billing exhaustion | Separate, explicit role-switch consent; installation consent does not authorize fallback |
| Transient overload/rate limit | Existing bounded read-only retry policy; mutating calls are never replayed |
| Missing validation command | Validation infrastructure failure; do not install arbitrary commands from task/config |
| Native Windows/other unsupported installer platform | Manual setup; no automatic installer execution |

Native Windows workflows now **fail closed** before any agent runs: Windows
locking alone does not establish process-tree cancellation or terminal consent.
The unsafe predictable-name access probe was removed. Use macOS/Linux or install
the tools within WSL; test WSL as a Linux deployment before release. Native
Windows support requires a dedicated implementation and validation gate.

The unfinished local-LLM adapter now returns an explicit unavailable error from
all operations. It cannot manufacture implementation summaries or approve work
based solely on validation flags. It is not exposed through the CLI.

## Verification and remaining gates

Offline tests cover install consent, failed/short output, piped input, cancellation,
stage deadline, error classification, successful/failed/partial setup simulations,
pins, unsafe executable locations, concurrent installation locking, configuration
precedence and missing-dependency workflows. Strict Gherkin scenarios use real
Git/process adapters and fixture agents. No test installs real software or calls
a model. The installer uses the existing tested Unix process-tree cancellation.

Before production, run real installation/permission/offline-network tests in
disposable macOS/Linux machines, authenticate each intended provider/model, and
complete the live workflow/review/repair gates in [testing.md](testing.md). Fixture
success is not a guarantee that every operating-system or provider edge case works.

Installation/sign-in guidance follows the official
[Codex CLI documentation](https://learn.chatgpt.com/docs/codex/cli) and
[OpenCode installation documentation](https://opencode.ai/en/docs).
