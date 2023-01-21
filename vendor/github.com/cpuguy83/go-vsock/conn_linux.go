package vsock

import (
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func FileConn(f *os.File) (net.Conn, error) {
	rc, err := f.SyscallConn()
	if err != nil {
		return nil, err
	}

	var (
		lsa *unix.SockaddrVM
		rsa *unix.SockaddrVM
	)
	rc.Control(func(fd uintptr) {
		var sa unix.Sockaddr
		sa, err = getSockName(int(fd))
		if err != nil {
			return
		}

		if sa, ok := sa.(*unix.SockaddrVM); ok {
			lsa = sa
		}

		sa, err = getPeerName(int(fd))
		if err != nil {
			return
		}

		if sa, ok := sa.(*unix.SockaddrVM); ok {
			rsa = sa
		}
	})

	if err != nil {
		return nil, err
	}

	if lsa == nil || rsa == nil {
		return net.FileConn(f)
	}

	return &VsockConn{
		f:  f,
		rc: rc,
		la: *lsa,
		ra: *rsa,
	}, nil
}

func (v *VsockConn) CloseRead() error {
	var err error
	e := v.rc.Control(func(fd uintptr) {
		err = shutdown(int(fd), unix.SHUT_RD)
	})
	if e != nil {
		return e
	}
	return err
}

func (v *VsockConn) CloseWrite() error {
	var err error
	e := v.rc.Control(func(fd uintptr) {
		err = shutdown(int(fd), unix.SHUT_WR)
	})
	if e != nil {
		return e
	}
	return err
}

func (v *VsockConn) control(f func(fd int) error) error {
	var err error
	e := v.rc.Control(func(fd uintptr) {
		err = f(int(fd))
	})
	if e != nil {
		return e
	}
	return err
}

func (v *VsockConn) SetDeadline(t time.Time) error {
	if err := v.SetReadDeadline(t); err != nil {
		return err
	}
	return v.SetWriteDeadline(t)
}

func (v *VsockConn) SetReadDeadline(t time.Time) error {
	return v.control(func(fd int) error {
		return setSockOptTime(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, t)
	})
}

func (v *VsockConn) SetWriteDeadline(t time.Time) error {
	return v.control(func(fd int) error {
		return setSockOptTime(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, t)
	})
}
