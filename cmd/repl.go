package cmd

import (
	"fmt"
	"sort"
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
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
	_ "gitlab-ai/pkg/platform/gitlab"
)

// replState holds the interactive session state.
type replState struct {
	cfg        *config.AppConfig
	provider   platform.Provider
	aiClient ai.ChatClient
	rl       *readline.Instance
	username   string

	startedAt   time.Time
	reviews     map[string]*reviewEntry
	reviewOrder []string
	stats       sessionStats

	activeTeam      string
	projectCache    []models.ProjectInfo
	cacheMu         sync.RWMutex
	cacheReady      chan struct{}
	lastRefreshTime time.Time

	idleTimer *time.Timer
	timedOut  atomic.Bool
}

type reviewEntry struct {
	project    string
	mrNumber   int
	mrTitle    string
	filePath   string
	comment    string
	reviewedAt time.Time
}

type sessionStats struct {
	mrsReviewed  int
	mrsCreated   int
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
		readline.PcItem("projects"),
		readline.PcItem("tickets"),
		readline.PcItem("ticket-open", projectDynamic),
		readline.PcItem("ticket-open-empty"),
		readline.PcItem("all-in-one"),
		readline.PcItem("ticket-close", projectDynamic),
		readline.PcItem("ticket-reopen", projectDynamic),
		readline.PcItem("ticket-update", projectDynamic),
		readline.PcItem("ticket-search", projectDynamic),
		readline.PcItem("create-ticket-desc", projectDynamic),
		readline.PcItem("tickets-black"),
		readline.PcItem("mr-status", projectDynamic),
		readline.PcItem("mr-open", projectDynamic),
		readline.PcItem("mr-review", projectDynamic),
		readline.PcItem("mr-comment", projectDynamic),
		readline.PcItem("mr-checks", projectDynamic),
		readline.PcItem("mr-merge", projectDynamic),
		readline.PcItem("mr-approve", projectDynamic),
		readline.PcItem("mr-unapprove", projectDynamic),
		readline.PcItem("mr-rebase", projectDynamic),
		readline.PcItem("mr-update", projectDynamic),
		readline.PcItem("mr-close", projectDynamic),
		readline.PcItem("mr-reopen", projectDynamic),
		readline.PcItem("pipeline", projectDynamic),
		readline.PcItem("pipeline-view", projectDynamic),
		readline.PcItem("pipeline-logs", projectDynamic),
		readline.PcItem("pipeline-retry", projectDynamic),
		readline.PcItem("pipeline-cancel", projectDynamic),
		readline.PcItem("diff", projectDynamic),
		readline.PcItem("branch-cleanup", projectDynamic),
		readline.PcItem("create-ticket-content", projectDynamic),
		readline.PcItem("create-epic-content", projectDynamic),
		readline.PcItem("release"),
		readline.PcItem("config"),
		readline.PcItem("help"),
		readline.PcItem("exit"),
	)
}

func (r *replState) projectSuggestions() []string {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	seen := make(map[string]bool, len(r.projectCache)*2)
	suggestions := make([]string, 0, len(r.projectCache)*2)

	for _, p := range r.projectCache {
		if !seen[p.Path] {
			suggestions = append(suggestions, p.Path)
			seen[p.Path] = true
		}
	}
	for _, p := range r.projectCache {
		if !seen[p.Name] {
			suggestions = append(suggestions, p.Name)
			seen[p.Name] = true
		}
	}
	return suggestions
}

// ─── Project Cache ───────────────────────────────────────────────────────────

func (r *replState) waitForCache() []models.ProjectInfo {
	if r.cacheReady != nil {
		select {
		case <-r.cacheReady:
		case <-time.After(30 * time.Second):
		}
	}
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	result := make([]models.ProjectInfo, len(r.projectCache))
	copy(result, r.projectCache)
	return result
}

func (r *replState) fetchProjectCache() {
	if r.cacheReady != nil {
		defer close(r.cacheReady)
	}
	r.refreshCache()
}

func (r *replState) refreshCache() {
	var projects []models.ProjectInfo
	var err error

	if r.lastRefreshTime.IsZero() {
		projects, err = r.provider.Repos().ListProjects()
	} else {
		projects, err = r.provider.Repos().ListProjectsSince(r.lastRefreshTime, 1)
	}
	if err != nil {
		return
	}

	r.cacheMu.Lock()
	if r.lastRefreshTime.IsZero() {
		// First load: replace cache entirely
		r.projectCache = r.filterByTeam(projects)
	} else {
		// Merge updated projects into existing cache
		r.mergeIntoCache(r.filterByTeam(projects))
	}

	sort.Slice(r.projectCache, func(i, j int) bool {
		return r.projectCache[i].LastActivity.After(r.projectCache[j].LastActivity)
	})
	r.cacheMu.Unlock()

	r.lastRefreshTime = time.Now()
}

func (r *replState) filterByTeam(projects []models.ProjectInfo) []models.ProjectInfo {
	if r.activeTeam == "" {
		return projects
	}
	team := strings.ToLower(strings.TrimSpace(r.activeTeam))
	filtered := make([]models.ProjectInfo, 0, len(projects))
	for _, p := range projects {
		path := strings.ToLower(p.Path)
		if strings.HasPrefix(path, team+"/") || strings.Contains(path, "/"+team+"/") {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// mergeIntoCache upserts updated projects into the existing cache.
// Must be called with cacheMu held.
func (r *replState) mergeIntoCache(updated []models.ProjectInfo) {
	index := make(map[int]int, len(r.projectCache))
	for i, p := range r.projectCache {
		index[p.ID] = i
	}
	for _, p := range updated {
		if idx, ok := index[p.ID]; ok {
			r.projectCache[idx] = p
		} else {
			r.projectCache = append(r.projectCache, p)
		}
	}
}

func (r *replState) refreshCacheAsync() {
	go r.refreshCache()
}

func (r *replState) syncProjectCache() {
	s := newSpinner(" Syncing projects...")
	s.Start()
	r.refreshCache()
	s.Stop()

	r.cacheMu.RLock()
	count := len(r.projectCache)
	r.cacheMu.RUnlock()

	output.PrintSuccess(fmt.Sprintf("Synced %d projects", count))
}

func (r *replState) smartRefresh() {
	if r.lastRefreshTime.IsZero() {
		return
	}
	r.refreshCache()
}

func (r *replState) resolveProject(input string) string {
	cache := r.waitForCache()
	if len(cache) == 0 {
		return input
	}

	lower := strings.ToLower(input)

	for _, p := range cache {
		if p.Path == input {
			return p.Path
		}
	}
	for _, p := range cache {
		if strings.ToLower(p.Path) == lower {
			return p.Path
		}
	}
	for _, p := range cache {
		if p.Name == input {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}
	for _, p := range cache {
		if strings.ToLower(p.Name) == lower {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}
	for _, p := range cache {
		if strings.HasSuffix(strings.ToLower(p.Path), "/"+lower) {
			output.PrintSuccess(fmt.Sprintf("Resolved '%s' → %s", input, p.Path))
			return p.Path
		}
	}

	return input
}

func (r *replState) resetIdle() {
	if !r.timedOut.Load() && r.idleTimer != nil {
		idleTimeout := time.Duration(r.cfg.CLI.IdleTimeoutMinutes) * time.Minute
		if idleTimeout <= 0 {
			idleTimeout = 1 * time.Hour
		}
		r.idleTimer.Reset(idleTimeout)
	}
}

// ─── REPL Entry Point ───────────────────────────────────────────────────────
// REPL: Read-Eval-Print-Loop
func RunREPL(cfg *config.AppConfig) {
	r := &replState{
		cfg:     cfg,
		reviews: make(map[string]*reviewEntry),
	}

	completer := r.buildCompleter()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "git-agent> ",
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistoryFile:       "/tmp/git-agent-history.tmp",
		HistoryLimit:      500,
		HistorySearchFold: true,
	})
	if err != nil {
		fmt.Printf("Failed to initialize: %v\n", err)
		return
	}
	defer rl.Close()
	r.rl = rl

	theme := output.GetTheme()
	bold := color.New(color.Bold)

	fmt.Println()
	theme.Header.Println("  git-agent — Interactive Mode")
	fmt.Println()
	bold.Println("Merge Requests:")
	fmt.Println("  mr-status  <project>                   - List open MRs")
	fmt.Println("  mr-review  <project> [mr-number]       - Review MR with AI")
	fmt.Println("  mr-comment <project> [mr-number]       - Post review as MR comment")
	fmt.Println("  mr-open    <project> [branch] [target] - Create a new MR")
	fmt.Println("  mr-merge   <project> <mr-number>       - Merge an MR")
	fmt.Println("  mr-approve <project> <mr-number>       - Approve an MR")
	fmt.Println("  mr-rebase  <project> <mr-number>       - Rebase MR source branch")
	fmt.Println("  mr-update  <project> <mr-number>       - Update MR metadata")
	fmt.Println("  mr-close   <project> <mr-number>       - Close an MR")
	fmt.Println("  mr-reopen  <project> <mr-number>       - Reopen a closed MR")
	fmt.Println("  mr-checks  <project> <mr-number>       - Show pipeline status for MR")
	fmt.Println()
	bold.Println("Pipelines / CI:")
	fmt.Println("  pipeline        <project>              - List recent pipelines")
	fmt.Println("  pipeline-view   <project> <id>         - View pipeline details & jobs")
	fmt.Println("  pipeline-logs   <project> <job-id>     - Show job log output")
	fmt.Println("  pipeline-retry  <project> <id>         - Retry a failed pipeline")
	fmt.Println("  pipeline-cancel <project> <id>         - Cancel a running pipeline")
	fmt.Println()
	bold.Println("Tickets / Issues:")
	fmt.Println("  ticket-open    [project]               - Create a new ticket")
	fmt.Println("  ticket-open-empty                      - Quick-create empty ticket (assigned to you)")
	fmt.Println("  ticket-close   <project> <number>      - Close a ticket")
	fmt.Println("  ticket-reopen  <project> <number>      - Reopen a ticket")
	fmt.Println("  ticket-update  <project> <number>      - Update ticket metadata")
	fmt.Println("  ticket-search  <project>               - Search tickets")
	fmt.Println("  tickets                                - Generate tickets report")
	fmt.Println("  tickets-black                          - Generate malformed tickets report")
	fmt.Println()
	bold.Println("Repository:")
	fmt.Println("  list / projects                        - List team projects")
	fmt.Println("  diff           <project>               - Compare tags or branches")
	fmt.Println("  branch-cleanup <project>               - Remove stale/merged branches")
	fmt.Println("  release                                - Check release status of all projects")
	fmt.Println()
	bold.Println("Content Generation:")
	fmt.Println("  create-ticket-content [project]        - Generate ticket from committed branch diff")
	fmt.Println("  create-ticket-desc    [project]        - Generate description from linked MRs")
	fmt.Println("  create-epic-content   [project]        - Generate epic from committed branch diff")
	fmt.Println()
	fmt.Println()
	bold.Println("Workflow:")
	fmt.Println("  all-in-one                             - Full flow: ticket → folder → commit → MR")
	fmt.Println()
	theme.Muted.Println("  start  - Start session  |  exit  - End session")
	theme.Muted.Println("  Any other input is sent to AI as a question.")
	fmt.Println()
	idleTimeout := time.Duration(cfg.CLI.IdleTimeoutMinutes) * time.Minute
	if idleTimeout <= 0 {
		idleTimeout = 1 * time.Hour
	}

	theme.Warning.Printf("  Auto-exit after %s of inactivity.\n", idleTimeout)
	theme.Muted.Println("  Shortcuts: Tab = auto-complete | ↑/↓ = history | Ctrl+R = search history")
	fmt.Println()

	r.idleTimer = time.AfterFunc(idleTimeout, func() {
		r.timedOut.Store(true)
		rl.Close()
	})
	defer r.idleTimer.Stop()

	for {
		line, err := rl.Readline()

		if r.timedOut.Load() {
			fmt.Printf("\n⏰ Session timed out (%s of inactivity)\n", idleTimeout)
			r.handleExit()
			return
		}

		if err != nil {
			if err == readline.ErrInterrupt {
				r.resetIdle()
				continue
			}
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

// ─── Command Dispatch ────────────────────────────────────────────────────────

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
	case "projects":
		r.handleList()
	case "mr-review":
		r.handleMRReview(parts[1:])
	case "mr-comment":
		r.handleMRComment(parts[1:])
	case "mr-status":
		r.handleMRStatus(parts[1:])
	case "mr-checks":
		r.handleMRChecks(parts[1:])
	case "mr-open":
		r.handleMROpen(parts[1:])
	case "mr-merge":
		r.handleMRMerge(parts[1:])
	case "mr-approve":
		r.handleMRApprove(parts[1:])
	case "mr-unapprove":
		r.handleMRUnapprove(parts[1:])
	case "mr-rebase":
		r.handleMRRebase(parts[1:])
	case "mr-update":
		r.handleMRUpdate(parts[1:])
	case "mr-close":
		r.handleMRClose(parts[1:])
	case "mr-reopen":
		r.handleMRReopen(parts[1:])
	case "pipeline":
		r.handlePipeline(parts[1:])
	case "pipeline-view":
		r.handlePipelineView(parts[1:])
	case "pipeline-logs":
		r.handlePipelineLogs(parts[1:])
	case "pipeline-retry":
		r.handlePipelineRetry(parts[1:])
	case "pipeline-cancel":
		r.handlePipelineCancel(parts[1:])
	case "diff":
		r.handleDiff(parts[1:])
	case "branch-cleanup":
		r.handleBranchCleanup(parts[1:])
	case "tickets":
		r.handleTickets(parts[1:])
	case "ticket-open":
		r.handleTicketOpen(parts[1:])
	case "ticket-open-empty":
		r.handleTicketOpenEmpty(parts[1:])
	case "all-in-one":
		r.handleAllInOne(parts[1:])
	case "ticket-close":
		r.handleTicketClose(parts[1:])
	case "ticket-reopen":
		r.handleTicketReopen(parts[1:])
	case "ticket-update":
		r.handleTicketUpdate(parts[1:])
	case "ticket-search":
		r.handleTicketSearch(parts[1:])
	case "create-ticket-desc":
		r.handleTicketDescribe(parts[1:])
	case "tickets-black":
		r.handleTicketsBlack(parts[1:])
	case "create-ticket-content":
		r.handleCreateTicketContent(parts[1:])
	case "create-epic-content":
		r.handleCreateEpicContent(parts[1:])
	case "release":
		r.handleRelease()
	case "config":
		r.handleConfig()
	case "help":
		r.showHelp()
	case "exit":
		r.handleExit()
		return true

	case "mr":
		output.PrintWarning("Command renamed → use 'mr-review' instead.")
	case "add":
		output.PrintWarning("Command renamed → use 'mr-comment' instead.")
	case "index":
		output.PrintWarning("Command removed. Context is managed automatically during reviews.")
	case "raise-mr":
		output.PrintWarning("Command renamed → use 'mr-open' instead.")

	default:
		r.handleChat(line)
	}

	return false
}

// ─── Session ─────────────────────────────────────────────────────────────────

func (r *replState) ensureSession() bool {
	if r.provider != nil {
		return true
	}

	output.PrintWarning("No active session — auto-starting...")

	platformName := r.cfg.Platform
	if platformName == "" {
		platformName = "gitlab"
	}

	s := newSpinner(fmt.Sprintf(" Authenticating with %s...", platformName))
	s.Start()

	prov, err := platform.NewProvider(r.cfg)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Authentication failed: %v", err))
		return false
	}

	r.provider = prov
	r.username = prov.Username()
	r.startedAt = time.Now()

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Authenticated as %s (@%s)", prov.UserDisplayName(), prov.Username()))
	output.PrintSuccess(fmt.Sprintf("Platform: %s", prov.Name()))
	output.PrintURL(r.cfg.GitLab.BaseURL)

	if len(r.cfg.Teams) == 1 {
		r.activeTeam = r.cfg.Teams[0]
		output.PrintSuccess(fmt.Sprintf("Team: %s", r.activeTeam))
	} else if len(r.cfg.Teams) > 1 {
		choice := r.interactiveSelect("Select team", r.cfg.Teams)
		if choice >= 0 {
			r.activeTeam = r.cfg.Teams[choice]
			output.PrintSuccess(fmt.Sprintf("Team: %s", r.activeTeam))
		}
	}

	r.cacheReady = make(chan struct{})
	go r.fetchProjectCache()

	output.PrintSuccess("Session started — ready for commands")
	fmt.Println()

	return true
}

func (r *replState) handleStart() {
	if r.provider != nil {
		output.PrintWarning("Session already active. Use 'exit' to end current session.")
		return
	}
	if !r.ensureSession() {
		return
	}

	s := newSpinner(" Fetching projects...")
	s.Start()
	projects := r.waitForCache()
	s.Stop()

	if len(projects) > 0 {
		output.PrintSuccess(fmt.Sprintf("Loaded %d projects for team '%s'", len(projects), r.activeTeam))
	} else {
		output.PrintWarning("No projects found for the selected team.")
	}
	fmt.Println()
}

func (r *replState) handleExit() {

	fmt.Println()
	if r.provider != nil && !r.startedAt.IsZero() {
		duration := time.Since(r.startedAt).Round(time.Second)
		output.ThemeHeader("Session Summary")
		output.ThemeBox([]string{
			fmt.Sprintf("Duration:      %s", duration),
			fmt.Sprintf("Team:          %s", r.activeTeam),
			fmt.Sprintf("MRs reviewed:  %d", r.stats.mrsReviewed),
			fmt.Sprintf("MRs created:   %d", r.stats.mrsCreated),
			fmt.Sprintf("Files created: %d", r.stats.filesCreated),
			fmt.Sprintf("Issues viewed: %d", r.stats.issuesViewed),
		})
	}
	fmt.Println()
	output.GetTheme().Muted.Println("  Goodbye.")
	fmt.Println()
}

// ─── Shared Utilities ────────────────────────────────────────────────────────

func newSpinner(suffix string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = suffix
	return s
}

func now() time.Time {
	return time.Now()
}
