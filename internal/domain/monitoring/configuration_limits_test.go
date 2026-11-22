package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestConfigurationLimitsAndAuthorizedReferences(t *testing.T) {
	c := configurationCases(t)[0]
	c.Authority.SecretReferences = []string{"snmp_profile:29"}
	c.Authority.PipelineReferences = []string{"pipeline1"}
	c.Configuration.Items[0].Parameters["credential_ref"] = ConfigurationValue{Type: "secret_ref", Value: []byte(`"snmp_profile:29"`)}
	c.Configuration.Items[0].Parameters["pipeline_ref"] = ConfigurationValue{Type: "text", Value: []byte(`"pipeline1"`)}
	raw, err := EncodeConfiguration(c.Configuration, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	padded := append(raw, bytes.Repeat([]byte(" "), MaxDefinitionBytes-len(raw))...)
	if _, err := DecodeConfiguration(padded, c.Authority); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConfiguration(append(padded, ' '), c.Authority); err == nil {
		t.Fatal("wire limit exceeded")
	}
	c.Authority.SecretReferences = []string{}
	if _, err := EncodeConfiguration(c.Configuration, c.Authority); err == nil {
		t.Fatal("revoked reference accepted")
	}
}

func TestConfigurationExactItemLimitAndUniqueSeries(t *testing.T) {
	c := configurationCases(t)[0]
	item := c.Configuration.Items[0]
	definition := c.Configuration.Definition.Items[0]
	c.Configuration.Items = []ConfigurationItem{}
	c.Configuration.Definition.Items = []ItemDefinition{}
	c.Authority.Bindings = map[string]ConfigurationBinding{}
	for n := 0; n < 128; n++ {
		id := int64(n + 10)
		key := fmt.Sprintf("catalog-item-%d", id)
		metric := fmt.Sprintf("reading_%d", n)
		raw, _ := json.Marshal(item)
		var next ConfigurationItem
		_ = json.Unmarshal(raw, &next)
		next.TemplateKey = key
		next.SourceItemID = id
		next.ItemKey = configurationKey(c.Authority.Definition, 72, "snmp", metric)
		encoded, _ := json.Marshal(metric)
		next.Parameters["metric"] = ConfigurationValue{Type: "text", Value: encoded}
		encoded, _ = json.Marshal(fmt.Sprintf("check-config:v1:3:1:%d:snmp:%064d", id, 0))
		next.Parameters["configuration_ref"] = ConfigurationValue{Type: "text", Value: encoded}
		d := definition
		d.Key = next.ItemKey
		d.Metric = metric
		c.Configuration.Items = append(c.Configuration.Items, next)
		c.Configuration.Definition.Items = append(c.Configuration.Definition.Items, d)
		c.Authority.Bindings[key] = ConfigurationBinding{CheckID: 72, SourceItemID: id, SourceTemplateID: 1}
	}
	raw, err := EncodeConfiguration(c.Configuration, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConfiguration(raw, c.Authority); err != nil {
		t.Fatal(err)
	}
	c.Configuration.Items = append(c.Configuration.Items, item)
	if _, err := EncodeConfiguration(c.Configuration, c.Authority); err == nil {
		t.Fatal("item cap exceeded")
	}
}

func TestConfigurationRejectsUnsafeParameterLiterals(t *testing.T) {
	c := configurationCases(t)[0]
	for name, values := range map[string][]ConfigurationValue{
		"nome":           {{Type: "text", Value: []byte(`""`)}, {Type: "text", Value: []byte(`"bad\nname"`)}, {Type: "text", Value: []byte(`"\ud800"`)}},
		"timeout_ms":     {{Type: "integer", Value: []byte(`499`)}, {Type: "integer", Value: []byte(`500.0`)}},
		"retries":        {{Type: "integer", Value: []byte(`6`)}},
		"fail_threshold": {{Type: "integer", Value: []byte(`0`)}},
		"depends_on":     {{Type: "text_list", Value: []byte(`null`)}, {Type: "text_list", Value: []byte(`["x","x"]`)}},
	} {
		for _, value := range values {
			original, existed := c.Configuration.Items[0].Parameters[name]
			c.Configuration.Items[0].Parameters[name] = value
			if _, err := EncodeConfiguration(c.Configuration, c.Authority); err == nil {
				t.Fatalf("unsafe %s accepted", name)
			}
			if existed {
				c.Configuration.Items[0].Parameters[name] = original
			} else {
				delete(c.Configuration.Items[0].Parameters, name)
			}
		}
	}
}

func TestConfigurationEvolutionRequiresIndependentAuthorityAndSameRoot(t *testing.T) {
	c := configurationCases(t)[0]
	raw, _ := json.Marshal(c.Configuration)
	next, err := DecodeConfiguration(raw, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	after := c.Authority
	after.Revision = 2
	next.Definition.Revision = 2
	if err := ValidateConfigurationEvolution(c.Configuration, c.Configuration, c.Authority, c.Authority); err == nil {
		t.Fatal("old revision replay accepted")
	}
	invalid := c.Configuration
	invalid.Contract = "future"
	if err := ValidateConfigurationEvolution(invalid, next, c.Authority, after); err == nil {
		t.Fatal("old invalid config accepted")
	}
	invalid = next
	invalid.Contract = "future"
	if err := ValidateConfigurationEvolution(c.Configuration, invalid, c.Authority, after); err == nil {
		t.Fatal("new invalid config accepted")
	}
	next.Root = ConfigurationReference{ID: 2, Revision: 1}
	next.Lineage = append(next.Lineage, next.Root)
	after.Root = next.Root
	after.TemplateRevisions = map[int64]int64{1: 1, 2: 1}
	if err := ValidateConfiguration(next, after); err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfigurationEvolution(c.Configuration, next, c.Authority, after); err == nil || err.Error() != "monitoring_configuration_scope_mismatch" {
		t.Fatal("root adoption accepted", err)
	}
}

func TestConfigurationRejectsAuthorityAliasesAndNestedShapeChanges(t *testing.T) {
	c := configurationCases(t)[0]
	raw, _ := json.Marshal(c.Configuration)
	for _, refs := range [][]string{nil, {"pipeline1", "pipeline1"}, {"https://fixture.invalid"}} {
		a := c.Authority
		a.PipelineReferences = refs
		if _, err := DecodeConfiguration(raw, a); err == nil {
			t.Fatal("invalid reference authority accepted")
		}
	}
	for _, change := range []struct{ from, to string }{
		{`"root":{"id":1,"revision":1}`, `"root":[]`},
		{`"lineage":[{"id":1,"revision":1}]`, `"lineage":[{"id":1,"revision":1,"extra":true}]`},
		{`"type":"boolean","value":true`, `"type":"boolean","value":true,"extra":true`},
	} {
		changed := bytes.Replace(raw, []byte(change.from), []byte(change.to), 1)
		if bytes.Equal(raw, changed) {
			t.Fatal("shape mutation did not apply")
		}
		if _, err := DecodeConfiguration(changed, c.Authority); err == nil {
			t.Fatal("nested shape accepted")
		}
	}
}
