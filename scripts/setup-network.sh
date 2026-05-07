#!/bin/sh
set -eu

MODE="${MODE:-lan-gateway}"
LISTEN_PORT="${LISTEN_PORT:-12345}"
LAN_INTERFACE="${LAN_INTERFACE:-eth0}"
EXCLUDE_CIDRS="${EXCLUDE_CIDRS:-10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8}"
UPSTREAM_PROXY_HOST="${UPSTREAM_PROXY_HOST:?UPSTREAM_PROXY_HOST is required}"
UPSTREAM_PROXY_PORT="${UPSTREAM_PROXY_PORT:?UPSTREAM_PROXY_PORT is required}"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
"${SCRIPT_DIR}/cleanup-network.sh"

sysctl -w net.ipv4.ip_forward=1 >/dev/null

UPSTREAM_IP=$(getent ahostsv4 "$UPSTREAM_PROXY_HOST" | awk 'NR == 1 { print $1 }')
if [ -z "$UPSTREAM_IP" ]; then
  echo "could not resolve UPSTREAM_PROXY_HOST=${UPSTREAM_PROXY_HOST}" >&2
  exit 1
fi

iptables -t nat -N TPROXY_CC_PREROUTING
iptables -t nat -N TPROXY_CC_OUTPUT

for cidr in $(echo "$EXCLUDE_CIDRS" | tr ',' ' '); do
  iptables -t nat -A TPROXY_CC_PREROUTING -d "$cidr" -j RETURN
  iptables -t nat -A TPROXY_CC_OUTPUT -d "$cidr" -j RETURN
done

iptables -t nat -A TPROXY_CC_PREROUTING -d "$UPSTREAM_IP" -p tcp --dport "$UPSTREAM_PROXY_PORT" -j RETURN
iptables -t nat -A TPROXY_CC_OUTPUT -d "$UPSTREAM_IP" -p tcp --dport "$UPSTREAM_PROXY_PORT" -j RETURN

iptables -t nat -A TPROXY_CC_PREROUTING -p tcp -m multiport --dports 80,443 -j REDIRECT --to-ports "$LISTEN_PORT"
iptables -t nat -A TPROXY_CC_OUTPUT -p tcp -m multiport --dports 80,443 -j REDIRECT --to-ports "$LISTEN_PORT"

case "$MODE" in
  lan-gateway)
    iptables -t nat -A PREROUTING -i "$LAN_INTERFACE" -j TPROXY_CC_PREROUTING
    ;;
  local)
    iptables -t nat -A OUTPUT -j TPROXY_CC_OUTPUT
    ;;
  *)
    echo "invalid MODE=${MODE}" >&2
    "${SCRIPT_DIR}/cleanup-network.sh"
    exit 1
    ;;
esac

echo "transparent-proxy network rules installed mode=${MODE} listen=${LISTEN_PORT} upstream=${UPSTREAM_IP}:${UPSTREAM_PROXY_PORT}"
