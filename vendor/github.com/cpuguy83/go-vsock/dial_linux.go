package vsock

import (
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

func DialVsock(cid, port uint32) (_ *VsockConn, retErr error) {
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

	addr := unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}
	e := rc.Control(func(fd uintptr) {
		err = connect(int(fd), &addr)
	})
	if e != nil {
		return nil, e
	}

	if err != nil && !errors.Is(err, unix.EINPROGRESS) {
		return nil, &net.OpError{Op: "connect", Net: "vsock", Err: err}
	}

	e = rc.Write(func(fd uintptr) bool {
		err = getSockOptErr(int(fd))
		if err != nil {
			return true
		}

		var a unix.Sockaddr
		a, err = getSockName(int(fd))
		if err != nil {
			return true
		}

		sa, ok := a.(*unix.SockaddrVM)
		if !ok {
			err = errors.New("unexpected socket address type")
			return true
		}

		addr = *sa
		return true
	})
	if e != nil {
		return nil, e
	}
	if err != nil {
		return nil, &net.OpError{Op: "connect", Net: "vsock", Err: err}
	}

	la, err := getSockName(int(fd))
	if err != nil {
		return nil, &net.OpError{Op: "getsockname", Net: "vsock", Err: err}
	}

	laddr, ok := la.(*unix.SockaddrVM)
	if !ok {
		return nil, &net.OpError{Op: "getsockname", Net: "vsock", Err: errors.New("unexpected socket address type")}
	}

	return &VsockConn{f: f, rc: rc, ra: addr, la: *laddr}, nil
}
