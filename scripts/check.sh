#!/usr/bin/env sh
set -eu

GO="${GO:-go}"
PACKAGES="${PACKAGES:-./...}"
GOCACHE="${GOCACHE:-$(pwd)/.gocache}"
export GOCACHE

require_command() {
	name="$1"
	hint="$2"
	if ! command -v "$name" >/dev/null 2>&1; then
		printf '%s not found. Install: %s\n' "$name" "$hint" >&2
		exit 1
	fi
}

go_files="$(git ls-files '*.go')"
if [ -n "$go_files" ]; then
	unformatted="$(gofmt -l $go_files)"
	if [ -n "$unformatted" ]; then
		printf 'gofmt check failed. Unformatted files:\n%s\n' "$unformatted" >&2
		printf 'Run: gofmt -w <files>\n' >&2
		exit 1
	fi
fi

"$GO" vet "$PACKAGES"
"$GO" test -count=1 "$PACKAGES"

require_command "staticcheck" "go install honnef.co/go/tools/cmd/staticcheck@latest"
staticcheck "$PACKAGES"

require_command "golangci-lint" "https://golangci-lint.run/welcome/install/"
golangci-lint run
