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
