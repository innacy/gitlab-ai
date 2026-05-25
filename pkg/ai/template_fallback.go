package ai

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gitlab-ai/internal/models"
)

// BuildTemplateDescription generates a structured MR description from diff metadata
// without any AI. Used as the last-resort fallback when all AI providers fail.
func BuildTemplateDescription(sourceBranch, targetBranch string, diff *models.DiffResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Overview\n\nMerge `%s` into `%s`.\n\n", sourceBranch, targetBranch))

	if len(diff.Files) > 0 || diff.TotalAdditions > 0 || diff.TotalDeletions > 0 {
		sb.WriteString(fmt.Sprintf("**%d** files changed ", len(diff.Files)))
		sb.WriteString(fmt.Sprintf("with **%d additions** and **%d deletions**.\n\n", diff.TotalAdditions, diff.TotalDeletions))
	}

	if purpose := inferPurpose(sourceBranch, diff); purpose != "" {
		sb.WriteString(fmt.Sprintf("**Purpose:** %s\n\n", purpose))
	}

	if len(diff.Commits) > 0 {
		sb.WriteString("## Changes\n\n")
		for _, msg := range diff.Commits {
			sb.WriteString(fmt.Sprintf("- %s\n", msg))
		}
		sb.WriteString("\n")
	}

	if len(diff.Files) > 0 {
		groups := groupFilesByDirectory(diff.Files)
		if len(groups) > 0 {
			sb.WriteString("## Affected Areas\n\n")
			for _, g := range groups {
				sb.WriteString(fmt.Sprintf("- **%s** — %d file(s) (+%d -%d)\n", g.dir, g.count, g.additions, g.deletions))
			}
			sb.WriteString("\n")
		}

		if newFiles, deleted, renamed := categorizeFiles(diff.Files); len(newFiles) > 0 || len(deleted) > 0 || len(renamed) > 0 {
			sb.WriteString("## Notable File Changes\n\n")
			if len(newFiles) > 0 {
				sb.WriteString("**New files:**\n")
				for _, f := range newFiles {
					sb.WriteString(fmt.Sprintf("- `%s`\n", f))
				}
			}
			if len(deleted) > 0 {
				sb.WriteString("**Deleted files:**\n")
				for _, f := range deleted {
					sb.WriteString(fmt.Sprintf("- `%s`\n", f))
				}
			}
			if len(renamed) > 0 {
				sb.WriteString("**Renamed files:**\n")
				for _, f := range renamed {
					sb.WriteString(fmt.Sprintf("- `%s`\n", f))
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("---\n")

	return sb.String()
}

func inferPurpose(branch string, diff *models.DiffResult) string {
	bl := strings.ToLower(branch)

	prefixes := map[string]string{
		"feature/":  "New feature",
		"feat/":     "New feature",
		"fix/":      "Bug fix",
		"bugfix/":   "Bug fix",
		"hotfix/":   "Hotfix",
		"chore/":    "Maintenance / chore",
		"refactor/": "Code refactoring",
		"docs/":     "Documentation update",
		"test/":     "Test improvements",
		"ci/":       "CI/CD changes",
		"release/":  "Release preparation",
	}

	for prefix, purpose := range prefixes {
		if strings.HasPrefix(bl, prefix) {
			slug := strings.TrimPrefix(bl, prefix)
			slug = strings.ReplaceAll(slug, "-", " ")
			slug = strings.ReplaceAll(slug, "_", " ")
			return fmt.Sprintf("%s — %s", purpose, slug)
		}
	}

	hasTests := false
	hasDocs := false
	hasConfig := false
	for _, f := range diff.Files {
		lp := strings.ToLower(f.NewPath)
		if strings.Contains(lp, "test") || strings.Contains(lp, "spec") {
			hasTests = true
		}
		if strings.HasSuffix(lp, ".md") || strings.Contains(lp, "doc") {
			hasDocs = true
		}
		if strings.Contains(lp, "config") || strings.HasSuffix(lp, ".yaml") || strings.HasSuffix(lp, ".yml") || strings.HasSuffix(lp, ".toml") {
			hasConfig = true
		}
	}

	var hints []string
	if hasTests {
		hints = append(hints, "includes test changes")
	}
	if hasDocs {
		hints = append(hints, "includes documentation updates")
	}
	if hasConfig {
		hints = append(hints, "includes configuration changes")
	}
	if len(hints) > 0 {
		return strings.Join(hints, ", ")
	}

	return ""
}

type dirGroup struct {
	dir       string
	count     int
	additions int
	deletions int
}

func groupFilesByDirectory(files []models.DiffFile) []dirGroup {
	dirs := make(map[string]*dirGroup)

	for _, f := range files {
		dir := filepath.Dir(f.NewPath)
		if dir == "." {
			dir = "(root)"
		}
		parts := strings.SplitN(dir, "/", 3)
		if len(parts) > 2 {
			dir = parts[0] + "/" + parts[1]
		}

		if g, ok := dirs[dir]; ok {
			g.count++
			g.additions += f.Additions
			g.deletions += f.Deletions
		} else {
			dirs[dir] = &dirGroup{dir: dir, count: 1, additions: f.Additions, deletions: f.Deletions}
		}
	}

	result := make([]dirGroup, 0, len(dirs))
	for _, g := range dirs {
		result = append(result, *g)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].count > result[j].count })

	if len(result) > 8 {
		result = result[:8]
	}
	return result
}

func categorizeFiles(files []models.DiffFile) (newFiles, deleted, renamed []string) {
	for _, f := range files {
		if f.NewFile {
			newFiles = append(newFiles, f.NewPath)
		}
		if f.Deleted {
			deleted = append(deleted, f.NewPath)
		}
		if f.Renamed {
			renamed = append(renamed, fmt.Sprintf("%s -> %s", f.OldPath, f.NewPath))
		}
	}
	return
}
