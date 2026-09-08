# Multiharness Core

Use **`magent`** to give coding tasks to Codex and OpenCode from your terminal.
It plans the work, makes changes, runs your configured checks, and reviews the result.

## 1. Install

For a bundled Linux environment on Windows, macOS or Linux, use the
[Docker preview](docs/docker.md). It includes the agent CLIs and common project
tools, mounts your project folder and keeps your own provider logins in a private
volume. Docker must be installed; project-specific SDKs may still be needed.

For installation directly on your operating system:

You need Git and the agent CLIs you select installed and signed in. Codex can
handle every role with different models; see [role selection](docs/cli.md#codex-implementation-without-opencode).
Use macOS or Linux;
on Windows, run inside WSL with Linux-installed tools. Native Windows execution
is not supported yet. See the [setup guide](docs/setup.md) if you need help.

Download the matching archive from [GitHub Releases](https://github.com/ErOr-0/Multiharness-Core/releases)
and follow the [download and installation guide](docs/releases.md). A downloaded
binary does not require Go, unless your project's own checks use it. If no
release has been published yet, build from source below.

To build from source, install Go 1.26.6 or newer and run from this project's directory:

```sh
make install
```

If your terminal cannot find `magent`, add its installation folder to your PATH:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Add that line to your shell profile to keep it for future terminal sessions.
Run `magent --version` to identify the installed build.

## 2. Open your project

```sh
cd /path/to/your/projects
magent
```

The interactive screen opens in your current terminal. Select one project, a
subfolder, or a parent folder containing multiple projects. Git repositories are
optional; Multiharness never initializes one for you. When Git is present,
existing uncommitted files remain protected. Resolve those changes before asking
`magent` to edit the same files. See [workspace behavior](docs/workspaces.md).

## 3. Configure your agents

Type **`/config`** and follow the prompts to choose your planner and models.

- Choose **Codex** or **OpenCode** as the planner.
- Enter an OpenCode model as `provider/model` using a model your account can access.
- Press Enter to keep a value. Incorrect answers can be retried.
- Type `/cancel` during setup to discard your changes.

When setup finishes, type **`/save`** to remember your settings.
Use **`/settings`** to check them at any time.

OpenCode writes the code, and Codex reviews it. Tasks use your configured provider
accounts and may consume paid usage.

## 4. Give it a task

Type a task and press Enter. For example:

```text
Add a health-check endpoint and tests for it.
```

Or ask a question:

```text
Explain how authentication works in this project.
```

Watch the progress in your terminal. When the task finishes, you can enter another.
Each task is independent, so include the context it needs.
Press **Ctrl+C** to cancel and exit, or type **`/quit`** at the prompt.

## Add your test command

Tests are not configured by default. For a Go project, enter:

```text
/set validation-checks [{"executable":"go","args":["test","./..."]}]
/save
```

Use a test command appropriate for your project. These checks run after code changes.

## Useful commands

| Command | What it does |
| --- | --- |
| `/config` | Choose agents and models |
| `/settings` | Show current settings |
| `/set max-repair-attempts 3` | Allow up to three repair attempts |
| `/set color never` | Turn off colors |
| `/options` | List all available settings |
| `/load /path/to/config.json` | Load settings from a file |
| `/save` | Save settings for future interactive sessions |
| `/help` | Show help |
| `/quit` | Exit |

## Understand the result

| Result | Meaning |
| --- | --- |
| `approved` | Review and any configured checks passed |
| `answered` | Your question was answered without code changes |
| `failed` | The task stopped because of an error |
| `repair_limit_reached` | More fixes are needed, but the repair limit was reached |
| `cancelled` | The task was interrupted or timed out |

Changes stay in your project for you to inspect and commit. Cancelling does not undo them.

For more options, see the [command reference](docs/cli.md).
For installation or sign-in problems, see the [setup guide](docs/setup.md).

Docker quick start: [download Magent and choose your folder through magent --config](https://multiharness.mdfahimhossen.space/#start). Original project files are bind-mounted and edited directly; logins/settings live in a named Docker volume. See [Docker setup](docs/docker.md).
