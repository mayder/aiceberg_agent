package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/browserjourney"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func runBrowserProbe(ctx context.Context, job entities.AgentlessJob) Result {
	selection, err := monitoring.BrowserSelectors(job.Config)
	if err != nil || job.Endpoint == nil || job.Endpoint.Tipo != "url" || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_browser_config"}
	}
	result := runConfiguredBrowser(ctx, job, selection)
	if result.Payload == nil {
		result.Payload = make(map[string]any, 1)
	}
	result.Payload["browser_recipe_sha256"] = selection.Digest
	return result
}

func runConfiguredBrowser(ctx context.Context, job entities.AgentlessJob, selection monitoring.BrowserSelection) Result {
	request := browserjourney.Request{Contract: browserjourney.RunContract, Origin: job.Endpoint.Endereco,
		Digest: selection.Digest, BudgetMS: int(job.ProbePolicy.BudgetMS),
		Scope: browserjourney.Scope{ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID),
			CheckID: int64(job.CheckID), EndpointID: int64(job.Endpoint.ID)}}
	if !request.Valid() {
		return Result{Status: "fail", Code: "invalid_browser_config"}
	}

	socket := config.AgentlessBrowserRunnerSocket()
	if socket == "" {
		return Result{Status: "fail", Code: "browser_runner_unavailable"}
	}
	receipt, err := requestBrowserRun(ctx, socket, request)
	if err != nil {
		return Result{Status: "fail", Code: "browser_runner_unavailable", failure: err}
	}
	if receipt.Outcome != "ok" {
		return Result{Status: "fail", Code: "browser_" + receipt.Outcome}
	}
	values := make(map[string]any, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		value := float64(receipt.ElapsedMS)
		if metric.Preset == "steps_completed" {
			value = float64(receipt.Completed)
		}
		values[selection.Reference(metric)] = value
	}
	return Result{Status: "ok", Payload: map[string]any{"browser_values_v1": values}}
}

func requestBrowserRun(ctx context.Context, socket string, request browserjourney.Request) (browserjourney.Receipt, error) {
	var blank browserjourney.Receipt
	payload, err := json.Marshal(request)
	if err != nil {
		return blank, err
	}
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true, MaxResponseHeaderBytes: browserjourney.MaxWire,
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://browser-runner"+browserjourney.RunPath, bytes.NewReader(payload))
	if err != nil {
		return blank, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return blank, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return blank, errors.New("browser_runner_http_error")
	}
	receipt, err := browserjourney.Decode[browserjourney.Receipt](response.Body)
	if err != nil || !receipt.Matches(request) {
		return blank, errors.New("browser_runner_invalid_receipt")
	}
	return receipt, nil
}
