# One-time interactive Docker setup. Does not install a host command.
$ErrorActionPreference = 'Stop'
$SetupRoot = Split-Path -Parent $PSScriptRoot
function Invoke-Docker {
    & docker @args
    if ($LASTEXITCODE -ne 0) { throw 'Docker could not complete setup. Check the message above, then run setup again.' }
}
if (!(Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Install and open Docker Desktop, then run setup again.' }
Invoke-Docker info | Out-Null
Invoke-Docker compose version | Out-Null
$SettingsFile = Join-Path $SetupRoot '.env'
if (!(Test-Path -LiteralPath $SettingsFile)) {
    Write-Host 'First-time setup. Choose the host folder that contains your projects.'
    $WorkspaceFolder = Read-Host 'Full folder path (no surrounding quotes)'
    if (!(Test-Path -LiteralPath $WorkspaceFolder -PathType Container)) { throw 'That folder does not exist. Run setup again with an existing folder.' }
    $ResolvedFolder = Resolve-Path -LiteralPath $WorkspaceFolder
    if ($ResolvedFolder.Provider.Name -ne 'FileSystem') { throw 'Choose a folder on your computer.' }
    $WorkspaceFolder = $ResolvedFolder.ProviderPath.Replace('\', '/')
    if ($WorkspaceFolder.Contains("`n") -or $WorkspaceFolder.Contains("`r")) { throw 'Folder paths containing line breaks are unsupported.' }
    $WorkspaceFolder = $WorkspaceFolder.Replace("'", "\'")
    $Settings = "MULTIHARNESS_WORKSPACE='$WorkspaceFolder'`nMULTIHARNESS_UID=1000`nMULTIHARNESS_GID=1000`n"
    $TemporaryFile = Join-Path $SetupRoot ('.env.' + [guid]::NewGuid().ToString())
    try {
        [IO.File]::WriteAllText($TemporaryFile, $Settings, (New-Object Text.UTF8Encoding($false)))
        Move-Item -LiteralPath $TemporaryFile -Destination $SettingsFile
    } finally {
        if (Test-Path -LiteralPath $TemporaryFile) { Remove-Item -LiteralPath $TemporaryFile }
    }
    Write-Host 'Folder saved. You will choose a project and configure your team inside the app.'
} else {
    Write-Host 'Using your saved setup.'
}
# Do not interrupt an existing interactive session.
$Existing = & docker ps --filter 'name=^/multiharness$' --format '{{.Names}}'
if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect running containers.' }
if ($Existing -contains 'multiharness') {
    Write-Host 'Multiharness is already running. Reconnect with: docker attach multiharness'
    exit 0
}
$ComposeArgs = @('compose', '--env-file', $SettingsFile, '-f', (Join-Path $SetupRoot 'compose.yaml'))
Invoke-Docker @ComposeArgs pull
Invoke-Docker @ComposeArgs up --no-start
Write-Host 'Later, start from any folder with: docker start -ai multiharness'
Invoke-Docker start -ai multiharness
