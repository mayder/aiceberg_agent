package browserprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	maxRequests     = 64
	maxResponseBody = 1 << 20
	maxTotalBodies  = 8 << 20
	fetchWorkers    = 4
)

type requestGate struct {
	recipe     validatedJourney
	client     *http.Client
	requests   atomic.Int64
	totalBytes atomic.Int64
	exceeded   atomic.Bool
}

func newRequestGate(journey validatedJourney, roots *x509.CertPool) (*requestGate, func()) {
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			address := net.JoinHostPort(journey.pin.String(), originPort(journey.origin))
			return dialer.DialContext(ctx, network, address)
		},
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, ServerName: journey.origin.Hostname(), RootCAs: roots},
		MaxConnsPerHost:        fetchWorkers,
		MaxResponseHeaderBytes: 32 << 10,
		DisableCompression:     true,
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return &requestGate{recipe: journey, client: client}, transport.CloseIdleConnections
}

func (gate *requestGate) attach(ctx context.Context, stopJourney context.CancelFunc) (func(), error) {
	queue := make(chan *fetch.EventRequestPaused, maxRequests)
	dispatch, cancel := context.WithCancel(ctx)
	if err := chromedp.Listen(dispatch, func(event any) {
		paused, ok := event.(*fetch.EventRequestPaused)
		if !ok || paused.Request == nil {
			return
		}
		if gate.requests.Add(1) > maxRequests {
			gate.exceeded.Store(true)
			stopJourney()
			return
		}
		select {
		case queue <- paused:
		case <-dispatch.Done():
		}
	}); err != nil {
		cancel()
		return nil, err
	}

	var workers sync.WaitGroup
	for range fetchWorkers {
		workers.Go(func() {
			for {
				select {
				case <-dispatch.Done():
					return
				case event := <-queue:
					gate.serve(dispatch, event)
					if gate.exceeded.Load() {
						stopJourney()
					}
				}
			}
		})
	}
	return func() { cancel(); workers.Wait() }, nil
}

func (gate *requestGate) serve(ctx context.Context, event *fetch.EventRequestPaused) {
	params, allowed := gate.response(ctx, event)
	_ = chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		if allowed {
			_, err := cdp.Call(ctx, target, fetch.FulfillRequest, params)
			return err
		}
		_, err := cdp.Call(ctx, target, fetch.FailRequest, fetch.FailRequestParams{
			RequestID: event.RequestID, ErrorReason: network.ErrorReasonBlockedByClient,
		})
		return err
	}))
}

func (gate *requestGate) response(ctx context.Context, event *fetch.EventRequestPaused) (fetch.FulfillRequestParams, bool) {
	blank := fetch.FulfillRequestParams{}
	candidate, err := url.Parse(event.Request.URL)
	if err != nil || candidate.Opaque != "" || !gate.recipe.sameOrigin(candidate) || len(event.Request.URL) > 2048 ||
		(event.Request.Method != http.MethodGet && event.Request.Method != http.MethodHead) {
		return blank, false
	}
	request, err := http.NewRequestWithContext(ctx, event.Request.Method, candidate.String(), nil)
	if err != nil {
		return blank, false
	}
	request.Header.Set("User-Agent", "AIceberg-browser-probe/1")
	response, err := gate.client.Do(request)
	if err != nil {
		return blank, false
	}
	defer response.Body.Close()
	if !gate.allowedResponse(response) {
		return blank, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if len(body) > maxResponseBody || gate.totalBytes.Add(int64(len(body))) > maxTotalBodies {
		gate.exceeded.Store(true)
		return blank, false
	}
	if err != nil {
		return blank, false
	}
	return fetch.FulfillRequestParams{RequestID: event.RequestID, ResponseCode: int64(response.StatusCode),
		ResponseHeaders: filteredHeaders(response.Header), Body: body}, true
}

func (gate *requestGate) allowedResponse(response *http.Response) bool {
	if response.StatusCode == http.StatusSwitchingProtocols ||
		strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Disposition")), "attachment") {
		return false
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return false
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		location, err := response.Location()
		return err == http.ErrNoLocation || err == nil && gate.recipe.sameOrigin(location)
	}
	return true
}

func filteredHeaders(source http.Header) []*fetch.HeaderEntry {
	headers := make([]*fetch.HeaderEntry, 0, 8)
	for _, key := range []string{"Content-Type", "Content-Security-Policy", "Cache-Control", "Location", "X-Content-Type-Options", "Referrer-Policy"} {
		for _, value := range source.Values(key) {
			headers = append(headers, &fetch.HeaderEntry{Name: key, Value: value})
		}
	}
	return headers
}
