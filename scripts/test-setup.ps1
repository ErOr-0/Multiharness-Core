# Verify the Windows setup flow with fake Docker and prompt functions.
$ErrorActionPreference = 'Stop'
$Scratch = Join-Path ([IO.Path]::GetTempPath()) ('multiharness-setup-' + [guid]::NewGuid())
$global:SetupTestCalls = [Collections.Generic.List[string]]::new()
$global:SetupTestPromptCount = 0
$global:SetupTestFolder = Join-Path $Scratch "User's `$project with spaces"
function global:docker {
    $global:LASTEXITCODE = 0
    $global:SetupTestCalls.Add(($args -join '|'))
}
function global:Read-Host {
    $global:SetupTestPromptCount++
    return $global:SetupTestFolder
}
try {
    New-Item -ItemType Directory -Path (Join-Path $Scratch 'scripts'), $global:SetupTestFolder | Out-Null
    Copy-Item (Join-Path $PSScriptRoot 'setup.ps1') (Join-Path $Scratch 'scripts/setup.ps1')
    & (Join-Path $Scratch 'scripts/setup.ps1')
    $SettingsFile = Join-Path $Scratch '.env'
    $Saved = [IO.File]::ReadAllText($SettingsFile)
    $ExpectedPath = $global:SetupTestFolder.Replace('\', '/').Replace("'", "\'")
    if (!$Saved.Contains("MULTIHARNESS_WORKSPACE='$ExpectedPath'")) { throw 'Folder was not saved literally.' }
    & (Join-Path $Scratch 'scripts/setup.ps1')
    if ($global:SetupTestPromptCount -ne 1) { throw 'Existing settings prompted again.' }
    if ([IO.File]::ReadAllText($SettingsFile) -ne $Saved) { throw 'Existing settings changed.' }
    if (@($global:SetupTestCalls | Where-Object { $_ -eq 'start|-ai|multiharness' }).Count -ne 2) { throw 'Named container was not started.' }
    if (@($global:SetupTestCalls | Where-Object { $_ -match 'up\|--no-start$' }).Count -ne 2) { throw 'Compose creation did not run.' }
    Write-Host 'PASS: PowerShell first-time prompt and saved-folder reuse; Docker was mocked.'
} finally {
    Remove-Item -Recurse -Force $Scratch
    Remove-Item Function:\docker, Function:\Read-Host
}
