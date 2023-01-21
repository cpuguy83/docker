package vsock

import (
	"time"

	"golang.org/x/sys/unix"
)

func accept(fd int) (connFd int, ra unix.Sockaddr, err error) {
	for {
		connFd, ra, err = unix.Accept4(fd, unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK)
		if err == unix.EINTR {
			continue
		}
		return
	}
}

func socket(domain, typ, proto int) (fd int, err error) {
	for {
		fd, err = unix.Socket(domain, typ, proto)
		if err == unix.EINTR {
			continue
		}
		return
	}
}

func connect(fd int, sa unix.Sockaddr) (err error) {
	for {
		err = unix.Connect(fd, sa)
		if err == unix.EINTR {
			continue
		}
		return
	}
}

func getSockOptErr(fd int) error {
	var (
		ret int
		err error
	)
	for {
		ret, err = unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
		if err == unix.EINTR {
			continue
		}
		break
	}
	if err != nil {
		return err
	}
	if ret != 0 {
		return unix.Errno(ret)
	}
	return nil
}

func getSockName(fd int) (unix.Sockaddr, error) {
	var (
		ret unix.Sockaddr
		err error
	)
	for {
		ret, err = unix.Getsockname(fd)
		if err == unix.EINTR {
			continue
		}
		break
	}
	return ret, err
}

func getPeerName(fd int) (unix.Sockaddr, error) {
	var (
		ret unix.Sockaddr
		err error
	)
	for {
		ret, err = unix.Getpeername(fd)
		if err == unix.EINTR {
			continue
		}
		break
	}
	return ret, err
}

func listen(fd int, n int) error {
	for {
		err := unix.Listen(fd, n)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}

func bind(fd int, sa unix.Sockaddr) error {
	for {
		err := unix.Bind(fd, sa)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}

func shutdown(fd, how int) error {
	for {
		err := unix.Shutdown(fd, how)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}

func closeFd(fd int) error {
	for {
		err := unix.Close(fd)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}

func setSockOptTime(fd int, level, opt int, t time.Time) error {
	tv := unix.NsecToTimeval(t.UnixNano())
	for {
		err := unix.SetsockoptTimeval(fd, level, opt, &tv)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}
