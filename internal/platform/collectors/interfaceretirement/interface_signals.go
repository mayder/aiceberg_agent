package interfaceretirement

import (
	"bytes"
	"context"
	"net"
	"time"
)

const maxKnownInterfaces = 4096

type interfaceSignal struct {
	identity uint64
	deleted  bool
	at       time.Time
}

type knownInterface struct {
	mac  net.HardwareAddr
	seen time.Time
}

// An IP interface notification alone is insufficient. A removed address family
// (IPv4/IPv6) may still have an entry in the logical/physical interface table.
func reconcileWindowsInterfaceSignals(ctx context.Context, inbox <-chan interfaceSignal,
	query func() (map[uint64]net.HardwareAddr, error), emit func([]removal),
) error {
	initial, err := query()
	if err != nil {
		return err
	}
	known := make(map[uint64]knownInterface)
	rememberInterfaces(known, initial, time.Now())
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if rows, err := query(); err == nil {
				rememberInterfaces(known, rows, time.Now())
			}
		case event, ok := <-inbox:
			if !ok {
				return nil
			}
			changes := takeInterfaceSignals(event, inbox)
			rows, err := confirmWindowsInterfaceTable(ctx, known, changes, query)
			if err != nil {
				// A failed enumeration is not proof of removal.
				continue
			}
			now := time.Now()
			removals := confirmInterfaceRemovals(known, rows, changes, now)
			rememberInterfaces(known, rows, now)
			if len(removals) > 0 {
				emit(removals)
			}
		}
	}
}

// IP deletion notifications can precede removal from the interface table.
func confirmWindowsInterfaceTable(ctx context.Context, known map[uint64]knownInterface, signals []interfaceSignal,
	query func() (map[uint64]net.HardwareAddr, error),
) (map[uint64]net.HardwareAddr, error) {
	rows, err := query()
	if err != nil {
		return nil, err
	}
	for _, signal := range signals {
		_, tracked := known[signal.identity]
		_, present := rows[signal.identity]
		if signal.deleted && tracked && present {
			timer := time.NewTimer(300 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
				return query()
			}
		}
	}
	return rows, nil
}

func takeInterfaceSignals(first interfaceSignal, inbox <-chan interfaceSignal) []interfaceSignal {
	signals := []interfaceSignal{first}
	for len(signals) < eventBatchLimit {
		select {
		case event, ok := <-inbox:
			if !ok {
				return signals
			}
			signals = append(signals, event)
		default:
			return signals
		}
	}
	return signals
}

func rememberInterfaces(known map[uint64]knownInterface, present map[uint64]net.HardwareAddr, at time.Time) {
	for id, row := range known {
		if _, exists := present[id]; !exists && at.Sub(row.seen) >= eventFreshnessSeconds*time.Second {
			delete(known, id)
		}
	}
	// Under churn, discard old evidence before accepting unbounded identities.
	if len(known)+len(present) > maxKnownInterfaces {
		clear(known)
	}
	for id, mac := range present {
		known[id] = knownInterface{mac: mac, seen: at}
	}
}

func confirmInterfaceRemovals(known map[uint64]knownInterface, present map[uint64]net.HardwareAddr,
	signals []interfaceSignal, now time.Time,
) []removal {
	var retired []removal
	for _, event := range signals {
		old, seen := known[event.identity]
		_, stillPresent := present[event.identity]
		if !event.deleted || !seen || stillPresent || len(old.mac) != 6 || now.Sub(event.at) >= eventFreshnessSeconds*time.Second {
			continue
		}
		if event.at.After(now) || event.at.Before(old.seen) || interfaceAddressPresent(old.mac, present) {
			continue
		}
		retired = append(retired, removal{mac: old.mac, observed: event.at})
		delete(known, event.identity)
	}
	return retired
}

func interfaceAddressPresent(address net.HardwareAddr, present map[uint64]net.HardwareAddr) bool {
	for _, mac := range present {
		if bytes.Equal(address, mac) {
			return true
		}
	}
	return false
}
