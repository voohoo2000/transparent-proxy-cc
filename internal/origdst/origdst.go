package origdst

import (
	"net"
)

type Resolver interface {
	OriginalDst(conn *net.TCPConn) (net.Addr, error)
}

type LinuxResolver struct{}
