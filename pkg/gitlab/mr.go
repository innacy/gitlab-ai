package gitlab

import (
	"fmt"
	"strings"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/utils"
)

// GetMergeRequest fetches a merge request by project and MR IID.
func (c *Client) GetMergeRequest(projectPath string, mrIID int) (*models.MergeRequestInfo, error) {
	// Get the project first
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	// Fetch MR details
	mr, _, err := c.api.MergeRequests.GetMergeRequest(project.ID, mrIID, nil)
	if err != nil {
		return nil, utils.NewMRNotFoundError(projectPath, mrIID)
	}

	// Fetch MR changes (diffs)
	changes, _, err := c.api.MergeRequests.GetMergeRequestChanges(project.ID, mrIID, nil)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to get changes for MR #%d", mrIID), err)
	}

	// Build change list
	var mrChanges []models.MRChange
	var diffParts []string

	for _, change := range changes.Changes {
		mrChange := models.MRChange{
			OldPath:     change.OldPath,
			NewPath:     change.NewPath,
			Diff:        change.Diff,
			NewFile:     change.NewFile,
			RenamedFile: change.RenamedFile,
			DeletedFile: change.DeletedFile,
		}
		mrChanges = append(mrChanges, mrChange)

		// Build a readable diff string
		header := fmt.Sprintf("--- a/%s\n+++ b/%s", change.OldPath, change.NewPath)
		diffParts = append(diffParts, header+"\n"+change.Diff)
	}

	// Get author name
	authorName := ""
	authorUser := ""
	if mr.Author != nil {
		authorName = mr.Author.Name
		authorUser = mr.Author.Username
	}

	// Get labels
	var labels []string
	labels = append(labels, mr.Labels...)

	info := &models.MergeRequestInfo{
		ID:           mr.ID,
		IID:          mr.IID,
		ProjectID:    project.ID,
		Title:        mr.Title,
		Description:  mr.Description,
		State:        mr.State,
		Author:       authorName,
		AuthorUser:   authorUser,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		WebURL:       mr.WebURL,
		Labels:       labels,
		Changes:      mrChanges,
		DiffContent:  strings.Join(diffParts, "\n\n"),
	}

	return info, nil
}

// PostMRComment posts a comment/note on a merge request.
func (c *Client) PostMRComment(projectPath string, mrIID int, body string) (string, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return "", err
	}

	note, _, err := c.api.Notes.CreateMergeRequestNote(project.ID, mrIID, &gogitlab.CreateMergeRequestNoteOptions{
		Body: gogitlab.Ptr(body),
	})
	if err != nil {
		return "", utils.NewGitLabError(fmt.Sprintf("failed to post comment on MR #%d", mrIID), err)
	}

	// Construct the note URL
	noteURL := fmt.Sprintf("%s/-/merge_requests/%d#note_%d",
		project.WebURL, mrIID, note.ID)

	return noteURL, nil
}

// ListProjectMRs returns merge requests for a project filtered by state.
func (c *Client) ListProjectMRs(projectPath, state string, limit int) ([]models.MRListItem, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 20
	}

	opts := &gogitlab.ListProjectMergeRequestsOptions{
		State:   gogitlab.Ptr(state),
		OrderBy: gogitlab.Ptr("updated_at"),
		Sort:    gogitlab.Ptr("desc"),
		ListOptions: gogitlab.ListOptions{PerPage: limit},
	}

	mrs, _, err := c.api.MergeRequests.ListProjectMergeRequests(project.ID, opts)
	if err != nil {
		return nil, utils.NewGitLabError("failed to list merge requests", err)
	}

	result := make([]models.MRListItem, 0, len(mrs))
	for _, mr := range mrs {
		author := ""
		if mr.Author != nil {
			author = mr.Author.Username
		}
		item := models.MRListItem{
			IID:          mr.IID,
			Title:        mr.Title,
			State:        mr.State,
			Author:       author,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			WebURL:       mr.WebURL,
		}
		if mr.UpdatedAt != nil {
			item.UpdatedAt = *mr.UpdatedAt
		}
		result = append(result, item)
	}
	return result, nil
}

// CreateMergeRequest creates a new MR and returns its info.
func (c *Client) CreateMergeRequest(projectPath, sourceBranch, targetBranch, title, description string) (*models.MRListItem, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	mr, _, err := c.api.MergeRequests.CreateMergeRequest(project.ID, &gogitlab.CreateMergeRequestOptions{
		Title:        gogitlab.Ptr(title),
		Description:  gogitlab.Ptr(description),
		SourceBranch: gogitlab.Ptr(sourceBranch),
		TargetBranch: gogitlab.Ptr(targetBranch),
	})
	if err != nil {
		return nil, utils.NewGitLabError("failed to create merge request", err)
	}

	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}

	item := &models.MRListItem{
		IID:          mr.IID,
		Title:        mr.Title,
		State:        mr.State,
		Author:       author,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		WebURL:       mr.WebURL,
	}
	if mr.UpdatedAt != nil {
		item.UpdatedAt = *mr.UpdatedAt
	}
	return item, nil
}

// GetMRPipeline returns the head pipeline and its jobs for a merge request.
func (c *Client) GetMRPipeline(projectPath string, mrIID int) (*models.PipelineInfo, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	mr, _, err := c.api.MergeRequests.GetMergeRequest(project.ID, mrIID, nil)
	if err != nil {
		return nil, utils.NewMRNotFoundError(projectPath, mrIID)
	}

	var pipelineID int
	info := &models.PipelineInfo{}

	if mr.HeadPipeline != nil {
		pipelineID = mr.HeadPipeline.ID
		info.ID = mr.HeadPipeline.ID
		info.Status = mr.HeadPipeline.Status
		info.Ref = mr.HeadPipeline.Ref
		info.WebURL = mr.HeadPipeline.WebURL
	} else if mr.Pipeline != nil {
		pipelineID = mr.Pipeline.ID
		info.ID = mr.Pipeline.ID
		info.Status = mr.Pipeline.Status
		info.Ref = mr.Pipeline.Ref
		info.WebURL = mr.Pipeline.WebURL
	} else {
		return nil, fmt.Errorf("no pipeline found for MR #%d", mrIID)
	}

	jobs, _, err := c.api.Jobs.ListPipelineJobs(project.ID, pipelineID, nil)
	if err == nil {
		for _, job := range jobs {
			info.Jobs = append(info.Jobs, models.JobInfo{
				ID:     job.ID,
				Name:   job.Name,
				Stage:  job.Stage,
				Status: job.Status,
				WebURL: job.WebURL,
			})
		}
	}

	return info, nil
}

// GetBranchDiff returns commit messages between two branches (from..to).
func (c *Client) GetBranchDiff(projectPath, from, to string) ([]string, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	compare, _, err := c.api.Repositories.Compare(project.ID, &gogitlab.CompareOptions{
		From: gogitlab.Ptr(from),
		To:   gogitlab.Ptr(to),
	})
	if err != nil {
		return nil, utils.NewGitLabError("failed to compare branches", err)
	}

	var messages []string
	for _, commit := range compare.Commits {
		messages = append(messages, commit.Title)
	}
	return messages, nil
}

// CountMRChanges computes additions and deletions from the diff.
func CountMRChanges(changes []models.MRChange) (additions int, deletions int) {
	for _, change := range changes {
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
