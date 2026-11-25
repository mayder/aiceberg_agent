package monitoring

import (
	"strings"
	"testing"
)

func TestPipelineXMLRejectsMalformedAttributes(t *testing.T) {
	path, _ := parsePipelineXMLPath("/r/@a")
	for _, document := range []string{
		`<r a="1"b="2"/>`,
		`<r a="<"/>`,
		`<r a=1/>`,
	} {
		if value, err := selectPipelineXML(path, document); value != "" || err == nil || err.Error() != "pipeline_xml_invalid" {
			t.Fatal("invalid attributes accepted", err)
		}
	}
}

func FuzzPipelineXMLBoundedScan(f *testing.F) {
	path, _ := parsePipelineXMLPath("/r/s[2]/@value")
	f.Add(`<r><s/><s value="1"/></r>`)
	f.Add(`<r><s/><s value="1"/></r><tail/>`)
	f.Add(`<!DOCTYPE r><r/>`)
	f.Add(strings.Repeat(" ", maxPipelineXMLBytes+1))
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > maxPipelineXMLBytes+1 {
			return
		}
		value, err := selectPipelineXML(path, data)
		if err != nil && value != "" {
			t.Fatal("failed selection returned a value")
		}
		if len(data) > maxPipelineXMLBytes && (err == nil || err.Error() != "pipeline_xml_limit") {
			t.Fatal("oversize document was not rejected")
		}
	})
}
