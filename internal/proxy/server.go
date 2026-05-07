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
