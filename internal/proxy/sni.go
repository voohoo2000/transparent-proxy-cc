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
