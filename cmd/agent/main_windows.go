//go:build windows
// +build windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc"

	app "github.com/you/aiceberg_agent/internal/bootstrap"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

const serviceName = "AIcebergAgent"

var configPath = flag.String("config", "", "path to config file (.env|.json|.yaml)")
var doctor = flag.Bool("doctor", false, "run local channel diagnostics and exit")

func main() {
	flag.Parse()

	if *doctor {
		cfg, err := config.Load(*configPath)
		report := usecase.RunChannelDoctor(context.Background(), cfg, err)
		_ = json.NewEncoder(os.Stdout).Encode(report)
		os.Exit(usecase.ChannelDoctorExitCode(report))
	}

	isSvc, err := svc.IsWindowsService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "svc detection failed: %v\n", err)
		os.Exit(1)
	}

	if !isSvc {
		runForeground()
		return
	}

	cfg, log, err := loadConfigAndLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	if err := svc.Run(serviceName, &agentService{cfg: cfg, log: log}); err != nil {
		log.Fatal("service failed", "err", err)
	}
}

func runForeground() {
	cfg, log, err := loadConfigAndLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	if err := app.Run(context.Background(), cfg, log); err != nil {
		log.Fatal("app run failed", "err", err)
	}
}

func loadConfigAndLogger() (config.Config, logger.Logger, error) {
	cfg, err := config.Load(*configPath)
	if err != nil {
		return cfg, nil, err
	}
	log := logger.New(cfg.Agent.LogLevel)
	return cfg, log, nil
}

type agentService struct {
	cfg config.Config
	log logger.Logger
}

func (s *agentService) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, s.cfg, s.log)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending, Accepts: accepted}
				cancel()
				<-done
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			default:
			}
		case err := <-done:
			if err != nil {
				s.log.Error(logger.KV("service stopped unexpectedly",
					"err", err,
				))
			} else {
				s.log.Info(logger.KV("service stopped"))
			}
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}
