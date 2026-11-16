$ErrorActionPreference = "Stop"

$workspace = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

$formatOutput = docker run --rm -v "${workspace}:/src" -w /src golang:1.27.4 /usr/local/go/bin/gofmt -l cmd internal schemas
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
if ($formatOutput) {
    $formatOutput | Write-Output
    throw "Go source is not formatted"
}

docker run --rm -v "${workspace}:/src" -w /src golang:1.27.4 /usr/local/go/bin/go test -race ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

docker run --rm -v "${workspace}:/src" -w /src golang:1.27.4 /usr/local/go/bin/go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

docker run --rm -v "${workspace}:/src" -w /src golang:1.27.4 /usr/local/go/bin/go build -o /tmp/dtkit ./cmd/dtkit
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
