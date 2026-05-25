package cmd

import (
	"fmt"
	"strings"

	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

func (r *replState) handleConfig() {
	theme := output.GetTheme()

	output.ThemeHeader("Current Configuration")
	fmt.Println()

	theme.Primary.Print("  Platform:       ")
	p := r.cfg.Platform
	if p == "" {
		p = "gitlab"
	}
	fmt.Println(p)

	theme.Primary.Print("  GitLab URL:     ")
	fmt.Println(r.cfg.GitLab.BaseURL)

	theme.Primary.Print("  AI Provider:    ")
	switch r.cfg.AI.Provider {
	case "anthropic", "claude", "":
		if r.cfg.AI.Anthropic.APIKey != "" {
			fmt.Printf("Anthropic (Claude) — %s\n", r.cfg.AI.Anthropic.Model)
		} else {
			theme.Warning.Println("anthropic (ANTHROPIC_API_KEY not set)")
		}
	case "gemini", "google":
		if r.cfg.AI.Gemini.APIKey != "" {
			fmt.Printf("Google Gemini — %s\n", r.cfg.AI.Gemini.Model)
		} else {
			theme.Warning.Println("gemini (GEMINI_API_KEY not set)")
		}
	default:
		theme.Warning.Printf("unknown provider %q\n", r.cfg.AI.Provider)
	}

	theme.Primary.Print("  Color Output:   ")
	fmt.Println(boolStr(r.cfg.CLI.ColorOutput))

	theme.Primary.Print("  Output Format:  ")
	f := r.cfg.CLI.OutputFormat
	if f == "" {
		f = "text"
	}
	fmt.Println(f)

	theme.Primary.Print("  Theme:          ")
	t := r.cfg.CLI.Theme
	if t == "" {
		t = "default"
	}
	fmt.Println(t)

	theme.Primary.Print("  Confirm Posts:  ")
	fmt.Println(boolStr(r.cfg.CLI.ConfirmBeforePost))

	theme.Primary.Print("  Idle Timeout:   ")
	fmt.Printf("%d minutes\n", r.cfg.CLI.IdleTimeoutMinutes)

	if len(r.cfg.Teams) > 0 {
		theme.Primary.Print("  Teams:          ")
		fmt.Println(strings.Join(r.cfg.Teams, ", "))
	}

	fmt.Println()

	supported := platform.SupportedPlatforms()
	theme.Muted.Printf("  Supported platforms: %s\n", strings.Join(supported, ", "))
	theme.Muted.Println("  Config: config.yaml (+ env var overrides)")

	if r.provider != nil {
		theme.Muted.Printf("  Active session:      %s (@%s)\n", r.provider.Name(), r.provider.Username())
	}
	fmt.Println()
}

func (r *replState) showHelp() {
	theme := output.GetTheme()

	fmt.Println()
	theme.Header.Println("  git-agent — Command Reference")
	fmt.Println()

	printSection := func(title string, cmds []helpCmd) {
		theme.Primary.Printf("  %s\n", title)
		for _, c := range cmds {
			fmt.Printf("    %-36s %s\n", c.name, c.desc)
		}
		fmt.Println()
	}

	printSection("Merge Requests", []helpCmd{
		{"mr-status <project>", "List open MRs"},
		{"mr-review <project> [mr]", "AI-powered MR review"},
		{"mr-comment <project> [mr]", "Post review as MR comment"},
		{"mr-open <project> [branch] [target]", "Create a new MR"},
		{"mr-merge <project> <mr>", "Merge an MR"},
		{"mr-approve <project> <mr>", "Approve an MR"},
		{"mr-unapprove <project> <mr>", "Remove approval"},
		{"mr-rebase <project> <mr>", "Rebase source branch"},
		{"mr-update <project> <mr>", "Update MR metadata"},
		{"mr-close <project> <mr>", "Close an MR"},
		{"mr-reopen <project> <mr>", "Reopen a closed MR"},
		{"mr-checks <project> <mr>", "Pipeline status for MR"},
	})

	printSection("Pipelines / CI", []helpCmd{
		{"pipeline <project>", "List recent pipelines"},
		{"pipeline-view <project> <id>", "Pipeline details and jobs"},
		{"pipeline-logs <project> <job-id>", "Job log output"},
		{"pipeline-retry <project> <id>", "Retry failed pipeline"},
		{"pipeline-cancel <project> <id>", "Cancel running pipeline"},
	})

	printSection("Tickets / Issues", []helpCmd{
		{"ticket-open [project]", "Create a new ticket"},
		{"ticket-open-empty", "Quick-create empty ticket (assigned to you)"},
		{"ticket-close <project> <number>", "Close a ticket"},
		{"ticket-reopen <project> <number>", "Reopen a ticket"},
		{"ticket-update <project> <number>", "Update ticket metadata"},
		{"ticket-search <project>", "Search tickets"},
		{"tickets", "Generate tickets report"},
	})

	printSection("Repository", []helpCmd{
		{"list / projects", "List team projects"},
		{"diff <project>", "Compare tags or branches"},
		{"branch-cleanup <project>", "Remove stale branches"},
		{"release", "Check release status"},
	})

	printSection("Content Generation", []helpCmd{
		{"create-ticket-content [project]", "Generate ticket content from diff"},
		{"create-ticket-desc [project]", "Generate description from linked MRs"},
		{"create-epic-content [project]", "Generate epic content from diff"},
	})

	printSection("Workflow", []helpCmd{
		{"all-in-one", "Full flow: ticket → folder → commit → push → MR"},
	})

	printSection("Session", []helpCmd{
		{"start", "Start session"},
		{"config", "Show current configuration"},
		{"help", "Show this help"},
		{"exit", "End session"},
	})

	theme.Muted.Println("  Any unrecognized input is sent to AI as a question.")
	theme.Muted.Println("  Tab = auto-complete | Up/Down = history | Ctrl+R = search history")
	fmt.Println()
}

type helpCmd struct {
	name string
	desc string
}

func boolStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
