package agentless

import (
	"math"
	"sort"
	"strconv"
	"strings"

	gosnmp "github.com/gosnmp/gosnmp"
)

const entitySensorRoot = "1.3.6.1.2.1.99.1.1.1."

type snmpTemperatureReading struct {
	Index     string   `json:"index"`
	Unit      string   `json:"unit"`
	State     string   `json:"state"`
	Gap       string   `json:"gap"`
	RawValue  *int64   `json:"raw_value"`
	Scale     *int64   `json:"scale"`
	Precision *int64   `json:"precision"`
	Value     *float64 `json:"value"`
	AgeMS     *int64   `json:"age_ms"`
}

// RFC3433: interpret only Celsius entries with complete typed metadata. This
// diagnostic projection never substitutes zero for absent, invalid or stale data.
func entitySensorTemperatures(tables map[string][]map[string]any, uptime *uint32, budgetExceeded bool) []snmpTemperatureReading {
	columns := make(map[string]map[string]map[string]any, 8)
	for _, column := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
		indexed := make(map[string]map[string]any)
		for _, row := range tables[entitySensorRoot+column] {
			index, _ := row["index"].(string)
			if id, err := strconv.ParseUint(index, 10, 31); err == nil && id > 0 {
				indexed[index] = row
			}
			if len(indexed) >= 16 {
				break
			}
		}
		columns[column] = indexed
	}
	indices := make([]string, 0, 16)
	for index, row := range columns["1"] {
		if value, ok := sensorInteger(row); ok && value == 8 {
			indices = append(indices, index)
		}
	}
	sort.Slice(indices, func(i, j int) bool {
		a, _ := strconv.ParseUint(indices[i], 10, 31)
		b, _ := strconv.ParseUint(indices[j], 10, 31)
		return a < b
	})
	readings := make([]snmpTemperatureReading, 0, len(indices))
	for _, index := range indices {
		readings = append(readings, entitySensorTemperature(index, columns, uptime, budgetExceeded))
	}
	return readings
}

func entitySensorTemperature(index string, columns map[string]map[string]map[string]any, uptime *uint32, budget bool) snmpTemperatureReading {
	reading := snmpTemperatureReading{Index: index, Unit: "celsius", State: "unknown", Gap: "missing_metadata"}
	if budget {
		reading.Gap = "time_budget"
		return reading
	}
	stamp, validStamp := sensorTypedInteger(columns["7"][index], "TimeTicks", 0, 4294967295)
	if uptime == nil || !validStamp {
		return reading
	}
	age := int64(*uptime-uint32(stamp)) * 10
	// sysUpTime is read at probe start; a sensor may update during the bounded five-second probe.
	if uint32(stamp) > *uptime && uint32(stamp)-*uptime <= 500 {
		age = 0
	}
	reading.AgeMS = &age
	if age > 300000 {
		reading.Gap = "stale"
		return reading
	}
	values := make(map[string]int64, 4)
	for _, key := range []string{"2", "3", "4", "5"} {
		value, ok := sensorInteger(columns[key][index])
		if !ok {
			return reading
		}
		values[key] = value
	}
	raw, scale, precision := values["4"], values["2"], values["3"]
	reading.RawValue, reading.Scale, reading.Precision = &raw, &scale, &precision
	switch {
	case values["5"] != 1:
		reading.Gap = "unavailable"
	case raw <= -1000000000 || raw >= 1000000000:
		reading.Gap = "invalid_value"
	case scale < 1 || scale > 17:
		reading.Gap = "invalid_scale"
	case precision < 0 || precision > 9:
		reading.Gap = "unsupported_precision"
	default:
		value := float64(raw) * math.Pow10(int(3*(scale-9)-precision))
		if math.IsNaN(value) || math.IsInf(value, 0) || value < -273.15 {
			reading.Gap = "invalid_value"
			return reading
		}
		reading.Value, reading.State, reading.Gap = &value, "ok", ""
	}
	return reading
}

func sensorInteger(row map[string]any) (int64, bool) {
	return sensorTypedInteger(row, "Integer", -2147483648, 2147483647)
}

func sensorTypedInteger(row map[string]any, kind string, minimum, maximum float64) (int64, bool) {
	if row["value_type"] != kind {
		return 0, false
	}
	value, valid := snmpNumericValue(row["value"])
	if !valid || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < minimum || value > maximum {
		return 0, false
	}
	return int64(value), true
}

func (c *snmpCollector) captureSensorUptime(variable gosnmp.SnmpPDU) {
	if variable.Type != gosnmp.TimeTicks || strings.TrimPrefix(variable.Name, ".") != "1.3.6.1.2.1.1.3.0" {
		return
	}
	value, valid := snmpNumericValue(snmpValueToAny(variable))
	if valid && value >= 0 && value <= 4294967295 && math.Trunc(value) == value {
		ticks := uint32(value)
		c.payload.SensorUptimeTicks = &ticks
	}
}
