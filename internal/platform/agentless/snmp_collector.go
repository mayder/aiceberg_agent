package agentless

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

var (
	errSNMPBudgetExceeded = errors.New("snmp budget exceeded")
	errSNMPMaxRowsReached = errors.New("snmp max rows reached")
)

type snmpClient interface {
	Connect() error
	Close() error
	Get(oids []string) (*gosnmp.SnmpPacket, error)
	GetNext(oids []string) (*gosnmp.SnmpPacket, error)
	BulkWalk(rootOid string, walkFn gosnmp.WalkFunc) error
	Walk(rootOid string, walkFn gosnmp.WalkFunc) error
}

type snmpClientFactory func(ctx context.Context, job entities.AgentlessJob, host string) (snmpClient, error)

var defaultSNMPClient snmpClientFactory = defaultSNMPClientFactory

type goSNMPClient struct {
	client    *gosnmp.GoSNMP
	basicWire *snmpBasicWire
}

func defaultSNMPClientFactory(ctx context.Context, job entities.AgentlessJob, host string) (snmpClient, error) {
	if job.SNMP == nil {
		return nil, errors.New("snmp_no_profile")
	}
	version, _, err := configuredSNMPVersion(job.SNMP.Version)
	if err != nil {
		return nil, err
	}
	port := job.SNMP.Port
	if port == 0 {
		port = 161
	}
	timeout := time.Millisecond * time.Duration(maxInt(job.SNMP.TimeoutMs, job.TimeoutMs))
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	sn := &gosnmp.GoSNMP{
		Target:         host,
		Port:           uint16(port),
		Timeout:        timeout,
		Retries:        maxInt(job.SNMP.Retries, job.Retries),
		MaxRepetitions: 25,
		Context:        ctx,
	}

	if version == gosnmp.Version3 {
		authProto := mapSNMPAuth(job.SNMP.V3AuthProtocol)
		privProto := mapSNMPPriv(job.SNMP.V3PrivProtocol)
		sn.Version = gosnmp.Version3
		sn.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 job.SNMP.V3User,
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: job.SNMP.V3AuthPassword,
			PrivacyProtocol:          privProto,
			PrivacyPassphrase:        job.SNMP.V3PrivPassword,
		}
		sn.SecurityModel = gosnmp.UserSecurityModel
		sn.ContextName = job.SNMP.ContextName
		if authProto == gosnmp.NoAuth {
			sn.MsgFlags = gosnmp.NoAuthNoPriv
		} else if privProto == gosnmp.NoPriv {
			sn.MsgFlags = gosnmp.AuthNoPriv
		} else {
			sn.MsgFlags = gosnmp.AuthPriv
		}
	} else {
		sn.Version = version
		sn.Community = job.SNMP.Community
	}

	return &goSNMPClient{client: sn, basicWire: attachSNMPBasicWire(sn, job.Config["snmp_basic_probe"] == true)}, nil
}

func (c *goSNMPClient) Connect() error {
	return c.client.Connect()
}

func (c *goSNMPClient) Close() error {
	if c.client == nil || c.client.Conn == nil {
		return nil
	}
	return c.client.Conn.Close()
}

func (c *goSNMPClient) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	packet, err := c.client.Get(oids)
	if err == nil && c.basicWire != nil && packet != nil && (packet.Community != c.client.Community || packet.RequestID == 0) {
		return nil, errSNMPBasicResponseInvalid
	}
	return packet, err
}

func (c *goSNMPClient) GetNext(oids []string) (*gosnmp.SnmpPacket, error) {
	return c.client.GetNext(oids)
}

func (c *goSNMPClient) BulkWalk(rootOid string, walkFn gosnmp.WalkFunc) error {
	return c.client.BulkWalk(rootOid, walkFn)
}

func (c *goSNMPClient) Walk(rootOid string, walkFn gosnmp.WalkFunc) error {
	return c.client.Walk(rootOid, walkFn)
}

type snmpCollector struct {
	ctx         context.Context
	v1Requests  int
	v1LastGet   time.Time
	job         entities.AgentlessJob
	plan        snmpPlan
	payload     *snmpPayload
	client      snmpClient
	started     time.Time
	budget      time.Duration
	ifaceMap    map[string]map[string]any
	partialSink snmpPartialSink
}

type snmpPartialResult struct {
	Group  string
	Result Result
}

type snmpPartialSink func(snmpPartialResult)

func runSNMPCollection(ctx context.Context, job entities.AgentlessJob) Result {
	return runSNMPCollectionWithPartials(ctx, job, nil)
}

func runSNMPCollectionWithPartials(ctx context.Context, job entities.AgentlessJob, partialSink snmpPartialSink) Result {
	if job.SNMP == nil {
		return Result{Status: "fail", Code: "snmp_no_profile", Message: "perfil SNMP ausente"}
	}
	host := endpointHost(job)
	if host == "" {
		return Result{Status: "fail", Code: "missing_endpoint", Message: "endpoint ausente"}
	}

	_, effectiveVersion, versionErr := configuredSNMPVersion(job.SNMP.Version)
	if versionErr != nil {
		return Result{Status: "fail", Code: "snmp_unsupported_version", Message: "Versão SNMP não suportada"}
	}
	if job.Config["snmp_basic_probe"] == true {
		return runSNMPBasicProbe(ctx, job, host, effectiveVersion)
	}
	plan := buildSNMPPlan(job, host)
	plan.SNMPVersion = effectiveVersion
	plan.DiagnosticProbe = job.Config["entity_sensor_probe"] == true
	ctx, cancel := context.WithTimeout(ctx, time.Duration(plan.TimeBudgetMs)*time.Millisecond)
	defer cancel()
	payload := newSNMPPayload(plan)
	if (len(plan.CustomGet) > 0 || len(plan.CustomWalk) > 0) && !stringSliceContains(payload.GroupsRequested, "custom") {
		payload.GroupsRequested = append(payload.GroupsRequested, "custom")
		payload.RuntimeApplied.GroupsRequested = cloneStringSlice(payload.GroupsRequested)
	}

	factory := defaultSNMPClient
	if factory == nil {
		factory = defaultSNMPClientFactory
	}
	client, err := factory(ctx, job, host)
	if err != nil {
		payload.addError("erro ao inicializar cliente SNMP: " + safeSNMPError(err))
		status, code, message := evaluateSNMPStatus(payload)
		return Result{
			Status:  status,
			Code:    code,
			Message: message,
			Payload: payload.toMap(),
		}
	}

	c := &snmpCollector{
		ctx:         ctx,
		job:         job,
		plan:        plan,
		payload:     payload,
		client:      client,
		started:     time.Now(),
		budget:      time.Duration(plan.TimeBudgetMs) * time.Millisecond,
		ifaceMap:    make(map[string]map[string]any),
		partialSink: partialSink,
	}
	return c.collect()
}

func stringSliceContains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func (c *snmpCollector) collect() Result {
	if err := c.client.Connect(); err != nil {
		c.payload.addError("connect SNMP falhou: " + safeSNMPError(err))
		return c.toResult()
	}
	defer func() {
		_ = c.client.Close()
	}()

	if err := c.ensureBudget("connect"); err != nil {
		_ = err
		return c.toResult()
	}

	for _, groupName := range c.plan.Groups {
		def, ok := snmpGroupDefs[groupName]
		if !ok {
			continue
		}
		if !c.collectGroup(groupName, def) {
			break
		}
	}

	if (len(c.plan.CustomGet) > 0 || len(c.plan.CustomWalk) > 0) && !c.payload.TimeBudgetExceeded {
		_ = c.collectCustom()
	}

	c.payload.Ifaces = ifaceSlice(c.ifaceMap)
	return c.toResult()
}

func (c *snmpCollector) collectGroup(groupName string, def snmpGroupDef) bool {
	groupStart := time.Now()
	groupStats := snmpGroupStats{}

	if c.plan.FetchMode != snmpFetchWalkOnly && len(def.Scalars) > 0 {
		if err := c.ensureBudget("group " + groupName + " get"); err != nil {
			groupStats.Errors = append(groupStats.Errors, err.Error())
			groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
			c.payload.addGroupStats(groupName, groupStats)
			c.emitGroupPartial(groupName, def, groupStats)
			return false
		}
		groupStats.GetAttempts += len(def.Scalars)
		resp, err := c.get(def.Scalars)
		if err != nil {
			msg := fmt.Sprintf("grupo %s GET falhou: %s", groupName, safeSNMPError(err))
			c.payload.addError(msg)
			groupStats.Errors = append(groupStats.Errors, msg)
		} else {
			if resp != nil && resp.Error != gosnmp.NoError {
				msg := fmt.Sprintf("grupo %s GET retorno SNMP error=%s", groupName, resp.Error.String())
				c.payload.addError(msg)
				groupStats.Errors = append(groupStats.Errors, msg)
			}
			for _, variable := range snmpVariables(resp) {
				if c.job.Config["entity_sensor_probe"] == true {
					c.captureSensorUptime(variable)
				}
				value := snmpValueToAny(variable)
				c.payload.Scalars[variable.Name] = value
				c.payload.OIDs[variable.Name] = value
				if groupName == "system" {
					c.payload.Sys[variable.Name] = value
				}
				groupStats.GetSuccess++
			}
		}
	}

	if c.plan.FetchMode != snmpFetchGetOnly && len(def.Tables) > 0 {
		for _, rootOID := range def.Tables {
			if err := c.ensureBudget("group " + groupName + " walk " + rootOID); err != nil {
				groupStats.Errors = append(groupStats.Errors, err.Error())
				groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
				c.payload.addGroupStats(groupName, groupStats)
				c.emitGroupPartial(groupName, def, groupStats)
				return false
			}
			groupStats.WalkAttempts++
			rows, truncated, err := c.walkTable(rootOID)
			if err != nil {
				msg := fmt.Sprintf("grupo %s WALK falhou oid=%s err=%s", groupName, rootOID, safeSNMPError(err))
				c.payload.addError(msg)
				groupStats.Errors = append(groupStats.Errors, msg)
				if errors.Is(err, errSNMPBudgetExceeded) {
					groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
					c.payload.addGroupStats(groupName, groupStats)
					c.emitGroupPartial(groupName, def, groupStats)
					return false
				}
				continue
			}
			if len(rows) == 0 {
				msg := "tabela sem retorno para OID base " + rootOID
				c.payload.addError(msg)
				groupStats.Errors = append(groupStats.Errors, msg)
				continue
			}
			if truncated {
				groupStats.Errors = append(groupStats.Errors, "limite snmp_max_rows atingido para OID "+rootOID)
			}
			groupStats.WalkRows += len(rows)
			c.payload.storeTable(rootOID, rows, truncated)
			if isInterfaceTableOID(rootOID) {
				updateIfaces(c.ifaceMap, rootOID, rows)
			}
		}
	}

	groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
	c.payload.addGroupStats(groupName, groupStats)
	c.emitGroupPartial(groupName, def, groupStats)
	return !c.payload.TimeBudgetExceeded
}

func (c *snmpCollector) collectCustom() bool {
	groupStart := time.Now()
	groupStats := snmpGroupStats{}

	if c.plan.FetchMode != snmpFetchWalkOnly && len(c.plan.CustomGet) > 0 {
		if err := c.ensureBudget("custom get"); err != nil {
			groupStats.Errors = append(groupStats.Errors, err.Error())
			groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
			c.payload.addGroupStats("custom", groupStats)
			c.emitCustomPartial(groupStats)
			return false
		}
		customGetOIDs := snmpOIDStrings(c.plan.CustomGet)
		groupStats.GetAttempts += len(customGetOIDs)
		resp, err := c.get(customGetOIDs)
		if err != nil {
			detail := safeSNMPError(err)
			msg := "custom GET falhou: " + detail
			c.payload.addError(msg)
			groupStats.Errors = append(groupStats.Errors, msg)
			for _, spec := range c.plan.CustomGet {
				item := snmpCustomItem(spec)
				item["error"] = detail
				c.payload.Custom["get"] = append(c.payload.Custom["get"], item)
				c.payload.addOIDFailed(spec, detail)
			}
		} else {
			values := make(map[string]any)
			for _, variable := range snmpVariables(resp) {
				v := snmpValueToAny(variable)
				values[strings.TrimPrefix(strings.TrimSpace(variable.Name), ".")] = v
				c.payload.Scalars[variable.Name] = v
				c.payload.OIDs[variable.Name] = v
			}
			for _, spec := range c.plan.CustomGet {
				item := snmpCustomItem(spec)
				if v, ok := values[spec.OID]; ok {
					groupStats.GetSuccess++
					item["value"] = v
					c.payload.Custom["get"] = append(c.payload.Custom["get"], item)
					c.payload.addOIDSuccess(spec, map[string]any{"value": v})
				} else {
					msg := "custom GET sem retorno para OID " + spec.OID
					item["error"] = "sem retorno"
					c.payload.Custom["get"] = append(c.payload.Custom["get"], item)
					c.payload.addOIDFailed(spec, "sem retorno")
					c.payload.addError(msg)
					groupStats.Errors = append(groupStats.Errors, msg)
				}
			}
		}
	}

	if c.plan.FetchMode != snmpFetchGetOnly && len(c.plan.CustomWalk) > 0 {
		for _, spec := range c.plan.CustomWalk {
			rootOID := spec.OID
			if err := c.ensureBudget("custom walk " + rootOID); err != nil {
				groupStats.Errors = append(groupStats.Errors, err.Error())
				groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
				c.payload.addGroupStats("custom", groupStats)
				c.emitCustomPartial(groupStats)
				return false
			}
			groupStats.WalkAttempts++
			rows, truncated, err := c.walkTable(rootOID)
			item := snmpCustomItem(spec)
			if err != nil {
				detail := safeSNMPError(err)
				msg := fmt.Sprintf("custom WALK falhou oid=%s err=%s", rootOID, detail)
				c.payload.addError(msg)
				groupStats.Errors = append(groupStats.Errors, msg)
				item["error"] = detail
				c.payload.Custom["walk"] = append(c.payload.Custom["walk"], item)
				c.payload.addOIDFailed(spec, detail)
				if errors.Is(err, errSNMPBudgetExceeded) {
					groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
					c.payload.addGroupStats("custom", groupStats)
					c.emitCustomPartial(groupStats)
					return false
				}
				continue
			}
			if len(rows) == 0 {
				msg := "tabela sem retorno para OID base " + rootOID
				c.payload.addError(msg)
				groupStats.Errors = append(groupStats.Errors, msg)
				item["error"] = "sem retorno"
				c.payload.addOIDFailed(spec, "sem retorno")
			}
			groupStats.WalkRows += len(rows)
			item["rows"] = len(rows)
			item["truncated"] = truncated
			item["data"] = rows
			c.payload.Custom["walk"] = append(c.payload.Custom["walk"], item)
			c.payload.storeTable(rootOID, rows, truncated)
			if len(rows) > 0 {
				c.payload.addOIDSuccess(spec, map[string]any{
					"rows":      len(rows),
					"truncated": truncated,
					"data":      rows,
				})
			}
		}
	}

	groupStats.LatencyMs = int(time.Since(groupStart).Milliseconds())
	c.payload.addGroupStats("custom", groupStats)
	c.emitCustomPartial(groupStats)
	return !c.payload.TimeBudgetExceeded
}

func (c *snmpCollector) emitGroupPartial(groupName string, def snmpGroupDef, groupStats snmpGroupStats) {
	if c.partialSink == nil {
		return
	}
	partial := newSNMPPayload(c.plan)
	partial.GroupsRequested = []string{groupName}
	partial.RuntimeApplied.GroupsRequested = cloneStringSlice(partial.GroupsRequested)
	partial.TimeBudgetExceeded = c.payload.TimeBudgetExceeded
	partial.addGroupStats(groupName, groupStats)
	partial.Errors = append(partial.Errors, groupStats.Errors...)

	for _, oid := range def.Scalars {
		if v, ok := c.payload.Scalars[oid]; ok {
			partial.Scalars[oid] = v
			partial.OIDs[oid] = v
			if groupName == "system" {
				partial.Sys[oid] = v
			}
		}
	}
	for _, rootOID := range def.Tables {
		rows, ok := c.payload.Tables[rootOID]
		if !ok {
			continue
		}
		partial.Tables[rootOID] = rows
	}
	partialIfaceMap := make(map[string]map[string]any)
	for rootOID, rows := range partial.Tables {
		if isInterfaceTableOID(rootOID) {
			updateIfaces(partialIfaceMap, rootOID, rows)
		}
	}
	partial.Ifaces = ifaceSlice(partialIfaceMap)
	c.emitPartial(groupName, partial)
}

func (c *snmpCollector) emitCustomPartial(groupStats snmpGroupStats) {
	if c.partialSink == nil {
		return
	}
	partial := newSNMPPayload(c.plan)
	partial.GroupsRequested = []string{"custom"}
	partial.RuntimeApplied.GroupsRequested = cloneStringSlice(partial.GroupsRequested)
	partial.TimeBudgetExceeded = c.payload.TimeBudgetExceeded
	partial.addGroupStats("custom", groupStats)
	partial.Errors = append(partial.Errors, groupStats.Errors...)
	partial.Custom["get"] = append(partial.Custom["get"], c.payload.Custom["get"]...)
	partial.Custom["walk"] = append(partial.Custom["walk"], c.payload.Custom["walk"]...)
	partial.OIDsSuccess = append(partial.OIDsSuccess, c.payload.OIDsSuccess...)
	partial.OIDsFailed = append(partial.OIDsFailed, c.payload.OIDsFailed...)
	partial.VendorProfileApplied = c.payload.VendorProfileApplied
	partial.CPUPercent = c.payload.CPUPercent
	partial.MemoryPercent = c.payload.MemoryPercent
	partial.CPUMemory = c.payload.CPUMemory
	for _, spec := range c.plan.CustomWalk {
		if rows, ok := c.payload.Tables[spec.OID]; ok {
			partial.Tables[spec.OID] = rows
		}
	}
	c.emitPartial("custom", partial)
}

func snmpCustomItem(spec entities.AgentlessSnmpOIDSpec) map[string]any {
	item := map[string]any{"oid": spec.OID}
	if spec.Name != "" {
		item["name"] = spec.Name
	}
	if spec.Label != "" {
		item["label"] = spec.Label
	}
	if spec.Metric != "" {
		item["metric"] = spec.Metric
	}
	if spec.SourceMIB != "" {
		item["source_mib"] = spec.SourceMIB
	}
	if spec.Unit != "" {
		item["unit"] = spec.Unit
	}
	if spec.CanonicalKey != "" {
		item["canonical_key"] = spec.CanonicalKey
	}
	return item
}

func (c *snmpCollector) emitPartial(groupName string, payload *snmpPayload) {
	status, code, message := evaluateSNMPStatus(payload)
	c.partialSink(snmpPartialResult{
		Group: groupName,
		Result: Result{
			Status:    status,
			LatencyMs: int(time.Since(c.started).Milliseconds()),
			Code:      code,
			Message:   message,
			Payload:   payload.toMap(),
		},
	})
}

func (c *snmpCollector) walkTable(rootOID string) ([]map[string]any, bool, error) {
	limit := c.plan.MaxRows
	if limit <= 0 {
		limit = defaultSNMPMaxRows
	}
	rows := make([]map[string]any, 0, minInt(limit, 32))
	count := 0

	walkFn := func(variable gosnmp.SnmpPDU) error {
		if err := c.ensureBudget("walk callback " + rootOID); err != nil {
			return err
		}
		if !usefulSNMPVariable(variable) || (c.plan.SNMPVersion == "v1" && variable.Type == gosnmp.Counter64) {
			return nil
		}
		count++
		if count > limit {
			return errSNMPMaxRowsReached
		}
		row := map[string]any{"oid": variable.Name, "index": tableIndex(rootOID, variable.Name), "value": snmpTableValue(rootOID, variable)}
		if enabled, _ := getBool(map[string]any(c.job.Config), "entity_sensor_probe"); enabled {
			row["value_type"] = variable.Type.String()
		}
		rows = append(rows, row)
		return nil
	}

	var err error
	if c.plan.SNMPVersion == "v1" {
		err = c.walkV1(rootOID, walkFn)
	} else {
		err = c.client.BulkWalk(rootOID, walkFn)
	}
	if c.plan.SNMPVersion != "v1" && err != nil && !errors.Is(err, errSNMPMaxRowsReached) && !errors.Is(err, errSNMPBudgetExceeded) {
		rows = rows[:0]
		count = 0
		err = c.client.Walk(rootOID, walkFn)
	}

	switch {
	case errors.Is(err, errSNMPMaxRowsReached):
		return rows, true, nil
	case errors.Is(err, errSNMPBudgetExceeded):
		return rows, false, errSNMPBudgetExceeded
	case err != nil:
		return nil, false, err
	default:
		return rows, false, nil
	}
}

func (c *snmpCollector) ensureBudget(scope string) error {
	if c.budget <= 0 {
		return nil
	}
	if time.Since(c.started) <= c.budget {
		return nil
	}
	c.payload.TimeBudgetExceeded = true
	msg := "time budget excedido em " + scope
	c.payload.addError(msg)
	return fmt.Errorf("%w: %s", errSNMPBudgetExceeded, scope)
}

func (c *snmpCollector) toResult() Result {
	if enabled, _ := getBool(map[string]any(c.job.Config), "entity_sensor_probe"); enabled {
		c.payload.TemperatureSensors = entitySensorTemperatures(c.payload.Tables, c.payload.SensorUptimeTicks, c.payload.TimeBudgetExceeded)
	}
	c.payload.Ifaces = ifaceSlice(c.ifaceMap)
	status, code, message := evaluateSNMPStatus(c.payload)
	return Result{
		Status:    status,
		LatencyMs: int(time.Since(c.started).Milliseconds()),
		Code:      code,
		Message:   message,
		Payload:   c.payload.toMap(),
	}
}

func snmpVariables(resp *gosnmp.SnmpPacket) []gosnmp.SnmpPDU {
	if resp == nil || resp.Error != gosnmp.NoError {
		return nil
	}
	variables := make([]gosnmp.SnmpPDU, 0, len(resp.Variables))
	for _, variable := range resp.Variables {
		if usefulSNMPVariable(variable) {
			variables = append(variables, variable)
		}
	}
	return variables
}

func snmpValueToAny(variable gosnmp.SnmpPDU) any {
	switch variable.Type {
	case gosnmp.OctetString:
		if b, ok := variable.Value.([]byte); ok {
			if isPlainTextBytes(b) {
				return string(b)
			}
			return fmt.Sprintf("%x", b)
		}
	case gosnmp.ObjectIdentifier, gosnmp.IPAddress:
		return fmt.Sprintf("%v", variable.Value)
	}
	if bi := gosnmp.ToBigInt(variable.Value); bi != nil {
		if bi.IsInt64() {
			return bi.Int64()
		}
		return bi.String()
	}
	return fmt.Sprintf("%v", variable.Value)
}

func isPlainTextBytes(v []byte) bool {
	if len(v) == 0 {
		return true
	}
	for _, b := range v {
		if b < 32 || b > 126 {
			return false
		}
	}
	return true
}

func tableIndex(rootOID, itemOID string) string {
	rootOID = strings.TrimPrefix(strings.TrimSpace(rootOID), ".")
	itemOID = strings.TrimPrefix(strings.TrimSpace(itemOID), ".")
	prefix := rootOID + "."
	if strings.HasPrefix(itemOID, prefix) {
		return strings.TrimPrefix(itemOID, prefix)
	}
	if itemOID == rootOID {
		return ""
	}
	return itemOID
}

func isInterfaceTableOID(rootOID string) bool {
	return strings.HasPrefix(rootOID, "1.3.6.1.2.1.2.2.1.") ||
		strings.HasPrefix(rootOID, "1.3.6.1.2.1.31.1.1.1.")
}

func updateIfaces(dst map[string]map[string]any, rootOID string, rows []map[string]any) {
	for _, row := range rows {
		index := fmt.Sprintf("%v", row["index"])
		if index == "" {
			continue
		}
		item, ok := dst[index]
		if !ok {
			item = map[string]any{"index": index}
			dst[index] = item
		}
		item[rootOID] = row["value"]
	}
}

func ifaceSlice(src map[string]map[string]any) []map[string]any {
	if len(src) == 0 {
		return []map[string]any{}
	}
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, okI := strconv.Atoi(keys[i])
		nj, okJ := strconv.Atoi(keys[j])
		if okI == nil && okJ == nil {
			return ni < nj
		}
		return keys[i] < keys[j]
	})
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, src[k])
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
