param(
    [string]$Version = "0.4.0-dev",
    [string]$OutDir = "dist",
    [switch]$RequireClean
)

$ErrorActionPreference = "Stop"
$workspace = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$validated = & (Join-Path $PSScriptRoot "validate-release-args.ps1") -Version $Version -OutDir $OutDir -Workspace $workspace
$outPath = $validated.OutPath
New-Item -ItemType Directory -Force -Path $outPath | Out-Null

$dirtyStatus = git -C $workspace status --porcelain 2>$null
if ($RequireClean -and -not [string]::IsNullOrWhiteSpace(($dirtyStatus -join ""))) {
    throw "Working tree is dirty; refusing clean release build under -RequireClean."
}

$commit = (git -C $workspace rev-parse HEAD 2>$null)
$shortCommit = (git -C $workspace rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
if (-not $shortCommit) { $shortCommit = "unknown" }
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$ldflags = "-s -w -X github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo.Version=$Version -X github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo.Commit=$shortCommit -X github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo.Date=$date"

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Out = "dtkit_windows_amd64.exe" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Out = "dtkit_linux_amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Out = "dtkit_darwin_arm64" }
)

foreach ($t in $targets) {
    $dest = "/src/$($validated.RelOutPosix)/$($t.Out)"
    docker run --rm -e CGO_ENABLED=0 -e GOOS=$($t.GOOS) -e GOARCH=$($t.GOARCH) -v "${workspace}:/src" -w /src golang:1.27.4 /usr/local/go/bin/go build -trimpath -ldflags "$ldflags" -o $dest ./cmd/dtkit
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

$versionMeta = @(
    "version=$Version",
    "commit=$commit",
    "shortCommit=$shortCommit",
    "date=$date",
    "dirty=$([bool](-not [string]::IsNullOrWhiteSpace(($dirtyStatus -join ''))))"
) -join "`n"
Set-Content -Path (Join-Path $outPath "VERSION") -Value ($versionMeta + "`n") -Encoding ascii -NoNewline

Write-Output "Built binaries and VERSION provenance in $outPath"
