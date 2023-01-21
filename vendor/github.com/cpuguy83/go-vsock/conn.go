package vsock

import (
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

type VsockConn struct {
	f  *os.File
	rc syscall.RawConn
	ra unix.SockaddrVM
	la unix.SockaddrVM
}

func (v *VsockConn) Read(b []byte) (n int, err error) {
	return v.f.Read(b)
}

func (v *VsockConn) Write(b []byte) (n int, err error) {
	return v.f.Write(b)
}

func (v *VsockConn) Close() error {
	return v.f.Close()
}

func (v *VsockConn) LocalAddr() net.Addr {
	return &VsockAddr{v.la.CID, v.la.Port}
}

func (v *VsockConn) RemoteAddr() net.Addr {
	return &VsockAddr{v.ra.CID, v.ra.Port}
}

func (v *VsockConn) SyscallConn() (syscall.RawConn, error) {
	return v.rc, nil
}

type VsockAddr struct {
	CID  uint32
	Port uint32
}

func (v *VsockAddr) Network() string {
	return "vsock"
}

func (v *VsockAddr) String() string {
	return fmt.Sprintf("%d:%d", v.CID, v.Port)
}
