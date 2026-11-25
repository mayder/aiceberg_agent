package usecase

import "time"

const agentlessRateKeysPerClient = 512

// A zero time holds one pacing slot for a dispatched worker. Completion
// replaces it with the actual start's next permitted time or releases it.
func (p *agentlessProbeAdmission) reserveRate(destination agentlessDestinationKey, now time.Time) (bool, time.Time) {
	if p == nil {
		return true, time.Time{}
	}
	if due, tracked := p.nextStart[destination]; tracked {
		if due.IsZero() || now.Before(due) {
			return false, due
		}
		p.nextStart[destination] = time.Time{}
		return true, time.Time{}
	}
	clientID := destination.clientID
	if p.rateCapacityExceeded(clientID) {
		p.pruneRates(now)
		if p.rateCapacityExceeded(clientID) {
			return false, p.nextRateVacancy(clientID)
		}
	}
	p.nextStart[destination] = time.Time{}
	p.ratesByClient[clientID]++
	return true, time.Time{}
}

func (p *agentlessProbeAdmission) finishRate(destination agentlessDestinationKey, started, now time.Time) {
	if p == nil {
		return
	}
	due := started.Add(agentlessProbeSpacing)
	if started.IsZero() || !now.Before(due) {
		p.removeRate(destination)
		return
	}
	p.nextStart[destination] = due
}

func (p *agentlessProbeAdmission) rateCapacityExceeded(clientID int) bool {
	return len(p.nextStart) >= agentlessAdmissionMaxKeys || p.ratesByClient[clientID] >= agentlessRateKeysPerClient
}

func (p *agentlessProbeAdmission) nextRateVacancy(clientID int) time.Time {
	clientFull := p.ratesByClient[clientID] >= agentlessRateKeysPerClient
	var next time.Time
	for key, due := range p.nextStart {
		if due.IsZero() || (clientFull && key.clientID != clientID) {
			continue
		}
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return next
}

func (p *agentlessProbeAdmission) pruneRates(now time.Time) {
	for destination, due := range p.nextStart {
		if !due.IsZero() && !now.Before(due) {
			p.removeRate(destination)
		}
	}
}

func (p *agentlessProbeAdmission) removeRate(destination agentlessDestinationKey) {
	delete(p.nextStart, destination)
	clientID := destination.clientID
	p.ratesByClient[clientID]--
	if p.ratesByClient[clientID] == 0 {
		delete(p.ratesByClient, clientID)
	}
}
