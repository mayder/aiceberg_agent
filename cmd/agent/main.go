//go:build !windows
// +build !windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	app "github.com/you/aiceberg_agent/internal/bootstrap"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/platform/telnetprobe"
)

var configPath = flag.String("config", "", "path to config file (.env|.json|.yaml)")
var doctor = flag.Bool("doctor", false, "run local channel diagnostics and exit")

func main() {
	if len(os.Args) == 2 && os.Args[1] == telnetprobe.WorkerArgument {
		os.Exit(telnetprobe.RunInherited(os.Stdout))
	}
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if *doctor {
		report := usecase.RunChannelDoctor(context.Background(), cfg, err)
		_ = json.NewEncoder(os.Stdout).Encode(report)
		os.Exit(usecase.ChannelDoctorExitCode(report))
	}
	if err != nil {
		fmt.Printf("config load error: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Agent.LogLevel)
	defer log.Sync()

	if err := app.Run(context.Background(), cfg, log); err != nil {
		log.Fatal("app run failed", "err", err)
	}
}
