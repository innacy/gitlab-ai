package gitlab

import (
	"fmt"
	"strings"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/utils"
)

// ListTags returns tags sorted by update date (descending).
func (c *Client) ListTags(projectPath string, limit int) ([]models.TagInfo, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 50
	}

	opts := &gogitlab.ListTagsOptions{
		OrderBy: gogitlab.Ptr("updated"),
		Sort:    gogitlab.Ptr("desc"),
		ListOptions: gogitlab.ListOptions{PerPage: limit},
	}

	tags, _, err := c.api.Tags.ListTags(project.ID, opts)
	if err != nil {
		return nil, utils.NewGitLabError("failed to list tags", err)
	}

	result := make([]models.TagInfo, 0, len(tags))
	for _, t := range tags {
		info := models.TagInfo{
			Name:    t.Name,
			Message: t.Message,
		}
		if t.Commit != nil {
			info.CommitTitle = t.Commit.Title
			if t.Commit.CommittedDate != nil {
				info.CommitDate = *t.Commit.CommittedDate
			}
		}
		result = append(result, info)
	}

	return result, nil
}

// TagExists checks if a tag exists in the project.
func (c *Client) TagExists(projectPath, tagName string) (bool, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return false, err
	}

	_, _, err = c.api.Tags.GetTag(project.ID, tagName)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// BranchExists checks if a branch exists in the project.
func (c *Client) BranchExists(projectPath, branchName string) (bool, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return false, err
	}

	_, _, err = c.api.Branches.GetBranch(project.ID, branchName)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// GetRefDiff returns the full diff between two refs (tags or branches).
func (c *Client) GetRefDiff(projectPath, from, to string) (*models.DiffResult, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	compare, _, err := c.api.Repositories.Compare(project.ID, &gogitlab.CompareOptions{
		From: gogitlab.Ptr(from),
		To:   gogitlab.Ptr(to),
	})
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to compare %s..%s", from, to), err)
	}

	var diffParts []string
	var fileChanges []models.DiffFile
	totalAdditions := 0
	totalDeletions := 0

	for _, d := range compare.Diffs {
		header := fmt.Sprintf("--- a/%s\n+++ b/%s", d.OldPath, d.NewPath)
		diffParts = append(diffParts, header+"\n"+d.Diff)

		adds, dels := countDiffLines(d.Diff)
		totalAdditions += adds
		totalDeletions += dels

		fileChanges = append(fileChanges, models.DiffFile{
			OldPath:   d.OldPath,
			NewPath:   d.NewPath,
			NewFile:   d.NewFile,
			Renamed:   d.RenamedFile,
			Deleted:   d.DeletedFile,
			Additions: adds,
			Deletions: dels,
		})
	}

	var commitMessages []string
	for _, commit := range compare.Commits {
		commitMessages = append(commitMessages, commit.Title)
	}

	return &models.DiffResult{
		From:           from,
		To:             to,
		DiffContent:    strings.Join(diffParts, "\n\n"),
		Files:          fileChanges,
		Commits:        commitMessages,
		TotalAdditions: totalAdditions,
		TotalDeletions: totalDeletions,
	}, nil
}

func countDiffLines(diff string) (additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	return
}
