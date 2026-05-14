GO ?= go
PACKAGES ?= ./...

.PHONY: fmt fmt-check vet test staticcheck golangci-lint lint check

fmt:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/format.ps1

fmt-check:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check.ps1 -OnlyFmtCheck

vet:
	powershell -NoProfile -ExecutionPolicy Bypass -Command "if (-not $$env:GOCACHE) { $$env:GOCACHE = Join-Path (Get-Location) '.gocache' }; $(GO) vet $(PACKAGES)"

test:
	powershell -NoProfile -ExecutionPolicy Bypass -Command "if (-not $$env:GOCACHE) { $$env:GOCACHE = Join-Path (Get-Location) '.gocache' }; $(GO) test -count=1 $(PACKAGES)"

staticcheck:
	powershell -NoProfile -ExecutionPolicy Bypass -Command "if (-not $$env:STATICCHECK_CACHE) { $$env:STATICCHECK_CACHE = Join-Path (Get-Location) '.gocache\staticcheck' }; $$tool = if (Test-Path '.tools\bin\staticcheck.exe') { '.tools\bin\staticcheck.exe' } else { 'staticcheck' }; & $$tool $(PACKAGES)"

golangci-lint:
	powershell -NoProfile -ExecutionPolicy Bypass -Command "if (-not $$env:GOLANGCI_LINT_CACHE) { $$env:GOLANGCI_LINT_CACHE = Join-Path (Get-Location) '.gocache\golangci-lint' }; $$tool = if (Test-Path '.tools\bin\golangci-lint.exe') { '.tools\bin\golangci-lint.exe' } else { 'golangci-lint' }; & $$tool run"

lint: fmt-check vet staticcheck golangci-lint

check: lint test
