package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/ai"
	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/gitlab"
	"gitlab-ai/pkg/output"
)

// replState holds the interactive session state.
type replState struct {
	cfg        *config.AppConfig
	glClient   *gitlab.Client
	aiClient   *ai.AnthropicClient
	username   string
	startedAt  time.Time
	lastReview *lastReviewCtx
	stats      sessionStats
}

type lastReviewCtx struct {
	project  string
	mrNumber int
	filePath string
	comment  string // formatted for GitLab MR comment
}

type sessionStats struct {
	mrsReviewed  int
	filesCreated int
	issuesViewed int
}

// RunREPL starts the interactive chatbot session.
func RunREPL(cfg *config.AppConfig) {
	r := &replState{cfg: cfg}

	cyan := color.New(color.FgCyan, color.Bold)
	bold := color.New(color.Bold)

	fmt.Println()
	cyan.Println("🤖 gitlab-ai — Interactive Mode")
	fmt.Println()
	bold.Println("Commands:")
	fmt.Println("  start                    Start GitLab session")
	fmt.Println("  mr <number> -p <project> Review MR with AI")
	fmt.Println("  add comment <number>     Post review as MR comment")
	fmt.Println("  tickets -p <project>     List project issues & save")
	fmt.Println("  exit                     End session")
	fmt.Println()
	fmt.Println("  Any other input is sent to Claude AI as a question.")
	fmt.Println()
	color.New(color.FgYellow).Println("Auto-exit after 1 minute of inactivity.")
	fmt.Println()

	inputCh := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			inputCh <- scanner.Text()
		}
		close(inputCh)
	}()

	timeout := time.NewTimer(1 * time.Minute)
	defer timeout.Stop()

	for {
		fmt.Print("gitlab-ai> ")
		timeout.Reset(1 * time.Minute)

		select {
		case line, ok := <-inputCh:
			if !ok {
				r.handleExit()
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if r.dispatch(line) {
				return
			}
		case <-timeout.C:
			fmt.Println("\n⏰ Session timed out (1 minute of inactivity)")
			r.handleExit()
			return
		}
	}
}

func (r *replState) dispatch(line string) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "start":
		r.handleStart()
	case "mr":
		r.handleMR(parts[1:])
	case "add":
		if len(parts) >= 3 && strings.ToLower(parts[1]) == "comment" {
			r.handleAddComment(parts[2:])
		} else {
			output.PrintError("Usage: add comment <mr_number>")
		}
	case "tickets":
		r.handleTickets(parts[1:])
	case "exit":
		r.handleExit()
		return true
	default:
		r.handleChat(line)
	}

	return false
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func (r *replState) handleStart() {
	if r.glClient != nil {
		output.PrintWarning("Session already active. Use 'exit' to end current session.")
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Authenticating with GitLab..."
	s.Start()

	client, err := gitlab.NewClient(&r.cfg.GitLab)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Authentication failed: %v", err))
		return
	}

	r.glClient = client
	r.username = client.User().Username
	r.startedAt = time.Now()

	user := client.User()
	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Authenticated as %s (@%s)", user.Name, user.Username))
	output.PrintSuccess(fmt.Sprintf("GitLab: %s", r.cfg.GitLab.BaseURL))
	output.PrintSuccess("Session started — ready for commands")
	fmt.Println()
}

func (r *replState) handleMR(args []string) {
	if r.glClient == nil {
		output.PrintError("No active session. Run 'start' first.")
		return
	}

	// Parse: 226 -p mgmt
	project, remaining := parseProjectFlag(args)
	if project == "" {
		output.PrintError("Usage: mr <number> -p <project>")
		return
	}
	if len(remaining) == 0 {
		output.PrintError("Usage: mr <number> -p <project>")
		return
	}

	mrNumber, err := strconv.Atoi(remaining[0])
	if err != nil {
		output.PrintError(fmt.Sprintf("Invalid MR number: %s", remaining[0]))
		return
	}

	// Step 1: Fetch MR from GitLab
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching MR #%d from '%s'...", mrNumber, project)
	s.Start()

	mrInfo, err := r.glClient.GetMergeRequest(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch MR: %v", err))
		return
	}

	output.PrintMRInfo(mrInfo)

	// Step 2: AI Review via Claude
	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Generating AI review via Claude..."
	s.Start()

	reviewText, err := r.reviewWithAI(mrInfo)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("AI review failed: %v", err))
		return
	}

	output.PrintSuccess("AI review generated")

	// Step 3: Build Review model
	sections := parseReviewSections(reviewText)
	additions, deletions := gitlab.CountMRChanges(mrInfo.Changes)

	review := &models.Review{
		ProjectName:  project,
		MRNumber:     mrInfo.IID,
		MRTitle:      mrInfo.Title,
		MRURL:        mrInfo.WebURL,
		Author:       fmt.Sprintf("%s (@%s)", mrInfo.Author, mrInfo.AuthorUser),
		Reviewer:     r.username,
		ReviewDate:   time.Now().UTC(),
		Description:  mrInfo.Description,
		SourceBranch: mrInfo.SourceBranch,
		TargetBranch: mrInfo.TargetBranch,
		FilesChanged: len(mrInfo.Changes),
		Additions:    additions,
		Deletions:    deletions,
		Sections:     sections,
		RawResponse:  reviewText,
	}

	// Display review in terminal
	output.PrintReview(review)

	// Step 4: Save to file (replace if exists)
	filename := fmt.Sprintf("%s-%d.md", sanitizeProject(project), mrNumber)
	content := buildReviewMarkdown(review)

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save review: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Review saved to: %s", filename))
	fmt.Println()

	// Store context for "add comment" command
	r.lastReview = &lastReviewCtx{
		project:  project,
		mrNumber: mrNumber,
		filePath: filename,
		comment:  output.GenerateGitLabComment(review),
	}
	r.stats.mrsReviewed++
	r.stats.filesCreated++
}

func (r *replState) handleAddComment(args []string) {
	if r.glClient == nil {
		output.PrintError("No active session. Run 'start' first.")
		return
	}
	if r.lastReview == nil {
		output.PrintError("No review to post. Run 'mr <number> -p <project>' first.")
		return
	}
	if len(args) == 0 {
		output.PrintError("Usage: add comment <mr_number>")
		return
	}

	mrNumber, err := strconv.Atoi(args[0])
	if err != nil {
		output.PrintError(fmt.Sprintf("Invalid MR number: %s", args[0]))
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Posting review to MR #%d...", mrNumber)
	s.Start()

	noteURL, err := r.glClient.PostMRComment(r.lastReview.project, mrNumber, r.lastReview.comment)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to post comment: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Review posted to MR #%d", mrNumber))
	output.PrintSuccess(fmt.Sprintf("View at: %s", noteURL))
	fmt.Println()
}

func (r *replState) handleTickets(args []string) {
	if r.glClient == nil {
		output.PrintError("No active session. Run 'start' first.")
		return
	}

	project, _ := parseProjectFlag(args)
	if project == "" {
		output.PrintError("Usage: tickets -p <project>")
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching tickets from '%s'...", project)
	s.Start()

	result, err := r.glClient.ListAssignedIssues(project, models.IssueFilter{State: "opened"})
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch tickets: %v", err))
		return
	}

	// Display table in terminal
	output.PrintIssuesTable(result)
	fmt.Println()

	// Save to file: project_date_time.md
	now := time.Now()
	filename := fmt.Sprintf("%s_%s.md", sanitizeProject(project), now.Format("2006-01-02_15-04"))
	content := buildTicketsMarkdown(result)

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save tickets: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Tickets saved to: %s", filename))
	fmt.Println()

	r.stats.issuesViewed += result.TotalCount
	r.stats.filesCreated++
}

func (r *replState) handleExit() {
	fmt.Println()
	if r.glClient != nil && !r.startedAt.IsZero() {
		duration := time.Since(r.startedAt).Round(time.Second)
		cyan := color.New(color.FgCyan, color.Bold)
		cyan.Println("Session Summary")
		fmt.Printf("  Duration:      %s\n", duration)
		fmt.Printf("  MRs reviewed:  %d\n", r.stats.mrsReviewed)
		fmt.Printf("  Files created: %d\n", r.stats.filesCreated)
		fmt.Printf("  Issues viewed: %d\n", r.stats.issuesViewed)
	}
	fmt.Println()
	fmt.Println("Goodbye! 👋")
	fmt.Println()
}

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

// ─── Claude AI Integration ───────────────────────────────────────────────────

func (r *replState) ensureAI() error {
	if r.aiClient != nil {
		return nil
	}

	cfg := r.cfg.AI.Anthropic

	// Priority: direct api_key in config → env var
	apiKey := cfg.APIKey
	if apiKey == "" && cfg.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.APIKeyEnv)
	}
	if apiKey == "" {
		return fmt.Errorf("Anthropic API key not configured.\n  Set 'ai.anthropic.api_key' in ~/.gitlab-ai/config.yaml\n  Or export %s environment variable.\n  Get your key at: https://console.anthropic.com/settings/keys", cfg.APIKeyEnv)
	}

	r.aiClient = ai.NewAnthropicClient(apiKey, cfg.Model, cfg.MaxTokens)
	return nil
}

func (r *replState) reviewWithAI(mr *models.MergeRequestInfo) (string, error) {
	ctx := context.Background()

	systemPrompt := ai.BuildSystemPrompt()
	userPrompt := ai.BuildReviewPrompt(mr, r.cfg.Review.Template.Sections)

	return r.aiClient.Chat(ctx, systemPrompt, userPrompt)
}

func (r *replState) askAI(prompt string) (string, error) {
	ctx := context.Background()

	systemPrompt := "You are a helpful AI assistant. Answer questions clearly and concisely."
	return r.aiClient.Chat(ctx, systemPrompt, prompt)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseProjectFlag extracts -p <project> from the argument list.
func parseProjectFlag(args []string) (string, []string) {
	var project string
	var remaining []string

	for i := 0; i < len(args); i++ {
		if args[i] == "-p" && i+1 < len(args) {
			project = args[i+1]
			i++ // skip value
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return project, remaining
}

func sanitizeProject(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

// parseReviewSections splits AI response text into named sections by ## headings.
func parseReviewSections(response string) []models.ReviewSection {
	var sections []models.ReviewSection
	lines := strings.Split(response, "\n")
	var current *models.ReviewSection
	var content []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if current != nil {
				current.Content = strings.TrimSpace(strings.Join(content, "\n"))
				sections = append(sections, *current)
			}
			current = &models.ReviewSection{
				Name: strings.TrimPrefix(trimmed, "## "),
			}
			content = nil
		} else if current != nil {
			content = append(content, line)
		}
	}
	if current != nil {
		current.Content = strings.TrimSpace(strings.Join(content, "\n"))
		sections = append(sections, *current)
	}

	if len(sections) == 0 {
		sections = []models.ReviewSection{{Name: "Review", Content: response}}
	}
	return sections
}

// buildReviewMarkdown generates the markdown file content for an MR review.
func buildReviewMarkdown(review *models.Review) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# MR Review: %s (#%d)\n\n", review.MRTitle, review.MRNumber))
	sb.WriteString(fmt.Sprintf("**Project:** %s  \n", review.ProjectName))
	sb.WriteString(fmt.Sprintf("**Author:** %s  \n", review.Author))
	sb.WriteString(fmt.Sprintf("**Branch:** %s → %s  \n", review.SourceBranch, review.TargetBranch))
	sb.WriteString(fmt.Sprintf("**Review Date:** %s  \n", review.ReviewDate.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**MR Link:** %s  \n", review.MRURL))
	sb.WriteString(fmt.Sprintf("**Changes:** %d files, +%d -%d lines\n\n", review.FilesChanged, review.Additions, review.Deletions))
	sb.WriteString("---\n\n")

	for _, section := range review.Sections {
		sb.WriteString(fmt.Sprintf("## %s\n\n", section.Name))
		sb.WriteString(section.Content)
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n\n*Generated by gitlab-ai CLI (Claude AI)*\n")
	return sb.String()
}

// buildTicketsMarkdown generates the markdown file content for project tickets.
func buildTicketsMarkdown(result *models.IssueListResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tickets: %s\n\n", result.ProjectName))
	sb.WriteString(fmt.Sprintf("**Generated:** %s  \n", result.GeneratedAt.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Total:** %d tickets\n\n", result.TotalCount))
	sb.WriteString("---\n\n")

	for _, issue := range result.Issues {
		sb.WriteString(fmt.Sprintf("## #%d: %s\n\n", issue.IID, issue.Title))
		sb.WriteString(fmt.Sprintf("- **State:** %s\n", issue.State))
		sb.WriteString(fmt.Sprintf("- **Author:** @%s\n", issue.Author))
		sb.WriteString(fmt.Sprintf("- **Assignee:** @%s\n", issue.Assignee))
		if len(issue.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("- **Labels:** %s\n", strings.Join(issue.Labels, ", ")))
		}
		if issue.Milestone != "" {
			sb.WriteString(fmt.Sprintf("- **Milestone:** %s\n", issue.Milestone))
		}
		if issue.DueDate != "" {
			sb.WriteString(fmt.Sprintf("- **Due Date:** %s\n", issue.DueDate))
		}
		sb.WriteString(fmt.Sprintf("- **Created:** %s\n", issue.CreatedAt.Format("2006-01-02")))
		sb.WriteString(fmt.Sprintf("- **Updated:** %s\n", issue.UpdatedAt.Format("2006-01-02")))
		sb.WriteString(fmt.Sprintf("- **URL:** %s\n\n", issue.WebURL))

		if issue.Description != "" {
			sb.WriteString("### Description\n\n")
			sb.WriteString(issue.Description)
			sb.WriteString("\n\n")
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString("*Generated by gitlab-ai CLI*\n")
	return sb.String()
}
