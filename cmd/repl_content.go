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

	diff, err := r.fetchBranchDiff(project, baseBranch, sourceBranch)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to get diff: %v", err))
		return
	}

	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Generating ticket content with AI..."
	s.Start()

	template := r.cfg.TicketContent.Template
	prompt := ai.BuildTicketContentPrompt(diff, template)
	systemPrompt := "You are a technical writer that creates precise, actionable GitLab tickets. Be concise — no filler, no extra detail."

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

	output.PrintSuccess(fmt.Sprintf("%s", filename))
	r.stats.filesCreated++
	fmt.Println()

	if r.promptForYesNo("Do you want to create a ticket with this content?") {
		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = fmt.Sprintf(" Creating ticket in '%s'...", project)
		s.Start()
		issue, err := r.glClient.CreateIssue(project, title, description, nil)
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to create ticket: %v", err))
			return
		}

		output.PrintSuccess(fmt.Sprintf("Ticket #%d created: %s", issue.IID, issue.Title))
		output.PrintSuccess(fmt.Sprintf("View at: %s", issue.WebURL))
		fmt.Println()
		r.refreshCacheAsync()
	}
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

	diff, err := r.fetchBranchDiff(project, baseBranch, sourceBranch)
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
	systemPrompt := "You are a technical writer that creates detailed, comprehensive GitLab epics. Be thorough — include background, technical details, and impact analysis."

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

	output.PrintSuccess(fmt.Sprintf("%s", filename))
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
		epic, err := r.glClient.CreateGroupEpic(groupPath, title, description)
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to create epic: %v", err))
			return
		}

		output.PrintSuccess(fmt.Sprintf("Epic #%d created: %s", epic.IID, epic.Title))
		output.PrintSuccess(fmt.Sprintf("View at: %s", epic.WebURL))
		fmt.Println()
	}
}

// ─── Shared Helpers ──────────────────────────────────────────────────────────

func (r *replState) promptForBranchPair(project string) (sourceBranch, baseBranch string) {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching branches from '%s'...", project)
	s.Start()

	branches, err := r.glClient.ListActiveBranches(project, 10)
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

func (r *replState) fetchBranchDiff(project, baseBranch, sourceBranch string) (*models.DiffResult, error) {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching diff %s..%s from '%s'...", baseBranch, sourceBranch, project)
	s.Start()

	diff, err := r.glClient.GetRefDiff(project, baseBranch, sourceBranch)
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
