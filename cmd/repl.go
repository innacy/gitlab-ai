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
	"gitlab-ai/pkg/gitlab"
	"gitlab-ai/pkg/output"
)

// replState holds the interactive session state.
type replState struct {
	cfg      *config.AppConfig
	glClient *gitlab.Client
	aiClient ai.ChatClient
	rl       *readline.Instance
	username string

	startedAt   time.Time
	reviews     map[string]*reviewEntry
	reviewOrder []string
	stats       sessionStats

	activeTeam   string
	projectCache []models.ProjectInfo
	cacheMu      sync.RWMutex
	cacheReady   chan struct{}
	cacheTicker  *time.Ticker
	cacheStop    chan struct{}

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
		readline.PcItem("tickets-black"),
		readline.PcItem("mr-status", projectDynamic),
		readline.PcItem("mr-open", projectDynamic),
		readline.PcItem("mr-review", projectDynamic),
		readline.PcItem("mr-comment", projectDynamic),
		readline.PcItem("mr-checks", projectDynamic),
		readline.PcItem("diff", projectDynamic),
		readline.PcItem("branch-cleanup", projectDynamic),
		readline.PcItem("create-ticket-content", projectDynamic),
		readline.PcItem("create-epic-content", projectDynamic),
		readline.PcItem("release"),
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
	projects, err := r.glClient.ListProjects()
	if err != nil {
		return
	}

	if r.activeTeam != "" {
		team := strings.ToLower(strings.TrimSpace(r.activeTeam))
		filtered := make([]models.ProjectInfo, 0, len(projects))
		for _, p := range projects {
			path := strings.ToLower(p.Path)
			if strings.HasPrefix(path, team+"/") || strings.Contains(path, "/"+team+"/") {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastActivity.After(projects[j].LastActivity)
	})

	r.cacheMu.Lock()
	r.projectCache = projects
	r.cacheMu.Unlock()
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

func (r *replState) startCacheTicker() {
	r.cacheTicker = time.NewTicker(5 * time.Minute)
	r.cacheStop = make(chan struct{})
	go func() {
		for {
			select {
			case <-r.cacheTicker.C:
				r.refreshCache()
			case <-r.cacheStop:
				r.cacheTicker.Stop()
				return
			}
		}
	}()
}

func (r *replState) stopCacheTicker() {
	if r.cacheStop != nil {
		close(r.cacheStop)
	}
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
		r.idleTimer.Reset(1 * time.Hour)
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
		Prompt:            "gitlab-ai> ",
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistoryFile:       "/tmp/gitlab-ai-history.tmp",
		HistoryLimit:      500,
		HistorySearchFold: true,
	})
	if err != nil {
		fmt.Printf("Failed to initialize: %v\n", err)
		return
	}
	defer rl.Close()
	r.rl = rl

	cyan := color.New(color.FgCyan, color.Bold)
	bold := color.New(color.Bold)

	fmt.Println()
	cyan.Println("🤖 gitlab-ai — Interactive Mode")
	fmt.Println()
	bold.Println("Commands:")
	fmt.Println("  start                                  - Start session, pick team & list projects")
	fmt.Println("  list                                   - List team projects")
	fmt.Println("  ticket-open                            - Create a new ticket")
	fmt.Println("  tickets                                - Generate open tickets report for all projects")
	fmt.Println("  tickets-black                          - Generate malformed tickets report for all projects")
	fmt.Println("  mr-status  <project>                   - List open/merged MRs")
	fmt.Println("  mr-review  <project> [mr-number]       - Review MR with AI")
	fmt.Println("  mr-comment <project> [mr-number]       - Post review as MR comment")
	fmt.Println("  mr-open    <project> [branch] [target] - Create a new MR")
	fmt.Println("  mr-checks  <project> <mr-number>       - Show CI/CD pipeline status")
	fmt.Println("  diff       <project>                   - Compare tags or branches (git diff)")
	fmt.Println("  branch-cleanup <project>               - Remove stale/merged branches")
	fmt.Println("  create-ticket-content [project]        - Generate ticket content from branch diff")
	fmt.Println("  create-epic-content   [project]        - Generate epic content from branch diff")
	fmt.Println("  release                                - Check release status of all projects")
	fmt.Println("  exit                                   - End session")
	fmt.Println()
	providerLabel := cfg.AI.Provider
	if providerLabel == "" {
		providerLabel = "anthropic"
	}
	fmt.Printf("  Any other input is sent to %s AI as a question.\n", providerLabel)
	fmt.Println()
	color.New(color.FgYellow).Println("Auto-exit after 1 hour of inactivity.")
	fmt.Println()
	dim := color.New(color.Faint)
	dim.Println("Shortcuts: Tab = auto-complete | ↑/↓ = history | ←/→ = move cursor | Ctrl+R = search history")
	fmt.Println()

	r.idleTimer = time.AfterFunc(1*time.Hour, func() {
		r.timedOut.Store(true)
		rl.Close()
	})
	defer r.idleTimer.Stop()

	for {
		line, err := rl.Readline()

		if r.timedOut.Load() {
			fmt.Println("\n⏰ Session timed out (1 hour of inactivity)")
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
	case "diff":
		r.handleDiff(parts[1:])
	case "branch-cleanup":
		r.handleBranchCleanup(parts[1:])
	case "tickets":
		r.handleTickets(parts[1:])
	case "ticket-open":
		r.handleTicketOpen(parts[1:])
	case "tickets-black":
		r.handleTicketsBlack(parts[1:])
	case "create-ticket-content":
		r.handleCreateTicketContent(parts[1:])
	case "create-epic-content":
		r.handleCreateEpicContent(parts[1:])
	case "release":
		r.handleRelease()
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
	if r.glClient != nil {
		return true
	}

	output.PrintWarning("No active session — auto-starting...")

	s := newSpinner(" Authenticating with GitLab...")
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

	r.startCacheTicker()

	output.PrintSuccess("Session started — ready for commands")
	fmt.Println()

	return true
}

func (r *replState) handleStart() {
	if r.glClient != nil {
		output.PrintWarning("Session already active. Use 'exit' to end current session.")
		return
	}
	if !r.ensureSession() {
		return
	}

	projects := r.waitForCache()
	if len(projects) > 0 {
		output.PrintSuccess(fmt.Sprintf("Loaded %d projects for team '%s'", len(projects), r.activeTeam))
	} else {
		output.PrintWarning("No projects found for the selected team.")
	}
	fmt.Println()
}

func (r *replState) handleExit() {
	r.stopCacheTicker()

	fmt.Println()
	if r.glClient != nil && !r.startedAt.IsZero() {
		duration := time.Since(r.startedAt).Round(time.Second)
		cyan := color.New(color.FgCyan, color.Bold)
		cyan.Println("Session Summary")
		fmt.Printf("  Duration:      %s\n", duration)
		fmt.Printf("  Team:          %s\n", r.activeTeam)
		fmt.Printf("  MRs reviewed:  %d\n", r.stats.mrsReviewed)
		fmt.Printf("  MRs created:   %d\n", r.stats.mrsCreated)
		fmt.Printf("  Files created: %d\n", r.stats.filesCreated)
		fmt.Printf("  Issues viewed: %d\n", r.stats.issuesViewed)
	}
	fmt.Println()
	fmt.Println("Goodbye! 👋")
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
