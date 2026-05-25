package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/ai"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

// ─── Create Ticket Content ───────────────────────────────────────────────────

func (r *replState) handleCreateTicketContent(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	} else {
		project = r.selectProjectWithConfig("Select project (for branch comparison)")
	}
	if project == "" {
		output.PrintError("No project selected.")
		return
	}
	project = r.resolveProject(project)

	sourceBranch, baseBranch := r.promptForBranchPair(project)
	if sourceBranch == "" || baseBranch == "" {
		return
	}

	diff, err := r.fetchDiff(project, baseBranch, sourceBranch)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to get diff: %v", err))
		return
	}

	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	// Fetch full file contents for changed files that need more context.
	// Skip go.mod/go.sum (handled by prompt rule) and deleted files.
	fileContents := r.fetchFileContextForDiff(project, sourceBranch, diff)

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Generating ticket content with AI..."
	s.Start()

	template := r.cfg.TicketContent.Template
	prompt := ai.BuildTicketContentPrompt(diff, template, fileContents)
	systemPrompt := "You are a technical writer that creates precise, actionable GitLab tickets from code diffs. Derive ALL content strictly from the provided diff and file context — do NOT assume, invent, or hallucinate any project context, business logic, or details not present in the diff. Follow the template structure exactly. Be concise — no filler, no extra detail."

	response, err := r.aiClient.Chat(context.Background(), systemPrompt, prompt)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("AI generation failed: %v", err))
		return
	}

	title, description := parseContentResponse(response)

	filename := filepath.Join(r.cfg.Issues.Output.Directory, fmt.Sprintf("ticket_content_%s_%s_%s.md",
		sanitizeProject(project),
		sanitizeRef(sourceBranch),
		time.Now().Format("2006-01-02_15-04"),
	))

	fileContent := fmt.Sprintf("# %s\n\n%s\n", title, description)
	if err := writeFile(filename, fileContent); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save ticket content: %v", err))
		return
	}

	output.PrintFilePath(filename)
	r.stats.filesCreated++
	fmt.Println()

	r.ticketPostAction(project, title, description)
}

// ticketPostAction shows options after content generation: create new, update existing, or skip.
func (r *replState) ticketPostAction(project, title, description string) {
	choice := r.promptForChoice("What would you like to do with this content?", []string{
		"Create new ticket",
		"Update existing ticket description",
		"Skip (content saved to file)",
	})

	switch choice {
	case 0:
		r.createNewTicket(project, title, description)
	case 1:
		r.updateExistingTicket(project, description)
	default:
		return
	}
}

func (r *replState) createNewTicket(project, title, description string) {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Creating ticket in '%s'...", project)
	s.Start()
	issue, err := r.provider.Issues().CreateIssue(project, title, description, nil)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to create ticket: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Ticket #%d created: %s", issue.IID, issue.Title))
	output.PrintURL(issue.WebURL)
	fmt.Println()
	r.refreshCacheAsync()
}

func (r *replState) updateExistingTicket(project, description string) {
	issueIID := r.pickTicket(project, "Select ticket to update")
	if issueIID <= 0 {
		output.PrintError("Invalid ticket number.")
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Updating ticket #%d...", issueIID)
	s.Start()
	updated, err := r.provider.Issues().UpdateIssue(project, issueIID, platform.UpdateIssueOptions{
		Description: &description,
	})
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to update ticket: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Ticket #%d updated: %s", updated.IID, updated.Title))
	output.PrintURL(updated.WebURL)
	fmt.Println()
}

// fetchFileContextForDiff reads full file contents for changed files where the diff alone may lack context.
// Skips dependency files (go.mod, go.sum, package-lock.json, etc.) and deleted files.
func (r *replState) fetchFileContextForDiff(project, ref string, diff *models.DiffResult) map[string]string {
	depFiles := map[string]bool{
		"go.mod": true, "go.sum": true,
		"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
		"Gemfile.lock": true, "poetry.lock": true, "Cargo.lock": true,
	}

	var filesToFetch []string
	for _, f := range diff.Files {
		if f.Deleted {
			continue
		}
		base := filepath.Base(f.NewPath)
		if depFiles[base] {
			continue
		}
		// Fetch full content when diff is small (likely lacks context)
		if f.Additions+f.Deletions < 30 {
			filesToFetch = append(filesToFetch, f.NewPath)
		}
	}

	if len(filesToFetch) == 0 {
		return nil
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Reading %d file(s) for context...", len(filesToFetch))
	s.Start()
	defer s.Stop()

	contents := make(map[string]string, len(filesToFetch))
	for _, path := range filesToFetch {
		content, err := r.provider.Repos().GetFileContent(project, path, ref)
		if err != nil {
			continue
		}
		contents[path] = content
	}
	return contents
}

// ─── Ticket Describe (from linked MRs) ───────────────────────────────────────

func (r *replState) handleTicketDescribe(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	} else {
		project = r.selectProjectWithConfig("Select project")
	}
	if project == "" {
		output.PrintError("No project selected.")
		return
	}
	project = r.resolveProject(project)

	// Step 1: Let user pick a ticket
	issueIID := r.pickTicket(project, "Select ticket to describe")
	if issueIID <= 0 {
		return
	}

	// Step 2: Fetch MRs linked to this ticket
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching MRs linked to #%d...", issueIID)
	s.Start()
	linkedMRs, err := r.provider.Issues().ListRelatedMergeRequests(project, issueIID)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch linked MRs: %v", err))
		return
	}
	if len(linkedMRs) == 0 {
		output.PrintError(fmt.Sprintf("No merge requests linked to ticket #%d.", issueIID))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Found %d linked MR(s):", len(linkedMRs)))
	for _, mr := range linkedMRs {
		fmt.Printf("    !%d  %s → %s  [%s]  %s\n", mr.IID, mr.SourceBranch, mr.TargetBranch, mr.State, mr.Title)
	}
	fmt.Println()

	// Step 3: Collect diffs from all linked MRs
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Fetching diffs from linked MRs..."
	s.Start()

	var entries []ai.MRDiffEntry
	var diffErrors []string
	for _, mr := range linkedMRs {
		diff, err := r.provider.Repos().GetMRDiffByProjectID(mr.ProjectID, mr.IID)
		if err != nil {
			diffErrors = append(diffErrors, fmt.Sprintf("!%d: %v", mr.IID, err))
			continue
		}
		entries = append(entries, ai.MRDiffEntry{
			RepoName:     extractRepoName(mr.WebURL),
			MRURL:        mr.WebURL,
			MRTitle:      mr.Title,
			MRIID:        mr.IID,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			Diff:         diff,
		})
	}
	s.Stop()

	if len(entries) == 0 {
		output.PrintError("Could not fetch diffs from any linked MR.")
		for _, e := range diffErrors {
			output.PrintError(fmt.Sprintf("  %s", e))
		}
		return
	}

	totalFiles, totalAdds, totalDels := 0, 0, 0
	for _, e := range entries {
		totalFiles += len(e.Diff.Files)
		totalAdds += e.Diff.TotalAdditions
		totalDels += e.Diff.TotalDeletions
	}
	output.PrintSuccess(fmt.Sprintf("Combined diff: %d files, +%d -%d lines across %d MR(s)",
		totalFiles, totalAdds, totalDels, len(entries)))

	// Step 4: Generate description with AI
	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Generating ticket description from linked MRs..."
	s.Start()

	template := r.cfg.TicketContent.Template
	prompt := ai.BuildMultiMRTicketContentPrompt(entries, template)
	systemPrompt := "You are a technical writer that creates precise, actionable GitLab tickets from code diffs. Derive ALL content strictly from the provided diffs — do NOT assume, invent, or hallucinate any project context, business logic, or details not present in the diffs. The ticket MUST include a per-repository breakdown. Follow the template structure exactly. Be concise — no filler, no extra detail."

	response, err := r.aiClient.Chat(context.Background(), systemPrompt, prompt)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("AI generation failed: %v", err))
		return
	}

	title, description := parseContentResponse(response)

	fmt.Println()

	// Step 7: Update the ticket (title + description)
	if !r.promptForYesNo(fmt.Sprintf("Update ticket #%d with this content?", issueIID)) {
		return
	}

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Updating ticket #%d...", issueIID)
	s.Start()
	updated, err := r.provider.Issues().UpdateIssue(project, issueIID, platform.UpdateIssueOptions{
		Title:       &title,
		Description: &description,
	})
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to update ticket: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Ticket #%d updated: %s", updated.IID, updated.Title))
	output.PrintURL(updated.WebURL)
	fmt.Println()
}

// pickTicket shows latest 5 tickets + manual entry option. Returns issue IID or 0 on cancel.
func (r *replState) pickTicket(project, title string) int {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Fetching latest tickets..."
	s.Start()

	result, err := r.provider.Issues().ListProjectIssues(project, models.IssueFilter{
		State:   "opened",
		PerPage: 5,
		Page:    1,
	})
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch tickets: %v", err))
		return 0
	}

	if len(result.Issues) == 0 {
		output.PrintWarning("No open tickets found.")
		return r.promptForNumber("ticket number")
	}

	options := make([]string, len(result.Issues)+1)
	for i, iss := range result.Issues {
		options[i] = fmt.Sprintf("#%d — %s (%s)", iss.IID, iss.Title, output.TimeAgo(iss.UpdatedAt))
	}
	options[len(result.Issues)] = "Enter ticket number manually..."

	choice := r.promptForChoice(title, options)
	if choice < 0 {
		return 0
	}
	if choice < len(result.Issues) {
		return result.Issues[choice].IID
	}
	return r.promptForNumber("ticket number")
}

// ─── Create Epic Content ─────────────────────────────────────────────────────

func (r *replState) handleCreateEpicContent(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	} else {
		project = r.promptForProject("Select project (for branch comparison)")
	}
	if project == "" {
		output.PrintError("No project selected.")
		return
	}
	project = r.resolveProject(project)

	sourceBranch, baseBranch := r.promptForBranchPair(project)
	if sourceBranch == "" || baseBranch == "" {
		return
	}

	diff, err := r.fetchDiff(project, baseBranch, sourceBranch)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to get diff: %v", err))
		return
	}

	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Generating epic content with AI..."
	s.Start()

	template := r.cfg.EpicContent.Template
	prompt := ai.BuildEpicContentPrompt(diff, template)
	systemPrompt := "You are a technical writer that creates detailed, comprehensive GitLab epics from code diffs. Derive ALL content strictly from the provided diff — do NOT assume, invent, or hallucinate any project context, business logic, or details not present in the diff. Follow the template structure exactly. Be thorough — include technical details and impact analysis."

	response, err := r.aiClient.Chat(context.Background(), systemPrompt, prompt)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("AI generation failed: %v", err))
		return
	}

	title, description := parseContentResponse(response)

	filename := filepath.Join(r.cfg.Issues.Output.Directory, fmt.Sprintf("epic_content_%s_%s_%s.md",
		sanitizeProject(project),
		sanitizeRef(sourceBranch),
		time.Now().Format("2006-01-02_15-04"),
	))

	fileContent := fmt.Sprintf("# %s\n\n%s\n", title, description)
	if err := writeFile(filename, fileContent); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save epic content: %v", err))
		return
	}

	output.PrintFilePath(filename)
	r.stats.filesCreated++
	fmt.Println()

	if r.promptForYesNo("Do you want to create an epic with this content?") {
		groupPath := r.activeTeam
		if groupPath == "" {
			output.PrintError("No active team set. Cannot create group epic.")
			return
		}

		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = fmt.Sprintf(" Creating epic in group '%s'...", groupPath)
		s.Start()
		epic, err := r.provider.Epics().CreateGroupEpic(groupPath, title, description)
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to create epic: %v", err))
			return
		}

		output.PrintSuccess(fmt.Sprintf("Epic #%d created: %s", epic.IID, epic.Title))
		output.PrintURL(epic.WebURL)
		fmt.Println()
	}
}

// ─── Shared Helpers ──────────────────────────────────────────────────────────

func (r *replState) promptForBranchPair(project string) (sourceBranch, baseBranch string) {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching branches from '%s'...", project)
	s.Start()

	branches, err := r.provider.Repos().ListActiveBranches(project, 10)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch branches: %v", err))
		return "", ""
	}
	if len(branches) == 0 {
		output.PrintWarning("No active branches found.")
		return "", ""
	}

	options := make([]string, len(branches))
	for i, b := range branches {
		options[i] = fmt.Sprintf("%s — %s (%s)", b.Name, b.CommitTitle, output.TimeAgo(b.CommitDate))
	}

	choice := r.promptForChoice("Select source branch (feature branch)", options)
	if choice < 0 {
		return "", ""
	}
	sourceBranch = branches[choice].Name

	targetOptions := []string{"development", "master"}
	targetChoice := r.promptForChoice("Select base branch", targetOptions)
	if targetChoice < 0 {
		return "", ""
	}
	baseBranch = targetOptions[targetChoice]

	return sourceBranch, baseBranch
}

// fetchDiff fetches the diff between two branches. It first looks for an existing MR
// (any state: open, merged, or closed) and uses the MR changes API if found.
// This ensures diffs work even when the source branch has been deleted after merge.
// Falls back to branch compare if no MR exists.
func (r *replState) fetchDiff(project, baseBranch, sourceBranch string) (*models.DiffResult, error) {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching diff %s..%s from '%s'...", baseBranch, sourceBranch, project)
	s.Start()

	var diff *models.DiffResult
	var err error

	mr, mrErr := r.provider.MRs().FindMRByBranches(project, sourceBranch, baseBranch)
	if mrErr == nil && mr != nil {
		diff, err = r.provider.Repos().GetMRDiff(project, mr.IID)
		if err == nil {
			s.Stop()
			output.PrintSuccess(fmt.Sprintf("Using MR !%d [%s] diff — %d files, +%d -%d lines",
				mr.IID, mr.State, len(diff.Files), diff.TotalAdditions, diff.TotalDeletions))
			return diff, nil
		}
	}

	diff, err = r.provider.Repos().GetRefDiff(project, baseBranch, sourceBranch)
	s.Stop()

	if err != nil {
		return nil, err
	}

	if len(diff.Files) == 0 && len(diff.Commits) == 0 {
		output.PrintWarning(fmt.Sprintf("No differences found between %s and %s.", baseBranch, sourceBranch))
		return nil, fmt.Errorf("no differences found")
	}

	output.PrintSuccess(fmt.Sprintf("Diff: %s..%s — %d files, +%d -%d lines, %d commits",
		baseBranch, sourceBranch, len(diff.Files), diff.TotalAdditions, diff.TotalDeletions, len(diff.Commits)))

	return diff, nil
}

// parseContentResponse splits AI output into title (first line) and description (rest).
func parseContentResponse(response string) (title, description string) {
	response = strings.TrimSpace(response)
	lines := strings.SplitN(response, "\n", 2)

	title = strings.TrimSpace(lines[0])
	title = strings.TrimPrefix(title, "# ")
	title = strings.TrimPrefix(title, "## ")
	if len(title) > 200 {
		title = strings.TrimSpace(title[:200])
	}

	if len(lines) > 1 {
		description = strings.TrimSpace(lines[1])
	}

	return title, description
}

// extractRepoName extracts the project/repo name from a GitLab MR web URL.
// e.g. "https://gitlab.com/group/subgroup/project/-/merge_requests/1" → "project"
func extractRepoName(mrWebURL string) string {
	idx := strings.Index(mrWebURL, "/-/")
	if idx < 0 {
		return mrWebURL
	}
	path := mrWebURL[:idx]
	if slashIdx := strings.LastIndex(path, "/"); slashIdx >= 0 {
		return path[slashIdx+1:]
	}
	return path
}
