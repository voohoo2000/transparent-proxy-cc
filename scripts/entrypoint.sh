#!/bin/sh
set -eu

SCRIPT_DIR=/app/scripts

cleanup() {
  "${SCRIPT_DIR}/cleanup-network.sh" || true
}

case "${1:-serve}" in
  serve)
    "${SCRIPT_DIR}/setup-network.sh"
    trap cleanup INT TERM EXIT
    shift || true
    exec /app/transparent-proxy serve
    ;;
  cleanup)
    cleanup
    ;;
  *)
    exec "$@"
    ;;
esac
