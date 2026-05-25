package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"gitlab-ai/cmd"
	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/utils"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if !cfg.CLI.ColorOutput || os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}

	if cfg.CLI.OutputFormat != "" {
		output.SetOutputFormat(cfg.CLI.OutputFormat)
	}

	utils.InitLogger(cfg.CLI.Verbose)

	cmd.RunREPL(cfg)
}
