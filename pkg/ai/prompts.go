package ai

import (
	"fmt"
	"strings"

	"gitlab-ai/internal/models"
)

// BuildReviewPrompt constructs the AI prompt for a merge request review.
func BuildReviewPrompt(mr *models.MergeRequestInfo, sections []models.ReviewTemplateSection) string {
	var sb strings.Builder

	sb.WriteString("You are a senior code reviewer. Review the following merge request thoroughly and provide structured feedback.\n\n")

	// MR context
	sb.WriteString(fmt.Sprintf("## Merge Request Details\n"))
	sb.WriteString(fmt.Sprintf("- **Title:** %s\n", mr.Title))
	sb.WriteString(fmt.Sprintf("- **Author:** %s (@%s)\n", mr.Author, mr.AuthorUser))
	sb.WriteString(fmt.Sprintf("- **Source Branch:** %s → %s\n", mr.SourceBranch, mr.TargetBranch))
	sb.WriteString(fmt.Sprintf("- **Files Changed:** %d\n", len(mr.Changes)))
	sb.WriteString(fmt.Sprintf("- **Labels:** %s\n\n", strings.Join(mr.Labels, ", ")))

	if mr.Description != "" {
		sb.WriteString(fmt.Sprintf("## Description\n%s\n\n", mr.Description))
	}

	// Diff content
	sb.WriteString("## Code Changes (Diff)\n")
	sb.WriteString("```diff\n")

	// Truncate extremely large diffs
	diff := mr.DiffContent
	const maxDiffSize = 50000 // ~50KB
	if len(diff) > maxDiffSize {
		diff = diff[:maxDiffSize] + "\n\n... [diff truncated due to size] ..."
	}
	sb.WriteString(diff)
	sb.WriteString("\n```\n\n")

	// Review sections
	sb.WriteString("## Review Instructions\n")
	sb.WriteString("Provide a structured review with the following sections. Use markdown formatting.\n")
	sb.WriteString("For each section, use a level-2 heading (##).\n\n")

	for _, section := range sections {
		sb.WriteString(fmt.Sprintf("### %s\n", section.Name))
		sb.WriteString(fmt.Sprintf("%s\n\n", section.Prompt))
	}

	sb.WriteString("\n## Output Format\n")
	sb.WriteString("Output the review in clean markdown with:\n")
	sb.WriteString("- Use ## for each section heading\n")
	sb.WriteString("- Use ✅ for positive findings\n")
	sb.WriteString("- Use ⚠️ for warnings\n")
	sb.WriteString("- Use 🔴 for critical issues\n")
	sb.WriteString("- Use code blocks with file paths when referencing specific code\n")
	sb.WriteString("- Be specific with line references where possible\n")

	return sb.String()
}

// BuildMRDescriptionPrompt constructs a prompt to generate a merge request description.
func BuildMRDescriptionPrompt(sourceBranch, targetBranch string, commitMessages []string) string {
	var sb strings.Builder
	sb.WriteString("Generate a concise merge request description based on the following commits.\n\n")
	sb.WriteString(fmt.Sprintf("Source branch: %s\nTarget branch: %s\n\n", sourceBranch, targetBranch))
	sb.WriteString("Commits:\n")
	for _, msg := range commitMessages {
		sb.WriteString(fmt.Sprintf("- %s\n", msg))
	}
	sb.WriteString("\nWrite a clear, professional MR description that:\n")
	sb.WriteString("1. Summarizes what changes are being made\n")
	sb.WriteString("2. Explains the purpose/motivation\n")
	sb.WriteString("3. Lists key changes as bullet points\n")
	sb.WriteString("\nOutput only the description text, no additional formatting or headings.\n")
	return sb.String()
}

// BuildTicketContentPrompt constructs a prompt for generating concise ticket content from a branch diff.
func BuildTicketContentPrompt(diff *models.DiffResult, template string) string {
	var sb strings.Builder

	sb.WriteString("Generate a concise GitLab ticket (issue) based on the code changes below.\n")
	sb.WriteString("Be precise with no extra detail. Keep it short and actionable.\n\n")

	sb.WriteString("## Branch Comparison\n")
	sb.WriteString(fmt.Sprintf("- **Base branch:** %s\n", diff.From))
	sb.WriteString(fmt.Sprintf("- **Feature branch:** %s\n", diff.To))
	sb.WriteString(fmt.Sprintf("- **Files changed:** %d (+%d -%d lines)\n\n", len(diff.Files), diff.TotalAdditions, diff.TotalDeletions))

	if len(diff.Commits) > 0 {
		sb.WriteString("## Commits\n")
		for _, msg := range diff.Commits {
			sb.WriteString(fmt.Sprintf("- %s\n", msg))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Changed Files\n")
	for _, f := range diff.Files {
		status := "modified"
		if f.NewFile {
			status = "added"
		} else if f.Deleted {
			status = "deleted"
		} else if f.Renamed {
			status = "renamed"
		}
		sb.WriteString(fmt.Sprintf("- `%s` (%s, +%d -%d)\n", f.NewPath, status, f.Additions, f.Deletions))
	}
	sb.WriteString("\n")

	diffContent := diff.DiffContent
	const maxDiffSize = 50000
	if len(diffContent) > maxDiffSize {
		diffContent = diffContent[:maxDiffSize] + "\n\n... [diff truncated due to size] ..."
	}
	sb.WriteString("## Code Diff\n```diff\n")
	sb.WriteString(diffContent)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Output Instructions\n")
	sb.WriteString("Follow this template structure EXACTLY. Fill in each section based on the changes above.\n")
	sb.WriteString("The FIRST line of your output must be the ticket title (plain text, no markdown heading).\n")
	sb.WriteString("Everything after the first line is the ticket description in markdown.\n\n")
	sb.WriteString("Template:\n")
	sb.WriteString(template)
	sb.WriteString("\n")

	return sb.String()
}

// BuildEpicContentPrompt constructs a prompt for generating detailed epic content from a branch diff.
func BuildEpicContentPrompt(diff *models.DiffResult, template string) string {
	var sb strings.Builder

	sb.WriteString("Generate a detailed GitLab epic based on the code changes below.\n")
	sb.WriteString("Be thorough and comprehensive. Include technical details, background context, and impact analysis.\n\n")

	sb.WriteString("## Branch Comparison\n")
	sb.WriteString(fmt.Sprintf("- **Base branch:** %s\n", diff.From))
	sb.WriteString(fmt.Sprintf("- **Feature branch:** %s\n", diff.To))
	sb.WriteString(fmt.Sprintf("- **Files changed:** %d (+%d -%d lines)\n\n", len(diff.Files), diff.TotalAdditions, diff.TotalDeletions))

	if len(diff.Commits) > 0 {
		sb.WriteString("## Commits\n")
		for _, msg := range diff.Commits {
			sb.WriteString(fmt.Sprintf("- %s\n", msg))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Changed Files\n")
	for _, f := range diff.Files {
		status := "modified"
		if f.NewFile {
			status = "added"
		} else if f.Deleted {
			status = "deleted"
		} else if f.Renamed {
			status = "renamed"
		}
		sb.WriteString(fmt.Sprintf("- `%s` (%s, +%d -%d)\n", f.NewPath, status, f.Additions, f.Deletions))
	}
	sb.WriteString("\n")

	diffContent := diff.DiffContent
	const maxDiffSize = 50000
	if len(diffContent) > maxDiffSize {
		diffContent = diffContent[:maxDiffSize] + "\n\n... [diff truncated due to size] ..."
	}
	sb.WriteString("## Code Diff\n```diff\n")
	sb.WriteString(diffContent)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Output Instructions\n")
	sb.WriteString("Follow this template structure EXACTLY. Fill in each section based on the changes above.\n")
	sb.WriteString("Provide detailed, comprehensive content for every section.\n")
	sb.WriteString("The FIRST line of your output must be the epic title (plain text, no markdown heading).\n")
	sb.WriteString("Everything after the first line is the epic description in markdown.\n\n")
	sb.WriteString("Template:\n")
	sb.WriteString(template)
	sb.WriteString("\n")

	return sb.String()
}

// BuildSystemPrompt returns the system prompt for the AI.
func BuildSystemPrompt() string {
	return `You are an expert senior code reviewer with deep knowledge of software engineering best practices, security, performance optimization, and clean code principles.

Your reviews are:
- Thorough but concise
- Actionable with specific suggestions
- Respectful and constructive
- Focused on the most impactful issues first
- Structured according to the provided template

When reviewing code:
1. Focus on logic errors, security vulnerabilities, and performance issues first
2. Then address code quality, maintainability, and style
3. Always provide specific file and line references
4. Suggest concrete improvements with code examples when helpful
5. Consider the broader architectural impact of changes
6. Evaluate test coverage and quality

Always format your response in clean, well-structured markdown.`
}
