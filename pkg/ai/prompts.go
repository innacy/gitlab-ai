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
	sb.WriteString("## Merge Request Details\n")
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

// BuildMRDescriptionPromptFull constructs a richer prompt using full diff metadata.
func BuildMRDescriptionPromptFull(sourceBranch, targetBranch string, diff *models.DiffResult) string {
	var sb strings.Builder
	sb.WriteString("Generate a concise, professional merge request description.\n\n")
	sb.WriteString(fmt.Sprintf("Source branch: %s\nTarget branch: %s\n", sourceBranch, targetBranch))
	sb.WriteString(fmt.Sprintf("Files changed: %d (+%d -%d lines)\n\n", len(diff.Files), diff.TotalAdditions, diff.TotalDeletions))

	if len(diff.Commits) > 0 {
		sb.WriteString("Commits:\n")
		for _, msg := range diff.Commits {
			sb.WriteString(fmt.Sprintf("- %s\n", msg))
		}
		sb.WriteString("\n")
	}

	if len(diff.Files) > 0 {
		sb.WriteString("Changed files:\n")
		for _, f := range diff.Files {
			status := "modified"
			if f.NewFile {
				status = "added"
			} else if f.Deleted {
				status = "deleted"
			} else if f.Renamed {
				status = "renamed"
			}
			sb.WriteString(fmt.Sprintf("- %s (%s, +%d -%d)\n", f.NewPath, status, f.Additions, f.Deletions))
		}
		sb.WriteString("\n")
	}

	diffContent := diff.DiffContent
	const maxDiffForPrompt = 30000
	if len(diffContent) > maxDiffForPrompt {
		diffContent = diffContent[:maxDiffForPrompt] + "\n\n... [diff truncated] ..."
	}
	if diffContent != "" {
		sb.WriteString("Code diff:\n```diff\n")
		sb.WriteString(diffContent)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("Write a clear MR description that:\n")
	sb.WriteString("1. Has a brief summary of what changes are being made and why\n")
	sb.WriteString("2. Lists key changes as bullet points\n")
	sb.WriteString("3. Notes any breaking changes or migration steps if apparent\n")
	sb.WriteString("\nOutput only the description text in markdown. No meta-commentary.\n")
	return sb.String()
}

// BuildTicketContentPrompt constructs a prompt for generating concise ticket content from a branch diff.
// fileContents maps file paths to their full source code (for files where the diff alone lacks context).
func BuildTicketContentPrompt(diff *models.DiffResult, template string, fileContents map[string]string) string {
	var sb strings.Builder

	sb.WriteString("You are writing a GitLab ticket (issue) specification based on code changes from a committed branch.\n")
	sb.WriteString("The ticket should read as a plan/specification in present tense, even though the code already exists.\n")
	sb.WriteString("A ticket describes WHAT needs to happen and WHY — it reads as a plan that a developer would follow.\n\n")

	sb.WriteString("## Critical Rules\n")
	sb.WriteString("1. **Specification style**: Write as a specification in present tense. Use \"This ticket addresses...\", \"The scope includes...\", \"The implementation covers...\". Do NOT use past tense (\"was changed\", \"was fixed\"). The ticket is the permanent record that describes the intent and scope.\n")
	sb.WriteString("2. **Diff-only content**: Derive ALL content ONLY from the code diff and file context below. Do NOT assume, invent, or hallucinate any project context, business logic, motivations, or details not directly visible in the diff.\n")
	sb.WriteString("3. **Replace all placeholders**: The template contains placeholder text, example text (lines starting with 'Example:'), and sample content (like 'item 1', 'Requirement 1'). Replace ALL of these with actual content derived from the diff. Never copy template examples into your output.\n")
	sb.WriteString("4. **If unclear, say so**: If the purpose of a change is not obvious from the diff, describe what the code change does technically without guessing the business reason.\n")
	sb.WriteString("5. **Dependency files**: If changes include `go.mod`, `go.sum`, `package-lock.json`, etc., summarise as \"Updated dependency versions\" — do NOT list individual modules.\n")
	sb.WriteString("6. **Bug fixes**: If the changes are a bug fix, include a Root Cause Analysis (RCA) section explaining the root cause and what scenarios the fix covers.\n\n")

	sb.WriteString("## How to Fill Template Sections\n")
	sb.WriteString("- **Summary**: Describes what this ticket covers and the expected outcome. Present tense (\"This ticket addresses...\", \"The change introduces...\").\n")
	sb.WriteString("- **Problem Statement**: Describe the problem or gap being addressed, as inferred from the diff (e.g. if a null check is added, the problem is a potential null pointer).\n")
	sb.WriteString("- **Objective**: State what the changes aim to achieve.\n")
	sb.WriteString("- **Scope**: List the concrete files and what is being modified in each.\n")
	sb.WriteString("- **Deliverables**: List what is delivered (files changed, features added, bugs fixed).\n")
	sb.WriteString("- **Acceptance Criteria**: Derive from the actual changes — what conditions must the implementation satisfy?\n")
	sb.WriteString("- If any section does not apply, write \"N/A\" — do NOT fabricate content.\n")
	sb.WriteString("- Use full file context (provided below) to understand the purpose of small diffs that lack surrounding context.\n\n")

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

	if len(fileContents) > 0 {
		sb.WriteString("## Full File Context\n")
		sb.WriteString("The following are full file contents for changed files where the diff alone may lack context.\n\n")
		const maxFileSize = 8000
		for path, content := range fileContents {
			if len(content) > maxFileSize {
				content = content[:maxFileSize] + "\n\n... [file truncated] ..."
			}
			sb.WriteString(fmt.Sprintf("### `%s`\n```\n%s\n```\n\n", path, content))
		}
	}

	sb.WriteString("## Output Instructions\n")
	sb.WriteString("The FIRST line of your output must be the ticket title (plain text, no markdown heading). Write in present tense as a specification.\n")
	sb.WriteString("Everything after the first line is the ticket description in markdown.\n")
	sb.WriteString("Follow the template structure below. Replace ALL placeholder/example text with actual diff-derived content.\n")
	sb.WriteString("Derive ALL content ONLY from the code diff — do NOT invent context.\n\n")
	sb.WriteString("Template:\n")
	sb.WriteString(template)
	sb.WriteString("\n")

	return sb.String()
}

// MRDiffEntry pairs an MR's metadata with its diff for multi-MR prompt building.
type MRDiffEntry struct {
	RepoName     string
	MRURL        string
	MRTitle      string
	MRIID        int
	SourceBranch string
	TargetBranch string
	Diff         *models.DiffResult
}

// BuildMultiMRTicketContentPrompt constructs a prompt that presents changes grouped by repo/MR,
// so the AI can summarize what changed in each repo individually.
func BuildMultiMRTicketContentPrompt(entries []MRDiffEntry, template string) string {
	var sb strings.Builder

	sb.WriteString("You are writing a GitLab ticket (issue) specification based on code changes across multiple repositories.\n")
	sb.WriteString("The ticket should read as a plan/specification in present tense, even though the code already exists.\n")
	sb.WriteString("A ticket describes WHAT needs to happen and WHY — it reads as a plan that a developer would follow.\n\n")

	sb.WriteString("## Critical Rules\n")
	sb.WriteString("1. **Specification style**: Write as a specification in present tense. Use \"This ticket addresses...\", \"The scope includes...\", \"The implementation covers...\". Do NOT use past tense (\"was changed\", \"was fixed\"). The ticket is the permanent record that describes the intent and scope.\n")
	sb.WriteString("2. **Diff-only content**: Derive ALL content ONLY from the code diffs below. Do NOT assume, invent, or hallucinate any project context, business logic, motivations, or details not directly visible in the diffs.\n")
	sb.WriteString("3. **Replace all placeholders**: The template contains placeholder text, example text (lines starting with 'Example:'), and sample content (like 'item 1', 'Requirement 1'). Replace ALL of these with actual content derived from the diffs. Never copy template examples into your output.\n")
	sb.WriteString("4. **Per-repository breakdown**: The Scope section MUST include a sub-section for EACH repository showing what is being changed there.\n")
	sb.WriteString("5. **If unclear, say so**: If the purpose of a change is not obvious from the diff, describe what the code change does technically without guessing the business reason.\n")
	sb.WriteString("6. **Dependency files**: If changes include `go.mod`, `go.sum`, `package-lock.json`, etc., summarise as \"Updated dependency versions\" — do NOT list individual modules.\n\n")

	sb.WriteString("## How to Fill Template Sections\n")
	sb.WriteString("- **Summary**: Describes what this ticket covers and the expected outcome across all affected repositories. Present tense.\n")
	sb.WriteString("- **Problem Statement**: Describe the problem or gap being addressed, as inferred from the diffs (e.g. if a null check is added, the problem is a potential null pointer).\n")
	sb.WriteString("- **Objective**: State what the changes aim to achieve.\n")
	sb.WriteString("- **Scope**: Organize by repository — one sub-section per repo listing its specific changes with concrete files.\n")
	sb.WriteString("- **Deliverables**: List what is delivered per repository (files changed, features added, bugs fixed).\n")
	sb.WriteString("- **Acceptance Criteria**: Derive from the actual changes — what conditions must the implementation satisfy?\n")
	sb.WriteString("- If any section does not apply, write \"N/A\" — do NOT fabricate content.\n\n")

	totalFiles, totalAdds, totalDels := 0, 0, 0
	for _, e := range entries {
		totalFiles += len(e.Diff.Files)
		totalAdds += e.Diff.TotalAdditions
		totalDels += e.Diff.TotalDeletions
	}

	sb.WriteString("## Overview\n")
	sb.WriteString(fmt.Sprintf("- **Repositories affected:** %d\n", len(entries)))
	sb.WriteString(fmt.Sprintf("- **Total files changed:** %d (+%d -%d lines)\n\n", totalFiles, totalAdds, totalDels))

	for i, e := range entries {
		sb.WriteString(fmt.Sprintf("---\n## Repository %d/%d: `%s`\n", i+1, len(entries), e.RepoName))
		if e.MRURL != "" {
			sb.WriteString(fmt.Sprintf("- **MR:** !%d — %s\n", e.MRIID, e.MRURL))
		}
		sb.WriteString(fmt.Sprintf("- **MR Title:** %s\n", e.MRTitle))
		sb.WriteString(fmt.Sprintf("- **Branch:** %s → %s\n", e.SourceBranch, e.TargetBranch))
		sb.WriteString(fmt.Sprintf("- **Files changed:** %d (+%d -%d lines)\n\n", len(e.Diff.Files), e.Diff.TotalAdditions, e.Diff.TotalDeletions))

		if len(e.Diff.Commits) > 0 {
			sb.WriteString("### Commits\n")
			for _, msg := range e.Diff.Commits {
				sb.WriteString(fmt.Sprintf("- %s\n", msg))
			}
			sb.WriteString("\n")
		}

		sb.WriteString("### Changed Files\n")
		for _, f := range e.Diff.Files {
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

		diffContent := e.Diff.DiffContent
		const maxPerMR = 25000
		if len(diffContent) > maxPerMR {
			diffContent = diffContent[:maxPerMR] + "\n\n... [diff truncated due to size] ..."
		}
		sb.WriteString(fmt.Sprintf("### Code Diff (`%s`)\n```diff\n", e.RepoName))
		sb.WriteString(diffContent)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("## Output Instructions\n")
	sb.WriteString("The FIRST line of your output must be the ticket title (plain text, no markdown heading). Write in present tense as a specification.\n")
	sb.WriteString("Everything after the first line is the ticket description in markdown.\n")
	sb.WriteString("Follow the template structure below. Replace ALL placeholder/example text with actual diff-derived content.\n")
	sb.WriteString("Organize the Scope section by repository — one sub-section per repo listing what is being changed there.\n")
	sb.WriteString("Derive ALL content ONLY from the code diffs above — do NOT invent context.\n\n")
	sb.WriteString("Template:\n")
	sb.WriteString(template)
	sb.WriteString("\n")

	return sb.String()
}

// BuildEpicContentPrompt constructs a prompt for generating detailed epic content from a branch diff.
func BuildEpicContentPrompt(diff *models.DiffResult, template string) string {
	var sb strings.Builder

	sb.WriteString("Generate a detailed GitLab epic based on the code changes below.\n")
	sb.WriteString("Be thorough and comprehensive. Include technical details and impact analysis.\n\n")

	sb.WriteString("## Important Rules\n")
	sb.WriteString("- Derive ALL content ONLY from the code diff provided below. Do NOT assume, invent, or hallucinate any project context, business logic, or details not present in the diff.\n")
	sb.WriteString("- If something is unclear from the diff alone, state what the diff shows without guessing the broader purpose.\n")
	sb.WriteString("- Follow the provided template structure EXACTLY — fill every section using evidence from the diff.\n\n")

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
	sb.WriteString("Everything after the first line is the epic description in markdown.\n")
	sb.WriteString("Derive ALL content ONLY from the code diff — do NOT invent context or project details.\n\n")
	sb.WriteString("Template:\n")
	sb.WriteString(template)
	sb.WriteString("\n")

	return sb.String()
}

// BuildTicketDescriptionPrompt constructs a prompt to enhance a user-provided ticket summary into a structured description.
func BuildTicketDescriptionPrompt(context string) string {
	return fmt.Sprintf(`Enhance the following ticket summary into a well-structured GitLab ticket.

User summary:
%s

Generate output in EXACTLY this format — two parts separated by a blank line:

FIRST LINE: A concise ticket title (max 72 chars, plain text, no markdown heading).

THEN the description in this markdown template:

## Summary
<Rewrite the user summary into a clear, professional 2-3 sentence description. Add relevant technical context if obvious from the summary.>

## Acceptance Criteria
- [ ] <Specific, testable criterion derived from the summary>
- [ ] <Another criterion if applicable>
- [ ] Verified in staging/test environment

## Notes
- Status: todo
- Assignee: unassigned

Keep it concise and actionable. Do not invent requirements beyond what the summary implies.`, context)
}

// BuildPipelineFailurePrompt constructs a prompt to analyze pipeline failure logs.
func BuildPipelineFailurePrompt(jobName, jobLog string) string {
	const maxLogSize = 20000
	if len(jobLog) > maxLogSize {
		jobLog = jobLog[len(jobLog)-maxLogSize:]
	}
	return fmt.Sprintf(`Analyze this CI/CD pipeline job failure and provide:
1. **Root Cause**: What caused the failure
2. **Fix Suggestion**: Specific steps to fix it
3. **Prevention**: How to prevent this in the future

Job: %s

Log output (last section):
%s

Be concise and actionable. Focus on the actual error, not setup output.`, jobName, jobLog)
}

// BuildCommitMessagePrompt constructs a prompt to generate a commit message from a diff.
func BuildCommitMessagePrompt(diff *models.DiffResult) string {
	var sb strings.Builder
	sb.WriteString("Generate a conventional commit message for the following changes.\n\n")
	sb.WriteString(fmt.Sprintf("Files changed: %d (+%d -%d)\n\n", len(diff.Files), diff.TotalAdditions, diff.TotalDeletions))

	if len(diff.Files) > 0 {
		sb.WriteString("Changed files:\n")
		for _, f := range diff.Files {
			status := "modified"
			if f.NewFile {
				status = "added"
			} else if f.Deleted {
				status = "deleted"
			}
			sb.WriteString(fmt.Sprintf("- %s (%s)\n", f.NewPath, status))
		}
		sb.WriteString("\n")
	}

	diffContent := diff.DiffContent
	const maxDiffSize = 15000
	if len(diffContent) > maxDiffSize {
		diffContent = diffContent[:maxDiffSize] + "\n... [truncated]"
	}
	if diffContent != "" {
		sb.WriteString("```diff\n")
		sb.WriteString(diffContent)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("Rules:\n")
	sb.WriteString("- Use conventional commit format: type(scope): description\n")
	sb.WriteString("- Types: feat, fix, refactor, docs, test, chore, ci, style, perf\n")
	sb.WriteString("- Keep the first line under 72 characters\n")
	sb.WriteString("- Add a body with bullet points if the change is complex\n")
	sb.WriteString("- Output ONLY the commit message, nothing else\n")
	return sb.String()
}

// BuildReleaseNotesPrompt constructs a prompt to generate release notes from commits/diff.
func BuildReleaseNotesPrompt(fromTag, toTag string, commits []string, diff *models.DiffResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Generate release notes for version %s (from %s).\n\n", toTag, fromTag))

	if len(commits) > 0 {
		sb.WriteString("Commits:\n")
		for _, msg := range commits {
			sb.WriteString(fmt.Sprintf("- %s\n", msg))
		}
		sb.WriteString("\n")
	}

	if diff != nil && len(diff.Files) > 0 {
		sb.WriteString(fmt.Sprintf("Files changed: %d (+%d -%d)\n\n", len(diff.Files), diff.TotalAdditions, diff.TotalDeletions))
	}

	sb.WriteString("Format the release notes as:\n")
	sb.WriteString("## What's New\n- New features\n\n")
	sb.WriteString("## Bug Fixes\n- Bug fixes\n\n")
	sb.WriteString("## Improvements\n- Improvements and refactoring\n\n")
	sb.WriteString("## Breaking Changes\n- Any breaking changes\n\n")
	sb.WriteString("Only include sections that have entries. Be concise but informative.\n")
	sb.WriteString("Output only the release notes in markdown.\n")
	return sb.String()
}

// BuildIssueSuggestionPrompt constructs a prompt to triage/analyze an issue.
func BuildIssueSuggestionPrompt(title, description string, existingLabels []string) string {
	var sb strings.Builder
	sb.WriteString("Analyze this GitLab issue and suggest:\n")
	sb.WriteString("1. **Priority**: critical / high / medium / low\n")
	sb.WriteString("2. **Type**: bug / feature / enhancement / documentation / chore\n")
	sb.WriteString("3. **Suggested Labels**: based on content\n")
	sb.WriteString("4. **Brief Analysis**: 1-2 sentences about effort/complexity\n\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n\n", title))
	if description != "" {
		desc := description
		if len(desc) > 5000 {
			desc = desc[:5000] + "..."
		}
		sb.WriteString(fmt.Sprintf("Description:\n%s\n\n", desc))
	}
	if len(existingLabels) > 0 {
		sb.WriteString(fmt.Sprintf("Existing project labels: %s\n", strings.Join(existingLabels, ", ")))
	}
	sb.WriteString("\nBe concise and actionable.\n")
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
