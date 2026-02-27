#!/bin/bash
set -e

# Validate required environment variables
if [ -z "${KCP_WORKSPACE_URL}" ]; then
    echo "ERROR: KCP_WORKSPACE_URL environment variable is not set" >&2
    exit 1
fi

# Start ttyd on port 8080
# - Runs setup.sh which will read token from first WebSocket message
# - setup.sh creates kubeconfig and starts interactive bash
exec ttyd \
    --port 8080 \
    --writable \
    --max-clients 1 \
    /usr/local/bin/setup.sh
