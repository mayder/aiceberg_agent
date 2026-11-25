package interfaceretirement

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxInterfaceSignals = 128

var windowsInterfaceListener atomic.Pointer[windowsInterfaceSubscription]

var windowsInterfaceCallback = sync.OnceValue(func() uintptr {
	return windows.NewCallback(receiveWindowsInterfaceChange)
})

type windowsInterfaceSubscription struct {
	events chan interfaceSignal
}

func receiveWindowsInterfaceChange(_ uintptr, row *windows.MibIpInterfaceRow, notification uintptr) uintptr {
	subscription := windowsInterfaceListener.Load()
	if subscription == nil || row == nil || row.InterfaceLuid == 0 || notification == windows.MibInitialNotification {
		return 0
	}
	event := interfaceSignal{identity: row.InterfaceLuid, deleted: notification == windows.MibDeleteInstance, at: time.Now()}
	// The callback must never wait on the collector or on CancelMibChangeNotify2.
	select {
	case subscription.events <- event:
	default:
	}
	return 0
}

func watchRemovals(ctx context.Context, emit func([]removal)) (result error) {
	subscription := &windowsInterfaceSubscription{events: make(chan interfaceSignal, maxInterfaceSignals)}
	if !windowsInterfaceListener.CompareAndSwap(nil, subscription) {
		return errors.New("interface notification watcher is already active")
	}
	var handle windows.Handle
	if err := windows.NotifyIpInterfaceChange(windows.AF_UNSPEC, windowsInterfaceCallback(), nil, false, &handle); err != nil {
		windowsInterfaceListener.CompareAndSwap(subscription, nil)
		return err
	}
	defer func() {
		if err := windows.CancelMibChangeNotify2(handle); err != nil {
			// Never associate callbacks from a registration still alive with a new watcher.
			result = errors.Join(result, err)
			return
		}
		windowsInterfaceListener.CompareAndSwap(subscription, nil)
	}()
	return reconcileWindowsInterfaceSignals(ctx, subscription.events, windowsInterfaceAddresses, emit)
}

func windowsInterfaceAddresses() (map[uint64]net.HardwareAddr, error) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormalWithoutStatistics, &table); err != nil {
		return nil, err
	}
	if table == nil {
		return nil, errors.New("missing Windows interface table")
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	if table.NumEntries > maxKnownInterfaces {
		return nil, errors.New("Windows interface table exceeds collection limit")
	}
	addresses := make(map[uint64]net.HardwareAddr, int(table.NumEntries))
	for _, row := range unsafe.Slice(&table.Table[0], int(table.NumEntries)) {
		if row.InterfaceLuid == 0 {
			continue
		}
		var mac net.HardwareAddr
		if row.PhysicalAddressLength == 6 {
			mac = append(mac, row.PhysicalAddress[:6]...)
		}
		// Keep the identity even without a MAC so an installed adapter cannot
		// be mistaken for a removed adapter when its address is unavailable.
		addresses[row.InterfaceLuid] = mac
	}
	return addresses, nil
}
