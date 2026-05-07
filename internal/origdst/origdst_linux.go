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
