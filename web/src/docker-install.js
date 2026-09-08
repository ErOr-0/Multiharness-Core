// Commands stay declarative: Docker fetches Compose and its policy files.
export const composeSource =
  "https://github.com/ErOr-0/Multiharness-Core.git#main";
export const policySource =
  "https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/4c3fe37e9346e1d0bcd2511601f3f5bfc2efdeb7/docker/apparmor.profile";

export function folderError(folder, platform) {
  if (!folder) return "Enter the full path to your projects folder.";
  if (/[\x00-\x1f\x7f]/.test(folder))
    return "Use a folder path without line breaks or control characters.";
  if (
    platform === "Windows"
      ? !/^[A-Za-z]:[\\/]/.test(folder)
      : !folder.startsWith("/")
  )
    return platform === "Windows"
      ? "Use a full path such as C:\\Users\\Sam\\Projects."
      : "Use a full path starting with /, such as /Users/sam/Projects.";
  return "";
}

function quote(value, windows) {
  return "'" + value.replaceAll("'", windows ? "''" : "'\\''") + "'";
}

export function launchCommand(folder, platform, appArmor = false) {
  if (folderError(folder, platform)) return "";
  const windows = platform === "Windows";
  const environment = windows
    ? `$env:MULTIHARNESS_WORKSPACE = ${quote(folder, true)}\n`
    : `MULTIHARNESS_WORKSPACE=${quote(folder, false)} `;
  const user =
    platform === "Linux"
      ? 'MULTIHARNESS_UID="$(id -u)" MULTIHARNESS_GID="$(id -g)" '
      : "";
  const policy =
    platform === "Linux" && appArmor
      ? ` -f '${composeSource}:docker/compose.linux.yaml'`
      : "";
  // A failed create must not start a pre-existing container with another folder.
  const start = windows
    ? "\nif ($LASTEXITCODE -eq 0) { docker start -ai multiharness }"
    : " &&\ndocker start -ai multiharness";
  return `${environment}${user}docker compose -f '${composeSource}'${policy} create${start}`;
}

export const linuxPolicyCommand =
  `curl --fail --location '${policySource}' --output "$HOME/.multiharness-apparmor.profile" &&\n` +
  'sudo apparmor_parser --replace --skip-read-cache "$HOME/.multiharness-apparmor.profile" &&\n' +
  'sudo install -m 644 "$HOME/.multiharness-apparmor.profile" /etc/apparmor.d/magent-container-v1';
