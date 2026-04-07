package gitlab

import (
	"sort"
	"strings"
	"time"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/utils"
)

var protectedBranchNames = map[string]bool{
	"master": true, "main": true,
	"development": true, "develop": true,
}

// ListBranches returns branches sorted by latest commit date (descending).
func (c *Client) ListBranches(projectPath string, limit int) ([]models.BranchInfo, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	opts := &gogitlab.ListBranchesOptions{
		ListOptions: gogitlab.ListOptions{PerPage: 100},
	}

	branches, _, err := c.api.Branches.ListBranches(project.ID, opts)
	if err != nil {
		return nil, utils.NewGitLabError("failed to list branches", err)
	}

	result := make([]models.BranchInfo, 0, len(branches))
	for _, b := range branches {
		info := models.BranchInfo{
			Name:      b.Name,
			Merged:    b.Merged,
			Protected: b.Protected,
			Default:   b.Default,
		}
		if b.Commit != nil {
			if b.Commit.CommittedDate != nil {
				info.CommitDate = *b.Commit.CommittedDate
			}
			info.CommitTitle = b.Commit.Title
			info.AuthorName = b.Commit.AuthorName
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CommitDate.After(result[j].CommitDate)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// ListActiveBranches returns non-default, non-protected branches sorted by latest commit.
func (c *Client) ListActiveBranches(projectPath string, limit int) ([]models.BranchInfo, error) {
	all, err := c.ListBranches(projectPath, 0)
	if err != nil {
		return nil, err
	}

	var active []models.BranchInfo
	for _, b := range all {
		if protectedBranchNames[strings.ToLower(b.Name)] || b.Default {
			continue
		}
		active = append(active, b)
	}

	if limit > 0 && len(active) > limit {
		active = active[:limit]
	}
	return active, nil
}

// ListMergedBranches returns branches that are merged or stale (>1 month old).
func (c *Client) ListMergedBranches(projectPath string) ([]models.BranchInfo, error) {
	all, err := c.ListBranches(projectPath, 0)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, -1, 0)
	var merged []models.BranchInfo
	for _, b := range all {
		if protectedBranchNames[strings.ToLower(b.Name)] || b.Default || b.Protected {
			continue
		}
		if b.Merged || b.CommitDate.Before(cutoff) {
			merged = append(merged, b)
		}
	}
	return merged, nil
}

// DeleteBranch deletes a branch from a project.
func (c *Client) DeleteBranch(projectPath, branchName string) error {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return err
	}
	_, err = c.api.Branches.DeleteBranch(project.ID, branchName)
	if err != nil {
		return utils.NewGitLabError("failed to delete branch "+branchName, err)
	}
	return nil
}
// GetBranch gets a specific branch
func (c *Client) GetBranch(projectPath, branchName string) (*models.BranchInfo, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	b, _, err := c.api.Branches.GetBranch(project.ID, branchName)
	if err != nil {
		return nil, utils.NewGitLabError("failed to get branch", err)
	}

	info := &models.BranchInfo{
		Name:      b.Name,
		Merged:    b.Merged,
		Protected: b.Protected,
		Default:   b.Default,
	}
	if b.Commit != nil {
		if b.Commit.CommittedDate != nil {
			info.CommitDate = *b.Commit.CommittedDate
		}
		info.CommitTitle = b.Commit.Title
		info.AuthorName = b.Commit.AuthorName
	}

	return info, nil
}
