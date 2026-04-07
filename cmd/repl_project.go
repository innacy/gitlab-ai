package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/output"
)

// ─── List ────────────────────────────────────────────────────────────────────

func (r *replState) handleList() {
	if !r.ensureSession() {
		return
	}

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

// ─── Branch Cleanup ──────────────────────────────────────────────────────────

func (r *replState) handleBranchCleanup(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	}
	if project == "" {
		project = r.promptForProject("Select project for branch cleanup")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Scanning branches in '%s'...", project)
	s.Start()

	merged, err := r.glClient.ListMergedBranches(project)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to list branches: %v", err))
		return
	}

	if len(merged) == 0 {
		output.PrintSuccess("No stale or merged branches found. All clean!")
		fmt.Println()
		return
	}

	output.PrintBranchesTable(merged, fmt.Sprintf("Stale/Merged Branches — %s", project))
	fmt.Println()

	if !r.promptForYesNo(fmt.Sprintf("Delete all %d branches listed above?", len(merged))) {
		output.PrintWarning("Branch cleanup cancelled.")
		fmt.Println()
		return
	}

	deleted := 0
	for _, b := range merged {
		err := r.glClient.DeleteBranch(project, b.Name)
		if err != nil {
			output.PrintError(fmt.Sprintf("  Failed to delete '%s': %v", b.Name, err))
		} else {
			output.PrintSuccess(fmt.Sprintf("  Deleted '%s'", b.Name))
			deleted++
		}
	}

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Cleanup complete: %d/%d branches deleted", deleted, len(merged)))
	fmt.Println()
}

// ─── Tickets ─────────────────────────────────────────────────────────────────

func (r *replState) handleTickets(args []string) {
	if !r.ensureSession() {
		return
	}
	_ = args

	projects := r.allTeamProjects()
	if len(projects) == 0 {
		output.PrintWarning("No projects found for the selected team.")
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching open tickets from %d projects...", len(projects))
	s.Start()

	type projectIssues struct {
		project string
		issues  []models.Issue
		err     error
	}
	ch := make(chan projectIssues, len(projects))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, p := range projects {
		wg.Add(1)
		go func(project string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := r.glClient.ListProjectIssues(project, models.IssueFilter{State: "opened"})
			if err != nil {
				ch <- projectIssues{project: project, err: err}
				return
			}
			ch <- projectIssues{project: project, issues: result.Issues}
		}(p.Path)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	projectCounts := make(map[string]int, len(projects))
	assigneeCounts := map[string]int{"Unassigned": 0}
	rows := make([]ticketReportRow, 0, 500)
	errorCount := 0

	for res := range ch {
		if res.err != nil {
			errorCount++
			continue
		}

		projectCounts[res.project] = len(res.issues)
		for _, issue := range res.issues {
			rows = append(rows, ticketReportRow{Project: res.project, Issue: issue})

			assignee := strings.TrimSpace(issue.Assignee)
			if assignee == "" {
				assignee = "Unassigned"
			}
			assigneeCounts[assignee]++
		}
	}

	s.Stop()

	now := time.Now()
	filename := filepath.Join(r.cfg.Issues.Output.Directory, fmt.Sprintf("tickets_report_%s.md", now.Format("2006-01-02_15-04")))
	content := buildTicketsAggregateMarkdown(projectCounts, assigneeCounts, rows, now)

	if err := writeFile(filename, content); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save tickets report: %v", err))
		return
	}

	if errorCount > 0 {
		output.PrintWarning(fmt.Sprintf("Completed with %d project fetch errors. See markdown report for available data.", errorCount))
	}
	output.PrintSuccess(fmt.Sprintf("Tickets report saved to: %s", filename))

	r.stats.issuesViewed += len(rows)
	r.stats.filesCreated++
}

func (r *replState) handleTicketOpen(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	} else {
		project = r.selectProjectWithConfig("Select project for new ticket")
	}
	if project == "" {
		output.PrintError("No project selected.")
		return
	}
	project = r.resolveProject(project)

	fmt.Println()
	output.PrintSuccess("Summarize ticket context in one clear sentence/paragraph.")
	context := r.promptForText("ticket-context")
	if context == "" {
		output.PrintError("Ticket context cannot be empty.")
		return
	}

	title, description := buildTicketContent(context)

	labels, err := r.glClient.ListProjectLabels(project)
	if err != nil {
		output.PrintWarning(fmt.Sprintf("Could not load labels for '%s': %v", project, err))
		labels = nil
	}

	var createLabels []string
	if len(labels) > 0 {
		options := make([]string, 0, len(labels)+1)
		options = append(options, labels...)
		options = append(options, "— no label —")

		choice := r.promptForChoice("Select label (optional)", options)
		if choice >= 0 && choice < len(labels) {
			createLabels = append(createLabels, labels[choice])
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Creating ticket in '%s'...", project)
	s.Start()
	issue, err := r.glClient.CreateIssue(project, title, description, createLabels)
	s.Stop()
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to create ticket: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Ticket #%d created: %s", issue.IID, issue.Title))
	output.PrintSuccess("Status: todo")
	output.PrintSuccess("Assignee: none")
	if len(createLabels) == 0 {
		output.PrintSuccess("Label: none")
	} else {
		output.PrintSuccess(fmt.Sprintf("Label: %s", strings.Join(createLabels, ", ")))
	}
	output.PrintSuccess(fmt.Sprintf("View at: %s", issue.WebURL))
	fmt.Println()

	r.refreshCacheAsync()
}

func (r *replState) handleTicketsBlack(args []string) {
	if !r.ensureSession() {
		return
	}

	_ = args
	const minDescriptionChars = 40

	projects := r.allTeamProjects()
	if len(projects) == 0 {
		output.PrintWarning("No projects found for the selected team.")
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Scanning malformed tickets across %d projects...", len(projects))
	s.Start()

	type projectIssues struct {
		project string
		issues  []models.Issue
		err     error
	}
	ch := make(chan projectIssues, len(projects))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, p := range projects {
		wg.Add(1)
		go func(project string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := r.glClient.ListProjectIssues(project, models.IssueFilter{State: "opened"})
			if err != nil {
				ch <- projectIssues{project: project, err: err}
				return
			}
			ch <- projectIssues{project: project, issues: result.Issues}
		}(p.Path)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	malformed := make([]ticketReportRow, 0, 200)
	errorCount := 0
	for res := range ch {
		if res.err != nil {
			errorCount++
			continue
		}
		for _, issue := range res.issues {
			descLen := len(strings.TrimSpace(issue.Description))
			if descLen == 0 || descLen < minDescriptionChars {
				malformed = append(malformed, ticketReportRow{
					Project: res.project,
					Issue:   issue,
				})
			}
		}
	}
	s.Stop()

	now := time.Now()
	filename := filepath.Join(r.cfg.Issues.Output.Directory, fmt.Sprintf("tickets_black_%s.md", now.Format("2006-01-02_15-04")))
	content := buildTicketsBlackMarkdown(malformed, minDescriptionChars, now)
	if err := writeFile(filename, content); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save malformed tickets report: %v", err))
		return
	}

	if errorCount > 0 {
		output.PrintWarning(fmt.Sprintf("Completed with %d project fetch errors. See markdown report for available data.", errorCount))
	}
	output.PrintSuccess(fmt.Sprintf("Malformed tickets report saved to: %s", filename))
	fmt.Println()

	r.stats.filesCreated++
}

func (r *replState) allTeamProjects() []models.ProjectInfo {
	projects := r.waitForCache()
	if len(projects) == 0 {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Fetching projects..."
		s.Start()

		fetched, err := r.glClient.ListProjects()
		s.Stop()
		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to list projects: %v", err))
			return nil
		}

		team := strings.ToLower(strings.TrimSpace(r.activeTeam))
		if team != "" {
			filtered := make([]models.ProjectInfo, 0, len(fetched))
			for _, p := range fetched {
				path := strings.ToLower(p.Path)
				if strings.HasPrefix(path, team+"/") || strings.Contains(path, "/"+team+"/") {
					filtered = append(filtered, p)
				}
			}
			projects = filtered
		} else {
			projects = fetched
		}
	}

	return r.filterIgnoredProjects(projects)
}

func (r *replState) filterIgnoredProjects(projects []models.ProjectInfo) []models.ProjectInfo {
	if len(r.cfg.IgnoredProjects) == 0 {
		return projects
	}

	ignored := make(map[string]bool, len(r.cfg.IgnoredProjects))
	for _, p := range r.cfg.IgnoredProjects {
		ignored[strings.ToLower(strings.TrimSpace(p))] = true
	}

	filtered := make([]models.ProjectInfo, 0, len(projects))
	for _, p := range projects {
		if !ignored[strings.ToLower(p.Path)] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// ─── Release ─────────────────────────────────────────────────────────────────

func (r *replState) handleRelease() {
	if !r.ensureSession() {
		return
	}

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

	if len(projects) == 0 {
		output.PrintWarning("No projects found.")
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Checking release status for %d projects...", len(projects))
	s.Start()

	type indexedResult struct {
		idx  int
		info models.ProjectReleaseInfo
	}

	results := make([]models.ProjectReleaseInfo, len(projects))
	ch := make(chan indexedResult, len(projects))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, p := range projects {
		wg.Add(1)
		go func(idx int, proj models.ProjectInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			info := r.glClient.CheckProjectRelease(proj.ID, proj.Path)
			ch <- indexedResult{idx: idx, info: info}
		}(i, p)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for res := range ch {
		results[res.idx] = res.info
	}

	s.Stop()

	report := &models.ReleaseReport{GeneratedAt: time.Now()}
	for _, info := range results {
		switch info.Status {
		case models.ReleasePending:
			report.Pending = append(report.Pending, info)
		case models.ReleaseUpToDate:
			report.Released = append(report.Released, info)
		case models.ReleaseInvalid:
			report.Invalid = append(report.Invalid, info)
		}
	}

	// Sort pending items by last dev commit date descending (most recent first).
	sort.Slice(report.Pending, func(i, j int) bool {
		return report.Pending[i].LastDevCommitDate.After(report.Pending[j].LastDevCommitDate)
	})

	output.PrintReleaseReport(report)

	filename := filepath.Join(r.cfg.Other.Directory, fmt.Sprintf("Release_items-%s.md", time.Now().Format("2006-01-02")))
	content := buildReleaseMarkdown(report)

	if err := writeFile(filename, content); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save release report: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Release report saved to: %s", filename))
	fmt.Println()

	r.stats.filesCreated++
}
