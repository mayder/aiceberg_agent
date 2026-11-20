package agentless

import (
	"testing"
)

var sensorFixtureUptime = uint32(10000)

func sensorFixture() map[string][]map[string]any {
	tables := make(map[string][]map[string]any)
	for column, value := range map[string]any{"1": 8, "2": 9, "3": 1, "4": 253, "5": 1, "6": "C"} {
		tables[entitySensorRoot+column] = []map[string]any{{"index": "1", "value": value, "value_type": "Integer"}}
	}
	tables[entitySensorRoot+"7"] = []map[string]any{{"index": "1", "value": 9990, "value_type": "TimeTicks"}}
	return tables
}

func TestEntitySensorTemperatureUsesTypeScalePrecisionAndStatus(t *testing.T) {
	tables := sensorFixture()
	readings := entitySensorTemperatures(tables, &sensorFixtureUptime, false)
	if len(readings) != 1 || readings[0].State != "ok" || readings[0].Value == nil || *readings[0].Value != 25.3 {
		t.Fatalf("wrong Celsius interpretation: %+v", readings)
	}
	for _, c := range []struct {
		column string
		value  any
		gap    string
	}{
		{"2", 0, "invalid_scale"}, {"3", -1, "unsupported_precision"}, {"4", 1000000000, "invalid_value"}, {"5", 2, "unavailable"},
	} {
		t.Run(c.gap, func(t *testing.T) {
			tables := sensorFixture()
			tables[entitySensorRoot+c.column][0]["value"] = c.value
			got := entitySensorTemperatures(tables, &sensorFixtureUptime, false)[0]
			if got.Value != nil || got.State != "unknown" || got.Gap != c.gap {
				t.Fatalf("invalid became a temperature: %+v", got)
			}
		})
	}
}

func TestEntitySensorTemperatureMissingMetadataAndBudgetRemainUnknown(t *testing.T) {
	for _, column := range []string{"2", "3", "4", "5", "7"} {
		tables := sensorFixture()
		delete(tables, entitySensorRoot+column)
		got := entitySensorTemperatures(tables, &sensorFixtureUptime, false)[0]
		if got.Value != nil || got.Gap != "missing_metadata" {
			t.Fatalf("missing metadata became value: %+v", got)
		}
	}
	tables := sensorFixture()
	tables[entitySensorRoot+"4"][0]["value_type"] = "OctetString"
	if got := entitySensorTemperatures(tables, &sensorFixtureUptime, false)[0]; got.Value != nil {
		t.Fatal("wrong ASN type accepted")
	}
	if got := entitySensorTemperatures(sensorFixture(), &sensorFixtureUptime, true)[0]; got.Value != nil || got.Gap != "time_budget" {
		t.Fatal("budget exhaustion became success")
	}
	tables = sensorFixture()
	tables[entitySensorRoot+"1"][0]["value"] = 9
	if got := entitySensorTemperatures(tables, &sensorFixtureUptime, false); len(got) != 0 {
		t.Fatal("humidity claimed as temperature")
	}
}

func TestEntitySensorTemperatureFreshnessAndTickWrap(t *testing.T) {
	tables := sensorFixture()
	uptime := uint32(40000)
	if got := entitySensorTemperatures(tables, &uptime, false)[0]; got.Value != nil || got.Gap != "stale" {
		t.Fatal("stale device value accepted")
	}
	if got := entitySensorTemperatures(sensorFixture(), nil, false)[0]; got.Value != nil || got.Gap != "missing_metadata" {
		t.Fatal("unknown device clock accepted")
	}
	tables = sensorFixture()
	tables[entitySensorRoot+"7"][0]["value"] = uint32(4294967290)
	uptime = 4
	if got := entitySensorTemperatures(tables, &uptime, false)[0]; got.Value == nil || got.AgeMS == nil || *got.AgeMS != 100 {
		t.Fatalf("tick wrap lost: %+v", got)
	}
	tables = sensorFixture()
	tables[entitySensorRoot+"7"][0]["value"] = 10005
	uptime = 10000
	if got := entitySensorTemperatures(tables, &uptime, false)[0]; got.Value == nil || *got.AgeMS != 0 {
		t.Fatal("update during probe rejected")
	}
}
