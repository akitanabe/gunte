#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${1:-${repository_root}/dist}"
mkdir -p "${output_dir}"
output_dir="$(cd "${output_dir}" && pwd)"

staging_dir="$(mktemp -d)"
trap 'rm -rf -- "${staging_dir}"' EXIT
binary_dir="${staging_dir}/binaries"
package_dir="${staging_dir}/packages"

"${repository_root}/scripts/build-release.sh" "${binary_dir}"
cd "${repository_root}"
go run ./internal/cmd/package-release "${binary_dir}" "${package_dir}"

for artifact in "${package_dir}"/*; do
  cp "${artifact}" "${output_dir}/"
done
