# Installs harness-bridge from the latest GitHub release, then starts its setup.
#
#   irm https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main/install.ps1 | iex
#
# HARNESS_BRIDGE_VERSION     a release tag such as v0.1.0 (default: latest)
# HARNESS_BRIDGE_INSTALL_DIR where to put it (default: %LOCALAPPDATA%\Programs\harness-bridge)
# HARNESS_BRIDGE_NO_PATH     set to leave your PATH unchanged
# HARNESS_BRIDGE_NO_SETUP    set to skip starting setup afterwards

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $repo = 'ErOr-0/Multiharness-Core'
    $version = if ($env:HARNESS_BRIDGE_VERSION) { $env:HARNESS_BRIDGE_VERSION } else { 'latest' }
    $dest = if ($env:HARNESS_BRIDGE_INSTALL_DIR) { $env:HARNESS_BRIDGE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\harness-bridge' }

    $arch = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "harness-bridge: unsupported CPU $env:PROCESSOR_ARCHITECTURE" }
    }
    $base = if ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$version" }
    if ($env:HARNESS_BRIDGE_DOWNLOAD_BASE) { $base = $env:HARNESS_BRIDGE_DOWNLOAD_BASE }
    $asset = "harness-bridge_windows_$arch.exe"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('harness-bridge-' + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading harness-bridge for windows/$arch..."
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
        $entry = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { ($_ -split '\s+')[1].TrimStart('*') -eq $asset } | Select-Object -First 1
        if (-not $entry) { throw "harness-bridge: checksums.txt has no entry for $asset" }
        $expected = ($entry -split '\s+')[0].ToLowerInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLowerInvariant()
        if ($actual -ne $expected) { throw "harness-bridge: checksum mismatch for $asset; nothing was installed" }

        New-Item -ItemType Directory -Force -Path $dest | Out-Null
        $exe = Join-Path $dest 'harness-bridge.exe'
        try {
            Copy-Item -Force (Join-Path $tmp $asset) $exe
        } catch {
            throw "harness-bridge: can't replace $exe, probably because an agent is using it. Close your coding agents and run this again."
        }
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
    Write-Host "Installed $exe"

    if (-not $env:HARNESS_BRIDGE_NO_PATH) {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not (($userPath -split ';') -contains $dest)) {
            $newPath = if ($userPath) { "$userPath;$dest" } else { $dest }
            [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
            $env:Path = "$env:Path;$dest"
            Write-Host "Added $dest to your PATH."
        }
    }

    if ($env:HARNESS_BRIDGE_NO_SETUP) {
        Write-Host 'Run harness-bridge to set it up.'
    } else {
        & $exe setup
    }
}
