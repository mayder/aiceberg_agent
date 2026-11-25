package interfaceretirement

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestWindowsSignalNeedsPriorIdentityAndPhysicalTableConfirmation(t *testing.T) {
	now := time.Now()
	original := net.HardwareAddr{2, 0, 0, 0, 0, 7}
	known := map[uint64]knownInterface{}
	rememberInterfaces(known, map[uint64]net.HardwareAddr{9: original}, now.Add(-time.Second))
	deleteEvent := interfaceSignal{identity: 9, deleted: true, at: now}

	for _, check := range []struct {
		name    string
		rows    map[uint64]net.HardwareAddr
		signals []interfaceSignal
	}{
		{"address family removed but device exists", map[uint64]net.HardwareAddr{9: nil}, []interfaceSignal{deleteEvent}},
		{"same MAC still present under another interface", map[uint64]net.HardwareAddr{10: original}, []interfaceSignal{deleteEvent}},
		{"rename or address update", nil, []interfaceSignal{{identity: 9, at: now}}},
		{"known device missing without delete", nil, nil},
		{"unknown LUID", nil, []interfaceSignal{{identity: 99, deleted: true, at: now}}},
		{"delete predates most recent observation", nil, []interfaceSignal{{identity: 9, deleted: true, at: now.Add(-2 * time.Second)}}},
	} {
		t.Run(check.name, func(t *testing.T) {
			if got := confirmInterfaceRemovals(known, check.rows, check.signals, now); len(got) != 0 {
				t.Fatalf("unexpected retirement: %+v", got)
			}
		})
	}
	got := confirmInterfaceRemovals(known, nil, []interfaceSignal{deleteEvent, deleteEvent}, now)
	if len(got) != 1 || got[0].mac.String() != original.String() {
		t.Fatalf("both Windows address families must yield one retirement: %+v", got)
	}
}

func TestWindowsInterfaceReconciliationIgnoresAnEnumerationFailure(t *testing.T) {
	now := time.Now()
	inbox := make(chan interfaceSignal, 1)
	inbox <- interfaceSignal{identity: 9, deleted: true, at: now.Add(time.Second)}
	close(inbox)
	calls := 0
	err := reconcileWindowsInterfaceSignals(context.Background(), inbox, func() (map[uint64]net.HardwareAddr, error) {
		calls++
		if calls == 1 {
			return map[uint64]net.HardwareAddr{9: {2, 0, 0, 0, 0, 7}}, nil
		}
		return nil, errors.New("enumeration unavailable")
	}, func([]removal) { t.Fatal("enumeration failure treated as removal") })
	if err != nil || calls != 2 {
		t.Fatalf("watcher did not recover: calls=%d, %v", calls, err)
	}
}

func TestWindowsRemovalWaitsForInterfaceTableConvergence(t *testing.T) {
	now := time.Now()
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 7}
	known := map[uint64]knownInterface{9: {mac: mac, seen: now.Add(-time.Second)}}
	event := []interfaceSignal{{identity: 9, deleted: true, at: now}}
	calls := 0
	rows, err := confirmWindowsInterfaceTable(context.Background(), known, event, func() (map[uint64]net.HardwareAddr, error) {
		calls++
		if calls == 1 {
			return map[uint64]net.HardwareAddr{9: mac}, nil
		}
		return map[uint64]net.HardwareAddr{}, nil
	})
	if err != nil || calls != 2 || len(confirmInterfaceRemovals(known, rows, event, time.Now())) != 1 {
		t.Fatalf("delayed table removal was not confirmed: calls=%d tables=%v error=%v", calls, rows, err)
	}
}
