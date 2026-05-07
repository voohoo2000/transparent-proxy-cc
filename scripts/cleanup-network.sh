#!/bin/sh
set -eu

LISTEN_PORT="${LISTEN_PORT:-12345}"

remove_jump() {
  table="$1"
  chain="$2"
  target="$3"
  while iptables -t "$table" -C "$chain" -j "$target" 2>/dev/null; do
    iptables -t "$table" -D "$chain" -j "$target" || true
  done
}

remove_chain() {
  table="$1"
  chain="$2"
  if iptables -t "$table" -L "$chain" >/dev/null 2>&1; then
    iptables -t "$table" -F "$chain" || true
    iptables -t "$table" -X "$chain" || true
  fi
}

remove_jump nat PREROUTING TPROXY_CC_PREROUTING
remove_jump nat OUTPUT TPROXY_CC_OUTPUT
remove_chain nat TPROXY_CC_PREROUTING
remove_chain nat TPROXY_CC_OUTPUT

echo "transparent-proxy network rules cleaned for listen port ${LISTEN_PORT}"
