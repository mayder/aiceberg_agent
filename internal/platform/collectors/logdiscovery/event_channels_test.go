package logdiscovery

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestEventChannelDiscoveryUsesRegisteredChannels(t *testing.T) {
	custom := "Fixture-Product/Operational"
	probes := map[string]int{}
	rows, unavailable := discoverEventChannels(context.Background(), []string{" system ", custom}, func(channel string) (channelPresence, error) {
		probes[channel]++
		switch channel {
		case "System", custom:
			return channelRegistered, nil
		case "Security":
			return channelUnknown, fs.ErrPermission
		default:
			return channelMissing, nil
		}
	})
	if !unavailable || len(rows) != 2 || rows[0].Channel != "System" || rows[1].Channel != custom || len(probes) != 5 || probes["System"] != 1 {
		t.Fatalf("unexpected registered channels: %+v, unavailable=%v, probes=%v", rows, unavailable, probes)
	}
	if eventChannelCandidate("system").Fingerprint != rows[0].Fingerprint {
		t.Fatal("case variants must preserve the channel identity")
	}
}

func TestEventChannelRetirementRequiresRepeatedExplicitAbsence(t *testing.T) {
	watcher := NewSourceRetirement(7, 17)
	channel := eventChannelCandidate(sysmonChannel)
	ctx := context.Background()
	now := time.Unix(1795550000, 0)
	var published []monitoring.DiscoveryEvent
	publish := func(_ context.Context, events []monitoring.DiscoveryEvent) error {
		published = append(published, events...)
		return nil
	}
	for minute, step := range []struct {
		status channelPresence
		err    error
		rows   []Candidate
	}{
		{status: channelRegistered, rows: []Candidate{channel}},
		{status: channelMissing},
		{status: channelUnknown, err: fs.ErrPermission},
		{status: channelMissing},
		{status: channelRegistered},
		{status: channelMissing},
		{status: channelMissing},
		{status: channelMissing},
	} {
		watcher.probeChannel = func(name string) (channelPresence, error) {
			if name != sysmonChannel {
				t.Fatalf("unexpected channel %q", name)
			}
			return step.status, step.err
		}
		wire, err := json.Marshal(map[string]payload{schemaVersion: {Candidates: step.rows}})
		if err != nil {
			t.Fatal(err)
		}
		if err := watcher.ObserveBuffered(ctx, wire, now.Add(time.Duration(minute)*time.Minute), publish); err != nil {
			t.Fatal(err)
		}
		if minute < 7 && len(published) != 0 {
			t.Fatalf("retirement emitted without sustained explicit absence: %v", published)
		}
	}
	want, err := monitoring.NewDiscoveryEvent(7, 17, "log_sources", []string{channel.Fingerprint}, "tombstone", 100,
		retirementFreshness, now.Add(7*time.Minute))
	if err != nil || len(published) != 1 || published[0] != want {
		t.Fatalf("expected retirement %v, got %v (%v)", want, published, err)
	}
}
