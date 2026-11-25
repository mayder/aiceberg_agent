package agentless

import (
	"context"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const maxOpenMetricsBytes = 1 << 20
const maxOpenMetricsEntries = 20000
const metricNameLabel = "__name__"

var errOpenMetricsResponse = errors.New("invalid_openmetrics_response")

func runOpenMetrics(ctx context.Context, job entities.AgentlessJob) Result {
	target, err := probeHTTPURL(job.Endpoint)
	selectors, selectorErr := monitoring.OpenMetricsSelectors(job.Config)
	if err != nil || selectorErr != nil || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_openmetrics_config"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_openmetrics_config"}
	}
	req.Header.Set("Accept", "application/openmetrics-text;version=1.0.0,text/plain;version=0.0.4;q=0.9")
	response, failure := doNumericHTTPRequest(req, job)
	if response == nil {
		return failure
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOpenMetricsBytes+1))
	if err != nil || len(raw) > maxOpenMetricsBytes {
		return Result{Status: "fail", Code: "invalid_openmetrics_response", failure: err}
	}
	parser, err := newOpenMetricsParser(raw, response.Header.Get("Content-Type"))
	if err != nil {
		return Result{Status: "fail", Code: "invalid_openmetrics_response"}
	}
	values, err := readOpenMetrics(ctx, parser, selectors, job.ProbePolicy.FreshnessSec)
	if err != nil {
		return Result{Status: "fail", Code: "invalid_openmetrics_response", failure: err}
	}
	return Result{Status: "ok", Payload: map[string]any{"openmetrics_values_v1": values}}
}

func newOpenMetricsParser(body []byte, contentType string) (textparse.Parser, error) {
	format, params, err := mime.ParseMediaType(contentType)
	if err != nil || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		return nil, errOpenMetricsResponse
	}
	switch {
	case format == "text/plain" && (params["version"] == "" || params["version"] == "0.0.4"):
		return textparse.NewPromParser(body, labels.NewSymbolTable(), false), nil
	case format == "application/openmetrics-text" && params["version"] == "1.0.0":
		return textparse.NewOpenMetricsParser(body, labels.NewSymbolTable()), nil
	default:
		return nil, errOpenMetricsResponse
	}
}

func readOpenMetrics(ctx context.Context, parser textparse.Parser, selectors []monitoring.OpenMetricsSelector, freshness int64) (map[string]any, error) {
	byName := make(map[string][]monitoring.OpenMetricsSelector, len(selectors))
	for _, selector := range selectors {
		byName[selector.Name] = append(byName[selector.Name], selector)
	}
	values := make(map[string]any, len(selectors))
	seen := make(map[string]bool, len(selectors))
	now := time.Now().UnixMilli()
	for n := 0; n <= maxOpenMetricsEntries; n++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, err := parser.Next()
		if errors.Is(err, io.EOF) {
			return values, nil
		}
		if err != nil || n == maxOpenMetricsEntries {
			return nil, errOpenMetricsResponse
		}
		if entry == textparse.EntrySeries {
			addOpenMetricsSample(parser, byName, values, seen, now, freshness)
		}
	}
	return nil, errOpenMetricsResponse
}

func addOpenMetricsSample(parser textparse.Parser, byName map[string][]monitoring.OpenMetricsSelector,
	values map[string]any, seen map[string]bool, now, freshness int64,
) {
	var actual labels.Labels
	parser.Labels(&actual)
	_, timestamp, value := parser.Series()
	for _, selector := range byName[actual.Get(metricNameLabel)] {
		if !openMetricsLabelsMatch(actual, selector.Labels) {
			continue
		}
		reference := selector.Reference()
		if seen[reference] {
			delete(values, reference)
			continue
		}
		seen[reference] = true
		fresh := timestamp == nil || (*timestamp <= now+1000 && *timestamp >= now-freshness*1000)
		if fresh && !math.IsNaN(value) && !math.IsInf(value, 0) {
			values[reference] = value
		}
	}
}

func openMetricsLabelsMatch(actual labels.Labels, configured map[string]string) bool {
	if actual.Len() != len(configured)+1 {
		return false
	}
	for key, value := range configured {
		if !actual.Has(key) || actual.Get(key) != value {
			return false
		}
	}
	return true
}
