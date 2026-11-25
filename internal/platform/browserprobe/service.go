package browserprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/browserjourney"
)

type service struct {
	recipes string
	run     func(context.Context, Journey) Receipt
	slot    chan struct{}
}

// Serve starts a local runner. The socket directory is provisioned for the
// service owner and a dedicated HUB group, without write access for the group.
func Serve(ctx context.Context, socket, recipes, chromium string) error {
	if !browserSupported(chromium) || !privateDirectory(recipes) || !privateDirectory(filepath.Dir(socket)) ||
		!filepath.IsAbs(socket) || socket != filepath.Clean(socket) || len(socket) > 100 {
		return errors.New("browser_service_invalid_configuration")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0o660); err != nil {
		return err
	}
	runner := service{recipes: recipes, run: func(ctx context.Context, journey Journey) Receipt {
		return Run(ctx, chromium, journey)
	}, slot: make(chan struct{}, 1)}
	server := &http.Server{
		Handler: runner.handler(), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second,
		WriteTimeout: 35 * time.Second, MaxHeaderBytes: 4096,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(stopped)
		shutdown, release := context.WithTimeout(context.Background(), 5*time.Second)
		defer release()
		if server.Shutdown(shutdown) != nil {
			_ = server.Close()
		}
	})
	defer func() {
		if !stop() {
			<-stopped
		}
	}()
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func privateDirectory(path string) bool {
	if !filepath.IsAbs(path) || path != filepath.Clean(path) {
		return false
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && currentOwner(info) && info.Mode().Perm()&0o027 == 0
}

func (s service) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != browserjourney.RunPath || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		request, err := browserjourney.Decode[browserjourney.Request](r.Body)
		if err != nil || !request.Valid() {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receipt := s.handle(r.Context(), request)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(receipt)
	})
}

func (s service) handle(ctx context.Context, request browserjourney.Request) Receipt {
	receipt := Receipt{Contract: Contract, Scope: request.Scope, Digest: request.Digest}
	journey, ok := s.load(request)
	if !ok {
		receipt.Outcome = "not_allowed"
		return receipt
	}
	select {
	case s.slot <- struct{}{}:
		defer func() { <-s.slot }()
	default:
		receipt.Outcome = "busy"
		return receipt
	}
	if request.BudgetMS < journey.BudgetMS {
		journey.BudgetMS = request.BudgetMS
	}
	receipt = s.run(ctx, journey)
	receipt.Digest = request.Digest
	return receipt
}

func (s service) load(request browserjourney.Request) (Journey, bool) {
	name := fmt.Sprintf("%d-%d-%d-%d.json", request.ClientID, request.AssetID, request.CheckID, request.EndpointID)
	raw, err := safefile.ReadPrivate(filepath.Join(s.recipes, name), MaxRecipe)
	if err != nil {
		return Journey{}, false
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != request.Digest {
		return Journey{}, false
	}
	j, err := Decode(raw)
	if err != nil || j.scope() != request.Scope {
		return Journey{}, false
	}
	approved, err := validate(j)
	if err != nil {
		return Journey{}, false
	}
	origin, _, err := validateOrigin(request.Origin, j.Address)
	return j, err == nil && approved.sameOrigin(origin)
}
