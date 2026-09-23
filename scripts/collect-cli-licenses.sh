#!/bin/sh
set -eu

license_dir=$1
tool_dir=$(mktemp -d)
trap 'rm -rf "$tool_dir"' EXIT

# The collector runs on the build host; dependency analysis uses the target OS and architecture.
GOOS=$(go env GOHOSTOS) GOARCH=$(go env GOHOSTARCH) GOBIN="$tool_dir" \
    go install github.com/google/go-licenses/v2@v2.0.1
"$tool_dir/go-licenses" save ./cmd/opeco --ignore=opeco.link --save_path="$license_dir"

# go-licenses excludes the Go standard library.
go_root=$(go env GOROOT)
mkdir -p "$license_dir/go"
cp "$go_root/LICENSE" "$go_root/PATENTS" "$license_dir/go/"
