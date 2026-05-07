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
