# Multiharness Core

Use **`magent`** to give coding tasks to Codex and OpenCode from your terminal.
It plans the work, makes changes, runs your configured checks, and reviews the result.

## 1. Set up Docker once

Install Docker Desktop (Linux containers on Windows), then follow the
[one-time Docker setup](docs/docker.md). It creates one named container and keeps
your original project folder mounted with your logins in `magent-state`.

```sh
docker pull er0r2/multiharness
```

Pull downloads the image. The setup guide provides the one-time container creation
command with the folder and sandbox configuration. No host launcher is installed.

## 2. Start from any directory

```sh
docker start -ai multiharness
```

Every start reuses this container. `/quit` stops it without deleting your files
or settings. `/workspace` changes projects inside the shared folder; its selection
is remembered. Plain folders and multiple Git repositories are supported.

Use `/login codex` or `/login opencode` inside the container for the providers you
choose. An all-Codex team needs no OpenCode login.

## 3. Configure your agents

Type **`/config`** and follow the prompts to choose your planner and models.

- Choose **Codex** or **OpenCode** as the planner.
- Enter an OpenCode model as `provider/model` using a model your account can access.
- Press Enter to keep a value. Incorrect answers can be retried.
- Type `/cancel` during setup to discard your changes.

When setup finishes, type **`/save`** to remember your settings.
Use **`/settings`** to check them at any time.

Codex or OpenCode writes the code, and Codex reviews it. Tasks use your configured provider
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

For source development, `make build-dev` creates `dist/multiharness-dev` without
installing a host command. Docker releases use the canonical Dockerfile and
[one publishing workflow](.github/workflows/docker.yml).
