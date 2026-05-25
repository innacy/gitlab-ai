package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/ai"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

// ─── Ticket Open Empty ──────────────────────────────────────────────────────

func (r *replState) handleTicketOpenEmpty(args []string) {
	if !r.ensureSession() {
		return
	}

	project := r.getConfiguredProject()
	if project == "" {
		output.PrintError("No project configured. Set 'gitlab.default_project' or 'projects' in config.")
		return
	}
	project = r.resolveProject(project)

	issue, err := r.createEmptyTicket(project)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to create ticket: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Ticket #%d created: %s", issue.IID, issue.Title))
	output.PrintSuccess(fmt.Sprintf("Assignee: @%s", r.provider.Username()))
	output.PrintURL(issue.WebURL)
	fmt.Println()
	r.refreshCacheAsync()
}

func (r *replState) createEmptyTicket(project string) (*models.Issue, error) {
	s := newSpinner(fmt.Sprintf(" Creating empty ticket in '%s'...", project))
	s.Start()
	issue, err := r.provider.Issues().CreateIssue(project, "cnips work", "", nil)
	s.Stop()
	if err != nil {
		return nil, err
	}

	userID := r.provider.UserID()
	s = newSpinner(fmt.Sprintf(" Assigning to @%s...", r.provider.Username()))
	s.Start()
	updated, err := r.provider.Issues().UpdateIssue(project, issue.IID, platform.UpdateIssueOptions{
		AssigneeID: &userID,
	})
	s.Stop()
	if err != nil {
		output.PrintWarning(fmt.Sprintf("Ticket created but assignment failed: %v", err))
		return issue, nil
	}
	return updated, nil
}

func (r *replState) getConfiguredProject() string {
	for _, p := range r.cfg.Projects {
		project := strings.TrimSpace(p)
		if project != "" {
			return project
		}
	}
	dp := strings.TrimSpace(r.cfg.GitLab.DefaultProject)
	if dp != "" {
		return dp
	}
	return ""
}

// ─── All-in-One ──────────────────────────────────────────────────────────────

func (r *replState) handleAllInOne(args []string) {
	if !r.ensureSession() {
		return
	}

	ticketIID, ticketURL, ticketProject := r.allInOneGetTicket()
	if ticketIID <= 0 {
		return
	}

	output.PrintSuccess(fmt.Sprintf("Working with ticket #%d", ticketIID))
	output.PrintURL(ticketURL)
	fmt.Println()

	processedFolders := make(map[string]bool)

	for {
		folder := r.allInOneSelectFolder(processedFolders)
		if folder == "" {
			return
		}

		mrURL, ok := r.allInOneProcessFolder(folder, ticketIID, ticketURL)
		if !ok {
			choice := r.promptForChoice("What would you like to do?", []string{
				"Select another folder",
				"Exit",
			})
			if choice == 0 {
				continue
			}
			return
		}

		processedFolders[folder] = true
		_ = mrURL

		choice := r.promptForChoice("What would you like to do next?", []string{
			"Continue with another folder",
			"Final update on ticket description",
			"Exit",
		})

		switch choice {
		case 0:
			continue
		case 1:
			r.allInOneUpdateTicketDesc(ticketProject, ticketIID)
			return
		default:
			return
		}
	}
}

func (r *replState) allInOneGetTicket() (iid int, webURL string, project string) {
	choice := r.promptForChoice("Ticket setup", []string{
		"Create new empty ticket",
		"Select existing ticket",
	})

	project = r.getConfiguredProject()
	if project == "" {
		output.PrintError("No project configured. Set 'gitlab.default_project' or 'projects' in config.")
		return 0, "", ""
	}
	project = r.resolveProject(project)

	switch choice {
	case 0:
		issue, err := r.createEmptyTicket(project)
		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to create ticket: %v", err))
			return 0, "", ""
		}
		output.PrintSuccess(fmt.Sprintf("Ticket #%d created: %s", issue.IID, issue.Title))
		output.PrintSuccess(fmt.Sprintf("Assignee: @%s", r.provider.Username()))
		output.PrintURL(issue.WebURL)
		fmt.Println()
		return issue.IID, issue.WebURL, project

	case 1:
		iid := r.pickTicket(project, "Select ticket to work with")
		if iid <= 0 {
			return 0, "", ""
		}
		baseURL := strings.TrimSuffix(r.cfg.GitLab.BaseURL, "/")
		webURL := fmt.Sprintf("%s/%s/-/issues/%d", baseURL, project, iid)
		return iid, webURL, project

	default:
		return 0, "", ""
	}
}

func (r *replState) allInOneSelectFolder(processed map[string]bool) string {
	parentFolder := strings.TrimSpace(r.cfg.GitLab.ParentFolder)
	if parentFolder == "" {
		output.PrintError("Parent folder not configured. Set 'gitlab.parent_folder' in config.")
		return ""
	}

	if strings.HasPrefix(parentFolder, "~/") {
		home, _ := os.UserHomeDir()
		parentFolder = filepath.Join(home, parentFolder[2:])
	}

	entries, err := os.ReadDir(parentFolder)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to read parent folder '%s': %v", parentFolder, err))
		return ""
	}

	type folderEntry struct {
		name    string
		path    string
		modTime time.Time
	}

	s := newSpinner(" Scanning folders for local changes...")
	s.Start()

	var folders []folderEntry
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fullPath := filepath.Join(parentFolder, entry.Name())
		if processed[fullPath] {
			continue
		}
		if !hasGitChanges(fullPath) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		folders = append(folders, folderEntry{
			name:    entry.Name(),
			path:    fullPath,
			modTime: info.ModTime(),
		})
	}
	s.Stop()

	sort.Slice(folders, func(i, j int) bool {
		return folders[i].modTime.After(folders[j].modTime)
	})

	limit := 5
	if len(folders) < limit {
		limit = len(folders)
	}

	if limit == 0 {
		output.PrintWarning("No folders with local changes found.")
		return ""
	}

	options := make([]string, limit+1)
	for i := 0; i < limit; i++ {
		options[i] = fmt.Sprintf("%s (%s)", folders[i].name, output.TimeAgo(folders[i].modTime))
	}
	options[limit] = "Enter folder name manually..."

	choice := r.promptForChoice("Select folder", options)
	if choice < 0 {
		return ""
	}

	if choice < limit {
		return folders[choice].path
	}

	name := r.promptForText("folder-name")
	if name == "" {
		return ""
	}
	fullPath := filepath.Join(parentFolder, name)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		output.PrintError(fmt.Sprintf("Folder '%s' does not exist.", fullPath))
		return ""
	}
	return fullPath
}

func (r *replState) allInOneProcessFolder(folder string, ticketIID int, ticketURL string) (mrURL string, ok bool) {
	folderName := filepath.Base(folder)
	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Processing: %s", folderName))

	gitDir := filepath.Join(folder, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		output.PrintError(fmt.Sprintf("'%s' is not a valid git repository.", folderName))
		return "", false
	}

	s := newSpinner(" Checking local changes...")
	s.Start()
	statusOut, err := runGitCmd(folder, "status", "--porcelain")
	s.Stop()
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to check git status: %v", err))
		return "", false
	}
	if strings.TrimSpace(statusOut) == "" {
		output.PrintWarning(fmt.Sprintf("No local changes in '%s'.", folderName))
		return "", false
	}

	changedLines := strings.Split(strings.TrimSpace(statusOut), "\n")
	output.PrintSuccess(fmt.Sprintf("Found %d changed file(s)", len(changedLines)))

	currentBranch := getCurrentBranch(folder)

	branchName := fmt.Sprintf("fix/%d", ticketIID)
	s = newSpinner(fmt.Sprintf(" Creating branch '%s'...", branchName))
	s.Start()
	_, err = runGitCmd(folder, "checkout", "-b", branchName)
	s.Stop()
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to create branch '%s': %v", branchName, err))
		return "", false
	}
	output.PrintSuccess(fmt.Sprintf("Created branch: %s", branchName))

	s = newSpinner(" Staging changes...")
	s.Start()
	_, err = runGitCmd(folder, "add", ".")
	s.Stop()
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to stage changes: %v", err))
		return "", false
	}

	s = newSpinner(" Generating commit message...")
	s.Start()
	commitMsg := r.generateShortCommitMsg(folder, ticketURL)
	s.Stop()

	s = newSpinner(" Committing changes...")
	s.Start()
	_, err = runGitCmd(folder, "commit", "-m", commitMsg)
	s.Stop()
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to commit: %v", err))
		return "", false
	}
	output.PrintSuccess(fmt.Sprintf("Committed: %s", commitMsg))

	s = newSpinner(fmt.Sprintf(" Pushing '%s'...", branchName))
	s.Start()
	pushOut, err := runGitCmd(folder, "push", "-u", "origin", branchName)
	s.Stop()
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to push: %v\n%s", err, pushOut))
		return "", false
	}
	output.PrintSuccess("Pushed successfully")

	s = newSpinner(" Resolving GitLab project...")
	s.Start()
	projectPath := r.resolveProject(folderName)
	if projectPath == folderName {
		remotePath := getGitLabProjectFromRemote(folder)
		if remotePath != "" {
			projectPath = remotePath
		}
	}
	s.Stop()
	if projectPath == "" {
		output.PrintWarning("Could not determine GitLab project. Push succeeded but MR was not created.")
		fmt.Println()
		return "", true
	}

	targetBranch := currentBranch
	if targetBranch == "" || targetBranch == branchName {
		targetBranch = detectTargetBranch(folder)
	}

	s = newSpinner(" Fetching branch info...")
	s.Start()
	mrTitle := branchToMRTitle(branchName)
	if bInfo, bErr := r.provider.Repos().GetBranch(projectPath, branchName); bErr == nil && bInfo.CommitTitle != "" {
		mrTitle = bInfo.CommitTitle
	}
	s.Stop()

	fmt.Println()
	s = newSpinner(" Generating MR description with AI...")
	s.Start()
	mrDesc, _, descErr := r.generateMRDescription(projectPath, branchName, targetBranch)
	s.Stop()
	if descErr != nil {
		output.PrintWarning(fmt.Sprintf("Could not generate AI description: %v", descErr))
		mrDesc = fmt.Sprintf("Merge %s into %s", branchName, targetBranch)
	}
	mrDesc = fmt.Sprintf("Relates to %s\n\n%s", ticketURL, mrDesc)

	s = newSpinner(fmt.Sprintf(" Creating MR: %s → %s in '%s'...", branchName, targetBranch, projectPath))
	s.Start()
	mr, err := r.provider.MRs().CreateMergeRequest(projectPath, branchName, targetBranch, mrTitle, mrDesc)
	s.Stop()

	if err != nil {
		output.PrintWarning(fmt.Sprintf("Push succeeded but MR creation failed: %v", err))
		fmt.Println()
		return "", true
	}

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("MR !%d created: %s", mr.IID, mr.Title))
	output.PrintSuccess(fmt.Sprintf("Branch: %s → %s", branchName, targetBranch))
	output.PrintURL(mr.WebURL)
	fmt.Println()
	r.stats.mrsCreated++

	return mr.WebURL, true
}

func (r *replState) generateShortCommitMsg(folder, ticketURL string) string {
	diffStat, _ := runGitCmd(folder, "diff", "--cached", "--stat")
	diffOut, _ := runGitCmd(folder, "diff", "--cached")

	if err := r.ensureAI(); err == nil {
		if len(diffOut) > 10000 {
			diffOut = diffOut[:10000] + "\n... [truncated]"
		}

		prompt := fmt.Sprintf(`Analyze the git diff below and write a commit subject line (10 words max).

Rules — strictly follow ALL of these:
- Use imperative mood (as if giving a command): "Add", "Fix", "Update", "Remove", "Correct", "Resolve"
- The line must complete: "If applied, this commit will ___"
- Capitalize the first letter
- NO period at the end
- NO past tense ("Added", "Fixed") and NO present participle ("Adding", "Fixing")
- Output ONLY the subject text — no quotes, no prefix, no formatting

Good examples:
- "Resolve login error for users with special characters in email"
- "Correct calculation error in billing module"
- "Address memory leak in image processing function"
- "Add retry logic for failed API calls"
- "Update validation rules for user registration"

Bad examples (DO NOT produce these):
- "Added feature X" (past tense)
- "Fixing bug in Y" (participle)
- "This commit fixes..." (sentence)

Diff stats:
%s

Diff:
%s`, diffStat, diffOut)

		ctx := context.Background()
		response, err := r.aiClient.Chat(ctx,
			"You write imperative-mood git commit subjects. Output only the subject line, nothing else.",
			prompt)
		if err == nil {
			summary := strings.TrimSpace(response)
			summary = strings.Trim(summary, "`\"'")
			summary = strings.TrimSuffix(summary, ".")
			for _, prefix := range []string{"fix:", "feat:", "chore:", "refactor:", "Fix:", "Feat:"} {
				summary = strings.TrimPrefix(summary, prefix)
			}
			summary = strings.TrimSpace(summary)
			if len(summary) > 0 {
				summary = strings.ToUpper(summary[:1]) + summary[1:]
			}
			if len(summary) > 0 && len(summary) < 120 {
				return fmt.Sprintf("fix(%s): %s", ticketURL, summary)
			}
		}
	}

	fileCount := strings.Count(diffStat, "|")
	if fileCount <= 0 {
		fileCount = len(strings.Split(strings.TrimSpace(diffStat), "\n"))
	}
	return fmt.Sprintf("fix(%s): Update %d files in %s", ticketURL, fileCount, filepath.Base(folder))
}

func (r *replState) allInOneUpdateTicketDesc(project string, ticketIID int) {
	s := newSpinner(fmt.Sprintf(" Fetching MRs linked to #%d...", ticketIID))
	s.Start()
	linkedMRs, err := r.provider.Issues().ListRelatedMergeRequests(project, ticketIID)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch linked MRs: %v", err))
		return
	}
	if len(linkedMRs) == 0 {
		output.PrintWarning(fmt.Sprintf("No merge requests linked to ticket #%d.", ticketIID))
		output.PrintWarning("Tip: MR descriptions should contain 'Relates to <ticket-url>' to auto-link.")
		return
	}

	output.PrintSuccess(fmt.Sprintf("Found %d linked MR(s):", len(linkedMRs)))
	for _, mr := range linkedMRs {
		fmt.Printf("    !%d  %s → %s  [%s]  %s\n", mr.IID, mr.SourceBranch, mr.TargetBranch, mr.State, mr.Title)
	}
	fmt.Println()

	s = newSpinner(" Fetching diffs from linked MRs...")
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
	if len(diffErrors) > 0 {
		output.PrintWarning(fmt.Sprintf("Could not fetch diffs from %d MR(s):", len(diffErrors)))
		for _, e := range diffErrors {
			output.PrintWarning(fmt.Sprintf("  %s", e))
		}
	}

	totalFiles, totalAdds, totalDels := 0, 0, 0
	for _, e := range entries {
		totalFiles += len(e.Diff.Files)
		totalAdds += e.Diff.TotalAdditions
		totalDels += e.Diff.TotalDeletions
	}
	output.PrintSuccess(fmt.Sprintf("Combined diff: %d files, +%d -%d lines across %d MR(s)",
		totalFiles, totalAdds, totalDels, len(entries)))

	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s = newSpinner(" Generating ticket description from linked MRs...")
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

	s = newSpinner(fmt.Sprintf(" Updating ticket #%d...", ticketIID))
	s.Start()
	updated, err := r.provider.Issues().UpdateIssue(project, ticketIID, platform.UpdateIssueOptions{
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

// ─── Git Helpers ─────────────────────────────────────────────────────────────

// hasGitChanges returns true if the folder is a git repo with uncommitted changes.
func hasGitChanges(folder string) bool {
	gitDir := filepath.Join(folder, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return false
	}
	out, err := runGitCmd(folder, "status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// getCurrentBranch returns the current checked-out branch name.
func getCurrentBranch(folder string) string {
	out, err := runGitCmd(folder, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// detectTargetBranch is a fallback: checks for well-known branches, defaults to "development".
func detectTargetBranch(folder string) string {
	// The repo is now on fix/<n>, so check which well-known branches exist locally.
	for _, candidate := range []string{"development", "master", "main"} {
		out, err := runGitCmd(folder, "rev-parse", "--verify", candidate)
		if err == nil && strings.TrimSpace(out) != "" {
			return candidate
		}
	}
	return "development"
}

func runGitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func getGitLabProjectFromRemote(folder string) string {
	remoteURL, err := runGitCmd(folder, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return parseGitLabProjectPath(strings.TrimSpace(remoteURL))
}

func parseGitLabProjectPath(remoteURL string) string {
	// SSH: git@gitlab.example.com:group/project.git
	if strings.HasPrefix(remoteURL, "git@") || strings.Contains(remoteURL, "@") {
		if idx := strings.Index(remoteURL, ":"); idx > 0 && !strings.Contains(remoteURL, "://") {
			path := remoteURL[idx+1:]
			path = strings.TrimSuffix(path, ".git")
			return path
		}
	}

	// HTTPS: https://gitlab.example.com/group/project.git
	if strings.Contains(remoteURL, "://") {
		parts := strings.SplitN(remoteURL, "://", 2)
		if len(parts) == 2 {
			rest := parts[1]
			if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
				path := rest[slashIdx+1:]
				path = strings.TrimSuffix(path, ".git")
				return path
			}
		}
	}

	return ""
}
