# Windows PowerShell 5.1+ and PowerShell 7. Run from your project folder.
[CmdletBinding()]
param(
  [string]$Project = (Get-Location).Path,
  [string]$Image = 'er0r2/multiharness-core:preview',
  [string]$StateVolume = 'magent-state',
  [switch]$NoTty,
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$Command = @()
)
$ErrorActionPreference = 'Stop'
$seccomp = Join-Path $PSScriptRoot '../docker/seccomp.json'
if (-not (Test-Path -LiteralPath $seccomp -PathType Leaf)) { throw 'Missing docker/seccomp.json beside the launcher package.' }
$seccomp = (Resolve-Path -LiteralPath $seccomp).ProviderPath
$projectPath = (Resolve-Path -LiteralPath $Project).ProviderPath
if (-not (Test-Path -LiteralPath $projectPath -PathType Container)) {
  throw 'Project must be a directory.'
}
if ($projectPath.IndexOfAny([char[]]',"') -ge 0) {
  throw 'Docker project paths containing commas or quotes are unsupported by this launcher.'
}
if ([string]::IsNullOrWhiteSpace($Image) -or $Image.StartsWith('-')) { throw 'Invalid image name.' }
if ($StateVolume -notmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]*$') { throw 'Invalid Docker volume name.' }
$dockerArgs = @(
  'run', '--rm', '--init', '--interactive',
  '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges=true', '--security-opt', "seccomp=$seccomp",
  '--mount', "type=bind,src=$projectPath,dst=/workspace",
  '--mount', "type=volume,src=$StateVolume,dst=/state"
)
$security = & docker info --format '{{json .SecurityOptions}}'
if ($LASTEXITCODE -ne 0) { throw 'Cannot connect to Docker. Start Docker Desktop or Engine first.' }
if ($security -match 'name=apparmor') {
  $dockerArgs += @('--security-opt', 'apparmor=magent-container-v1')
}
if (-not $NoTty -and -not [Console]::IsInputRedirected -and -not [Console]::IsOutputRedirected) {
  $dockerArgs += '--tty'
}
$dockerArgs += $Image
$dockerArgs += $Command
& docker @dockerArgs
exit $LASTEXITCODE
