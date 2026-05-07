# Transparent Proxy Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Docker-first Go transparent proxy gateway that forwards local or LAN HTTP/HTTPS TCP traffic through an upstream unauthenticated HTTP proxy without per-application proxy settings.

**Architecture:** The Go service owns configuration parsing, original-destination lookup, HTTP forwarding, HTTPS CONNECT tunneling, and SNI extraction. Shell scripts own Linux iptables setup and cleanup, and Docker Compose provides the deployment entrypoint.

**Tech Stack:** Go 1.22+, standard library networking, Linux `iptables`, Docker, Docker Compose, shell scripts, Go unit/integration tests.

---

## File Structure

Create these files:

- `go.mod` — Go module definition for `github.com/voohoo2000/transparent-proxy-cc`.
- `cmd/transparent-proxy/main.go` — CLI entrypoint for `serve` and config validation.
- `internal/config/config.go` — environment parsing and validation.
- `internal/config/config_test.go` — configuration unit tests.
- `internal/origdst/origdst.go` — Linux original destination lookup interface and implementation.
- `internal/origdst/origdst_linux.go` — `SO_ORIGINAL_DST` implementation for Linux.
- `internal/proxy/server.go` — TCP listener and per-connection dispatcher.
- `internal/proxy/http.go` — HTTP transparent request forwarding through upstream proxy.
- `internal/proxy/https.go` — HTTPS CONNECT tunnel setup and bidirectional copy.
- `internal/proxy/sni.go` — TLS ClientHello SNI extraction without decrypting TLS.
- `internal/proxy/sni_test.go` — SNI extraction tests.
- `internal/proxy/connect_test.go` — CONNECT handling tests with a fake upstream proxy.
- `internal/proxy/http_test.go` — HTTP forwarding tests with a fake upstream proxy.
- `scripts/entrypoint.sh` — container entrypoint for `serve` and `cleanup` commands.
- `scripts/setup-network.sh` — idempotent iptables and forwarding setup.
- `scripts/cleanup-network.sh` — safe cleanup for project-owned iptables rules.
- `Dockerfile` — multi-stage Go build image.
- `docker-compose.yml` — recommended deployment template.
- `.env.example` — documented environment variables.
- `README.md` — setup, verification, limitations, and recovery guide.

Modify these files:

- `docs/superpowers/specs/2026-05-07-transparent-proxy-design.md` only if implementation discoveries require clarifying the spec.

---

### Task 1: Go module and configuration parser

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing configuration tests**

Create `internal/config/config_test.go`:

```go
package config

import (
	"testing"
)

func TestFromEnvValidLANMode(t *testing.T) {
	env := map[string]string{
		"MODE":                "lan-gateway",
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "8080",
		"LISTEN_PORT":        "12345",
		"LAN_INTERFACE":      "eth0",
		"EXCLUDE_CIDRS":      "10.0.0.0/8,192.168.0.0/16",
		"LOG_LEVEL":          "debug",
	}

	cfg, err := FromMap(env)
	if err != nil {
		t.Fatalf("FromMap returned error: %v", err)
	}
	if cfg.Mode != ModeLANGateway {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeLANGateway)
	}
	if cfg.UpstreamProxyHost != "proxy.company.local" {
		t.Fatalf("UpstreamProxyHost = %q", cfg.UpstreamProxyHost)
	}
	if cfg.UpstreamProxyPort != 8080 {
		t.Fatalf("UpstreamProxyPort = %d", cfg.UpstreamProxyPort)
	}
	if cfg.ListenPort != 12345 {
		t.Fatalf("ListenPort = %d", cfg.ListenPort)
	}
	if cfg.LANInterface != "eth0" {
		t.Fatalf("LANInterface = %q", cfg.LANInterface)
	}
	if len(cfg.ExcludeCIDRs) != 2 {
		t.Fatalf("ExcludeCIDRs length = %d", len(cfg.ExcludeCIDRs))
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
}

func TestFromEnvDefaults(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "8080",
	}

	cfg, err := FromMap(env)
	if err != nil {
		t.Fatalf("FromMap returned error: %v", err)
	}
	if cfg.Mode != ModeLANGateway {
		t.Fatalf("Mode = %q, want default %q", cfg.Mode, ModeLANGateway)
	}
	if cfg.ListenPort != 12345 {
		t.Fatalf("ListenPort = %d, want 12345", cfg.ListenPort)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestFromEnvRejectsInvalidMode(t *testing.T) {
	env := map[string]string{
		"MODE":                "bad",
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "8080",
	}

	_, err := FromMap(env)
	if err == nil {
		t.Fatal("FromMap returned nil error for invalid mode")
	}
}

func TestFromEnvRejectsMissingUpstreamHost(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_PROXY_PORT": "8080",
	}

	_, err := FromMap(env)
	if err == nil {
		t.Fatal("FromMap returned nil error for missing upstream host")
	}
}

func TestFromEnvRejectsInvalidPorts(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "70000",
	}

	_, err := FromMap(env)
	if err == nil {
		t.Fatal("FromMap returned nil error for invalid upstream port")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/config
```

Expected: FAIL because `go.mod` and `internal/config` implementation do not exist.

- [ ] **Step 3: Add module and minimal implementation**

Create `go.mod`:

```go
module github.com/voohoo2000/transparent-proxy-cc

go 1.22
```

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	ModeLANGateway = "lan-gateway"
	ModeLocal      = "local"
)

type Config struct {
	Mode              string
	UpstreamProxyHost string
	UpstreamProxyPort int
	ListenPort        int
	LANInterface      string
	ExcludeCIDRs      []*net.IPNet
	LogLevel          string
}

func FromEnv() (Config, error) {
	return FromMap(map[string]string{
		"MODE":                os.Getenv("MODE"),
		"UPSTREAM_PROXY_HOST": os.Getenv("UPSTREAM_PROXY_HOST"),
		"UPSTREAM_PROXY_PORT": os.Getenv("UPSTREAM_PROXY_PORT"),
		"LISTEN_PORT":        os.Getenv("LISTEN_PORT"),
		"LAN_INTERFACE":      os.Getenv("LAN_INTERFACE"),
		"EXCLUDE_CIDRS":      os.Getenv("EXCLUDE_CIDRS"),
		"LOG_LEVEL":          os.Getenv("LOG_LEVEL"),
	})
}

func FromMap(env map[string]string) (Config, error) {
	cfg := Config{
		Mode:         valueOrDefault(env["MODE"], ModeLANGateway),
		ListenPort:   12345,
		LANInterface: env["LAN_INTERFACE"],
		LogLevel:     valueOrDefault(env["LOG_LEVEL"], "info"),
	}

	if cfg.Mode != ModeLANGateway && cfg.Mode != ModeLocal {
		return Config{}, fmt.Errorf("invalid MODE %q", cfg.Mode)
	}

	cfg.UpstreamProxyHost = strings.TrimSpace(env["UPSTREAM_PROXY_HOST"])
	if cfg.UpstreamProxyHost == "" {
		return Config{}, fmt.Errorf("UPSTREAM_PROXY_HOST is required")
	}

	upstreamPort, err := parsePort(env["UPSTREAM_PROXY_PORT"], "UPSTREAM_PROXY_PORT")
	if err != nil {
		return Config{}, err
	}
	cfg.UpstreamProxyPort = upstreamPort

	if strings.TrimSpace(env["LISTEN_PORT"]) != "" {
		listenPort, err := parsePort(env["LISTEN_PORT"], "LISTEN_PORT")
		if err != nil {
			return Config{}, err
		}
		cfg.ListenPort = listenPort
	}

	cidrs, err := parseCIDRs(env["EXCLUDE_CIDRS"])
	if err != nil {
		return Config{}, err
	}
	cfg.ExcludeCIDRs = cidrs

	if cfg.LogLevel != "info" && cfg.LogLevel != "debug" {
		return Config{}, fmt.Errorf("invalid LOG_LEVEL %q", cfg.LogLevel)
	}

	return cfg, nil
}

func valueOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parsePort(value string, name string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be a port from 1 to 65535", name)
	}
	return port, nil
}

func parseCIDRs(value string) ([]*net.IPNet, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8"
	}

	parts := strings.Split(value, ",")
	cidrs := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("invalid EXCLUDE_CIDRS entry %q: %w", part, err)
		}
		cidrs = append(cidrs, ipNet)
	}
	return cidrs, nil
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add go.mod internal/config/config.go internal/config/config_test.go
git commit -m "$(cat <<'EOF'
Add proxy configuration parser

Validate environment-driven runtime settings before network rules or proxy listeners start.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Original destination lookup abstraction

**Files:**
- Create: `internal/origdst/origdst.go`
- Create: `internal/origdst/origdst_linux.go`

- [ ] **Step 1: Create original destination interface**

Create `internal/origdst/origdst.go`:

```go
package origdst

import (
	"net"
)

type Resolver interface {
	OriginalDst(conn *net.TCPConn) (net.Addr, error)
}

type LinuxResolver struct{}
```

- [ ] **Step 2: Create Linux implementation**

Create `internal/origdst/origdst_linux.go`:

```go
//go:build linux

package origdst

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
)

const soOriginalDst = 80

func (LinuxResolver) OriginalDst(conn *net.TCPConn) (net.Addr, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("get raw connection: %w", err)
	}

	var addr *net.TCPAddr
	var sockErr error
	err = rawConn.Control(func(fd uintptr) {
		raw, err := syscall.GetsockoptIPv6Mreq(int(fd), syscall.IPPROTO_IP, soOriginalDst)
		if err != nil {
			sockErr = err
			return
		}
		port := int(binary.BigEndian.Uint16(raw.Multiaddr[2:4]))
		ip := net.IPv4(raw.Multiaddr[4], raw.Multiaddr[5], raw.Multiaddr[6], raw.Multiaddr[7])
		addr = &net.TCPAddr{IP: ip, Port: port}
	})
	if err != nil {
		return nil, fmt.Errorf("control raw connection: %w", err)
	}
	if sockErr != nil {
		return nil, fmt.Errorf("get SO_ORIGINAL_DST: %w", sockErr)
	}
	if addr == nil {
		return nil, fmt.Errorf("SO_ORIGINAL_DST returned no address")
	}
	return addr, nil
}
```

- [ ] **Step 3: Run package tests/build**

Run:

```bash
go test ./internal/origdst
```

Expected: PASS or `[no test files]`.

- [ ] **Step 4: Commit**

Run:

```bash
git add internal/origdst/origdst.go internal/origdst/origdst_linux.go
git commit -m "$(cat <<'EOF'
Add Linux original destination resolver

Expose a focused interface for reading redirected TCP targets from Linux sockets.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: HTTPS SNI extraction

**Files:**
- Create: `internal/proxy/sni.go`
- Create: `internal/proxy/sni_test.go`

- [ ] **Step 1: Write failing SNI tests**

Create `internal/proxy/sni_test.go`:

```go
package proxy

import (
	"bytes"
	"crypto/tls"
	"net"
	"testing"
	"time"
)

func TestPeekSNIExtractsServerName(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		cfg := &tls.Config{ServerName: "example.com", InsecureSkipVerify: true}
		tlsClient := tls.Client(client, cfg)
		done <- tlsClient.Handshake()
	}()

	serverName, prefix, err := PeekSNI(server, 4096, time.Second)
	if err != nil {
		t.Fatalf("PeekSNI returned error: %v", err)
	}
	if serverName != "example.com" {
		t.Fatalf("serverName = %q, want example.com", serverName)
	}
	if len(prefix) == 0 {
		t.Fatal("prefix is empty")
	}
	if !bytes.HasPrefix(prefix, []byte{0x16, 0x03}) {
		t.Fatalf("prefix does not look like TLS handshake: %x", prefix[:2])
	}

	server.Close()
	<-done
}

func TestPeekSNIReturnsEmptyForPlainText(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		_, _ = client.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	}()

	serverName, prefix, err := PeekSNI(server, 4096, time.Second)
	if err != nil {
		t.Fatalf("PeekSNI returned error: %v", err)
	}
	if serverName != "" {
		t.Fatalf("serverName = %q, want empty", serverName)
	}
	if string(prefix) != "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n" {
		t.Fatalf("prefix = %q", string(prefix))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/proxy -run TestPeekSNI -v
```

Expected: FAIL because `PeekSNI` does not exist.

- [ ] **Step 3: Implement SNI extraction**

Create `internal/proxy/sni.go`:

```go
package proxy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

func PeekSNI(conn net.Conn, maxBytes int, timeout time.Duration) (string, []byte, error) {
	if maxBytes <= 0 {
		maxBytes = 4096
	}
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		defer conn.SetReadDeadline(time.Time{})
	}

	buf := make([]byte, maxBytes)
	n, err := conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			return "", buf[:n], nil
		}
		return "", buf[:n], err
	}
	prefix := buf[:n]
	name, _ := parseSNI(prefix)
	return name, prefix, nil
}

func parseSNI(data []byte) (string, error) {
	if len(data) < 5 || data[0] != 0x16 {
		return "", nil
	}
	recordLen := int(binary.BigEndian.Uint16(data[3:5]))
	if len(data) < 5+recordLen {
		return "", fmt.Errorf("incomplete TLS record")
	}
	handshake := data[5 : 5+recordLen]
	if len(handshake) < 4 || handshake[0] != 0x01 {
		return "", nil
	}
	handshakeLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if len(handshake) < 4+handshakeLen {
		return "", fmt.Errorf("incomplete ClientHello")
	}
	body := handshake[4 : 4+handshakeLen]
	if len(body) < 34 {
		return "", nil
	}
	pos := 34
	sessionLen := int(body[pos])
	pos++
	if len(body) < pos+sessionLen+2 {
		return "", nil
	}
	pos += sessionLen
	cipherLen := int(binary.BigEndian.Uint16(body[pos : pos+2]))
	pos += 2
	if len(body) < pos+cipherLen+1 {
		return "", nil
	}
	pos += cipherLen
	compressionLen := int(body[pos])
	pos++
	if len(body) < pos+compressionLen+2 {
		return "", nil
	}
	pos += compressionLen
	extLen := int(binary.BigEndian.Uint16(body[pos : pos+2]))
	pos += 2
	if len(body) < pos+extLen {
		return "", nil
	}
	extensions := body[pos : pos+extLen]
	for len(extensions) >= 4 {
		extType := binary.BigEndian.Uint16(extensions[0:2])
		extDataLen := int(binary.BigEndian.Uint16(extensions[2:4]))
		if len(extensions) < 4+extDataLen {
			return "", nil
		}
		extData := extensions[4 : 4+extDataLen]
		if extType == 0x0000 {
			return parseServerNameExtension(extData)
		}
		extensions = extensions[4+extDataLen:]
	}
	return "", nil
}

func parseServerNameExtension(data []byte) (string, error) {
	if len(data) < 2 {
		return "", nil
	}
	listLen := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+listLen {
		return "", nil
	}
	items := data[2 : 2+listLen]
	for len(items) >= 3 {
		nameType := items[0]
		nameLen := int(binary.BigEndian.Uint16(items[1:3]))
		if len(items) < 3+nameLen {
			return "", nil
		}
		if nameType == 0 {
			name := string(items[3 : 3+nameLen])
			if name != "" && !bytes.ContainsAny([]byte(name), "\x00\r\n") {
				return name, nil
			}
		}
		items = items[3+nameLen:]
	}
	return "", nil
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/proxy -run TestPeekSNI -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/proxy/sni.go internal/proxy/sni_test.go
git commit -m "$(cat <<'EOF'
Add TLS SNI extraction

Prefer domain-based CONNECT targets for HTTPS while keeping payloads encrypted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: HTTPS CONNECT tunnel

**Files:**
- Create: `internal/proxy/https.go`
- Create: `internal/proxy/connect_test.go`

- [ ] **Step 1: Write failing CONNECT tests**

Create `internal/proxy/connect_test.go`:

```go
package proxy

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestConnectUpstreamSuccess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requestLine := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, _ := reader.ReadString('\n')
		requestLine <- strings.TrimSpace(line)
		for {
			header, _ := reader.ReadString('\n')
			if header == "\r\n" || header == "" {
				break
			}
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		buf := make([]byte, 4)
		_, _ = reader.Read(buf)
		_, _ = conn.Write([]byte("pong"))
	}()

	conn, err := ConnectUpstream(listener.Addr().String(), "example.com:443", 2*time.Second)
	if err != nil {
		t.Fatalf("ConnectUpstream returned error: %v", err)
	}
	defer conn.Close()

	if got := <-requestLine; got != "CONNECT example.com:443 HTTP/1.1" {
		t.Fatalf("request line = %q", got)
	}
	_, _ = conn.Write([]byte("ping"))
	buf := make([]byte, 4)
	_, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("read tunnel response: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("response = %q", string(buf))
	}
}

func TestConnectUpstreamRejectsNon200(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = fmt.Fprint(conn, "HTTP/1.1 403 Forbidden\r\n\r\n")
	}()

	conn, err := ConnectUpstream(listener.Addr().String(), "example.com:443", 2*time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("ConnectUpstream returned nil error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v, want status code", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/proxy -run TestConnectUpstream -v
```

Expected: FAIL because `ConnectUpstream` does not exist.

- [ ] **Step 3: Implement CONNECT helper**

Create `internal/proxy/https.go`:

```go
package proxy

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"
)

func ConnectUpstream(upstreamAddr string, targetAddr string, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("tcp", upstreamAddr)
	if err != nil {
		return nil, fmt.Errorf("connect upstream proxy %s: %w", upstreamAddr, err)
	}

	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n", targetAddr, targetAddr)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("send CONNECT %s: %w", targetAddr, err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("upstream CONNECT %s rejected with %s", targetAddr, resp.Status)
	}
	return conn, nil
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/proxy -run TestConnectUpstream -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/proxy/https.go internal/proxy/connect_test.go
git commit -m "$(cat <<'EOF'
Add upstream CONNECT tunnel helper

Create HTTPS tunnels through the company proxy and surface rejection status clearly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: HTTP forwarding through upstream proxy

**Files:**
- Create: `internal/proxy/http.go`
- Create: `internal/proxy/http_test.go`

- [ ] **Step 1: Write failing HTTP forwarding test**

Create `internal/proxy/http_test.go`:

```go
package proxy

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

func TestForwardHTTPRequestUsesAbsoluteURL(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	requestLine := make(chan string, 1)
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, _ := reader.ReadString('\n')
		requestLine <- strings.TrimSpace(line)
		for {
			header, _ := reader.ReadString('\n')
			if header == "\r\n" || header == "" {
				break
			}
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	}()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		done <- ForwardHTTP(server, upstream.Addr().String(), "example.com:80", 2*time.Second)
	}()

	_, _ = client.Write([]byte("GET /path?q=1 HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	response := make([]byte, 64)
	n, err := client.Read(response)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(string(response[:n]), "200 OK") {
		t.Fatalf("response = %q", string(response[:n]))
	}
	client.Close()

	if err := <-done; err != nil {
		t.Fatalf("ForwardHTTP returned error: %v", err)
	}
	if got := <-requestLine; got != "GET http://example.com/path?q=1 HTTP/1.1" {
		t.Fatalf("request line = %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/proxy -run TestForwardHTTP -v
```

Expected: FAIL because `ForwardHTTP` does not exist.

- [ ] **Step 3: Implement HTTP forwarding**

Create `internal/proxy/http.go`:

```go
package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

func ForwardHTTP(client net.Conn, upstreamAddr string, targetAddr string, timeout time.Duration) error {
	defer client.Close()
	reader := bufio.NewReader(client)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return fmt.Errorf("read HTTP request: %w", err)
	}
	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}
	if req.URL.Host == "" {
		req.URL.Host = targetAddr
	}
	if req.RequestURI != "" {
		req.RequestURI = ""
	}

	dialer := net.Dialer{Timeout: timeout}
	upstream, err := dialer.Dial("tcp", upstreamAddr)
	if err != nil {
		return fmt.Errorf("connect upstream proxy %s: %w", upstreamAddr, err)
	}
	defer upstream.Close()

	if err := req.WriteProxy(upstream); err != nil {
		return fmt.Errorf("write proxy request: %w", err)
	}
	if _, err := io.Copy(client, upstream); err != nil {
		return fmt.Errorf("copy HTTP response: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/proxy -run TestForwardHTTP -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/proxy/http.go internal/proxy/http_test.go
git commit -m "$(cat <<'EOF'
Add HTTP upstream forwarding

Translate transparent HTTP requests into explicit proxy requests for the upstream proxy.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: TCP server and connection dispatcher

**Files:**
- Create: `internal/proxy/server.go`
- Create: `cmd/transparent-proxy/main.go`

- [ ] **Step 1: Create server implementation**

Create `internal/proxy/server.go`:

```go
package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/voohoo2000/transparent-proxy-cc/internal/config"
	"github.com/voohoo2000/transparent-proxy-cc/internal/origdst"
)

type Server struct {
	Config   config.Config
	Resolver origdst.Resolver
}

func (s Server) ListenAndServe(ctx context.Context) error {
	addr := ":" + strconv.Itoa(s.Config.ListenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	log.Printf("transparent proxy listening on %s", addr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept connection: %w", err)
		}
		go s.handle(conn)
	}
}

func (s Server) handle(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		log.Printf("connection is not TCP")
		conn.Close()
		return
	}

	target, err := s.Resolver.OriginalDst(tcpConn)
	if err != nil {
		log.Printf("original destination error: %v", err)
		conn.Close()
		return
	}
	tcpTarget, ok := target.(*net.TCPAddr)
	if !ok {
		log.Printf("original destination is not TCP: %v", target)
		conn.Close()
		return
	}

	upstream := net.JoinHostPort(s.Config.UpstreamProxyHost, strconv.Itoa(s.Config.UpstreamProxyPort))
	targetAddr := net.JoinHostPort(tcpTarget.IP.String(), strconv.Itoa(tcpTarget.Port))
	log.Printf("connection target=%s", targetAddr)

	switch tcpTarget.Port {
	case 80:
		if err := ForwardHTTP(conn, upstream, targetAddr, 10*time.Second); err != nil {
			log.Printf("HTTP forward failed target=%s: %v", targetAddr, err)
		}
	case 443:
		if err := s.forwardHTTPS(conn, upstream, targetAddr); err != nil {
			log.Printf("HTTPS forward failed target=%s: %v", targetAddr, err)
		}
	default:
		log.Printf("unsupported redirected port target=%s", targetAddr)
		conn.Close()
	}
}

func (s Server) forwardHTTPS(client net.Conn, upstream string, fallbackTarget string) error {
	defer client.Close()
	serverName, prefix, err := PeekSNI(client, 4096, 3*time.Second)
	if err != nil {
		return fmt.Errorf("peek SNI: %w", err)
	}
	target := fallbackTarget
	if serverName != "" {
		target = net.JoinHostPort(serverName, "443")
	}
	upstreamConn, err := ConnectUpstream(upstream, target, 10*time.Second)
	if err != nil {
		return err
	}
	defer upstreamConn.Close()
	if len(prefix) > 0 {
		if _, err := upstreamConn.Write(prefix); err != nil {
			return fmt.Errorf("write TLS prefix: %w", err)
		}
	}
	return copyBoth(client, upstreamConn)
}

func copyBoth(a net.Conn, b net.Conn) error {
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(a, b)
		done <- err
	}()
	go func() {
		_, err := io.Copy(b, a)
		done <- err
	}()
	err := <-done
	_ = a.Close()
	_ = b.Close()
	return err
}
```

- [ ] **Step 2: Create CLI entrypoint**

Create `cmd/transparent-proxy/main.go`:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/voohoo2000/transparent-proxy-cc/internal/config"
	"github.com/voohoo2000/transparent-proxy-cc/internal/origdst"
	"github.com/voohoo2000/transparent-proxy-cc/internal/proxy"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runServe()
		return
	}
	log.Fatalf("usage: transparent-proxy serve")
}

func runServe() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	log.Printf("starting transparent proxy mode=%s upstream=%s:%d listen=%d", cfg.Mode, cfg.UpstreamProxyHost, cfg.UpstreamProxyPort, cfg.ListenPort)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := proxy.Server{Config: cfg, Resolver: origdst.LinuxResolver{}}
	if err := server.ListenAndServe(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 3: Run full Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Build binary**

Run:

```bash
go build ./cmd/transparent-proxy
```

Expected: PASS and binary `transparent-proxy` appears in the current directory.

- [ ] **Step 5: Remove local build artifact**

Run:

```bash
rm -f transparent-proxy
```

Expected: command exits 0.

- [ ] **Step 6: Commit**

Run:

```bash
git add cmd/transparent-proxy/main.go internal/proxy/server.go
git commit -m "$(cat <<'EOF'
Add transparent proxy server

Wire original destination dispatch to HTTP forwarding and HTTPS CONNECT tunneling.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Network setup and cleanup scripts

**Files:**
- Create: `scripts/setup-network.sh`
- Create: `scripts/cleanup-network.sh`
- Create: `scripts/entrypoint.sh`

- [ ] **Step 1: Create cleanup script**

Create `scripts/cleanup-network.sh`:

```sh
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
```

- [ ] **Step 2: Create setup script**

Create `scripts/setup-network.sh`:

```sh
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
```

- [ ] **Step 3: Create entrypoint script**

Create `scripts/entrypoint.sh`:

```sh
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
```

- [ ] **Step 4: Make scripts executable**

Run:

```bash
chmod +x scripts/setup-network.sh scripts/cleanup-network.sh scripts/entrypoint.sh
```

Expected: command exits 0.

- [ ] **Step 5: Syntax-check scripts**

Run:

```bash
sh -n scripts/setup-network.sh && sh -n scripts/cleanup-network.sh && sh -n scripts/entrypoint.sh
```

Expected: PASS with no output.

- [ ] **Step 6: Commit**

Run:

```bash
git add scripts/setup-network.sh scripts/cleanup-network.sh scripts/entrypoint.sh
git commit -m "$(cat <<'EOF'
Add network rule setup and cleanup scripts

Manage project-owned iptables chains so transparent mode can be enabled and safely restored.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Docker deployment assets

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.env.example`

- [ ] **Step 1: Create Dockerfile**

Create `Dockerfile`:

```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/transparent-proxy ./cmd/transparent-proxy

FROM alpine:3.20
RUN apk add --no-cache iptables iproute2 bind-tools ca-certificates
WORKDIR /app
COPY --from=build /out/transparent-proxy /app/transparent-proxy
COPY scripts /app/scripts
ENTRYPOINT ["/app/scripts/entrypoint.sh"]
CMD ["serve"]
```

- [ ] **Step 2: Create Compose template**

Create `docker-compose.yml`:

```yaml
services:
  transparent-proxy:
    build: .
    container_name: transparent-proxy-cc
    network_mode: host
    cap_add:
      - NET_ADMIN
      - NET_RAW
    environment:
      MODE: ${MODE:-lan-gateway}
      UPSTREAM_PROXY_HOST: ${UPSTREAM_PROXY_HOST:?set UPSTREAM_PROXY_HOST}
      UPSTREAM_PROXY_PORT: ${UPSTREAM_PROXY_PORT:?set UPSTREAM_PROXY_PORT}
      LISTEN_PORT: ${LISTEN_PORT:-12345}
      LAN_INTERFACE: ${LAN_INTERFACE:-eth0}
      EXCLUDE_CIDRS: ${EXCLUDE_CIDRS:-10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8}
      LOG_LEVEL: ${LOG_LEVEL:-info}
    restart: unless-stopped
```

- [ ] **Step 3: Create environment example**

Create `.env.example`:

```env
MODE=lan-gateway
UPSTREAM_PROXY_HOST=proxy.company.local
UPSTREAM_PROXY_PORT=8080
LISTEN_PORT=12345
LAN_INTERFACE=eth0
EXCLUDE_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8
LOG_LEVEL=info
```

- [ ] **Step 4: Build Docker image**

Run:

```bash
docker compose build
```

Expected: PASS and image builds successfully.

- [ ] **Step 5: Commit**

Run:

```bash
git add Dockerfile docker-compose.yml .env.example
git commit -m "$(cat <<'EOF'
Add Docker Compose deployment

Package the transparent proxy with host networking and NET_ADMIN for Docker-first setup.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: README usage and recovery guide

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write README**

Create `README.md`:

```markdown
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
```

- [ ] **Step 2: Commit**

Run:

```bash
git add README.md
git commit -m "$(cat <<'EOF'
Document transparent proxy usage

Explain Docker setup, network modes, verification, HTTPS behavior, and recovery steps.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Final verification and push

**Files:**
- No new files.

- [ ] **Step 1: Run all Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Build Docker image**

Run:

```bash
docker compose build
```

Expected: PASS.

- [ ] **Step 3: Check script syntax**

Run:

```bash
sh -n scripts/setup-network.sh && sh -n scripts/cleanup-network.sh && sh -n scripts/entrypoint.sh
```

Expected: PASS with no output.

- [ ] **Step 4: Check git status**

Run:

```bash
git status --short
```

Expected: no output.

- [ ] **Step 5: Push branch**

Run:

```bash
git push
```

Expected: branch pushes to `origin/main`.

---

## Self-Review

Spec coverage:

- Docker-first Go transparent proxy: covered by Tasks 1, 6, and 8.
- Local and LAN modes: covered by Tasks 7, 8, and 9.
- Upstream unauthenticated HTTP proxy: covered by Tasks 4 and 5.
- HTTP/HTTPS TCP 80/443: covered by Tasks 5, 6, and 7.
- HTTPS without decryption and SNI fallback: covered by Tasks 3, 4, and 6.
- Automatic setup and cleanup: covered by Task 7.
- README verification and limitations: covered by Task 9.
- Tests and final verification: covered by Tasks 1, 3, 4, 5, and 10.

Placeholder scan: no placeholder implementation steps remain.

Type consistency: `config.Config`, `origdst.Resolver`, `proxy.Server`, `PeekSNI`, `ConnectUpstream`, and `ForwardHTTP` names are consistent across tasks.
