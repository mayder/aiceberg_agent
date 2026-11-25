package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/platform/browserprobe"
)

func main() {
	recipePath := flag.String("recipe", "", "absolute path to the private browser journey recipe")
	chromium := flag.String("chromium", "", "absolute path to the managed Chrome or Chromium binary")
	socket := flag.String("socket", "", "absolute path for the local HUB runner socket")
	recipes := flag.String("recipes", "", "directory containing approved private journey recipes")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *socket != "" {
		if *recipePath != "" || browserprobe.Serve(ctx, *socket, *recipes, *chromium) != nil {
			_, _ = os.Stderr.WriteString("browser runner service unavailable\n")
			os.Exit(1)
		}
		return
	}
	data, err := safefile.ReadPrivate(*recipePath, browserprobe.MaxRecipe)
	if err != nil {
		emit(browserprobe.Receipt{Contract: browserprobe.Contract, Outcome: "invalid_recipe"})
		return
	}
	journey, err := browserprobe.Decode(data)
	if err != nil {
		emit(browserprobe.Receipt{Contract: browserprobe.Contract, Outcome: "invalid_recipe"})
		return
	}
	emit(browserprobe.Run(ctx, *chromium, journey))
}

func emit(receipt browserprobe.Receipt) {
	_ = json.NewEncoder(os.Stdout).Encode(receipt)
	if receipt.Outcome != "ok" {
		os.Exit(1)
	}
}
