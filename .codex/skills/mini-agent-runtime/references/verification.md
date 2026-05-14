# Verification Reference

本项目默认使用 PowerShell。验证命令应尽量可跨机器运行，不写死本机绝对路径。

## Go 验证

代码变更后运行：

```powershell
$env:GOCACHE = Join-Path (Get-Location) ".gocache"
go test -count=1 ./...
go build -buildvcs=false ./...
```

## 文档与换行验证

文档、工程文件、换行策略相关变更后运行：

```powershell
git diff --check
```

检查目标文件是否存在裸 LF：

```powershell
$targets = Get-ChildItem -Recurse -File -Include *.go,*.md,*.mod,.gitignore,.gitattributes,.editorconfig |
  Where-Object { $_.FullName -notmatch '\\.git\\|\\.gocache\\|\\.idea\\' }

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
  $bad
  exit 1
}

"CRLF check passed"
```

## GoDoc 注释检查

涉及新增、删除、重命名或大规模调整函数时，检查所有函数和方法是否有注释，且注释以函数名或方法名开头。

```powershell
go test -count=1 ./...
```

当前测试中应覆盖关键结构的行为；如果以后注释检查没有测试覆盖，可以补一个轻量脚本或测试。
