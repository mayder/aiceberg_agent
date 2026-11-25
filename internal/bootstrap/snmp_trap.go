package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	agentlessstore "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

func startSNMPTrapReceiver(ctx context.Context, cfg config.Config, log logger.Logger, client *remote.AgentlessHubClient,
	settings func() usecase.AgentlessSettings,
) func() {
	listen := os.Getenv("AICEBERG_SNMP_TRAP_LISTEN")
	file := os.Getenv("AICEBERG_SNMP_TRAP_PROFILES_FILE")
	noop := func() {}
	if listen == "" && file == "" {
		return noop
	}
	addr, err := netip.ParseAddrPort(listen)
	if err != nil || addr.Port() == 0 || cfg.AgentlessOutboxPath == "" {
		log.Error(logger.KV("SNMP trap receiver unavailable", "code", "snmp_trap_configuration_invalid"))
		return noop
	}
	raw, err := safefile.ReadPrivate(file, 128*1024)
	if err != nil {
		log.Error(logger.KV("SNMP trap receiver unavailable", "code", "snmp_trap_profiles_unavailable"))
		return noop
	}
	defer clear(raw)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var profiles []agentless.SNMPTrapProfile
	if decoder.Decode(&profiles) != nil || decoder.Decode(new(any)) != io.EOF {
		log.Error(logger.KV("SNMP trap receiver unavailable", "code", "snmp_trap_configuration_invalid"))
		return noop
	}
	receiver, err := agentless.ListenSNMPTraps(listen, profiles)
	if err != nil {
		log.Error(logger.KV("SNMP trap receiver unavailable", "code", "snmp_trap_listen_failed"))
		return noop
	}
	// Keep rejected or undelivered traps out of routine batches and their storage budget.
	store, err := agentlessstore.NewBoltStore(cfg.AgentlessOutboxPath+".snmp-traps", max(1, cfg.AgentlessOutboxMaxMB/4))
	if err != nil {
		_ = receiver.Close()
		log.Error(logger.KV("SNMP trap receiver unavailable", "code", "snmp_trap_outbox_unavailable"))
		return noop
	}
	runCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		if err := receiver.Serve(runCtx, func() bool { return settings().Enabled }, store.PushSNMPTrap, func() {
			log.Error(logger.KV("SNMP trap storage unavailable", "code", "snmp_trap_outbox_unavailable"))
		}); err != nil {
			log.Error(logger.KV("SNMP trap receiver stopped", "code", "snmp_trap_read_failed"))
		}
	}()
	cfg.AgentlessDeliveryPriority = true
	flusher := usecase.NewAgentlessHub(cfg, log, client, repositories.NewAgentlessOutboxRepository(store), settings, nil)
	go func() {
		defer workers.Done()
		flushSNMPTraps(runCtx, log, flusher, store, settings)
	}()
	return func() { cancel(); workers.Wait(); _ = store.Close() }
}

func flushSNMPTraps(ctx context.Context, log logger.Logger, flusher *usecase.AgentlessHub, store *agentlessstore.BoltStore,
	settings func() usecase.AgentlessSettings,
) {
	flush := time.NewTicker(5 * time.Second)
	defer flush.Stop()
	var nextPrune time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-flush.C:
			if !now.Before(nextPrune) {
				if removed, err := store.Prune(24 * time.Hour); err == nil && removed > 0 {
					log.Info(logger.KV("Expired SNMP traps discarded", "count", removed))
				}
				nextPrune = now.Add(time.Minute)
			}
			if settings().Enabled {
				_ = flusher.Flush(ctx)
			}
		}
	}
}
