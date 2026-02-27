#!/usr/bin/env bash

set -euo pipefail

URL=$1
TOOL=$2
VERSION=$3

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="${SCRIPT_DIR}/../bin"

mkdir -p "${BIN_DIR}"
cd "${BIN_DIR}"

# Extract tool - handle both root-level and bin/ subdirectory layouts
curl -sL "${URL}" | tar -xz --strip-components=0
if [ -f "bin/${TOOL}" ]; then
    mv "bin/${TOOL}" "${TOOL}"
    rmdir bin 2>/dev/null || true
fi

echo "Downloaded ${TOOL} ${VERSION} to ${BIN_DIR}"
