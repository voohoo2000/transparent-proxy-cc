# Transparent Proxy CC

Transparent Proxy CC is a Docker-first Linux transparent proxy gateway. It forwards HTTP and HTTPS TCP traffic through an upstream company HTTP/HTTPS proxy so browsers, apt, Docker, and terminal tools do not each need separate proxy settings.

## What it supports

- Linux local transparent mode.
- LAN gateway mode for other devices.
- Upstream unauthenticated HTTP/HTTPS explicit proxy.
- TCP HTTP on port 80.
- TCP HTTPS on port 443 through CONNECT tunneling.
- HTTPS SNI detection for domain-based CONNECT when available.
- Automatic iptables setup and cleanup.

## What it does not support yet

- DHCP.
- DNS proxying.
- UDP forwarding.
- QUIC/HTTP3.
- HTTPS decryption or certificate installation.
- Upstream proxy authentication.
- Web UI.

## Configure

Copy the example environment file:

```bash
cp .env.example .env
```

Edit `.env`:

```env
MODE=lan-gateway
UPSTREAM_PROXY_HOST=proxy.company.local
UPSTREAM_PROXY_PORT=8080
LISTEN_PORT=12345
LAN_INTERFACE=eth0
EXCLUDE_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8
LOG_LEVEL=info
```

## LAN gateway mode

Use this when other computers should use this Linux machine as the gateway.

Network path:

```text
client device -> transparent proxy machine -> company original gateway -> company proxy -> internet
```

Settings:

- Client devices: set default gateway to the transparent proxy machine IP.
- Transparent proxy machine: keep the company IP, netmask, DNS, and original gateway.
- `.env`: set `MODE=lan-gateway` and `LAN_INTERFACE` to the interface receiving client traffic.

Start:

```bash
docker compose up -d --build
```

## Local mode

Use this when only the Linux machine itself needs transparent proxying.

Settings:

- Keep the machine IP, netmask, DNS, and default gateway as required by the company network.
- `.env`: set `MODE=local`.

Start:

```bash
docker compose up -d --build
```

## Verify

Do not set `http_proxy` or `https_proxy` in the shell for these checks.

```bash
curl -v http://example.com
curl -v https://example.com
```

Then test browser access, apt updates, or Docker downloads according to your environment.

## Cleanup and recovery

To remove project-owned iptables rules:

```bash
docker compose run --rm transparent-proxy cleanup
```

or:

```bash
./scripts/cleanup-network.sh
```

If network access is still abnormal after cleanup:

1. Restart Docker or the network service.
2. Reboot only as the last fallback.

The cleanup script deletes only chains and jump rules owned by this project. It does not flush your system firewall.

## Logs

View logs:

```bash
docker compose logs -f transparent-proxy
```

Use `LOG_LEVEL=debug` for more connection detail. The proxy does not log HTTP bodies, HTTPS payloads, or page contents.

## HTTPS behavior

HTTPS traffic is not decrypted. The proxy reads the public TLS ClientHello when possible to find the target domain, asks the company proxy to CONNECT to that domain, and then forwards encrypted bytes in both directions.

If a client uses QUIC or HTTP/3 over UDP/443, that traffic is outside the first version. Browsers normally fall back to TCP HTTPS.
