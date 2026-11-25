package interfaceretirement

import (
	"context"
	"net"
	"syscall"
	"time"

	"github.com/you/aiceberg_agent/internal/platform/collectors/kernelnetlink"
	"golang.org/x/sys/unix"
)

func watchRemovals(ctx context.Context, emit func([]removal)) error {
	return kernelnetlink.Receive(ctx, unix.NETLINK_ROUTE, unix.RTMGRP_LINK, func(buffer []byte) {
		if removals := decodeRemovedLinks(buffer, time.Now()); len(removals) > 0 {
			emit(removals)
		}
	})
}

func decodeRemovedLinks(buffer []byte, observed time.Time) []removal {
	messages, err := syscall.ParseNetlinkMessage(buffer)
	if err != nil {
		return nil
	}
	var removals []removal
	for _, message := range messages {
		if message.Header.Type != unix.RTM_DELLINK || len(message.Data) < unix.SizeofIfInfomsg {
			continue
		}
		attributes, err := syscall.ParseNetlinkRouteAttr(&message)
		if err != nil {
			return nil
		}
		var mac []byte
		for _, attribute := range attributes {
			if attribute.Attr.Type != unix.IFLA_ADDRESS {
				continue
			}
			if mac != nil || len(attribute.Value) != 6 {
				return nil
			}
			mac = attribute.Value
		}
		if mac != nil {
			removals = append(removals, removal{mac: net.HardwareAddr(append([]byte(nil), mac...)), observed: observed})
		}
		if len(removals) > eventBatchLimit {
			return nil
		}
	}
	return removals
}
