#!/usr/bin/env bash
# Regenerate after changing assets/windows/netmerge.ico. No runtime dependency.
set -euo pipefail
cd "$(dirname "$0")/.."
for arch in amd64 arm64 386; do
  go run github.com/akavel/rsrc@v0.10.2 -arch "$arch" \
    -ico assets/windows/netmerge.ico -o "netmerge_windows_${arch}.syso"
done
