package agentless

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

var ErrSNMPTrapConfig = errors.New("invalid SNMP trap receiver configuration")

type SNMPTrapReceiver struct {
	conn     *net.UDPConn
	profiles map[netip.Addr]trapPeer
}

type trapPeer struct {
	profile SNMPTrapProfile
	usm     *trapUSM
}

func (p trapPeer) observation(raw []byte, source netip.Addr, now time.Time) (entities.AgentlessObservation, *entities.SNMPTrapReplayProof, bool) {
	if p.usm == nil {
		obs, valid := trapObservation(raw, source, p.profile, now)
		return obs, nil, valid
	}
	packet, proof := p.usm.decode(raw, now)
	if packet == nil {
		return entities.AgentlessObservation{}, nil, false
	}
	obs, valid := newTrapObservation(packet, source, p.profile, now)
	return obs, proof, valid
}

func ListenSNMPTraps(address string, profiles []SNMPTrapProfile) (*SNMPTrapReceiver, error) {
	bind, err := netip.ParseAddrPort(address)
	if err != nil || bind.Addr().IsUnspecified() || bind.Addr().IsMulticast() || len(profiles) == 0 || len(profiles) > 256 {
		return nil, ErrSNMPTrapConfig
	}
	byIP := make(map[netip.Addr]trapPeer, len(profiles))
	engines := make(map[[32]byte]bool)
	for _, profile := range profiles {
		source, err := netip.ParseAddr(profile.SourceIP)
		if err != nil || source.IsUnspecified() || source.IsMulticast() || source.Zone() != "" {
			return nil, ErrSNMPTrapConfig
		}
		source = source.Unmap()
		if _, exists := byIP[source]; exists || !validTrapProfile(profile) {
			return nil, ErrSNMPTrapConfig
		}
		profile.OIDs = append([]string(nil), profile.OIDs...)
		peer := trapPeer{profile: profile}
		if profile.Version == "v3" {
			peer.usm, err = newTrapUSM(profile.USM)
			if err != nil || engines[peer.usm.engineKey] {
				return nil, ErrSNMPTrapConfig
			}
			engines[peer.usm.engineKey] = true
			peer.profile.USM = nil
		}
		byIP[source] = peer
	}
	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(bind))
	if err != nil {
		return nil, err
	}
	return &SNMPTrapReceiver{conn: conn, profiles: byIP}, nil
}

func validTrapProfile(profile SNMPTrapProfile) bool {
	if profile.Version == "v3" {
		if profile.Community != "" || profile.USM == nil {
			return false
		}
	} else if (profile.Version != "v1" && profile.Version != "v2c") || len(profile.Community) == 0 ||
		len(profile.Community) > 128 || profile.USM != nil {
		return false
	}
	if profile.ClientID <= 0 || profile.AssetID <= 0 || profile.CheckID <= 0 || profile.EndpointID <= 0 ||
		len(profile.OIDs) == 0 || len(profile.OIDs) > 32 {
		return false
	}
	for _, oid := range profile.OIDs {
		if !validTrapOID(oid) {
			return false
		}
	}
	return true
}

func (r *SNMPTrapReceiver) Close() error {
	return r.conn.Close()
}

func (r *SNMPTrapReceiver) Serve(ctx context.Context, enabled func() bool, appendEvent func(entities.AgentlessObservation, *entities.SNMPTrapReplayProof) error, onStorageError func()) error {
	stop := context.AfterFunc(ctx, func() { _ = r.conn.Close() })
	defer stop()
	defer r.conn.Close()
	buf := make([]byte, maxTrapBytes+1)
	lastEvent := make(map[netip.Addr]time.Time, len(r.profiles))
	var nextEvent, nextWarning time.Time
	for ctx.Err() == nil {
		size, remote, err := r.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		source := remote.Addr().WithZone("").Unmap()
		peer, allowed := r.profiles[source]
		now := time.Now()
		if !allowed || size > maxTrapBytes || now.Before(nextEvent) || now.Before(lastEvent[source]) || !enabled() {
			continue
		}
		lastEvent[source] = now.Add(time.Second)
		observation, proof, valid := peer.observation(buf[:size], source, now.UTC())
		if !valid {
			continue
		}
		nextEvent = now.Add(125 * time.Millisecond)
		if err := appendEvent(observation, proof); err != nil && now.After(nextWarning) {
			nextWarning = now.Add(time.Minute)
			onStorageError()
		}
	}
	return nil
}
