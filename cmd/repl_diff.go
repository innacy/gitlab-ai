package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/output"
)

// ─── Diff ────────────────────────────────────────────────────────────────────

func (r *replState) handleDiff(args []string) {
	if !r.ensureSession() {
		return
	}

	// Step 1: Select project
	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	}
	if project == "" {
		project = r.promptForProject("Select project for diff")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	// Step 2: Choose between branches or tags
	refType := r.promptForChoice("Compare by", []string{"Tags", "Branches"})
	if refType < 0 {
		return
	}

	var from, to string
	if refType == 0 {
		// Tags
		from, to = r.promptForTwoTags(project)
	} else {
		// Branches
		from, to = r.promptForTwoBranches(project)
	}

	if from == "" || to == "" {
		return
	}

	// Step 3: Fetch the diff
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching diff %s..%s from '%s'...", from, to, project)
	s.Start()

	diffResult, err := r.provider.Repos().GetRefDiff(project, from, to)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to get diff: %v", err))
		return
	}

	if len(diffResult.Files) == 0 {
		output.PrintWarning(fmt.Sprintf("No differences found between %s and %s.", from, to))
		return
	}

	// Step 4: Display summary
	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Diff: %s..%s", from, to))
	output.PrintSuccess(fmt.Sprintf("Files changed: %d, +%d -%d lines, %d commits",
		len(diffResult.Files), diffResult.TotalAdditions, diffResult.TotalDeletions, len(diffResult.Commits)))

	// Step 5: If diff is large (>5000 chars), write to file; else show in terminal
	diffContent := buildDiffMarkdown(project, diffResult)

	if len(diffContent) > 5000 {
		filename := filepath.Join(r.cfg.Other.Directory, fmt.Sprintf("diff_%s_%s_%s.md",
			sanitizeProject(project),
			sanitizeRef(from),
			sanitizeRef(to),
		))
		if err := writeFile(filename, diffContent); err != nil {
			output.PrintError(fmt.Sprintf("Failed to save diff: %v", err))
			return
		}
		output.PrintSuccess(fmt.Sprintf("Diff is large (%d lines). Saved to:", len(strings.Split(diffContent, "\n"))))
		output.PrintFilePath(filename)
		r.stats.filesCreated++
	} else {
		fmt.Println()
		fmt.Println(diffContent)
	}
	fmt.Println()
}

func (r *replState) promptForTwoTags(project string) (string, string) {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching tags from '%s'...", project)
	s.Start()

	tags, err := r.provider.Repos().ListTags(project, 20)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch tags: %v", err))
		return "", ""
	}

	if len(tags) < 2 {
		output.PrintError("Project needs at least 2 tags to compare.")
		return "", ""
	}

	options := make([]string, len(tags))
	for i, t := range tags {
		options[i] = fmt.Sprintf("%s — %s (%s)", t.Name, t.CommitTitle, output.TimeAgo(t.CommitDate))
	}

	// Select FROM tag
	fromChoice := r.promptForChoice("Select FROM tag (older)", options)
	if fromChoice < 0 {
		return "", ""
	}
	from := tags[fromChoice].Name

	// Select TO tag (exclude the FROM choice)
	toOptions := make([]string, 0, len(tags)-1)
	toTags := make([]string, 0, len(tags)-1)
	for i, t := range tags {
		if i == fromChoice {
			continue
		}
		toOptions = append(toOptions, fmt.Sprintf("%s — %s (%s)", t.Name, t.CommitTitle, output.TimeAgo(t.CommitDate)))
		toTags = append(toTags, t.Name)
	}

	toChoice := r.promptForChoice("Select TO tag (newer)", toOptions)
	if toChoice < 0 {
		return "", ""
	}
	to := toTags[toChoice]

	return from, to
}

func (r *replState) promptForTwoBranches(project string) (string, string) {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching branches from '%s'...", project)
	s.Start()

	branches, err := r.provider.Repos().ListBranches(project, 20)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch branches: %v", err))
		return "", ""
	}

	if len(branches) < 2 {
		output.PrintError("Project needs at least 2 branches to compare.")
		return "", ""
	}

	options := make([]string, len(branches))
	for i, b := range branches {
		options[i] = fmt.Sprintf("%s — %s (%s)", b.Name, b.CommitTitle, output.TimeAgo(b.CommitDate))
	}

	// Select FROM branch
	fromChoice := r.promptForChoice("Select FROM branch", options)
	if fromChoice < 0 {
		return "", ""
	}
	from := branches[fromChoice].Name

	// Select TO branch (exclude the FROM choice)
	toOptions := make([]string, 0, len(branches)-1)
	toBranches := make([]string, 0, len(branches)-1)
	for i, b := range branches {
		if i == fromChoice {
			continue
		}
		toOptions = append(toOptions, fmt.Sprintf("%s — %s (%s)", b.Name, b.CommitTitle, output.TimeAgo(b.CommitDate)))
		toBranches = append(toBranches, b.Name)
	}

	toChoice := r.promptForChoice("Select TO branch", toOptions)
	if toChoice < 0 {
		return "", ""
	}
	to := toBranches[toChoice]

	return from, to
}

func sanitizeRef(ref string) string {
	ref = strings.ReplaceAll(ref, "/", "_")
	ref = strings.ReplaceAll(ref, ".", "-")
	return ref
}

func buildDiffMarkdown(project string, diff *models.DiffResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Diff: %s..%s\n\n", diff.From, diff.To))
	sb.WriteString(fmt.Sprintf("**Project:** %s  \n", project))
	sb.WriteString(fmt.Sprintf("**Files Changed:** %d  \n", len(diff.Files)))
	sb.WriteString(fmt.Sprintf("**Additions:** +%d  \n", diff.TotalAdditions))
	sb.WriteString(fmt.Sprintf("**Deletions:** -%d  \n", diff.TotalDeletions))
	sb.WriteString(fmt.Sprintf("**Commits:** %d  \n\n", len(diff.Commits)))

	if len(diff.Commits) > 0 {
		sb.WriteString("## Commits\n\n")
		for _, msg := range diff.Commits {
			sb.WriteString(fmt.Sprintf("- %s\n", msg))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Changed Files\n\n")
	for _, f := range diff.Files {
		status := "modified"
		if f.NewFile {
			status = "added"
		} else if f.Deleted {
			status = "deleted"
		} else if f.Renamed {
			status = "renamed"
		}
		sb.WriteString(fmt.Sprintf("- `%s` (%s) +%d -%d\n", f.NewPath, status, f.Additions, f.Deletions))
	}
	sb.WriteString("\n")

	sb.WriteString("## Diff\n\n")
	sb.WriteString("```diff\n")
	sb.WriteString(diff.DiffContent)
	sb.WriteString("\n```\n\n")

	sb.WriteString("---\n\n*Generated by gitlab-ai CLI*\n")
	return sb.String()
}
