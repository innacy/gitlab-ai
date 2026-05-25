package ai

import (
	"strings"
	"testing"

	"gitlab-ai/internal/models"
)

func sampleDiff() *models.DiffResult {
	return &models.DiffResult{
		From:           "development",
		To:             "fix/42",
		DiffContent:    "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new",
		Files:          []models.DiffFile{{NewPath: "main.go", Additions: 1, Deletions: 1}},
		Commits:        []string{"Fix null pointer in handler"},
		TotalAdditions: 1,
		TotalDeletions: 1,
	}
}

const testTemplate = `## Summary
<summary>

## Acceptance Criteria
- [ ] criterion`

// ─── BuildTicketContentPrompt ────────────────────────────────────────────────

func TestBuildTicketContentPrompt_ContainsTemplate(t *testing.T) {
	prompt := BuildTicketContentPrompt(sampleDiff(), testTemplate, nil)
	if !strings.Contains(prompt, "## Summary") || !strings.Contains(prompt, "## Acceptance Criteria") {
		t.Error("prompt must contain the ticket template sections")
	}
}

func TestBuildTicketContentPrompt_NoHallucinationInstruction(t *testing.T) {
	prompt := BuildTicketContentPrompt(sampleDiff(), testTemplate, nil)
	if !strings.Contains(prompt, "Do NOT assume") && !strings.Contains(prompt, "do not assume") &&
		!strings.Contains(prompt, "Do NOT invent") && !strings.Contains(prompt, "do not invent") &&
		!strings.Contains(prompt, "Do NOT hallucinate") && !strings.Contains(prompt, "do not hallucinate") {
		t.Error("prompt must contain an instruction to not assume/invent/hallucinate project context")
	}
}

func TestBuildTicketContentPrompt_TemplateStrictInstruction(t *testing.T) {
	prompt := BuildTicketContentPrompt(sampleDiff(), testTemplate, nil)
	if !strings.Contains(prompt, "ONLY from the code diff") && !strings.Contains(prompt, "only from the code diff") &&
		!strings.Contains(prompt, "ONLY from the diff") && !strings.Contains(prompt, "only from the diff") {
		t.Error("prompt must instruct AI to derive content only from the provided diff")
	}
}

func TestBuildTicketContentPrompt_ContainsDiffData(t *testing.T) {
	prompt := BuildTicketContentPrompt(sampleDiff(), testTemplate, nil)
	if !strings.Contains(prompt, "main.go") {
		t.Error("prompt must contain changed file names from the diff")
	}
	if !strings.Contains(prompt, "development") || !strings.Contains(prompt, "fix/42") {
		t.Error("prompt must contain branch names")
	}
}

func TestBuildTicketContentPrompt_IncludesFileContents(t *testing.T) {
	fc := map[string]string{"main.go": "package main\nfunc main() {}"}
	prompt := BuildTicketContentPrompt(sampleDiff(), testTemplate, fc)
	if !strings.Contains(prompt, "package main") {
		t.Error("prompt must include provided file contents")
	}
}

// ─── BuildMultiMRTicketContentPrompt ─────────────────────────────────────────

func TestBuildMultiMRTicketContentPrompt_ContainsTemplate(t *testing.T) {
	entries := []MRDiffEntry{{
		RepoName: "backend", MRIID: 1, SourceBranch: "fix/42", TargetBranch: "dev",
		Diff: sampleDiff(),
	}}
	prompt := BuildMultiMRTicketContentPrompt(entries, testTemplate)
	if !strings.Contains(prompt, "## Summary") || !strings.Contains(prompt, "## Acceptance Criteria") {
		t.Error("prompt must contain the ticket template sections")
	}
}

func TestBuildMultiMRTicketContentPrompt_NoHallucinationInstruction(t *testing.T) {
	entries := []MRDiffEntry{{
		RepoName: "backend", MRIID: 1, SourceBranch: "fix/42", TargetBranch: "dev",
		Diff: sampleDiff(),
	}}
	prompt := BuildMultiMRTicketContentPrompt(entries, testTemplate)
	if !strings.Contains(prompt, "Do NOT assume") && !strings.Contains(prompt, "do not assume") &&
		!strings.Contains(prompt, "Do NOT invent") && !strings.Contains(prompt, "do not invent") &&
		!strings.Contains(prompt, "Do NOT hallucinate") && !strings.Contains(prompt, "do not hallucinate") {
		t.Error("prompt must contain an instruction to not assume/invent/hallucinate project context")
	}
}

func TestBuildMultiMRTicketContentPrompt_PerRepoStructure(t *testing.T) {
	entries := []MRDiffEntry{
		{RepoName: "backend", MRIID: 1, SourceBranch: "fix/42", TargetBranch: "dev", Diff: sampleDiff()},
		{RepoName: "frontend", MRIID: 2, SourceBranch: "fix/42", TargetBranch: "dev", Diff: sampleDiff()},
	}
	prompt := BuildMultiMRTicketContentPrompt(entries, testTemplate)
	if !strings.Contains(prompt, "`backend`") || !strings.Contains(prompt, "`frontend`") {
		t.Error("prompt must contain per-repo sections with repo names")
	}
}

func TestBuildMultiMRTicketContentPrompt_DeriveFromDiffOnly(t *testing.T) {
	entries := []MRDiffEntry{{
		RepoName: "backend", MRIID: 1, SourceBranch: "fix/42", TargetBranch: "dev",
		Diff: sampleDiff(),
	}}
	prompt := BuildMultiMRTicketContentPrompt(entries, testTemplate)
	if !strings.Contains(prompt, "ONLY from the code diff") && !strings.Contains(prompt, "only from the code diff") &&
		!strings.Contains(prompt, "ONLY from the diff") && !strings.Contains(prompt, "only from the diff") {
		t.Error("prompt must instruct AI to derive content only from the provided diffs")
	}
}

func TestBuildMultiMRTicketContentPrompt_SpecificationFraming(t *testing.T) {
	entries := []MRDiffEntry{{
		RepoName: "backend", MRIID: 1, SourceBranch: "fix/42", TargetBranch: "dev",
		Diff: sampleDiff(),
	}}
	prompt := BuildMultiMRTicketContentPrompt(entries, testTemplate)
	hasSpecFraming := strings.Contains(prompt, "specification") ||
		strings.Contains(prompt, "present tense") ||
		strings.Contains(prompt, "reads as a plan")
	if !hasSpecFraming {
		t.Error("prompt must frame the ticket as a present-tense specification/plan")
	}
}

func TestBuildMultiMRTicketContentPrompt_TemplateSectionMapping(t *testing.T) {
	entries := []MRDiffEntry{{
		RepoName: "backend", MRIID: 1, SourceBranch: "fix/42", TargetBranch: "dev",
		Diff: sampleDiff(),
	}}
	prompt := BuildMultiMRTicketContentPrompt(entries, testTemplate)
	hasMapping := (strings.Contains(prompt, "Summary") && strings.Contains(prompt, "describes")) ||
		strings.Contains(prompt, "Problem Statement") ||
		strings.Contains(prompt, "Scope")
	if !hasMapping {
		t.Error("prompt must guide AI on how to map diff content to template sections")
	}
}

func TestBuildMultiMRTicketContentPrompt_IgnoreTemplateExamples(t *testing.T) {
	entries := []MRDiffEntry{{
		RepoName: "backend", MRIID: 1, SourceBranch: "fix/42", TargetBranch: "dev",
		Diff: sampleDiff(),
	}}
	prompt := BuildMultiMRTicketContentPrompt(entries, testTemplate)
	hasExampleWarning := strings.Contains(prompt, "placeholder") ||
		strings.Contains(prompt, "example text") ||
		strings.Contains(prompt, "Example:") ||
		strings.Contains(prompt, "sample content")
	if !hasExampleWarning {
		t.Error("prompt must instruct AI to replace all template placeholders/examples with actual diff-derived content")
	}
}

func TestBuildTicketContentPrompt_SpecificationFraming(t *testing.T) {
	prompt := BuildTicketContentPrompt(sampleDiff(), testTemplate, nil)
	hasSpecFraming := strings.Contains(prompt, "specification") ||
		strings.Contains(prompt, "present tense") ||
		strings.Contains(prompt, "reads as a plan")
	if !hasSpecFraming {
		t.Error("prompt must frame the ticket as a present-tense specification/plan")
	}
}

func TestBuildTicketContentPrompt_TemplateSectionMapping(t *testing.T) {
	prompt := BuildTicketContentPrompt(sampleDiff(), testTemplate, nil)
	hasMapping := (strings.Contains(prompt, "Summary") && strings.Contains(prompt, "describes")) ||
		strings.Contains(prompt, "Problem Statement") ||
		strings.Contains(prompt, "Scope")
	if !hasMapping {
		t.Error("prompt must guide AI on how to map diff content to template sections")
	}
}

// ─── BuildEpicContentPrompt ─────────────────────────────────────────────────

func TestBuildEpicContentPrompt_ContainsTemplate(t *testing.T) {
	prompt := BuildEpicContentPrompt(sampleDiff(), testTemplate)
	if !strings.Contains(prompt, "## Summary") || !strings.Contains(prompt, "## Acceptance Criteria") {
		t.Error("prompt must contain the epic template sections")
	}
}

func TestBuildEpicContentPrompt_NoHallucinationInstruction(t *testing.T) {
	prompt := BuildEpicContentPrompt(sampleDiff(), testTemplate)
	if !strings.Contains(prompt, "Do NOT assume") && !strings.Contains(prompt, "do not assume") &&
		!strings.Contains(prompt, "Do NOT invent") && !strings.Contains(prompt, "do not invent") &&
		!strings.Contains(prompt, "Do NOT hallucinate") && !strings.Contains(prompt, "do not hallucinate") {
		t.Error("prompt must contain an instruction to not assume/invent/hallucinate project context")
	}
}

func TestBuildEpicContentPrompt_DeriveFromDiffOnly(t *testing.T) {
	prompt := BuildEpicContentPrompt(sampleDiff(), testTemplate)
	if !strings.Contains(prompt, "ONLY from the code diff") && !strings.Contains(prompt, "only from the code diff") &&
		!strings.Contains(prompt, "ONLY from the diff") && !strings.Contains(prompt, "only from the diff") {
		t.Error("prompt must instruct AI to derive content only from the provided diff")
	}
}

func TestBuildEpicContentPrompt_ContainsDiffData(t *testing.T) {
	prompt := BuildEpicContentPrompt(sampleDiff(), testTemplate)
	if !strings.Contains(prompt, "main.go") {
		t.Error("prompt must contain changed file names from the diff")
	}
	if !strings.Contains(prompt, "Fix null pointer in handler") {
		t.Error("prompt must contain commit messages")
	}
}
