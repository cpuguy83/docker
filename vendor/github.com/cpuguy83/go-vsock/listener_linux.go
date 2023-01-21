package vsock

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

type Listener struct {
	f    *os.File
	rc   syscall.RawConn
	addr unix.SockaddrVM
}

func ListenVsock(cid, port uint32) (_ *Listener, retErr error) {
	fd, err := socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, &net.OpError{Op: "socket", Net: "vsock", Err: err}
	}

	f := os.NewFile(uintptr(fd), fmt.Sprintf("vsock://%d:%d", cid, port))
	defer func() {
		if retErr != nil {
			f.Close()
		}
	}()

	rc, err := f.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw socket fd: %w", err)
	}

	var addr unix.SockaddrVM

	e := rc.Control(func(fd uintptr) {
		err = bind(int(fd), &unix.SockaddrVM{
			CID:  cid,
			Port: port,
		})
		if err != nil {
			return
		}

		// Get the actual cid/port that was bound to.
		// Since the address specified may have included VMADDR_CID_ANY or VMADDR_PORT_ANY.
		var name unix.Sockaddr
		name, err = getSockName(int(fd))
		if err != nil {
			return
		}
		addr = *name.(*unix.SockaddrVM)

		err = listen(int(fd), 10)
	})
	if e != nil {
		return nil, e
	}
	if err != nil {
		return nil, &net.OpError{Op: "bind", Net: "vsock", Err: err}
	}
	return &Listener{f: f, rc: rc, addr: addr}, nil
}

func (l *Listener) read(f func(fd int) error) error {
	var err error
	e := l.rc.Read(func(fd uintptr) bool {
		err = f(int(fd))
		return !errors.Is(err, unix.EAGAIN)
	})
	if e != nil {
		return e
	}
	return err
}

func (l *Listener) Accept() (net.Conn, error) {
	var (
		connFd int
		ra     *unix.SockaddrVM
	)
	err := l.read(func(fd int) error {
		var err error
		nfd, sysRA, err := accept(fd)
		if err != nil {
			return err
		}

		var ok bool
		ra, ok = sysRA.(*unix.SockaddrVM)
		if !ok {
			closeFd(nfd)
			return fmt.Errorf("unexpected sockaddr type: %T", sysRA)
		}

		connFd = nfd
		return nil
	})
	if err != nil {
		return nil, &net.OpError{Op: "accept", Net: "vsock", Err: err}
	}

	f := os.NewFile(uintptr(connFd), fmt.Sprintf("vsock://%d:%d", ra.CID, ra.Port))
	rc, err := f.SyscallConn()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to get raw socket fd: %w", err)
	}

	return &VsockConn{
		f:  f,
		rc: rc,
		ra: *ra,
		la: l.addr,
	}, nil
}

func (l *Listener) Close() error {
	return l.f.Close()
}

func (l *Listener) Addr() net.Addr {
	return &VsockAddr{
		CID:  l.addr.CID,
		Port: l.addr.Port,
	}
}
