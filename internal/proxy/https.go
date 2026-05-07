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
