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
