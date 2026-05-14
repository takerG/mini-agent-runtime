$ErrorActionPreference = "Stop"

$goFiles = git ls-files "*.go"
if ($LASTEXITCODE -ne 0) {
    throw "git ls-files failed"
}

$goFiles = @($goFiles | Where-Object { $_ -ne "" })
if ($goFiles.Count -gt 0) {
    & gofmt -w @goFiles
    if ($LASTEXITCODE -ne 0) {
        throw "gofmt failed"
    }
}

& (Join-Path $PSScriptRoot "normalize-crlf.ps1")
if ($LASTEXITCODE -ne 0) {
    throw "normalize-crlf failed"
}

