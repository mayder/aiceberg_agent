package browserprobe

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func browserOptions(binary, profile, denyProxy string) []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(binary),
		chromedp.UserDataDir(profile),
		chromedp.Headless,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoInheritEnv,
		chromedp.Env("HOME="+profile, "XDG_CONFIG_HOME="+profile, "XDG_CACHE_HOME="+profile, "LANG=C"),
		chromedp.Flag("no-sandbox", false),
		chromedp.Flag("enable-automation", true),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-quic", true),
		chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"),
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND"),
		chromedp.Flag("proxy-server", denyProxy),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		chromedp.Flag("disable-features", "MediaRouter,OptimizationHints"),
	}
}

func allowsBrowserSandbox(arguments []string) bool {
	if len(arguments) == 0 {
		return false
	}
	for _, argument := range arguments[1:] {
		flag, _, _ := strings.Cut(argument, "=")
		switch flag {
		case "--no-sandbox", "--disable-seccomp-filter-sandbox", "--single-process":
			return false
		}
	}
	return true
}

func startDenyProxy() (string, func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := http.Server{
		ReadHeaderTimeout: time.Second, WriteTimeout: time.Second, MaxHeaderBytes: 4096,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	return "http://" + listener.Addr().String(), func() { _ = server.Close(); <-done }, nil
}
