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
