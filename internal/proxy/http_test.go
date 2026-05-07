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
