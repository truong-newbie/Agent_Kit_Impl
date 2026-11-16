param(
    [string]$Version = "0.1.0-dev",
    [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
$workspace = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$outPath = Join-Path $workspace $OutDir
New-Item -ItemType Directory -Force -Path $outPath | Out-Null

$commit = (git -C $workspace rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$ldflags = "-s -w -X dtkit.local/dtkit/internal/buildinfo.Version=$Version -X dtkit.local/dtkit/internal/buildinfo.Commit=$commit -X dtkit.local/dtkit/internal/buildinfo.Date=$date"

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Out = "dtkit_windows_amd64.exe" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Out = "dtkit_linux_amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Out = "dtkit_darwin_arm64" }
)

foreach ($t in $targets) {
    $dest = "/src/$OutDir/$($t.Out)"
    docker run --rm -e CGO_ENABLED=0 -e GOOS=$($t.GOOS) -e GOARCH=$($t.GOARCH) -v "${workspace}:/src" -w /src golang:1.27.4 /usr/local/go/bin/go build -trimpath -ldflags "$ldflags" -o $dest ./cmd/dtkit
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
Write-Output "Built binaries in $outPath"
