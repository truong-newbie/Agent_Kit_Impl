param(
    [string]$Version = "0.4.0-rc.1",
    [string]$BinDir = "",
    [string]$BaseUrl = "",
    [switch]$DryRun,
    [switch]$SkipBinaryExecCheck
)

$ErrorActionPreference = "Stop"

$semverPattern = '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if ($Version -notmatch $semverPattern) {
    throw "Invalid semver version '$Version'."
}

$archOverride = $env:DTKIT_TEST_ARCH
$osArch = if ($archOverride) {
    $archOverride
} elseif (-not [string]::IsNullOrWhiteSpace($env:PROCESSOR_ARCHITECTURE)) {
    $env:PROCESSOR_ARCHITECTURE
} else {
    [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
}
if ($osArch -notin @("X64", "amd64", "AMD64")) {
    throw "Unsupported Windows architecture '$osArch'. Prebuilt Windows archives only target amd64 (X64); build from source with Go 1.27.4+ instead."
}

if ([string]::IsNullOrWhiteSpace($BinDir)) {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw "LOCALAPPDATA is unset; pass -BinDir explicitly."
    }
    $BinDir = Join-Path $env:LOCALAPPDATA "Programs\dtkit\bin"
}
$resolvedBinDir = [System.IO.Path]::GetFullPath($BinDir)
$rootDir = [System.IO.Path]::GetPathRoot($resolvedBinDir)
if ($resolvedBinDir.TrimEnd('\') -eq $rootDir.TrimEnd('\')) {
    throw "Refusing drive root as BinDir: '$resolvedBinDir'."
}

$Archive = "dtkit_${Version}_windows_amd64.zip"
if ([string]::IsNullOrWhiteSpace($BaseUrl)) {
    $BaseUrl = "https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v$Version"
}
$BaseUrl = $BaseUrl.TrimEnd('/')

if ($DryRun) {
    Write-Output "dry-run: would verify and install $Archive from $BaseUrl into $(Join-Path $resolvedBinDir 'dtkit.exe')"
    exit 0
}

$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("dtkit-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

try {
    $archivePath = Join-Path $tmpDir $Archive
    $sumsPath = Join-Path $tmpDir "SHA256SUMS"
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/$Archive" -OutFile $archivePath -ErrorAction Stop
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/SHA256SUMS" -OutFile $sumsPath -ErrorAction Stop

    $escapedArchive = [regex]::Escape($Archive)
    $rawSums = Get-Content -Path $sumsPath -ErrorAction SilentlyContinue
    $matchingLines = @($rawSums | Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+(\./)?$escapedArchive$" })
    if ($matchingLines.Count -ne 1) {
        throw "Expected exactly 1 SHA256SUMS entry for $Archive, found $($matchingLines.Count); aborting."
    }
    $expectedHash = ($matchingLines[0] -split "\s+")[0].ToLowerInvariant()
    $actualHash = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "SHA256 checksum mismatch for ${Archive}: expected $expectedHash, got $actualHash; aborting before extraction."
    }

    $extractDir = Join-Path $tmpDir "extracted"
    New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
    Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force -ErrorAction Stop

    $extractedExe = Join-Path $extractDir "dtkit.exe"
    if (-not (Test-Path -LiteralPath $extractedExe)) {
        throw "Archive $Archive did not contain expected dtkit.exe."
    }

    if (-not $SkipBinaryExecCheck) {
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $extractedExe
        $psi.Arguments = "version --json"
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $proc = [System.Diagnostics.Process]::Start($psi)
        $verStdOut = $proc.StandardOutput.ReadToEnd()
        $verStdErr = $proc.StandardError.ReadToEnd()
        $proc.WaitForExit()
        $verCombined = ($verStdOut + "`n" + $verStdErr).Trim()
        if ($proc.ExitCode -ne 0 -or -not ($verCombined -match '"version"\s*:\s*"' + [regex]::Escape($Version) + '"')) {
            throw "Extracted dtkit.exe failed version verification (exit=$($proc.ExitCode)): $verCombined"
        }
    }

    New-Item -ItemType Directory -Force -Path $resolvedBinDir | Out-Null
    $destExe = Join-Path $resolvedBinDir "dtkit.exe"
    $stageExe = Join-Path $resolvedBinDir ("dtkit.exe.tmp." + [System.Guid]::NewGuid().ToString("N"))
    Copy-Item -LiteralPath $extractedExe -Destination $stageExe -Force -ErrorAction Stop

    $backupExe = $null
    try {
        if (Test-Path -LiteralPath $destExe) {
            $backupExe = Join-Path $resolvedBinDir ("dtkit.exe.bak." + [System.Guid]::NewGuid().ToString("N"))
            # Move existing binary aside first; if locked by an active process, this fails before replacing it.
            Move-Item -LiteralPath $destExe -Destination $backupExe -Force -ErrorAction Stop
        }
        Move-Item -LiteralPath $stageExe -Destination $destExe -Force -ErrorAction Stop
        if ($backupExe -and (Test-Path -LiteralPath $backupExe)) {
            Remove-Item -LiteralPath $backupExe -Force -ErrorAction SilentlyContinue
        }
    } catch {
        if (Test-Path -LiteralPath $stageExe) {
            Remove-Item -LiteralPath $stageExe -Force -ErrorAction SilentlyContinue
        }
        if ($backupExe -and (Test-Path -LiteralPath $backupExe) -and -not (Test-Path -LiteralPath $destExe)) {
            Move-Item -LiteralPath $backupExe -Destination $destExe -Force -ErrorAction SilentlyContinue
        }
        throw "Failed to replace '$destExe' (existing binary preserved): $($_.Exception.Message)"
    }

    Write-Output "Installed $Archive -> $destExe"
} finally {
    if (Test-Path -LiteralPath $tmpDir) {
        Remove-Item -Recurse -Force -Path $tmpDir -ErrorAction SilentlyContinue
    }
}
