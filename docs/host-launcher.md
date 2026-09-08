# Magent Docker launcher

The native `magent` launcher runs on your computer. Agent tools run in Docker.
Your selected folder is mounted directly; no project copy or Compose editing is
needed. The launcher is separate from the native workflow CLI and from the image.
`docker pull` alone cannot install a host command.

## Windows

1. Install and start Docker Desktop in Linux-container mode.
2. Extract the Windows launcher ZIP. Open PowerShell in that folder.
3. Run `./magent.exe --install` once to install the command for your user account.
4. Open a new terminal and run `magent --config`.

Choose **Folder** and enter an existing full path such as `D:\QNE`. Choose
**Models** to set the planner, implementer and reviewer; Enter keeps a value.
Choose **Accounts** to sign in to the providers you use. No OpenCode login is
required for an all-Codex team. Choose **Start Magent**, or run `magent` later.

If you prefer no installation, use `./magent.exe --config` and `./magent.exe`
from the extracted folder. The binaries are preview builds, not code-signed.

## macOS and Linux

Extract the archive for your processor. Run `chmod +x magent` if necessary, then
`./magent --config` and `./magent`. Optionally place `magent` in a directory
already on your PATH to invoke it without `./`. macOS Docker Desktop verification
is still pending; these builds are not Apple signed or notarized.

On Linux with AppArmor, load the included scoped profile once before starting:
`sudo sh scripts/magent-apparmor.sh`. The launcher uses your UID/GID automatically
and never elevates privileges or changes global Docker restrictions.

## Changing folders

Exit the current session and run `magent --config`. Choose **Folder** and enter
any accessible existing full host path. The next start connects that folder;
there are no configuration files to edit. You can use the in-container
`/workspace` browser to navigate inside the already shared folder.

Host settings contain only the chosen folder and live in the OS user config
directory under `magent-launcher`. Account credentials and model settings remain
in the existing Docker volume `magent-state`. Changing folders does not delete
or reset that volume. The container never receives Docker socket access.

## Updates

Run `magent --update` to pull `er0r2/multiharness-core:preview`, then restart.
Download a new launcher ZIP when launcher updates are published. Keep
`magent-state` to preserve sign-in and team settings. Advanced users can select
a compatible image through `MAGENT_DOCKER_IMAGE`.

Files are edited directly. Actual Git repositories retain dirty-file protection;
plain folders do not require Git initialization. A foreign tool ID in `.git`
does not turn a plain folder into a repository. Damaged real Git metadata still
requires attention to avoid silently losing its protection.
