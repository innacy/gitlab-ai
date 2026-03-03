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
	successIcon = color.GreenString("✓")
	warningIcon = color.YellowString("⚠")
	errorIcon   = color.RedString("✗")
)

// PrintSuccess prints a success message.
func PrintSuccess(msg string) {
	fmt.Printf("%s %s\n", successIcon, msg)
}

// PrintWarning prints a warning message.
func PrintWarning(msg string) {
	fmt.Printf("%s %s\n", warningIcon, msg)
}

// PrintError prints an error message.
func PrintError(msg string) {
	fmt.Printf("%s %s\n", errorIcon, msg)
}

// PrintMRInfo displays merge request information.
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
		updated := timeAgo(issue.UpdatedAt)

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

// Helper functions

func timeAgo(t time.Time) string {
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
