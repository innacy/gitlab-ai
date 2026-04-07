package gitlab

import (
	"fmt"
	"strings"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/internal/models"
)

// CheckProjectRelease checks whether a project's development branch has
// unreleased commits compared to master/main and retrieves the latest tag.
func (c *Client) CheckProjectRelease(projectID int, projectPath string) models.ProjectReleaseInfo {
	info := models.ProjectReleaseInfo{
		Name: extractProjectName(projectPath),
		Path: projectPath,
	}

	masterBranch := c.findBranch(projectID, "master", "main")
	if masterBranch == "" {
		info.Status = models.ReleaseInvalid
		info.InvalidReason = "No master/main branch"
		return info
	}

	devBranch := c.findBranch(projectID, "development", "develop")
	if devBranch == "" {
		info.Status = models.ReleaseInvalid
		info.InvalidReason = "No development/develop branch"
		return info
	}

	compare, _, err := c.api.Repositories.Compare(projectID, &gogitlab.CompareOptions{
		From: gogitlab.Ptr(masterBranch),
		To:   gogitlab.Ptr(devBranch),
	})
	if err != nil {
		info.Status = models.ReleaseInvalid
		info.InvalidReason = fmt.Sprintf("Compare failed: %v", err)
		return info
	}

	info.LatestTag = c.getLatestTag(projectID)

	if len(compare.Commits) > 0 {
		info.Status = models.ReleasePending
		info.CommitsAhead = len(compare.Commits)
		if last := compare.Commits[len(compare.Commits)-1]; last.CommittedDate != nil {
			info.LastDevCommitDate = *last.CommittedDate
		}
	} else {
		info.Status = models.ReleaseUpToDate
	}

	return info
}

func (c *Client) findBranch(projectID int, candidates ...string) string {
	for _, name := range candidates {
		_, _, err := c.api.Branches.GetBranch(projectID, name)
		if err == nil {
			return name
		}
	}
	return ""
}

func (c *Client) getLatestTag(projectID int) string {
	tags, _, err := c.api.Tags.ListTags(projectID, &gogitlab.ListTagsOptions{
		OrderBy: gogitlab.Ptr("updated"),
		Sort:    gogitlab.Ptr("desc"),
		ListOptions: gogitlab.ListOptions{PerPage: 1},
	})
	if err != nil || len(tags) == 0 {
		return "no tags"
	}
	return tags[0].Name
}

func extractProjectName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
