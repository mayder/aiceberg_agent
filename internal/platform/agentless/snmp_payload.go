package agentless

import (
	"encoding/json"
	"fmt"
	"strings"
)

type snmpStats struct {
	GetAttempts  int `json:"get_attempts"`
	GetSuccess   int `json:"get_success"`
	WalkAttempts int `json:"walk_attempts"`
	WalkRows     int `json:"walk_rows"`
}

type snmpGroupStats struct {
	GetAttempts  int      `json:"get_attempts"`
	GetSuccess   int      `json:"get_success"`
	WalkAttempts int      `json:"walk_attempts"`
	WalkRows     int      `json:"walk_rows"`
	LatencyMs    int      `json:"latency_ms,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

type snmpPayload struct {
	Host               string                      `json:"host"`
	ProfileID          int                         `json:"profile_id"`
	CollectionProfile  string                      `json:"collection_profile"`
	CollectionKind     string                      `json:"collection_kind"`
	FetchMode          string                      `json:"fetch_mode"`
	TimeBudgetMs       int                         `json:"time_budget_ms"`
	TimeBudgetExceeded bool                        `json:"time_budget_exceeded"`
	GroupsRequested    []string                    `json:"groups_requested"`
	Stats              snmpStats                   `json:"stats"`
	GroupStats         map[string]snmpGroupStats   `json:"group_stats"`
	Errors             []string                    `json:"errors"`
	Sys                map[string]any              `json:"sys"`
	Ifaces             []map[string]any            `json:"ifaces"`
	Scalars            map[string]any              `json:"scalars"`
	Tables             map[string][]map[string]any `json:"tables"`
	Custom             map[string][]map[string]any `json:"custom"`
	OIDs               map[string]any              `json:"oids,omitempty"`
}

func newSNMPPayload(plan snmpPlan) *snmpPayload {
	return &snmpPayload{
		Host:              plan.Host,
		ProfileID:         plan.ProfileID,
		CollectionProfile: plan.CollectionProfile,
		CollectionKind:    plan.CollectionKind,
		FetchMode:         string(plan.FetchMode),
		TimeBudgetMs:      plan.TimeBudgetMs,
		GroupsRequested:   cloneStringSlice(plan.Groups),
		GroupStats:        make(map[string]snmpGroupStats),
		Errors:            []string{},
		Sys:               map[string]any{},
		Ifaces:            []map[string]any{},
		Scalars:           map[string]any{},
		Tables:            map[string][]map[string]any{},
		Custom: map[string][]map[string]any{
			"get":  {},
			"walk": {},
		},
		OIDs: map[string]any{},
	}
}

func (s *snmpStats) AddGroup(g snmpGroupStats) {
	s.GetAttempts += g.GetAttempts
	s.GetSuccess += g.GetSuccess
	s.WalkAttempts += g.WalkAttempts
	s.WalkRows += g.WalkRows
}

func (p *snmpPayload) addGroupStats(group string, g snmpGroupStats) {
	p.GroupStats[group] = g
	p.Stats.AddGroup(g)
}

func (p *snmpPayload) addError(err string) {
	err = strings.TrimSpace(err)
	if err == "" {
		return
	}
	p.Errors = append(p.Errors, err)
}

func (p *snmpPayload) toMap() map[string]any {
	raw, _ := json.Marshal(p)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func evaluateSNMPStatus(p *snmpPayload) (status, code, message string) {
	if p.Stats.GetSuccess > 0 || p.Stats.WalkRows > 0 {
		return "ok", "", "SNMP OK"
	}
	if p.TimeBudgetExceeded {
		return "fail", "snmp_time_budget_exceeded", "Coleta SNMP interrompida: budget excedido"
	}
	for _, msg := range p.Errors {
		l := strings.ToLower(msg)
		switch {
		case strings.Contains(l, "timeout"), strings.Contains(l, "no response"), strings.Contains(l, "i/o timeout"):
			return "fail", "snmp_timeout", fmt.Sprintf("SNMP sem resposta (timeout). Detalhe: %s", trimString(msg, 180))
		case strings.Contains(l, "authentication"), strings.Contains(l, "authorization"), strings.Contains(l, "unknown username"), strings.Contains(l, "wrongdigest"), strings.Contains(l, "decryption"):
			return "fail", "snmp_auth", fmt.Sprintf("Falha de credencial/versao SNMP. Detalhe: %s", trimString(msg, 180))
		case strings.Contains(l, "connection refused"):
			return "fail", "snmp_refused", fmt.Sprintf("SNMP desabilitado ou porta bloqueada. Detalhe: %s", trimString(msg, 180))
		case strings.Contains(l, "tabela sem retorno"):
			return "fail", "snmp_table_empty", fmt.Sprintf("Tabela SNMP sem retorno. Detalhe: %s", trimString(msg, 180))
		}
	}
	if len(p.Errors) > 0 {
		return "fail", "snmp_fail", trimString(p.Errors[0], 255)
	}
	return "fail", "snmp_no_data", "SNMP sem dados uteis"
}
