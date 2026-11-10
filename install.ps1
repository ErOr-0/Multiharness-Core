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
    # Windows PowerShell 5.1 on older systems doesn't offer TLS 1.2 by default.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $repo = 'ErOr-0/Multiharness-Core'

    # Windows PowerShell 5.1 reports a missing file (404) as "the connection was
    # closed unexpectedly", so say what was being downloaded and what to check.
    function Get-Download($url, $out) {
        for ($attempt = 1; ; $attempt++) {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $out
                return
            } catch {
                if ($attempt -ge 3) {
                    throw "harness-bridge: couldn't download $url ($($_.Exception.Message)). Check that https://github.com/$repo/releases has a release and that you're online, then run this again."
                }
                Start-Sleep -Seconds (2 * $attempt)
            }
        }
    }
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
        Get-Download "$base/$asset" (Join-Path $tmp $asset)
        Get-Download "$base/checksums.txt" (Join-Path $tmp 'checksums.txt')
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
        # Read and write the raw registry value so entries like
        # %USERPROFILE%\... stay unexpanded; [Environment] would freeze them.
        $userPath = (Get-Item HKCU:\Environment).GetValue('Path', '', 'DoNotExpandEnvironmentNames')
        $present = ($userPath -split ';') | Where-Object { $_ -and $_.TrimEnd('\') -ieq $dest.TrimEnd('\') }
        if (-not $present) {
            $newPath = if ($userPath) { "$userPath;$dest" } else { $dest }
            Set-ItemProperty -Path HKCU:\Environment -Name Path -Value $newPath -Type ExpandString
            # Tell running programs (mainly Explorer) about the change so new
            # processes see it; a failed broadcast is harmless.
            try {
                if (-not ('Win32.Settings' -as [type])) {
                    Add-Type -Namespace Win32 -Name Settings -MemberDefinition '[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);'
                }
                $result = [UIntPtr]::Zero
                [Win32.Settings]::SendMessageTimeout([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result) | Out-Null
            } catch { }
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
