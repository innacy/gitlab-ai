package output

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"

	"gitlab-ai/internal/models"
)

var (
	warningIcon = color.YellowString("⚠")
)

func PrintSuccess(msg string) {
	t := GetTheme()
	t.Success.Print("  ✓ ")
	fmt.Println(msg)
}

func PrintWarning(msg string) {
	t := GetTheme()
	t.Warning.Print("  ⚠ ")
	fmt.Println(msg)
}

func PrintError(msg string) {
	t := GetTheme()
	t.Error.Print("  ✗ ")
	fmt.Println(msg)
}

func PrintURL(url string) {
	t := GetTheme()
	t.Accent.Printf("    → %s\n", url)
}

func PrintFilePath(path string) {
	t := GetTheme()
	t.Accent.Printf("    → %s\n", path)
}

func PrintMRInfo(mr *models.MergeRequestInfo) {
	additions, deletions := countChanges(mr)
	PrintSuccess(fmt.Sprintf("MR fetched: \"%s\"", mr.Title))
	PrintSuccess(fmt.Sprintf("Changes: %d files, +%d -%d lines", len(mr.Changes), additions, deletions))
}

// PrintReview displays a review in the terminal.
func PrintReview(review *models.Review) {
	fmt.Println()
	titleColor := color.New(color.FgCyan, color.Bold)
	titleColor.Printf("# MR Review: %s (#%d)\n\n", review.MRTitle, review.MRNumber)

	if review.ProjectName != "" {
		fmt.Printf("**Project:** %s\n", review.ProjectName)
	}
	fmt.Printf("**Author:** %s\n", review.Author)
	fmt.Printf("**Branch:** %s → %s\n", review.SourceBranch, review.TargetBranch)
	fmt.Printf("**Changes:** %d files, +%d -%d lines\n\n", review.FilesChanged, review.Additions, review.Deletions)

	sectionColor := color.New(color.FgYellow, color.Bold)
	for _, section := range review.Sections {
		sectionColor.Printf("## %s\n\n", section.Name)
		fmt.Println(section.Content)
		fmt.Println()
	}
}

// PrintIssuesTable displays issues in a formatted table.
func PrintIssuesTable(result *models.IssueListResult) {
	fmt.Println()
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Printf("Issues for project '%s' (Assigned to you)\n\n", result.ProjectName)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Title", "State", "Labels", "Updated"})
	table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
	table.SetCenterSeparator("┼")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetColMinWidth(1, 30)
	table.SetColWidth(50)

	for _, issue := range result.Issues {
		labels := strings.Join(issue.Labels, ", ")
		updated := TimeAgo(issue.UpdatedAt)

		// Truncate title if too long
		title := issue.Title
		if len(title) > 45 {
			title = title[:42] + "..."
		}

		table.Append([]string{
			fmt.Sprintf("%d", issue.IID),
			title,
			issue.State,
			labels,
			updated,
		})
	}

	table.Render()
	fmt.Printf("\nTotal: %d issues\n", result.TotalCount)
}

// PrintProjectsTable displays a list of projects in a formatted table.
func PrintProjectsTable(projects []models.ProjectInfo) {
	fmt.Println()
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Println("Accessible Projects")
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Path", "Name", "Branch", "Last Activity"})
	table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
	table.SetCenterSeparator("┼")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetColMinWidth(0, 25)
	table.SetColWidth(50)

	for _, p := range projects {
		activity := TimeAgo(p.LastActivity)

		name := p.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}

		table.Append([]string{
			p.Path,
			name,
			p.DefaultBranch,
			activity,
		})
	}

	table.Render()
	fmt.Printf("\nTotal: %d projects\n", len(projects))
}

// PrintMRListTable displays merge requests in a table.
func PrintMRListTable(mrs []models.MRListItem, title string) {
	fmt.Println()
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Println(title)
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"#", "Title", "Author", "Branch", "Updated"})
	table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
	table.SetCenterSeparator("┼")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetColMinWidth(1, 30)
	table.SetColWidth(50)

	for _, mr := range mrs {
		title := mr.Title
		if len(title) > 45 {
			title = title[:42] + "..."
		}
		branch := fmt.Sprintf("%s → %s", mr.SourceBranch, mr.TargetBranch)
		if len(branch) > 35 {
			branch = branch[:32] + "..."
		}
		table.Append([]string{
			fmt.Sprintf("%d", mr.IID),
			title,
			mr.Author,
			branch,
			TimeAgo(mr.UpdatedAt),
		})
	}

	table.Render()
	fmt.Printf("\nTotal: %d merge requests\n", len(mrs))
}

// PrintBranchesTable displays branches in a formatted table.
func PrintBranchesTable(branches []models.BranchInfo, title string) {
	fmt.Println()
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Println(title)
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"#", "Branch", "Last Commit", "Author", "Age", "Status"})
	table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
	table.SetCenterSeparator("┼")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetColMinWidth(1, 25)
	table.SetColWidth(50)

	for i, b := range branches {
		commit := b.CommitTitle
		if len(commit) > 40 {
			commit = commit[:37] + "..."
		}
		status := ""
		if b.Merged {
			status = "merged"
		}
		if b.Protected {
			status = "protected"
		}
		table.Append([]string{
			fmt.Sprintf("%d", i+1),
			b.Name,
			commit,
			b.AuthorName,
			TimeAgo(b.CommitDate),
			status,
		})
	}

	table.Render()
}

// PrintPipelinesTable displays a list of pipelines in a table.
func PrintPipelinesTable(pipelines []models.PipelineInfo, title string) {
	fmt.Println()
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Println(title)
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Status", "Ref", "URL"})
	table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
	table.SetCenterSeparator("┼")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, pl := range pipelines {
		icon := "⬜"
		switch pl.Status {
		case "success":
			icon = "✅"
		case "failed":
			icon = "❌"
		case "running":
			icon = "🔄"
		case "pending":
			icon = "⏳"
		case "canceled":
			icon = "🚫"
		}
		table.Append([]string{
			fmt.Sprintf("%d", pl.ID),
			fmt.Sprintf("%s %s", icon, pl.Status),
			pl.Ref,
			pl.WebURL,
		})
	}

	table.Render()
	fmt.Printf("\nTotal: %d pipelines\n", len(pipelines))
}

// PrintPipelineStatus displays CI/CD pipeline status with jobs.
func PrintPipelineStatus(pipeline *models.PipelineInfo) {
	fmt.Println()
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Printf("Pipeline #%d — %s\n", pipeline.ID, pipeline.Ref)

	statusColor := color.New(color.FgRed, color.Bold)
	switch pipeline.Status {
	case "success":
		statusColor = color.New(color.FgGreen, color.Bold)
	case "running", "pending":
		statusColor = color.New(color.FgYellow, color.Bold)
	case "canceled", "skipped":
		statusColor = color.New(color.FgHiBlack)
	}
	statusColor.Printf("Status: %s\n", strings.ToUpper(pipeline.Status))

	if pipeline.WebURL != "" {
		PrintURL(pipeline.WebURL)
	}
	fmt.Println()

	if len(pipeline.Jobs) > 0 {
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Stage", "Job", "Status"})
		table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
		table.SetCenterSeparator("┼")
		table.SetColumnSeparator("│")
		table.SetRowSeparator("─")
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)

		for _, job := range pipeline.Jobs {
			icon := "⬜"
			switch job.Status {
			case "success":
				icon = "✅"
			case "failed":
				icon = "❌"
			case "running":
				icon = "🔄"
			case "pending":
				icon = "⏳"
			case "canceled":
				icon = "🚫"
			case "skipped":
				icon = "⏭️"
			}
			table.Append([]string{
				job.Stage,
				job.Name,
				fmt.Sprintf("%s %s", icon, job.Status),
			})
		}
		table.Render()
	}
	fmt.Println()
}

// PrintReleaseReport displays the release status of all projects.
func PrintReleaseReport(report *models.ReleaseReport) {
	fmt.Println()
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Println("Release Status Report")
	headerColor.Printf("Generated: %s\n", report.GeneratedAt.Format("2006-01-02 15:04"))
	fmt.Println()

	maxLen := 0
	for _, p := range report.Pending {
		if len(p.Name) > maxLen {
			maxLen = len(p.Name)
		}
	}
	for _, p := range report.Released {
		if len(p.Name) > maxLen {
			maxLen = len(p.Name)
		}
	}
	for _, p := range report.Invalid {
		if len(p.Name) > maxLen {
			maxLen = len(p.Name)
		}
	}
	if maxLen < 20 {
		maxLen = 20
	}

	pendingStyle := color.New(color.FgYellow, color.Bold)
	releasedStyle := color.New(color.FgGreen)
	invalidStyle := color.New(color.FgHiBlack)
	sectionTitle := color.New(color.Bold)
	dim := color.New(color.Faint)

	if len(report.Pending) > 0 {
		sectionTitle.Printf("  Pending Master Merge (%d)\n", len(report.Pending))
		for _, p := range report.Pending {
			padded := fmt.Sprintf("%-*s", maxLen, p.Name)
			pendingStyle.Printf("    %s", padded)
			fmt.Printf(" - ***Pending Master Merge - %s", p.LatestTag)
			extra := fmt.Sprintf("  (%d commits ahead", p.CommitsAhead)
			if !p.LastDevCommitDate.IsZero() {
				extra += fmt.Sprintf(", last: %s", p.LastDevCommitDate.Format("2006-01-02"))
			}
			extra += ")"
			dim.Printf("%s\n", extra)
		}
		fmt.Println()
	}

	if len(report.Released) > 0 {
		sectionTitle.Printf("  Merged To Master (%d)\n", len(report.Released))
		for _, p := range report.Released {
			padded := fmt.Sprintf("%-*s", maxLen, p.Name)
			releasedStyle.Printf("    %s", padded)
			fmt.Printf(" - Merged To Master - %s\n", p.LatestTag)
		}
		fmt.Println()
	}

	if len(report.Invalid) > 0 {
		sectionTitle.Printf("  Invalid Projects (%d)\n", len(report.Invalid))
		for _, p := range report.Invalid {
			padded := fmt.Sprintf("%-*s", maxLen, p.Name)
			invalidStyle.Printf("    %s", padded)
			fmt.Printf(" - %s %s\n", warningIcon, p.InvalidReason)
		}
		fmt.Println()
	}

	total := len(report.Pending) + len(report.Released) + len(report.Invalid)
	fmt.Printf("  Total: %d projects checked\n", total)
	fmt.Println()
}

// Helper functions

// TimeAgo returns a human-readable relative time string.
func TimeAgo(t time.Time) string {
	duration := time.Since(t)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%d min ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case duration < 30*24*time.Hour:
		weeks := int(duration.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	default:
		months := int(duration.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}
}

func countChanges(mr *models.MergeRequestInfo) (additions, deletions int) {
	for _, change := range mr.Changes {
		for _, line := range strings.Split(change.Diff, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				additions++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				deletions++
			}
		}
	}
	return
}
