package main

import (
	"fmt"
	"os"

	"gitlab-ai/cmd"
	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/utils"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	utils.InitLogger(cfg.CLI.Verbose)

	cmd.RunREPL(cfg)
}
