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
		r.aiClient = ai.NewAnthropicClient(apiKey, cfg.Model, cfg.MaxTokens)

	case "gemini", "google":
		cfg := r.cfg.AI.Gemini
		apiKey := cfg.APIKey
		if apiKey == "" && cfg.APIKeyEnv != "" {
			apiKey = os.Getenv(cfg.APIKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("Gemini API key not configured.\n  Set 'ai.gemini.api_key' in config.yaml\n  Or export %s environment variable.\n  Get your key at: https://aistudio.google.com/apikey", cfg.APIKeyEnv)
		}
		r.aiClient = ai.NewGeminiClient(apiKey, cfg.Model, cfg.MaxTokens)

	default:
		return fmt.Errorf("unknown AI provider: %q (supported: anthropic, gemini)", r.cfg.AI.Provider)
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

func (r *replState) generateMRDescription(projectPath, sourceBranch, targetBranch string) (string, error) {
	if err := r.ensureAI(); err != nil {
		return "", err
	}

	commitMessages, err := r.glClient.GetBranchDiff(projectPath, targetBranch, sourceBranch)
	if err != nil {
		return "", err
	}
	if len(commitMessages) == 0 {
		return fmt.Sprintf("Merge %s into %s\n\nNo new commits.", sourceBranch, targetBranch), nil
	}

	ctx := context.Background()
	prompt := ai.BuildMRDescriptionPrompt(sourceBranch, targetBranch, commitMessages)
	systemPrompt := "You are a helpful assistant that writes clear, professional merge request descriptions."
	return r.aiClient.Chat(ctx, systemPrompt, prompt)
}

func (r *replState) askAI(prompt string) (string, error) {
	ctx := context.Background()

	systemPrompt := `You are a helpful AI assistant integrated into the gitlab-ai CLI tool. Answer questions clearly and concisely.

This CLI tool provides the following commands:
- start                                — Start a GitLab session (authenticate using ~/.netrc credentials)
- list                                 — List all accessible GitLab projects
- mr-review  <project> [mr-number]     — Review a merge request with AI and save to markdown
- mr-comment <project> [mr-number]     — Post a previously generated review as a GitLab MR comment
- mr-status  <project>                 — List open merge requests for a project
- mr-checks  <project> <mr-number>     — Show CI/CD pipeline status and jobs for an MR
- mr-open    <project> [branch] [target] — Create a new merge request with AI-generated description
- branch-cleanup <project>             — Find and delete stale/merged branches
- ticket-open                          — Create a new ticket in a selected project
- tickets                              — Generate open tickets report across all projects
- tickets-black                        — Generate malformed tickets report across all projects
- release                              — Check release status of all projects (compare master vs development)
- exit                                 — End the session and show a summary

Tips you can share:
- Tab auto-completes commands and project names.
- Arrow keys navigate history. Ctrl+R searches history.
- The session auto-starts on first command that needs GitLab access.
- Most commands support interactive mode: omit optional args and you'll get prompts with suggestions.
- mr-review without an MR number shows top 5 open MRs to choose from.
- mr-open without a branch shows top 5 active branches to choose from.
- After mr-review, you're prompted to optionally post the review as a comment.
- Session times out after 1 hour of inactivity.

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
