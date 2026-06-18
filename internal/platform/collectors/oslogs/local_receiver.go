//go:build !windows
// +build !windows

package oslogs

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

type localLogEntry struct {
	Transport  string
	Message    string
	ReceivedAt time.Time
}

type localReceiver struct {
	udpAddr   string
	tcpAddr   string
	maxBytes  int
	entries   chan localLogEntry
	warnings  chan string
	closed    chan struct{}
	closeOnce sync.Once
	udpConn   net.PacketConn
	tcpLn     net.Listener
}

func newLocalReceiver(udpAddr, tcpAddr string, maxEntries, maxBytes int) *localReceiver {
	udpAddr = strings.TrimSpace(udpAddr)
	tcpAddr = strings.TrimSpace(tcpAddr)
	if udpAddr == "" && tcpAddr == "" {
		return nil
	}
	if maxEntries <= 0 {
		maxEntries = 200
	}
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	r := &localReceiver{
		udpAddr:  udpAddr,
		tcpAddr:  tcpAddr,
		maxBytes: maxBytes,
		entries:  make(chan localLogEntry, maxEntries),
		warnings: make(chan string, 16),
		closed:   make(chan struct{}),
	}
	if udpAddr != "" {
		r.startUDP(udpAddr)
	}
	if tcpAddr != "" {
		r.startTCP(tcpAddr)
	}
	return r
}

func (r *localReceiver) matches(udpAddr, tcpAddr string) bool {
	if r == nil {
		return strings.TrimSpace(udpAddr) == "" && strings.TrimSpace(tcpAddr) == ""
	}
	return r.udpAddr == strings.TrimSpace(udpAddr) && r.tcpAddr == strings.TrimSpace(tcpAddr)
}

func (r *localReceiver) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		close(r.closed)
		if r.udpConn != nil {
			_ = r.udpConn.Close()
		}
		if r.tcpLn != nil {
			_ = r.tcpLn.Close()
		}
	})
}

func (r *localReceiver) Drain(limit int) ([]localLogEntry, []string) {
	if r == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = cap(r.entries)
	}
	out := make([]localLogEntry, 0, limit)
	for len(out) < limit {
		select {
		case ev := <-r.entries:
			out = append(out, ev)
		default:
			goto warnings
		}
	}
warnings:
	var warnings []string
	for {
		select {
		case warning := <-r.warnings:
			warnings = append(warnings, warning)
		default:
			return out, warnings
		}
	}
}

func (r *localReceiver) startUDP(addr string) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		r.warn("udp listen failed: " + err.Error())
		return
	}
	r.udpConn = conn
	r.udpAddr = conn.LocalAddr().String()
	go func() {
		buf := make([]byte, r.maxBytes+1)
		for {
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				if !isClosedNetworkErr(err) {
					r.warn("udp read failed: " + err.Error())
				}
				return
			}
			r.push("udp", string(buf[:minInt(n, r.maxBytes)]))
		}
	}()
}

func (r *localReceiver) startTCP(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		r.warn("tcp listen failed: " + err.Error())
		return
	}
	r.tcpLn = ln
	r.tcpAddr = ln.Addr().String()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if !isClosedNetworkErr(err) {
					r.warn("tcp accept failed: " + err.Error())
				}
				return
			}
			go r.handleTCP(conn)
		}
	}()
}

func (r *localReceiver) handleTCP(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024), r.maxBytes)
	for scanner.Scan() {
		r.push("tcp", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		r.warn("tcp read failed: " + err.Error())
	}
}

func (r *localReceiver) push(transport, message string) {
	message = strings.TrimRight(message, "\r\n")
	if strings.TrimSpace(message) == "" {
		return
	}
	ev := localLogEntry{Transport: transport, Message: message, ReceivedAt: time.Now().UTC()}
	select {
	case r.entries <- ev:
	default:
		r.warn(transport + " local log queue full; event dropped")
	}
}

func (r *localReceiver) warn(message string) {
	select {
	case r.warnings <- message:
	default:
	}
}

func (r *localReceiver) udpLocalAddr() string {
	if r == nil || r.udpConn == nil {
		return ""
	}
	return r.udpConn.LocalAddr().String()
}

func (r *localReceiver) tcpLocalAddr() string {
	if r == nil || r.tcpLn == nil {
		return ""
	}
	return r.tcpLn.Addr().String()
}

func isClosedNetworkErr(err error) bool {
	return err == nil || errors.Is(err, net.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
