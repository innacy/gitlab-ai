package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/ai"
	"gitlab-ai/pkg/output"
)

// ─── AI Client Setup ─────────────────────────────────────────────────────────

func (r *replState) ensureAI() error {
	if r.aiClient != nil {
		return nil
	}

	provider := strings.ToLower(r.cfg.AI.Provider)

	timeout := time.Duration(r.cfg.AI.TimeoutSeconds) * time.Second

	switch provider {
	case "anthropic", "claude", "":
		cfg := r.cfg.AI.Anthropic
		apiKey := cfg.APIKey
		if apiKey == "" && cfg.APIKeyEnv != "" {
			apiKey = os.Getenv(cfg.APIKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("Anthropic API key not configured.\n  Set 'ai.anthropic.api_key' in config.yaml\n  Or export %s environment variable.\n  Get your key at: https://console.anthropic.com/settings/keys", cfg.APIKeyEnv)
		}
		r.aiClient = ai.NewAnthropicClient(apiKey, cfg.Model, cfg.MaxTokens, timeout)

	case "gemini", "google":
		cfg := r.cfg.AI.Gemini
		apiKey := cfg.APIKey
		if apiKey == "" && cfg.APIKeyEnv != "" {
			apiKey = os.Getenv(cfg.APIKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("Gemini API key not configured.\n  Set 'ai.gemini.api_key' in config.yaml\n  Or export %s environment variable.\n  Get your key at: https://aistudio.google.com/apikey", cfg.APIKeyEnv)
		}
		r.aiClient = ai.NewGeminiClient(apiKey, cfg.Model, cfg.MaxTokens, timeout)

	case "nvidia":
		cfg := r.cfg.AI.Nvidia
		apiKey := cfg.APIKey
		if apiKey == "" && cfg.APIKeyEnv != "" {
			apiKey = os.Getenv(cfg.APIKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("NVIDIA API key not configured.\n  Set 'ai.nvidia.api_key' in config.yaml\n  Or export %s environment variable.\n  Get your key at: https://build.nvidia.com/", cfg.APIKeyEnv)
		}
		r.aiClient = ai.NewNvidiaClient(apiKey, cfg.Model, cfg.MaxTokens, timeout)

	default:
		return fmt.Errorf("unknown AI provider: %q (supported: anthropic, gemini, nvidia)", r.cfg.AI.Provider)
	}

	return nil
}

// ─── AI Operations ───────────────────────────────────────────────────────────

func (r *replState) reviewWithAI(mr *models.MergeRequestInfo, projectContext string) (string, error) {
	ctx := context.Background()

	systemPrompt := ai.BuildSystemPrompt()
	if projectContext != "" {
		systemPrompt += "\n\n## Project Context\n" +
			"Use the following project knowledge (code structure, past reviews, tickets) " +
			"to provide more accurate, context-aware reviews:\n\n" + projectContext
	}

	userPrompt := ai.BuildReviewPrompt(mr, r.cfg.Review.Template.Sections)

	return r.aiClient.Chat(ctx, systemPrompt, userPrompt)
}

func (r *replState) generateMRDescription(projectPath, sourceBranch, targetBranch string) (description string, commits []string, err error) {
	diff, err := r.provider.MRs().GetBranchDiffFull(projectPath, targetBranch, sourceBranch)
	if err != nil {
		commits, simpleErr := r.provider.MRs().GetBranchDiff(projectPath, targetBranch, sourceBranch)
		if simpleErr != nil {
			return "", nil, simpleErr
		}
		return r.generateMRDescriptionFromCommits(sourceBranch, targetBranch, commits)
	}

	if len(diff.Commits) == 0 && len(diff.Files) == 0 {
		return fmt.Sprintf("Merge %s into %s\n\nNo new commits.", sourceBranch, targetBranch), nil, nil
	}

	ctx := context.Background()
	systemPrompt := "You are a helpful assistant that writes clear, professional merge request descriptions."

	// Tier 1: Direct AI with full-diff prompt
	if aiErr := r.ensureAI(); aiErr == nil {
		prompt := ai.BuildMRDescriptionPromptFull(sourceBranch, targetBranch, diff)
		if desc, chatErr := r.aiClient.Chat(ctx, systemPrompt, prompt); chatErr == nil {
			return desc, diff.Commits, nil
		} else {
			output.PrintWarning(fmt.Sprintf("Primary AI failed: %v", chatErr))
		}
	} else {
		output.PrintWarning(fmt.Sprintf("Primary AI unavailable: %v", aiErr))
	}

	// Tier 2: Raw template fallback
	output.PrintWarning("Using template-based description (no AI available)")
	return ai.BuildTemplateDescription(sourceBranch, targetBranch, diff), diff.Commits, nil
}

func (r *replState) generateMRDescriptionFromCommits(sourceBranch, targetBranch string, commits []string) (string, []string, error) {
	if len(commits) == 0 {
		return fmt.Sprintf("Merge %s into %s\n\nNo new commits.", sourceBranch, targetBranch), commits, nil
	}

	ctx := context.Background()
	systemPrompt := "You are a helpful assistant that writes clear, professional merge request descriptions."

	// Tier 1: Direct AI with commit-based prompt
	if aiErr := r.ensureAI(); aiErr == nil {
		prompt := ai.BuildMRDescriptionPrompt(sourceBranch, targetBranch, commits)
		if desc, chatErr := r.aiClient.Chat(ctx, systemPrompt, prompt); chatErr == nil {
			return desc, commits, nil
		}
	}

	// Tier 2: Raw commit list
	rawDesc := fmt.Sprintf("## Overview\n\nMerge `%s` into `%s`.\n\n## Changes\n\n- %s\n",
		sourceBranch, targetBranch, strings.Join(commits, "\n- "))
	return rawDesc, commits, nil
}

func (r *replState) enhanceTicketDescription(userContext string) (string, string, error) {
	ctx := context.Background()
	systemPrompt := "You are a technical writer that creates precise, actionable GitLab tickets. Be concise — no filler, no extra detail."
	prompt := ai.BuildTicketDescriptionPrompt(userContext)

	response, err := r.aiClient.Chat(ctx, systemPrompt, prompt)
	if err != nil {
		return "", "", err
	}

	response = strings.TrimSpace(response)
	lines := strings.SplitN(response, "\n", 2)

	title := strings.TrimSpace(lines[0])
	title = strings.TrimPrefix(title, "# ")
	title = strings.TrimPrefix(title, "## ")
	if len(title) > 72 {
		title = strings.TrimSpace(title[:72])
	}

	description := ""
	if len(lines) > 1 {
		description = strings.TrimSpace(lines[1])
	}

	return title, description, nil
}

func (r *replState) askAI(prompt string) (string, error) {
	ctx := context.Background()

	systemPrompt := `You are a helpful AI assistant integrated into the gitlab-ai CLI tool. Answer questions clearly and concisely.

This CLI tool provides the following commands:

Merge Requests:
- mr-status  <project>                   — List open MRs
- mr-review  <project> [mr]              — AI-powered MR review
- mr-comment <project> [mr]              — Post review as MR comment
- mr-open    <project> [branch] [target] — Create a new MR
- mr-merge   <project> <mr>              — Merge an MR
- mr-approve <project> <mr>              — Approve an MR
- mr-unapprove <project> <mr>            — Remove approval
- mr-rebase  <project> <mr>              — Rebase source branch
- mr-update  <project> <mr>              — Update MR metadata
- mr-close   <project> <mr>              — Close an MR
- mr-reopen  <project> <mr>              — Reopen a closed MR
- mr-checks  <project> <mr>              — Pipeline status for MR

Pipelines / CI:
- pipeline        <project>              — List recent pipelines
- pipeline-view   <project> <id>         — Pipeline details and jobs
- pipeline-logs   <project> <job-id>     — Job log output
- pipeline-retry  <project> <id>         — Retry failed pipeline
- pipeline-cancel <project> <id>         — Cancel running pipeline

Tickets / Issues:
- ticket-open   [project]                — Create a new ticket
- ticket-close  <project> <number>       — Close a ticket
- ticket-reopen <project> <number>       — Reopen a ticket
- ticket-update <project> <number>       — Update ticket metadata
- ticket-search <project>                — Search tickets
- tickets                                — Generate tickets report

Repository:
- list / projects                        — List team projects
- diff <project>                         — Compare tags or branches
- branch-cleanup <project>               — Remove stale branches
- release                                — Check release status

Content Generation:
- create-ticket-content [project]        — Generate ticket content from branch diff
- create-ticket-desc    [project]        — Generate/update ticket description from linked MRs
- create-epic-content   [project]        — Generate epic content from diff

Session:
- start                                  — Start session
- config                                 — Show current configuration
- help                                   — Show available commands
- exit                                   — End session

Tips you can share:
- Tab auto-completes commands and project names.
- Arrow keys navigate history. Ctrl+R searches history.
- The session auto-starts on first command that needs GitLab access.
- Most commands support interactive mode: omit optional args and you'll get prompts with suggestions.
- mr-review without an MR number shows top 5 open MRs to choose from.
- mr-open without a branch shows top 5 active branches to choose from.
- After mr-review, you're prompted to optionally post the review as a comment.
- create-ticket-desc picks a ticket, finds its linked MRs, and updates the description.
- Any unrecognized input is sent to AI as a question.

When users ask about commands, capabilities, or how to use this tool, provide helpful guidance based on these commands. For all other questions, answer as a general-purpose AI assistant.`
	return r.aiClient.Chat(ctx, systemPrompt, prompt)
}

// ─── Chat Handler ────────────────────────────────────────────────────────────

func (r *replState) handleChat(input string) {
	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Thinking..."
	s.Start()

	response, err := r.askAI(input)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("AI error: %v", err))
		return
	}

	fmt.Println()
	fmt.Println(response)
	fmt.Println()
}
