$ErrorActionPreference = "Stop"

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

$encoding = [System.Text.UTF8Encoding]::new($false)
foreach ($file in $targets) {
    $text = [System.IO.File]::ReadAllText($file.FullName)
    $normalized = $text -replace "`r?`n", "`r`n"
    [System.IO.File]::WriteAllText($file.FullName, $normalized, $encoding)
}

"CRLF normalized"
