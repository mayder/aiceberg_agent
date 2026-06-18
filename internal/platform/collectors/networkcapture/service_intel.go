package networkcapture

import (
	"net"
	"sort"
	"strconv"
	"strings"
)

type serviceMapPayload struct {
	Enabled      bool                   `json:"enabled"`
	Source       string                 `json:"source,omitempty"`
	Services     []discoveredServiceRow `json:"services,omitempty"`
	Dependencies []serviceDependencyRow `json:"dependencies,omitempty"`
	SystemProbe  systemProbeStatus      `json:"system_probe"`
	Privacy      privacyPolicyRow       `json:"privacy"`
}

type discoveredServiceRow struct {
	Service     string   `json:"service,omitempty"`
	Env         string   `json:"env,omitempty"`
	Version     string   `json:"version,omitempty"`
	Status      string   `json:"status,omitempty"`
	Confidence  string   `json:"confidence,omitempty"`
	Source      string   `json:"source,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
	ListenPorts []uint32 `json:"listen_ports,omitempty"`
	Processes   []string `json:"processes,omitempty"`
}

type serviceDependencyRow struct {
	SourceService string   `json:"source_service,omitempty"`
	TargetService string   `json:"target_service,omitempty"`
	TargetType    string   `json:"target_type,omitempty"`
	RemoteIP      string   `json:"remote_ip,omitempty"`
	RemotePort    uint32   `json:"remote_port,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	Direction     string   `json:"direction,omitempty"`
	Confidence    string   `json:"confidence,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
	Samples       int      `json:"samples,omitempty"`
	BytesIn       int      `json:"bytes_in,omitempty"`
	BytesOut      int      `json:"bytes_out,omitempty"`
	ResetHits     int      `json:"reset_hits,omitempty"`
	TimeoutHits   int      `json:"timeout_hits,omitempty"`
}

type systemProbeStatus struct {
	RequestedMode string              `json:"requested_mode,omitempty"`
	AppliedMode   string              `json:"applied_mode,omitempty"`
	EBPFSupported bool                `json:"ebpf_supported"`
	EBPFActive    bool                `json:"ebpf_active"`
	Fallback      string              `json:"fallback,omitempty"`
	Reason        string              `json:"reason,omitempty"`
	Capabilities  passiveCapabilities `json:"capabilities"`
}

type privacyPolicyRow struct {
	Redaction       []string `json:"redaction,omitempty"`
	Retention       string   `json:"retention,omitempty"`
	DestructiveMode bool     `json:"destructive_mode"`
}

type networkPerfPayload struct {
	LocalHostOnly     bool               `json:"local_host_only"`
	States            map[string]int     `json:"states,omitempty"`
	TopTalkers        []topTalkerRow     `json:"top_talkers,omitempty"`
	ExposedAdminPorts []adminExposureRow `json:"exposed_admin_ports,omitempty"`
}

type topTalkerRow struct {
	RemoteIP    string `json:"remote_ip,omitempty"`
	RemotePort  uint32 `json:"remote_port,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Samples     int    `json:"samples,omitempty"`
	BytesIn     int    `json:"bytes_in,omitempty"`
	BytesOut    int    `json:"bytes_out,omitempty"`
	ResetHits   int    `json:"reset_hits,omitempty"`
	TimeoutHits int    `json:"timeout_hits,omitempty"`
}

type adminExposureRow struct {
	Service    string `json:"service,omitempty"`
	RemoteIP   string `json:"remote_ip,omitempty"`
	RemotePort uint32 `json:"remote_port,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
}

type workloadSecurityPayload struct {
	Enabled           bool                `json:"enabled"`
	RuleVersion       string              `json:"rule_version"`
	Signals           []securitySignalRow `json:"signals,omitempty"`
	DestructiveAction bool                `json:"destructive_action"`
}

type securitySignalRow struct {
	RuleID     string   `json:"rule_id,omitempty"`
	Severity   string   `json:"severity,omitempty"`
	Category   string   `json:"category,omitempty"`
	Service    string   `json:"service,omitempty"`
	Process    string   `json:"process,omitempty"`
	RemoteIP   string   `json:"remote_ip,omitempty"`
	RemotePort uint32   `json:"remote_port,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
}

func buildServiceMapPayload(flows []flowRow, listeners []listenerRow, advanced *passiveAdvancedPayload, options passiveCollectOptions) *serviceMapPayload {
	services := make(map[string]*discoveredServiceRow)
	ensureService := func(name, status, source string, evidence []string) *discoveredServiceRow {
		service := sanitizeServiceToken(name)
		if service == "" {
			service = "unknown"
		}
		row, ok := services[service]
		if !ok {
			row = &discoveredServiceRow{
				Service:    service,
				Status:     status,
				Confidence: confidenceForService(service, source),
				Source:     source,
				Evidence:   uniqueStrings(evidence, 6),
			}
			services[service] = row
			return row
		}
		row.Evidence = uniqueStrings(append(row.Evidence, evidence...), 6)
		if row.Status == "unknown" && status != "" {
			row.Status = status
		}
		if row.Source == "" || row.Source == "traffic" {
			row.Source = source
		}
		if row.Confidence == "unknown" && service != "unknown" {
			row.Confidence = confidenceForService(service, source)
		}
		return row
	}

	for _, listener := range listeners {
		service := serviceFromLocalEndpoint(listener.ServiceName, listener.Process, listener.LocalPort)
		row := ensureService(service, "confirmed", "listener", []string{
			"listener:" + listener.Protocol + "/" + strconv.FormatUint(uint64(listener.LocalPort), 10),
		})
		if listener.LocalPort > 0 {
			row.ListenPorts = appendUniquePort(row.ListenPorts, listener.LocalPort, 16)
		}
		if p := sanitizeProcessName(listener.Process); p != "" {
			row.Processes = uniqueStrings(append(row.Processes, p), 8)
		}
	}

	depsByKey := make(map[string]*serviceDependencyRow)
	for _, flow := range flows {
		sourceService := serviceFromLocalEndpoint(flow.ServiceName, flow.Process, flow.LocalPort)
		source := ensureService(sourceService, "inferred", "traffic", []string{
			"flow:" + flow.Protocol + "/" + strconv.FormatUint(uint64(flow.RemotePort), 10),
		})
		if p := sanitizeProcessName(flow.Process); p != "" {
			source.Processes = uniqueStrings(append(source.Processes, p), 8)
		}
		targetService, targetType := serviceFromRemoteEndpoint(flow)
		ensureService(targetService, "inferred", "traffic", []string{
			"remote:" + flow.Protocol + "/" + strconv.FormatUint(uint64(flow.RemotePort), 10),
		})
		key := source.Service + "|" + targetService + "|" + flow.Protocol + "|" + strconv.FormatUint(uint64(flow.RemotePort), 10)
		dep, ok := depsByKey[key]
		if !ok {
			dep = &serviceDependencyRow{
				SourceService: source.Service,
				TargetService: targetService,
				TargetType:    targetType,
				RemoteIP:      maskPublicAddress(flow.RemoteIP),
				RemotePort:    flow.RemotePort,
				Protocol:      flow.Protocol,
				Direction:     flow.Direction,
				Confidence:    confidenceForDependency(flow),
				Evidence:      []string{"socket_snapshot"},
			}
			depsByKey[key] = dep
		}
		dep.Samples += flow.Samples
		dep.BytesIn += flow.BytesIn
		dep.BytesOut += flow.BytesOut
		dep.ResetHits += flow.ResetHits
		dep.TimeoutHits += flow.TimeoutHits
		if flow.DNSQuery != "" {
			dep.Evidence = uniqueStrings(append(dep.Evidence, "dns:"+sanitizeHostToken(flow.DNSQuery)), 6)
		}
	}

	serviceRows := make([]discoveredServiceRow, 0, len(services))
	for _, row := range services {
		sort.Slice(row.ListenPorts, func(i, j int) bool { return row.ListenPorts[i] < row.ListenPorts[j] })
		serviceRows = append(serviceRows, *row)
	}
	sort.Slice(serviceRows, func(i, j int) bool {
		if serviceRows[i].Confidence == serviceRows[j].Confidence {
			return serviceRows[i].Service < serviceRows[j].Service
		}
		return serviceRows[i].Confidence < serviceRows[j].Confidence
	})

	deps := make([]serviceDependencyRow, 0, len(depsByKey))
	for _, dep := range depsByKey {
		deps = append(deps, *dep)
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Samples == deps[j].Samples {
			return deps[i].SourceService+deps[i].TargetService < deps[j].SourceService+deps[j].TargetService
		}
		return deps[i].Samples > deps[j].Samples
	})
	if len(deps) > 200 {
		deps = deps[:200]
	}

	return &serviceMapPayload{
		Enabled:      options.advancedEnabled || options.usmEnabled,
		Source:       "network_capture",
		Services:     serviceRows,
		Dependencies: deps,
		SystemProbe:  buildSystemProbeStatus(advanced, options),
		Privacy: privacyPolicyRow{
			Redaction:       []string{"command_args", "username", "absolute_path", "public_ip_suffix"},
			Retention:       "mesma retencao do payload network_capture no backend",
			DestructiveMode: false,
		},
	}
}

func buildSystemProbeStatus(advanced *passiveAdvancedPayload, options passiveCollectOptions) systemProbeStatus {
	status := systemProbeStatus{
		RequestedMode: normalizePassiveMode(options.requestedMode),
		Fallback:      "socket_snapshot",
		Reason:        "fallback seguro sem eBPF ativo",
	}
	if advanced != nil {
		status.AppliedMode = advanced.AppliedMode
		status.EBPFSupported = advanced.Capabilities.EBPF
		status.Capabilities = advanced.Capabilities
	}
	if status.AppliedMode == "" {
		status.AppliedMode = "socket_snapshot"
	}
	if strings.Contains(status.AppliedMode, "ebpf_probe") {
		status.EBPFActive = true
		status.Fallback = ""
		status.Reason = "system probe habilitado e suportado"
	}
	return status
}

func buildNetworkPerfPayload(flows []flowRow) *networkPerfPayload {
	out := &networkPerfPayload{
		LocalHostOnly: true,
		States:        make(map[string]int),
	}
	for _, flow := range flows {
		state := strings.ToUpper(strings.TrimSpace(flow.State))
		if state == "" {
			state = "UNKNOWN"
		}
		out.States[state] += flow.Samples
		out.TopTalkers = append(out.TopTalkers, topTalkerRow{
			RemoteIP:    maskPublicAddress(flow.RemoteIP),
			RemotePort:  flow.RemotePort,
			Protocol:    flow.Protocol,
			Samples:     flow.Samples,
			BytesIn:     flow.BytesIn,
			BytesOut:    flow.BytesOut,
			ResetHits:   flow.ResetHits,
			TimeoutHits: flow.TimeoutHits,
		})
		if _, ok := adminPorts[flow.RemotePort]; ok && (flow.RemoteScope == "public" || flow.RemoteScope == "unknown" || flow.RemoteScope == "") {
			out.ExposedAdminPorts = append(out.ExposedAdminPorts, adminExposureRow{
				Service:    sanitizeServiceToken(serviceFromLocalEndpoint(flow.ServiceName, flow.Process, flow.LocalPort)),
				RemoteIP:   maskPublicAddress(flow.RemoteIP),
				RemotePort: flow.RemotePort,
				Scope:      flow.RemoteScope,
				Evidence:   "network_capture_problematic_tag",
			})
		}
	}
	sort.Slice(out.TopTalkers, func(i, j int) bool {
		if out.TopTalkers[i].Samples == out.TopTalkers[j].Samples {
			return out.TopTalkers[i].RemoteIP < out.TopTalkers[j].RemoteIP
		}
		return out.TopTalkers[i].Samples > out.TopTalkers[j].Samples
	})
	if len(out.TopTalkers) > 20 {
		out.TopTalkers = out.TopTalkers[:20]
	}
	if len(out.ExposedAdminPorts) > 50 {
		out.ExposedAdminPorts = out.ExposedAdminPorts[:50]
	}
	return out
}

func buildWorkloadSecurityPayload(flows []flowRow) *workloadSecurityPayload {
	out := &workloadSecurityPayload{
		Enabled:           true,
		RuleVersion:       "network-workload-v1",
		DestructiveAction: false,
	}
	for _, flow := range flows {
		signals := classifyWorkloadSignals(flow)
		out.Signals = append(out.Signals, signals...)
	}
	sort.Slice(out.Signals, func(i, j int) bool {
		if out.Signals[i].Severity == out.Signals[j].Severity {
			return out.Signals[i].RuleID < out.Signals[j].RuleID
		}
		return severityRank(out.Signals[i].Severity) > severityRank(out.Signals[j].Severity)
	})
	if len(out.Signals) > 100 {
		out.Signals = out.Signals[:100]
	}
	return out
}

func classifyWorkloadSignals(flow flowRow) []securitySignalRow {
	signals := make([]securitySignalRow, 0, 3)
	service := sanitizeServiceToken(serviceFromLocalEndpoint(flow.ServiceName, flow.Process, flow.LocalPort))
	processName := sanitizeProcessName(flow.Process)
	base := securitySignalRow{
		Service:    service,
		Process:    processName,
		RemoteIP:   maskPublicAddress(flow.RemoteIP),
		RemotePort: flow.RemotePort,
	}
	if _, ok := adminPorts[flow.RemotePort]; ok && (flow.RemoteScope == "public" || flow.RemoteScope == "unknown" || flow.RemoteScope == "") {
		s := base
		s.RuleID = "network.admin_port_public"
		s.Severity = "high"
		s.Category = "suspicious_external_connection"
		s.Evidence = []string{"admin_port", "scope:" + defaultString(flow.RemoteScope, "unknown")}
		signals = append(signals, s)
	}
	for _, tag := range flow.RiskTags {
		if tag == "failed_handshake" || tag == "dns_instability" || tag == "unknown_reputation_ip" {
			s := base
			s.RuleID = "network." + tag
			s.Severity = "medium"
			s.Category = "network_anomaly"
			s.Evidence = uniqueStrings([]string{tag, sanitizeHostToken(flow.DNSQuery), flow.State}, 4)
			signals = append(signals, s)
		}
	}
	if isSuspiciousProcessName(processName) && flow.RemoteScope == "public" {
		s := base
		s.RuleID = "process.suspicious_public_connection"
		s.Severity = "medium"
		s.Category = "suspicious_process"
		s.Evidence = []string{"process_name", "public_destination"}
		signals = append(signals, s)
	}
	return signals
}

func serviceFromLocalEndpoint(serviceName, processName string, port uint32) string {
	if service := sanitizeServiceToken(serviceName); service != "" {
		return service
	}
	if process := sanitizeProcessName(processName); process != "" {
		return process
	}
	if inferred := serviceNameFromPort(port); inferred != "" {
		return inferred
	}
	return "unknown"
}

func serviceFromRemoteEndpoint(flow flowRow) (string, string) {
	if host := sanitizeHostToken(flow.DNSQuery); host != "" {
		return host, targetTypeForPort(flow.RemotePort)
	}
	if host := sanitizeHostToken(flow.SNI); host != "" {
		return host, targetTypeForPort(flow.RemotePort)
	}
	if host := sanitizeHostToken(flow.ReverseDNS); host != "" {
		return host, targetTypeForPort(flow.RemotePort)
	}
	if inferred := serviceNameFromPort(flow.RemotePort); inferred != "" {
		return inferred, targetTypeForPort(flow.RemotePort)
	}
	return "unknown", "unknown"
}

func serviceNameFromPort(port uint32) string {
	switch port {
	case 53, 853:
		return "dns"
	case 80, 8080, 8081, 8000:
		return "http"
	case 443, 8443, 9443:
		return "https"
	case 5432:
		return "postgresql"
	case 3306:
		return "mysql"
	case 1433:
		return "sqlserver"
	case 6379:
		return "redis"
	case 5672, 15672:
		return "rabbitmq"
	case 27017:
		return "mongodb"
	case 9200:
		return "elasticsearch"
	default:
		return ""
	}
}

func targetTypeForPort(port uint32) string {
	switch port {
	case 5432, 3306, 1433, 1521, 27017:
		return "database"
	case 6379:
		return "cache"
	case 5672, 15672:
		return "queue"
	case 80, 443, 8080, 8081, 8443, 9443:
		return "service"
	case 53, 853:
		return "dns"
	default:
		return "service"
	}
}

func confidenceForService(service, source string) string {
	if strings.TrimSpace(service) == "" || service == "unknown" {
		return "unknown"
	}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "listener":
		return "confirmed"
	default:
		return "inferred"
	}
}

func confidenceForDependency(flow flowRow) string {
	if flow.Samples >= 2 && (flow.DNSQuery != "" || flow.SNI != "" || flow.ReverseDNS != "") {
		return "confirmed"
	}
	if flow.RemotePort > 0 || flow.RemoteIP != "" {
		return "inferred"
	}
	return "unknown"
}

func appendUniquePort(ports []uint32, port uint32, limit int) []uint32 {
	if port == 0 {
		return ports
	}
	for _, existing := range ports {
		if existing == port {
			return ports
		}
	}
	if limit > 0 && len(ports) >= limit {
		return ports
	}
	return append(ports, port)
}

func sanitizeServiceToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, ".service")
	value = strings.Trim(value, ".-_ ")
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
		if b.Len() >= maxServiceNameLen {
			break
		}
	}
	return strings.Trim(b.String(), ".-_")
}

func sanitizeProcessName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if i := strings.LastIndexAny(value, "/\\"); i >= 0 && i < len(value)-1 {
		value = value[i+1:]
	}
	return sanitizeServiceToken(value)
}

func sanitizeHostToken(value string) string {
	value = normalizeReverseDNSName(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, "/\\ ") {
		return ""
	}
	if len(value) > maxResolutionLen {
		value = value[:maxResolutionLen]
	}
	return value
}

func maskPublicAddress(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return ip.String()
	}
	if v4 := ip.To4(); v4 != nil {
		return net.IPv4(v4[0], v4[1], v4[2], 0).String() + "/24"
	}
	return ip.String()
}

func isSuspiciousProcessName(processName string) bool {
	switch sanitizeProcessTokenForMatch(processName) {
	case "nc", "netcat", "ncat", "socat", "cryptominer", "xmrig", "mimikatz":
		return true
	default:
		return false
	}
}

func sanitizeProcessTokenForMatch(value string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".-_ ")
}

func severityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
