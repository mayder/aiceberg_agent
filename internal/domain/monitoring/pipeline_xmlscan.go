package monitoring

import (
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

var pipelineXMLDeclaration = regexp.MustCompile(`^<\?xml +version=(?:"1\.0"|'1\.0')(?: +encoding=(?:"UTF-8"|'UTF-8'))? *\?>$`)

// The standard decoder permits adjacent attributes. Enforce whitespace and
// disallow literal '<' in quoted values within our namespace-free profile.
var pipelineXMLStartTag = regexp.MustCompile(`^<` + pipelineXMLNamePattern +
	`(?:[ \n\t]+` + pipelineXMLNamePattern + `[ \n\t]*=[ \n\t]*(?:"[^"<]*"|'[^'<]*'))*[ \n\t]*/?>$`)

type pipelineXMLScan struct {
	path      pipelineXMLPath
	depth     int
	roots     int
	matches   [maxPipelineXMLDepth]int
	selected  [maxPipelineXMLDepth]bool
	found     bool
	ambiguous bool
	composite bool
	value     strings.Builder
}

func selectPipelineXML(path pipelineXMLPath, input string) (string, error) {
	invalid := errors.New("pipeline_xml_invalid")
	if len(input) > maxPipelineXMLBytes {
		return "", errors.New("pipeline_xml_limit")
	}
	decoder := xml.NewDecoder(strings.NewReader(input))
	scan := pipelineXMLScan{path: path}
	for count := 0; ; count++ {
		start := decoder.InputOffset()
		part, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", invalid
		}
		if count == maxPipelineXMLTokens {
			return "", errors.New("pipeline_xml_limit")
		}
		if err := scan.consume(part, input[start:decoder.InputOffset()], start); err != nil {
			return "", err
		}
	}
	if scan.roots != 1 || scan.depth != 0 {
		return "", invalid
	}
	if scan.ambiguous {
		return "", errors.New("pipeline_xmlpath_ambiguous")
	}
	if !scan.found {
		return "", errors.New("pipeline_xmlpath_missing")
	}
	if scan.composite {
		return "", errors.New("pipeline_value_invalid")
	}
	return scan.value.String(), nil
}

func (s *pipelineXMLScan) consume(part xml.Token, wire string, offset int64) error {
	invalid := errors.New("pipeline_xml_invalid")
	switch part := part.(type) {
	case xml.StartElement:
		return s.startElement(part, wire)
	case xml.EndElement:
		s.depth--
	case xml.CharData:
		if s.depth == 0 && strings.Trim(wire, " \r\n\t") != "" {
			return invalid
		}
		if s.path.attribute == "" && s.depth == len(s.path.elements) && s.atSelection() {
			s.value.Write(part)
		}
	case xml.ProcInst:
		if offset != 0 || !pipelineXMLDeclaration.MatchString(wire) {
			return invalid
		}
	case xml.Directive:
		return invalid
	}
	return nil
}

func (s *pipelineXMLScan) startElement(element xml.StartElement, wire string) error {
	if s.depth == maxPipelineXMLDepth || len(element.Attr) > maxPipelineXMLAttributes {
		return errors.New("pipeline_xml_limit")
	}
	if err := validatePipelineXMLElement(element, wire); err != nil {
		return err
	}
	if s.depth == 0 {
		s.roots++
		if s.roots > 1 {
			return errors.New("pipeline_xml_invalid")
		}
	}
	if s.path.attribute == "" && s.depth == len(s.path.elements) && s.atSelection() {
		s.composite = true
	}
	if s.depth < len(s.path.elements) {
		s.matchElement(element)
	}
	s.depth++
	return nil
}

func (s *pipelineXMLScan) atSelection() bool {
	return s.depth > 0 && s.selected[s.depth-1]
}

func (s *pipelineXMLScan) matchElement(element xml.StartElement) {
	depth := s.depth
	s.selected[depth] = false
	segment := s.path.elements[depth]
	if (depth > 0 && !s.atSelection()) || element.Name.Local != segment.name {
		return
	}
	s.matches[depth]++
	if segment.position == 0 && s.matches[depth] > 1 {
		s.ambiguous = true
		return
	}
	if segment.position != 0 && s.matches[depth] != segment.position {
		return
	}
	s.selected[depth] = true
	if depth+1 == len(s.path.elements) {
		s.capture(element)
	}
}

func (s *pipelineXMLScan) capture(element xml.StartElement) {
	if s.path.attribute == "" {
		s.found = true
		return
	}
	for _, attribute := range element.Attr {
		if attribute.Name.Local == s.path.attribute {
			s.found = true
			s.value.WriteString(attribute.Value)
			return
		}
	}
}

func validatePipelineXMLElement(element xml.StartElement, wire string) error {
	invalid := errors.New("pipeline_xml_invalid")
	if element.Name.Space != "" || !pipelineXMLStartTag.MatchString(wire) {
		return invalid
	}
	seen := make(map[string]bool, len(element.Attr))
	for _, attribute := range element.Attr {
		name := attribute.Name.Local
		if attribute.Name.Space != "" || name == "xmlns" || seen[name] {
			return invalid
		}
		seen[name] = true
	}
	return nil
}
