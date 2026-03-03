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
