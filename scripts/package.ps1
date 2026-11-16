param(
    [string]$Version = "0.1.0",
    [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
$workspace = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$distDir = Join-Path $workspace $OutDir
& (Join-Path $PSScriptRoot "build.ps1") -Version $Version -OutDir $OutDir

$stageRoot = Join-Path $distDir "stage"
if (Test-Path $stageRoot) { Remove-Item -Recurse -Force $stageRoot }
New-Item -ItemType Directory -Force -Path $stageRoot | Out-Null

$archives = @()

# Windows zip
$winStage = Join-Path $stageRoot "win"
New-Item -ItemType Directory -Force -Path $winStage | Out-Null
Copy-Item (Join-Path $distDir "dtkit_windows_amd64.exe") (Join-Path $winStage "dtkit.exe")
Copy-Item (Join-Path $workspace "README.md") (Join-Path $winStage "README.md")
$winZip = Join-Path $distDir "dtkit_${Version}_windows_amd64.zip"
if (Test-Path $winZip) { Remove-Item -Force $winZip }
Compress-Archive -Path (Join-Path $winStage "*") -DestinationPath $winZip
$archives += $winZip

# Linux / macOS tar.gz (via tar in container or Windows tar)
foreach ($pair in @(
    @{ Bin = "dtkit_linux_amd64";  Archive = "dtkit_${Version}_linux_amd64.tar.gz" },
    @{ Bin = "dtkit_darwin_arm64"; Archive = "dtkit_${Version}_darwin_arm64.tar.gz" }
)) {
    $stage = Join-Path $stageRoot $pair.Bin
    New-Item -ItemType Directory -Force -Path $stage | Out-Null
    Copy-Item (Join-Path $distDir $pair.Bin) (Join-Path $stage "dtkit")
    Copy-Item (Join-Path $workspace "README.md") (Join-Path $stage "README.md")
    $archPath = Join-Path $distDir $pair.Archive
    if (Test-Path $archPath) { Remove-Item -Force $archPath }
    docker run --rm -v "${workspace}:/src" -w "/src/$OutDir/stage/$($pair.Bin)" golang:1.27.4 tar -czf "/src/$OutDir/$($pair.Archive)" dtkit README.md
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $archives += $archPath
}

$sumsPath = Join-Path $distDir "SHA256SUMS"
$lines = foreach ($a in $archives) {
    $h = (Get-FileHash -Path $a -Algorithm SHA256).Hash.ToLowerInvariant()
    "$h  $(Split-Path -Leaf $a)"
}
$lines | Set-Content -Path $sumsPath -Encoding ascii
Remove-Item -Recurse -Force $stageRoot
Write-Output "Created release archives and $sumsPath"
