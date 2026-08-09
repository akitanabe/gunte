#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${1:-${repository_root}/dist}"
mkdir -p "${output_dir}"
output_dir="$(cd "${output_dir}" && pwd)"

targets=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
  "windows amd64"
  "windows arm64"
)

cd "${repository_root}"
for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  binary="gunte-${goos}-${goarch}"
  if [[ "${goos}" == "windows" ]]; then
    binary="${binary}.exe"
  fi

  echo "building ${binary}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -buildvcs=false -trimpath -ldflags="-s -w" \
    -o "${output_dir}/${binary}" ./cmd/gunte
done
