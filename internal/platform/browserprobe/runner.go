package browserprobe

import (
	"context"
	"crypto/x509"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Run executes a recipe with an isolated profile and Chromium's built-in sandbox.
func Run(ctx context.Context, chromePath string, configured Journey) Receipt {
	return run(ctx, chromePath, configured, nil)
}

func run(ctx context.Context, chromePath string, configured Journey, roots *x509.CertPool) Receipt {
	started := time.Now()
	receipt := Receipt{Contract: Contract, Scope: configured.scope()}
	journey, err := validate(configured)
	if err != nil {
		receipt.Outcome = "invalid_recipe"
		return receipt
	}
	if !browserSupported(chromePath) {
		receipt.Outcome = "runner_unavailable"
		return receipt
	}
	ctx, stop := context.WithTimeout(ctx, time.Duration(journey.BudgetMS)*time.Millisecond)
	defer stop()
	receipt.Completed, receipt.Outcome = execute(ctx, chromePath, journey, roots)
	receipt.ElapsedMS = time.Since(started).Milliseconds()
	if errors.Is(ctx.Err(), context.Canceled) {
		receipt.Outcome = "cancelled"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		receipt.Outcome = "timeout"
	}
	return receipt
}

func browserSupported(path string) bool {
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || os.Geteuid() < 1 || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func execute(ctx context.Context, chromePath string, journey validatedJourney, roots *x509.CertPool) (int, string) {
	profile, err := os.MkdirTemp("", "aiceberg-browser-probe-")
	if err != nil {
		return 0, "runner_unavailable"
	}
	defer os.RemoveAll(profile)
	denyProxy, closeProxy, err := startDenyProxy()
	if err != nil {
		return 0, "runner_unavailable"
	}
	defer closeProxy()

	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, browserOptions(chromePath, profile, denyProxy)...)
	defer stopAllocator()
	tab, stopTab := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer stopTab()
	operation, stopOperation := context.WithCancel(tab)
	defer stopOperation()

	gate, closeTransport := newRequestGate(journey, roots)
	defer closeTransport()
	detach, err := gate.attach(operation, stopOperation)
	if err != nil {
		return 0, "runner_unavailable"
	}
	defer detach()
	if err := chromedp.Do(operation, chromedp.Func(configureBrowser)); err != nil {
		return 0, "runner_unavailable"
	}
	for index, step := range journey.Steps {
		err := executeStep(operation, journey, step)
		if gate.exceeded.Load() {
			return index, "resource_limit"
		}
		if err != nil {
			return index, "step_failed"
		}
	}
	return len(journey.Steps), "ok"
}

func configureBrowser(ctx context.Context, target *chromedp.Target) error {
	browserSession := chromedp.FromContext(ctx).Browser
	commandLine, err := cdp.Call(ctx, browserSession, browser.GetBrowserCommandLine, cdp.Empty{})
	if err != nil || !allowsBrowserSandbox(commandLine.Arguments) {
		return errors.New("browser_sandbox_unavailable")
	}
	if _, err := cdp.Call(ctx, browserSession, browser.SetDownloadBehavior, browser.SetDownloadBehaviorParams{
		Behavior: browser.SetDownloadBehaviorBehaviorDeny,
	}); err != nil {
		return err
	}
	if _, err := cdp.Call(ctx, target, network.SetBypassServiceWorker, network.SetBypassServiceWorkerParams{Bypass: true}); err != nil {
		return err
	}
	_, err = cdp.Call(ctx, target, fetch.Enable, fetch.EnableParams{Patterns: []*fetch.RequestPattern{{URLPattern: "*", RequestStage: fetch.RequestStageRequest}}})
	return err
}

func executeStep(ctx context.Context, journey validatedJourney, step Step) error {
	selector := `[data-testid="` + step.TestID + `"]`
	switch step.Action {
	case "navigate":
		target := *journey.origin
		target.Path = step.Path
		response, err := chromedp.Run(ctx, chromedp.NavigateResponse(target.String()))
		if err != nil || response == nil || response.Status < 200 || response.Status >= 300 {
			return errors.New("browser_navigation_failed")
		}
	case "click":
		if err := chromedp.Do(ctx, chromedp.Click(chromedp.CSS(selector), chromedp.NodeVisible)); err != nil {
			return err
		}
	case "expect_text":
		if _, err := chromedp.Run(ctx, chromedp.PollFunction[bool](assertVisibleText, chromedp.WithPollingArgs(selector, step.Text))); err != nil {
			return err
		}
	}
	current, err := chromedp.Run(ctx, chromedp.Location())
	if err != nil {
		return err
	}
	parsed, err := url.Parse(current)
	if err != nil || !journey.sameOrigin(parsed) {
		return errors.New("browser_origin_outside_recipe")
	}
	return nil
}

const assertVisibleText = `(selector, expected) => {
	const element = document.querySelector(selector);
	return !!element && element.checkVisibility({checkOpacity: true, checkVisibilityCSS: true})
		&& element.innerText.includes(expected);
}`
