package telnetprobe

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const (
	WorkerArgument = "-agentless-telnet-worker"
	ReplyLimit     = 4096

	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240
)

const (
	Banner       = "banner"
	Negotiated   = "negotiated"
	Unrecognized = "unrecognized"
	ReplyTooLong = "limit"
	SocketError  = "socket_error"
)

// RunInherited processes the single connected socket passed by the HUB on fd 3.
func RunInherited(out io.Writer) int {
	socket := os.NewFile(3, "telnet-probe-socket")
	if socket == nil {
		return 1
	}
	conn, err := net.FileConn(socket)
	_ = socket.Close()
	if err != nil {
		return 1
	}
	defer conn.Close()
	if _, ok := conn.(*net.TCPConn); !ok {
		return 1
	}
	if _, err := fmt.Fprintln(out, inspect(conn)); err != nil {
		return 1
	}
	return 0
}

func inspect(conn net.Conn) string {
	deadline := time.Now().Add(5 * time.Second)
	if err := conn.SetDeadline(deadline); err != nil {
		return SocketError
	}
	limited := &io.LimitedReader{R: conn, N: ReplyLimit}
	input := bufio.NewReaderSize(limited, 512)
	state := negotiation{}
	for {
		value, err := input.ReadByte()
		if err != nil {
			return state.finish(err, limited.N == 0)
		}
		switch {
		case value == iac:
			if err := state.command(input, conn, deadline); err != nil {
				return state.finish(err, limited.N == 0)
			}
		case value >= 33 && value <= 126:
			state.banner = true
		}
		if state.negotiated && state.banner {
			return Banner
		}
	}
}

type negotiation struct {
	negotiated bool
	banner     bool
	remote     [256]bool
	local      [256]bool
}

func (state *negotiation) command(input *bufio.Reader, conn net.Conn, deadline time.Time) error {
	command, err := input.ReadByte()
	if err != nil {
		return err
	}
	switch command {
	case sb:
		return discardSubnegotiation(input)
	case do, dont, will, wont:
		option, err := input.ReadByte()
		if err != nil {
			return err
		}
		if !state.negotiated {
			bannerDeadline := time.Now().Add(350 * time.Millisecond)
			if deadline.Before(bannerDeadline) {
				bannerDeadline = deadline
			}
			if err := conn.SetDeadline(bannerDeadline); err != nil {
				return err
			}
			state.negotiated = true
		}
		return state.decline(conn, command, option)
	default:
		return nil
	}
}

func (state *negotiation) decline(conn net.Conn, command, option byte) error {
	switch command {
	case will:
		if !state.remote[option] {
			state.remote[option] = true
			_, err := conn.Write([]byte{iac, dont, option})
			return err
		}
	case do:
		if !state.local[option] {
			state.local[option] = true
			_, err := conn.Write([]byte{iac, wont, option})
			return err
		}
	case wont:
		state.remote[option] = false
	case dont:
		state.local[option] = false
	}
	return nil
}

func discardSubnegotiation(input *bufio.Reader) error {
	for {
		value, err := input.ReadByte()
		if err != nil {
			return err
		}
		if value != iac {
			continue
		}
		command, err := input.ReadByte()
		if err != nil {
			return err
		}
		if command == se {
			return nil
		}
	}
}

func (state negotiation) finish(err error, limit bool) string {
	if limit {
		return ReplyTooLong
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrDeadlineExceeded) {
		return SocketError
	}
	if state.negotiated {
		return Negotiated
	}
	return Unrecognized
}
