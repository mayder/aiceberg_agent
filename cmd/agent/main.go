//go:build !windows
// +build !windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	app "github.com/you/aiceberg_agent/internal/bootstrap"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
)

var configPath = flag.String("config", "", "path to config file (.env|.json|.yaml)")

func main() {
	flag.Parse()

	cfg, err := config.Load(*configPath)
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
