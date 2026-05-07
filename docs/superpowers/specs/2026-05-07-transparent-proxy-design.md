# Transparent Proxy Gateway Design

## Goal

Build a Docker-first transparent proxy gateway in Go. Users should only need normal network settings plus a gateway choice, without configuring `http_proxy` or `https_proxy` separately for browsers, apt, Docker, or terminal tools.

The first version supports:

- Linux local transparent mode.
- LAN gateway mode for other devices on the same network.
- Upstream unauthenticated HTTP/HTTPS explicit proxy.
- Transparent TCP HTTP and HTTPS traffic on ports 80 and 443.
- HTTPS tunneling without decryption or certificate installation.
- Automatic network rule setup and cleanup.

The first version does not support DHCP, DNS proxying, UDP forwarding, QUIC/HTTP3, HTTPS MITM, authentication to the upstream proxy, or a management UI.

## Architecture

The system has three layers:

1. **Startup and configuration layer**
   - Reads configuration from environment variables.
   - Validates mode, upstream proxy host and port, listen port, LAN interface, and excluded CIDRs.
   - Initializes Linux forwarding and iptables rules.
   - Fails fast with clear logs when required privileges or configuration are missing.

2. **Transparent proxy layer**
   - Listens on an internal redirected port, for example `:12345`.
   - Accepts redirected TCP connections.
   - Reads the original destination address from the socket.
   - Dispatches HTTP and HTTPS flows based on the original destination port.

3. **Upstream proxy connection layer**
   - For HTTP destinations, forwards requests through the upstream HTTP proxy in explicit proxy format.
   - For HTTPS destinations, connects to the upstream proxy with `CONNECT host:443` and then performs raw bidirectional byte forwarding.
   - Does not decrypt TLS or inspect request bodies.

## Runtime Modes

### LAN gateway mode

Other computers use the transparent proxy machine as their default gateway.

The network path is:

```text
Client device
  -> transparent proxy machine as default gateway
  -> company original gateway
  -> company upstream HTTP/HTTPS proxy
  -> internet
```

Client devices are configured manually with their IP, netmask, DNS, and the transparent proxy machine as default gateway. The transparent proxy machine itself keeps the company-required IP, netmask, DNS, and original company gateway.

In this mode the container enables IP forwarding and installs iptables rules for TCP traffic from the LAN interface to destination ports 80 and 443. The rules redirect matching traffic to the Go proxy service.

### Local mode

The transparent proxy runs on the same Linux machine that needs internet access.

The local machine keeps its normal company network settings:

- Company-assigned IP.
- Company-required netmask.
- Company original gateway.
- Company DNS.

The container installs OUTPUT-chain rules for local process traffic to TCP ports 80 and 443. It excludes the proxy process, upstream proxy address, loopback addresses, local networks, and configured excluded CIDRs to avoid loops.

## HTTPS Strategy

HTTPS support uses tunneling and does not decrypt traffic.

For a redirected TCP 443 connection:

1. The proxy gets the original destination IP and port.
2. It reads the beginning of the TLS ClientHello.
3. If the ClientHello contains SNI, the proxy uses that domain for upstream `CONNECT domain:443`.
4. If SNI is unavailable, the proxy falls back to `CONNECT original-ip:443`.
5. After the upstream proxy accepts CONNECT, the proxy sends the bytes already read to the tunnel and then performs bidirectional copying.

This design improves compatibility with company proxies that expect domain-based CONNECT requests while still keeping HTTPS private and transparent.

QUIC and HTTP/3 use UDP/443 and are outside the first version. Browsers normally fall back to TCP HTTPS when UDP/443 is unavailable.

## Components

### Go service

Responsibilities:

- Listen on the redirected TCP port.
- Read original destination addresses.
- Parse configuration from environment variables.
- Forward HTTP requests through the upstream proxy.
- Establish HTTPS CONNECT tunnels.
- Extract SNI from TLS ClientHello when available.
- Log startup, connection targets, CONNECT results, and errors.

### Network initialization and cleanup scripts

Responsibilities:

- Configure Linux forwarding when needed.
- Create project-owned iptables chains and jump rules.
- Exclude upstream proxy, loopback, local, and reserved networks.
- Clean up project-owned rules on startup, failure, shutdown, and manual recovery.

### Docker assets

Responsibilities:

- Build a small Go binary image.
- Provide Docker Compose deployment.
- Run with `NET_ADMIN` or privileged mode as required for network rule management.
- Pass configuration through environment variables.

### Documentation

The README explains:

- LAN gateway mode topology and client settings.
- Local mode behavior and host network settings.
- Docker Compose startup.
- Verification with browser, curl, apt, and Docker.
- Recovery commands.
- Known limitations.

## Configuration

The first version uses environment variables:

```env
MODE=lan-gateway
UPSTREAM_PROXY_HOST=proxy.company.local
UPSTREAM_PROXY_PORT=8080
LISTEN_PORT=12345
LAN_INTERFACE=eth0
EXCLUDE_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8
LOG_LEVEL=info
```

`MODE` supports:

- `lan-gateway`
- `local`

Configuration changes require restarting the container.

## Error Handling and Logging

Startup checks:

- Invalid mode fails startup.
- Missing upstream proxy host or port fails startup.
- Invalid listen port fails startup.
- Missing privileges for iptables or forwarding fails startup.
- Network rule setup failure triggers cleanup and fails startup.

Connection handling:

- Failure to read the original destination is logged and the connection is closed.
- Failure to connect to the upstream proxy is logged with target and upstream address.
- Upstream CONNECT rejection is logged with HTTP status code.
- SNI extraction failure is not fatal; the proxy falls back to the original IP.

Logging levels:

- `info`: startup configuration summary, rule setup result, connection success/failure summary.
- `debug`: target addresses, SNI extraction result, CONNECT request result, detailed forwarding errors.

The proxy does not log HTTP bodies, HTTPS payloads, or page contents.

## Network Recovery and Cleanup

The first version includes explicit fast recovery support.

Rules are isolated:

- Project-created chains use fixed names such as `TPROXY_CC_PREROUTING` and `TPROXY_CC_OUTPUT`.
- Main chains only contain jump rules into project chains.
- Cleanup deletes only project-owned jump rules and chains.
- Cleanup does not flush system firewall rules.

Startup is idempotent:

- The startup script first removes old project rules.
- It then creates fresh chains and jump rules.
- If setup fails, it runs cleanup before exiting.

Shutdown cleanup:

- The container traps termination signals.
- On normal shutdown it removes project-owned rules.
- `docker compose down` should leave the host without project iptables rules.

Manual recovery commands:

```bash
docker compose run --rm transparent-proxy cleanup
```

or:

```bash
./scripts/cleanup-network.sh
```

README recovery order:

1. Run cleanup.
2. If still abnormal, restart Docker or the network service.
3. Reboot only as the last fallback.

## Testing

### Unit tests

- Configuration parsing for valid and invalid environment variables.
- TLS ClientHello SNI extraction.
- SNI fallback when no domain is present.
- HTTP request forwarding format for upstream proxy.
- CONNECT response handling for success, rejection, and malformed responses.

### Integration tests

- Start a fake upstream HTTP proxy.
- Send transparent HTTP traffic and verify the upstream proxy receives explicit proxy requests.
- Send simulated HTTPS traffic and verify CONNECT creation.
- Verify bytes pass through the HTTPS tunnel in both directions.
- Verify HTTPS payload is not decrypted or inspected.

### Manual acceptance tests

Local mode:

- Start the Docker Compose stack.
- Do not configure `http_proxy` or `https_proxy` in the shell.
- Verify HTTP and HTTPS access with curl.
- Verify browser access to HTTP and HTTPS websites.
- Verify apt or Docker downloads when applicable.

LAN gateway mode:

- Configure another client device to use the transparent proxy machine as default gateway.
- Do not configure application proxy settings on the client.
- Verify browser, curl, apt, or Docker HTTP/HTTPS access.

Failure checks:

- Wrong upstream proxy address produces clear logs.
- Upstream proxy CONNECT rejection produces clear logs.
- Cleanup restores project-owned network rules.

## Completion Criteria

The first version is complete when one Linux host can run both modes and common HTTP/HTTPS applications can access the internet through the company upstream proxy without per-application proxy settings.
