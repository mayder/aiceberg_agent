package monitoring

import "errors"

// TransportProfile is caller-resolved compatibility evidence, never telemetry authority.
// All participants (producer, optional relays, consumer) must be explicitly supplied.
type TransportProfile struct {
	OpaqueJSON bool     `json:"opaque_json"`
	Contract   string   `json:"contract"`
	Sources    []string `json:"sources"`
	ValueTypes []string `json:"value_types"`
	MaxBytes   int      `json:"max_bytes"`
	MaxRecords int      `json:"max_records"`
}

// NegotiateRecordsTransport intersects 2..8 profiles without mutating any input.
// Missing/old consumers fail closed; no lossy downgrade, split or implicit defaults.
func NegotiateRecordsTransport(hops []TransportProfile) (TransportProfile, error) {
	if len(hops) < 2 || len(hops) > 8 {
		return TransportProfile{}, errors.New("invalid_monitoring_transport_profile")
	}
	for _, hop := range hops {
		if err := validateTransportProfile(hop); err != nil {
			return TransportProfile{}, err
		}
	}
	selected := hops[0]
	selected.Sources = append([]string{}, selected.Sources...)
	selected.ValueTypes = append([]string{}, selected.ValueTypes...)
	for _, hop := range hops[1:] {
		selected.Sources = transportIntersection(selected.Sources, hop.Sources)
		selected.ValueTypes = transportIntersection(selected.ValueTypes, hop.ValueTypes)
		selected.MaxBytes = min(selected.MaxBytes, hop.MaxBytes)
		selected.MaxRecords = min(selected.MaxRecords, hop.MaxRecords)
	}
	if len(selected.Sources) == 0 || len(selected.ValueTypes) == 0 {
		return TransportProfile{}, errors.New("incompatible_monitoring_transport")
	}
	return selected, nil
}

func validateTransportProfile(p TransportProfile) error {
	sources := []string{"snmp", "icmp", "tcp", "http", "dns", "tls", "host", "custom", "derived"}
	types := []string{"number", "integer", "boolean", "text", "event"}
	if !p.OpaqueJSON || p.Contract != "monitoring.records.v1" || p.MaxBytes < 1 || p.MaxBytes > MaxRecordBytes || p.MaxRecords < 1 || p.MaxRecords > MaxRecords || !transportSet(p.Sources, sources) || !transportSet(p.ValueTypes, types) {
		return errors.New("invalid_monitoring_transport_profile")
	}
	return nil
}

func transportSet(values, allowed []string) bool {
	if len(values) == 0 || len(values) > len(allowed) {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] || !definitionEnum(value, allowed...) {
			return false
		}
		seen[value] = true
	}
	return true
}

func transportIntersection(left, right []string) []string {
	out := []string{}
	for _, value := range left {
		if definitionEnum(value, right...) {
			out = append(out, value)
		}
	}
	return out
}

func validateTransportBatch(b RecordsBatch, d Definition, p TransportProfile) error {
	if len(b.Records) > p.MaxRecords {
		return errors.New("monitoring_transport_limit")
	}
	sources := map[string]string{}
	for _, item := range d.Items {
		sources[item.Key] = item.Source
	}
	for _, record := range b.Records {
		if !definitionEnum(sources[record.ItemKey], p.Sources...) || !definitionEnum(record.ValueType, p.ValueTypes...) {
			return errors.New("incompatible_monitoring_transport")
		}
	}
	return nil
}
