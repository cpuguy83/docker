package vsock

import (
	"net"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func Listen(network string, address string) (net.Listener, error) {
	if network != "vsock" {
		return net.Listen(network, address)
	}

	cid, port, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}
	return ListenVsock(cid, port)
}

func Dial(network, addr string) (net.Conn, error) {
	if network != "vsock" {
		return net.Dial(network, addr)
	}

	cid, port, err := ParseAddress(addr)
	if err != nil {
		return nil, err
	}
	return DialVsock(cid, port)
}

func ParseAddress(address string) (cid, port uint32, _ error) {
	cidStr, portStr, _ := strings.Cut(address, ":")

	if cidStr != "" {
		v, err := strconv.ParseUint(cidStr, 10, 32)
		if err != nil {
			return 0, 0, err
		}
		cid = uint32(v)
	}

	if portStr != "" {
		v, err := strconv.ParseUint(portStr, 10, 32)
		if err != nil {
			return 0, 0, err
		}
		port = uint32(v)
	}

	if cid == 0 {
		cid = unix.VMADDR_CID_ANY
	}

	if port == 0 {
		port = unix.VMADDR_PORT_ANY
	}

	return cid, port, nil
}
