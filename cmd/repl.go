package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/briandowns/spinner"
	"github.com/chzyer/readline"
	"github.com/fatih/color"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/ai"
	"gitlab-ai/pkg/config"
	projectctx "gitlab-ai/pkg/context"
	"gitlab-ai/pkg/gitlab"
	"gitlab-ai/pkg/output"
)

// replState holds the interactive session state.
type replState struct {
	cfg        *config.AppConfig
	glClient   *gitlab.Client
	aiClient   ai.ChatClient
	rl         *readline.Instance
	username   string
	startedAt  time.Time
	lastReview *lastReviewCtx
	stats      sessionStats

	// Project cache (populated async on session start)
	projectCache []models.ProjectInfo
	cacheMu      sync.RWMutex
	cacheReady   chan struct{}

	// Idle timeout
	idleTimer *time.Timer
	timedOut  atomic.Bool
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

// ─── Auto-complete ───────────────────────────────────────────────────────────

func (r *replState) buildCompleter() *readline.PrefixCompleter {
	projectDynamic := readline.PcItemDynamic(func(line string) []string {
		return r.projectSuggestions()
	})

	return readline.NewPrefixCompleter(
		readline.PcItem("start"),
		readline.PcItem("list"),
		readline.PcItem("index",
			projectDynamic,
		),
		readline.PcItem("mr",
			readline.PcItem("-p", projectDynamic),
		),
		readline.PcItem("add",
			readline.PcItem("comment"),
		),
		readline.PcItem("tickets",
			readline.PcItem("-p", projectDynamic),
		),
		readline.PcItem("exit"),
	)
}

// projectSuggestions returns cached project paths + names for auto-complete (thread-safe).
func (r *replState) projectSuggestions() []string {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	seen := make(map[string]bool, len(r.projectCache)*2)
	suggestions := make([]string, 0, len(r.projectCache)*2)

	// Paths first (full qualified)
	for _, p := range r.projectCache {
		if !seen[p.Path] {
			suggestions = append(suggestions, p.Path)
			seen[p.Path] = true
		}
	}
	// Then short names (for convenience)
	for _, p := range r.projectCache {
		if !seen[p.Name] {
			suggestions = append(suggestions, p.Name)
			seen[p.Name] = true
		}
	}
	return suggestions
}

// waitForCache waits for the project cache to be ready (up to 5s) and returns it.
func (r *replState) waitForCache() []models.ProjectInfo {
	if r.cacheReady != nil {
		select {
		case <-r.cacheReady:
		case <-time.After(5 * time.Second):
		}
	}
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	result := make([]models.ProjectInfo, len(r.projectCache))
	copy(result, r.projectCache)
	return result
}

// fetchProjectCache fetches projects and stores them in the cache.
func (r *replState) fetchProjectCache() {
	if r.cacheReady != nil {
		defer close(r.cacheReady)
	}
	projects, err := r.glClient.ListProjects()
	if err != nil {
		return // silently — cache stays empty
	}
	r.cacheMu.Lock()
	r.projectCache = projects
	r.cacheMu.Unlock()
}

// resolveProject resolves user input to a project path.
// Priority: exact path match → case-insensitive path match →
// exact name match → case-insensitive name match → return input as-is.
func (r *replState) resolveProject(input string) string {
	cache := r.waitForCache()
	if len(cache) == 0 {
		return input // no cache, let the API handle it
	}

	lower := strings.ToLower(input)

	// 1. Exact path match
	for _, p := range cache {
		if p.Path == input {
			return p.Path
		}
	}

	// 2. Case-insensitive path match
	for _, p := range cache {
		if strings.ToLower(p.Path) == lower {
			return p.Path
		}
	}

	// 3. Exact name match → return path
	for _, p := range cache {
		if p.Name == input {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}

	// 4. Case-insensitive name match → return path
	for _, p := range cache {
		if strings.ToLower(p.Name) == lower {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}

	// 5. Partial path suffix match (e.g. "mgmt-srv" matches "cnips/mgmt-srv")
	for _, p := range cache {
		if strings.HasSuffix(strings.ToLower(p.Path), "/"+lower) {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}

	// Not found in cache — return as-is, let the API try
	return input
}

// resetIdle resets the idle timeout timer.
func (r *replState) resetIdle() {
	if !r.timedOut.Load() && r.idleTimer != nil {
		r.idleTimer.Reset(5 * time.Minute)
	}
}

// ─── REPL Entry Point ───────────────────────────────────────────────────────

// RunREPL starts the interactive chatbot session.
func RunREPL(cfg *config.AppConfig) {
	r := &replState{cfg: cfg}

	completer := r.buildCompleter()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "gitlab-ai> ",
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistoryFile:       "/tmp/gitlab-ai-history.tmp",
		HistoryLimit:      500,
		HistorySearchFold: true, // case-insensitive prefix search on ↑/↓
	})
	if err != nil {
		fmt.Printf("Failed to initialize: %v\n", err)
		return
	}
	defer rl.Close()
	r.rl = rl

	// Welcome banner
	cyan := color.New(color.FgCyan, color.Bold)
	bold := color.New(color.Bold)

	fmt.Println()
	cyan.Println("🤖 gitlab-ai — Interactive Mode")
	fmt.Println()
	bold.Println("Commands:")
	fmt.Println("  start                        Start GitLab session")
	fmt.Println("  list                         List accessible projects")
	fmt.Println("  index <project> <path>       Index local code for context")
	fmt.Println("  mr <number> <project_name>   Review MR with AI (uses context)")
	fmt.Println("  add comment <number>         Post review as MR comment")
	fmt.Println("  tickets <project_name>       List project issues & save")
	fmt.Println("  exit                         End session")
	fmt.Println()
	providerLabel := cfg.AI.Provider
	if providerLabel == "" {
		providerLabel = "anthropic"
	}
	fmt.Printf("  Any other input is sent to %s AI as a question.\n", providerLabel)
	fmt.Println()
	color.New(color.FgYellow).Println("Auto-exit after 5 minutes of inactivity.")
	fmt.Println()
	dim := color.New(color.Faint)
	dim.Println("Shortcuts: Tab = auto-complete | ↑/↓ = history | ←/→ = move cursor | Ctrl+R = search history")
	fmt.Println()

	// Idle timeout — fires once, closes readline
	r.idleTimer = time.AfterFunc(5*time.Minute, func() {
		r.timedOut.Store(true)
		rl.Close()
	})
	defer r.idleTimer.Stop()

	// Main read loop
	for {
		line, err := rl.Readline()

		// Check timeout first
		if r.timedOut.Load() {
			fmt.Println("\n⏰ Session timed out (5 minutes of inactivity)")
			r.handleExit()
			return
		}

		if err != nil {
			if err == readline.ErrInterrupt {
				r.resetIdle()
				continue
			}
			// io.EOF or readline closed
			r.handleExit()
			return
		}

		r.resetIdle()

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if r.dispatch(line) {
			r.idleTimer.Stop()
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
	case "list":
		r.handleList()
	case "index":
		r.handleIndex(parts[1:])
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

// ensureSession auto-starts a GitLab session if one isn't active.
func (r *replState) ensureSession() bool {
	if r.glClient != nil {
		return true
	}

	output.PrintWarning("No active session — auto-starting...")

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Authenticating with GitLab..."
	s.Start()

	client, err := gitlab.NewClient(&r.cfg.GitLab)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Authentication failed: %v", err))
		return false
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

	// Async fetch projects for cache & auto-complete
	r.cacheReady = make(chan struct{})
	go r.fetchProjectCache()

	return true
}

func (r *replState) handleStart() {
	if r.glClient != nil {
		output.PrintWarning("Session already active. Use 'exit' to end current session.")
		return
	}
	r.ensureSession()
}

func (r *replState) handleList() {
	if !r.ensureSession() {
		return
	}

	// Use cached projects if available, otherwise fetch
	projects := r.waitForCache()
	if len(projects) == 0 {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Fetching projects..."
		s.Start()

		var err error
		projects, err = r.glClient.ListProjects()
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to list projects: %v", err))
			return
		}
	}

	output.PrintProjectsTable(projects)
	fmt.Println()
}

func (r *replState) handleIndex(args []string) {
	var project, localPath string

	// Parse: index <project> <local_path>
	if len(args) >= 2 {
		project = args[0]
		localPath = strings.Join(args[1:], " ")
	} else if len(args) == 1 {
		project = args[0]
	}

	// Resolve project name if we have a session
	if project != "" && r.glClient != nil {
		project = r.resolveProject(project)
	}

	// Prompt for missing args
	if project == "" {
		project = r.promptForProject("Select project to index")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
		if r.glClient != nil {
			project = r.resolveProject(project)
		}
	}

	if localPath == "" {
		fmt.Println()
		color.New(color.FgCyan, color.Bold).Println("Local Code Path")
		fmt.Println("  Enter the absolute path to the project source code:")

		r.rl.SetPrompt("path> ")
		line, err := r.rl.Readline()
		r.rl.SetPrompt("gitlab-ai> ")
		r.resetIdle()
		if err != nil {
			return
		}
		localPath = strings.TrimSpace(line)
		if localPath == "" {
			output.PrintError("No path provided.")
			return
		}
	}

	// Expand ~ if present
	if strings.HasPrefix(localPath, "~/") {
		home, _ := os.UserHomeDir()
		localPath = filepath.Join(home, localPath[2:])
	}

	// Index the directory
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Indexing %s...", localPath)
	s.Start()

	indexContent, err := projectctx.IndexDirectory(localPath)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to index: %v", err))
		return
	}

	// Save to context file
	if err := projectctx.SaveIndex(project, indexContent); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save context: %v", err))
		return
	}

	ctxPath := projectctx.ContextPath(project)
	fi, _ := os.Stat(ctxPath)
	sizeKB := float64(0)
	if fi != nil {
		sizeKB = float64(fi.Size()) / 1024
	}

	output.PrintSuccess(fmt.Sprintf("Project indexed → %s (%.1f KB)", ctxPath, sizeKB))
	fmt.Println("  This context will be used for AI reviews of this project.")
	fmt.Println()
}

func (r *replState) handleMR(args []string) {
	if !r.ensureSession() {
		return
	}

	// Parse: mr <number> <project_name>  OR  mr <number> -p <project>
	project, remaining := parseProjectFlag(args)

	// Collect MR number and project name from remaining positional args
	var mrNumberStr string
	if project == "" {
		// No -p flag — expect positional args: <number> <project_name>
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && mrNumberStr == "" {
				mrNumberStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else {
		// -p was used — remaining should have the MR number
		if len(remaining) > 0 {
			mrNumberStr = remaining[0]
		}
	}

	// If no project, prompt interactively
	if project == "" {
		project = r.promptForProject("Select project for MR review")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}

	// Resolve project name → path
	project = r.resolveProject(project)

	// Parse MR number
	var mrNumber int
	if mrNumberStr != "" {
		n, err := strconv.Atoi(mrNumberStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid MR number: %s", mrNumberStr))
			return
		}
		mrNumber = n
	} else {
		// Prompt for MR number
		mrNumber = r.promptForNumber("MR number")
		if mrNumber <= 0 {
			output.PrintError("Invalid MR number.")
			return
		}
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

	// Step 2: Load project context (if indexed)
	projContext, _ := projectctx.LoadContextTruncated(project, 60000) // ~60KB cap
	if projContext != "" {
		output.PrintSuccess("Project context loaded for AI review")
	}

	// Step 3: AI Review
	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	suffix := fmt.Sprintf(" Generating AI review via %s", r.aiClient.ProviderName())
	if projContext != "" {
		suffix += " (with project context)"
	}
	s.Suffix = suffix + "..."
	s.Start()

	reviewText, err := r.reviewWithAI(mrInfo, projContext)
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

	// Step 6: Update project context with this review
	if err := projectctx.AppendMRReview(project, mrNumber, mrInfo.Title, reviewText); err != nil {
		output.PrintWarning(fmt.Sprintf("Could not update project context: %v", err))
	} else {
		output.PrintSuccess("Project context updated with MR review")
	}
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
	if !r.ensureSession() {
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
	if !r.ensureSession() {
		return
	}

	// Parse: tickets <project_name>  OR  tickets -p <project>
	project, remaining := parseProjectFlag(args)

	// No -p flag — treat remaining args as project name
	if project == "" && len(remaining) > 0 {
		project = strings.Join(remaining, " ")
	}

	// If still no project, prompt interactively
	if project == "" {
		project = r.promptForProject("Select project for tickets")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}

	// Resolve project name → path
	project = r.resolveProject(project)

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

	// Update project context with ticket data
	ticketsSummary := buildTicketsSummaryForContext(result)
	if err := projectctx.UpdateTickets(project, ticketsSummary); err != nil {
		output.PrintWarning(fmt.Sprintf("Could not update project context: %v", err))
	} else {
		output.PrintSuccess("Project context updated with tickets")
	}
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

// ─── Interactive Prompts ─────────────────────────────────────────────────────

// promptForProject asks for a project name (with tab auto-complete from cache).
func (r *replState) promptForProject(title string) string {
	fmt.Println()
	color.New(color.FgCyan, color.Bold).Println(title)
	fmt.Println("  Enter project name or path (Tab to auto-complete):")

	r.rl.SetPrompt("project> ")
	defer r.rl.SetPrompt("gitlab-ai> ")

	line, err := r.rl.Readline()
	r.resetIdle()

	if err != nil {
		return ""
	}

	input := strings.TrimSpace(line)
	if input == "" {
		return ""
	}

	return input
}

// promptForNumber asks the user for a number.
func (r *replState) promptForNumber(label string) int {
	r.rl.SetPrompt(fmt.Sprintf("%s> ", label))
	defer r.rl.SetPrompt("gitlab-ai> ")

	line, err := r.rl.Readline()
	r.resetIdle()

	if err != nil {
		return 0
	}

	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		return 0
	}
	return n
}

// ─── AI Integration ──────────────────────────────────────────────────────────

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

	sb.WriteString("---\n\n*Generated by gitlab-ai CLI*\n")
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

// buildTicketsSummaryForContext creates a compact summary of tickets for the context file.
func buildTicketsSummaryForContext(result *models.IssueListResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%d open tickets**\n\n", result.TotalCount))

	for _, issue := range result.Issues {
		sb.WriteString(fmt.Sprintf("- **#%d** %s", issue.IID, issue.Title))
		if issue.Assignee != "" {
			sb.WriteString(fmt.Sprintf(" (@%s)", issue.Assignee))
		}
		if len(issue.Labels) > 0 {
			sb.WriteString(fmt.Sprintf(" [%s]", strings.Join(issue.Labels, ", ")))
		}
		sb.WriteString("\n")
		if issue.Description != "" {
			// Include first 200 chars of description for context
			desc := issue.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("  > %s\n", strings.ReplaceAll(desc, "\n", " ")))
		}
	}

	return sb.String()
}
