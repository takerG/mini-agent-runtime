param(
    [switch]$OnlyFmtCheck
)

$ErrorActionPreference = "Stop"

function Resolve-CommandPath {
    param(
        [string]$Name,
        [string]$InstallHint
    )

    $localBin = Join-Path (Get-Location) ".tools\bin"
    foreach ($candidateName in @("$Name.exe", $Name)) {
        $candidate = Join-Path $localBin $candidateName
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    throw "$Name not found. Install: $InstallHint"
}

function Get-TrackedGoFiles {
    $files = git ls-files "*.go"
    if ($LASTEXITCODE -ne 0) {
        throw "git ls-files failed"
    }
    return @($files | Where-Object { $_ -ne "" })
}

function Normalize-LineEndingText {
    param([string]$Text)

    return (($Text -replace "`r`n", "`n") -replace "`r", "`n")
}

function Invoke-GoFmtOutput {
    param([string]$Path)

    $processInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $processInfo.FileName = "gofmt"
    $escapedPath = $Path.Replace('"', '\"')
    $processInfo.Arguments = '"' + $escapedPath + '"'
    $processInfo.RedirectStandardOutput = $true
    $processInfo.RedirectStandardError = $true
    $processInfo.UseShellExecute = $false

    $process = [System.Diagnostics.Process]::Start($processInfo)
    $stdout = $process.StandardOutput.ReadToEnd()
    $stderr = $process.StandardError.ReadToEnd()
    $process.WaitForExit()

    if ($process.ExitCode -ne 0) {
        throw "gofmt failed for ${Path}: $stderr"
    }
    return $stdout
}

function Assert-GoFmt {
    $goFiles = Get-TrackedGoFiles
    if ($goFiles.Count -eq 0) {
        return
    }

    $unformatted = foreach ($file in $goFiles) {
        $fullPath = (Resolve-Path -LiteralPath $file).Path
        $current = [System.IO.File]::ReadAllText($fullPath)
        $formatted = Invoke-GoFmtOutput $file
        if ((Normalize-LineEndingText $current) -ne (Normalize-LineEndingText $formatted)) {
            $file
        }
    }

    if ($unformatted) {
        $message = @(
            "gofmt check failed. Unformatted files:",
            ($unformatted -join [Environment]::NewLine),
            "Run: powershell -NoProfile -ExecutionPolicy Bypass -File scripts/format.ps1"
        ) -join [Environment]::NewLine
        throw $message
    }
}

function Assert-CRLF {
    $patterns = @("*.go", "*.md", "*.mod", ".gitignore", ".gitattributes", ".editorconfig", "*.yaml", "*.yml", "Makefile", "*.sh", "*.ps1")
    $targets = Get-ChildItem -Recurse -File -Force |
        Where-Object {
            $path = $_.FullName
            if ($path -match "\\.git\\|\\.gocache\\|\\.idea\\") {
                return $false
            }
            foreach ($pattern in $patterns) {
                if ($_.Name -like $pattern) {
                    return $true
                }
            }
            return $false
        }

    $bad = foreach ($file in $targets) {
        $bytes = [System.IO.File]::ReadAllBytes($file.FullName)
        for ($i = 0; $i -lt $bytes.Length; $i++) {
            if ($bytes[$i] -eq 10 -and ($i -eq 0 -or $bytes[$i - 1] -ne 13)) {
                $file.FullName
                break
            }
        }
    }

    if ($bad) {
        $message = @(
            "CRLF check failed. Files contain bare LF:",
            ($bad -join [Environment]::NewLine),
            "Run: powershell -NoProfile -ExecutionPolicy Bypass -File scripts/normalize-crlf.ps1"
        ) -join [Environment]::NewLine
        throw $message
    }
}

function Invoke-GoCommand {
    param([string[]]$Command)

    $commandName = $Command[0]
    $commandArgs = @()
    if ($Command.Count -gt 1) {
        $commandArgs = $Command[1..($Command.Count - 1)]
    }

    & $commandName @commandArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed: $($Command -join ' ')"
    }
}

if (-not $env:GOCACHE) {
    $env:GOCACHE = Join-Path (Get-Location) ".gocache"
}
if (-not $env:STATICCHECK_CACHE) {
    $env:STATICCHECK_CACHE = Join-Path (Get-Location) ".gocache\staticcheck"
}
if (-not $env:GOLANGCI_LINT_CACHE) {
    $env:GOLANGCI_LINT_CACHE = Join-Path (Get-Location) ".gocache\golangci-lint"
}

Assert-GoFmt
Assert-CRLF

if ($OnlyFmtCheck) {
    exit 0
}

Invoke-GoCommand @("go", "vet", "./...")
Invoke-GoCommand @("go", "test", "-count=1", "./...")

$staticcheck = Resolve-CommandPath "staticcheck" "go install honnef.co/go/tools/cmd/staticcheck@latest"
Invoke-GoCommand @($staticcheck, "./...")

$golangciLint = Resolve-CommandPath "golangci-lint" "https://golangci-lint.run/welcome/install/"
Invoke-GoCommand @($golangciLint, "run")
