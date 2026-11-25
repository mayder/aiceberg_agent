package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const maxJolokiaBytes = 128 << 10

var errJolokiaResponse = errors.New("invalid_jolokia_response")

type jolokiaRead struct {
	Type      string `json:"type"`
	MBean     string `json:"mbean"`
	Attribute string `json:"attribute"`
	Path      string `json:"path,omitempty"`
}

type jolokiaResponse struct {
	Request   *jolokiaRead    `json:"request"`
	Status    int             `json:"status"`
	Timestamp int64           `json:"timestamp"`
	RawValue  json.RawMessage `json:"value"`
}

func runJolokia(ctx context.Context, job entities.AgentlessJob) Result {
	target, targetErr := probeHTTPURL(job.Endpoint)
	selectors, selectorErr := monitoring.JolokiaSelectors(job.Config)
	if targetErr != nil || selectorErr != nil || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_jolokia_config"}
	}
	requests := make([]jolokiaRead, 0, len(selectors))
	for _, s := range selectors {
		requests = append(requests, jolokiaRead{Type: "read", MBean: s.MBean, Attribute: s.Attribute, Path: s.Path})
	}
	body, err := json.Marshal(requests)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_jolokia_config"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return Result{Status: "fail", Code: "invalid_jolokia_config"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain;q=0.9")
	response, failure := doNumericHTTPRequest(req, job)
	if response == nil {
		return failure
	}
	defer response.Body.Close()
	format, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (format != "application/json" && format != "text/plain") ||
		(params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		return Result{Status: "fail", Code: "invalid_jolokia_response"}
	}
	values, err := readJolokia(response.Body, requests, selectors, job.ProbePolicy.FreshnessSec, time.Now().Unix())
	if err != nil {
		return Result{Status: "fail", Code: "invalid_jolokia_response", failure: err}
	}
	return Result{Status: "ok", Payload: map[string]any{"jolokia_values_v1": values}}
}

func readJolokia(body io.Reader, requests []jolokiaRead, selectors []monitoring.JolokiaSelector, freshness, now int64) (map[string]any, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxJolokiaBytes+1))
	if err != nil {
		return nil, err
	}
	var responses []jolokiaResponse
	if len(raw) > maxJolokiaBytes || json.Unmarshal(raw, &responses) != nil || len(responses) != len(requests) {
		return nil, errJolokiaResponse
	}
	values := make(map[string]any, len(selectors))
	var success bool
	for index, response := range responses {
		if response.Request != nil {
			echo := *response.Request
			echo.MBean = monitoring.CanonicalJolokiaMBean(echo.MBean)
			if echo != requests[index] {
				return nil, errJolokiaResponse
			}
		}
		if response.Status != http.StatusOK {
			continue
		}
		success = true
		if response.Timestamp < now-freshness || response.Timestamp > now+1 {
			continue
		}
		var rawValue any
		decoder := json.NewDecoder(bytes.NewReader(response.RawValue))
		decoder.UseNumber()
		if decoder.Decode(&rawValue) != nil {
			continue
		}
		if value, ok := monitoring.NumericProbeValue(rawValue); ok {
			values[selectors[index].Reference()] = value
		}
	}
	if !success {
		return nil, errJolokiaResponse
	}
	return values, nil
}
