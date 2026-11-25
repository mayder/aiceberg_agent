package kernelnetlink

import (
	"context"
	"errors"

	"golang.org/x/sys/unix"
)

// Receive invokes consume synchronously for complete datagrams from the local
// kernel. The receive buffer is reused, so callbacks must copy retained data.
func Receive(ctx context.Context, protocol int, groups uint32, consume func([]byte)) error {
	socket, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, protocol)
	if err != nil {
		return err
	}
	defer unix.Close(socket)
	if err := unix.Bind(socket, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: groups}); err != nil {
		return err
	}
	poll := []unix.PollFd{{Fd: int32(socket), Events: unix.POLLIN}}
	buffer := make([]byte, 32*1024)
	for ctx.Err() == nil {
		count, err := unix.Poll(poll, 1000)
		if errors.Is(err, unix.EINTR) || count == 0 && err == nil {
			continue
		}
		if err != nil {
			return err
		}
		read, _, flags, from, err := unix.Recvmsg(socket, buffer, nil, unix.MSG_DONTWAIT)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) || errors.Is(err, unix.ENOBUFS) {
			continue
		}
		if err != nil {
			return err
		}
		sender, ok := from.(*unix.SockaddrNetlink)
		if ok && sender.Pid == 0 && flags&unix.MSG_TRUNC == 0 {
			consume(buffer[:read])
		}
	}
	return nil
}
