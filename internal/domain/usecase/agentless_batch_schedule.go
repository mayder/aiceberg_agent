package usecase

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

type agentlessBatchSchedule struct {
	tenants         []*agentlessBatchTenant
	cursor          int
	share           int
	waiting         int
	admission       *agentlessProbeAdmission
	manual          bool
	wakeAt          time.Time
	circuitDeferred int
	budgetDeferred  int
	remaining       int
	reserved        int
	allowOversize   bool
}

type agentlessBatchTenant struct {
	destinations []*agentlessBatchDestination
	cursor       int
	active       int
	queued       int
}

type agentlessBatchDestination struct {
	key    agentlessDestinationKey
	jobs   []agentlessBatchJob
	cursor int
	active bool
}

type agentlessBatchJob struct {
	job  entities.AgentlessJob
	cost int
}

type agentlessBatchWork struct {
	tenant      *agentlessBatchTenant
	destination *agentlessBatchDestination
	job         entities.AgentlessJob
}

type agentlessDestinationKey struct {
	clientID   int
	host       string
	endpointID int
	assetID    int
}

func newAgentlessBatchSchedule(jobs []entities.AgentlessJob, workers, budget int) *agentlessBatchSchedule {
	schedule := &agentlessBatchSchedule{share: (workers + 1) / 2, remaining: budget}
	tenants := make(map[int]*agentlessBatchTenant)
	destinations := make(map[agentlessDestinationKey]*agentlessBatchDestination)
	for _, job := range jobs {
		tenant := tenants[job.ClienteID]
		if tenant == nil {
			tenant = &agentlessBatchTenant{}
			tenants[job.ClienteID] = tenant
			schedule.tenants = append(schedule.tenants, tenant)
		}
		key := agentlessDestination(job)
		destination := destinations[key]
		if destination == nil {
			destination = &agentlessBatchDestination{key: key}
			destinations[key] = destination
			tenant.destinations = append(tenant.destinations, destination)
		}
		destination.jobs = append(destination.jobs, agentlessBatchJob{job: job, cost: agentless.ObservationCost(job)})
		tenant.queued++
	}
	schedule.waiting = len(schedule.tenants)
	return schedule
}

func (s *agentlessBatchSchedule) take(now time.Time) (agentlessBatchWork, bool) {
	s.wakeAt = time.Time{}
	if s.remaining == 0 {
		return agentlessBatchWork{}, false
	}
	for offset := range s.tenants {
		index := (s.cursor + offset) % len(s.tenants)
		tenant := s.tenants[index]
		if tenant.queued == 0 || (s.waiting > 1 && tenant.active >= s.share) {
			continue
		}
		destination := s.nextDestination(tenant, now)
		if destination == nil {
			continue
		}
		selected := destination.jobs[destination.cursor]
		work := agentlessBatchWork{tenant: tenant, destination: destination, job: selected.job}
		cost := selected.cost
		s.reserved += cost
		s.remaining = max(0, s.remaining-cost)
		destination.cursor++
		destination.active = true
		tenant.active++
		s.advanceTenant(tenant)
		s.cursor = (index + 1) % len(s.tenants)
		return work, true
	}
	return agentlessBatchWork{}, false
}

func (s *agentlessBatchSchedule) nextDestination(tenant *agentlessBatchTenant, now time.Time) *agentlessBatchDestination {
	for offset := range tenant.destinations {
		index := (tenant.cursor + offset) % len(tenant.destinations)
		destination := tenant.destinations[index]
		if destination.active {
			continue
		}
		s.skipOpenCircuit(tenant, destination, now)
		if destination.cursor == len(destination.jobs) {
			continue
		}
		if destination.jobs[destination.cursor].cost > s.remaining &&
			(!s.allowOversize || s.reserved != 0) {
			continue
		}
		allowed, allowedAt := s.admission.reserveRate(destination.key, now)
		if !allowed {
			if !allowedAt.IsZero() && (s.wakeAt.IsZero() || allowedAt.Before(s.wakeAt)) {
				s.wakeAt = allowedAt
			}
			continue
		}
		tenant.cursor = (index + 1) % len(tenant.destinations)
		return destination
	}
	return nil
}

func (s *agentlessBatchSchedule) skipOpenCircuit(tenant *agentlessBatchTenant, destination *agentlessBatchDestination, now time.Time) {
	if s.admission == nil || s.manual {
		return
	}
	for destination.cursor < len(destination.jobs) {
		job := destination.jobs[destination.cursor].job
		circuit := agentlessCircuit(job, destination.key)
		if s.admission.circuitOpen(circuit, now) {
			s.circuitDeferred++
		} else {
			circuit.checkID = job.CheckID
			if circuit.checkID <= 0 || !s.admission.circuitOpen(circuit, now) {
				return
			}
			s.budgetDeferred++
		}
		destination.cursor++
		s.advanceTenant(tenant)
	}
}

func (s *agentlessBatchSchedule) advanceTenant(tenant *agentlessBatchTenant) {
	tenant.queued--
	if tenant.queued == 0 {
		s.waiting--
	}
}

func (s *agentlessBatchSchedule) release(work agentlessBatchWork) {
	work.destination.active = false
	work.tenant.active--
}

func agentlessDestination(job entities.AgentlessJob) agentlessDestinationKey {
	key := agentlessDestinationKey{clientID: job.ClienteID, assetID: job.AtivoID}
	if job.Endpoint == nil {
		return key
	}
	key.endpointID = job.Endpoint.ID
	host := strings.TrimSpace(job.Endpoint.Endereco)
	if host == "" {
		return key
	}
	if effective, _, _ := agentless.ActiveProbeTarget(job); effective != "" {
		host = strings.TrimSpace(effective)
	} else if strings.Contains(host, "://") {
		endpoint, err := url.Parse(host)
		if err != nil || endpoint.Hostname() == "" {
			return key
		}
		host = endpoint.Hostname()
	} else if address, _, err := net.SplitHostPort(host); err == nil {
		host = address
	}
	if address, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		host = address.Unmap().String()
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return key
	}
	return agentlessDestinationKey{clientID: job.ClienteID, host: host}
}
