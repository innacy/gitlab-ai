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
			ProjectID:    mr.ProjectID,
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

// FindMRByBranches finds the most recent MR (any state) matching the given source and target branches.
// Returns nil, nil if no MR is found.
func (c *Client) FindMRByBranches(projectPath, source, target string) (*models.MRListItem, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	opts := &gogitlab.ListProjectMergeRequestsOptions{
		SourceBranch: gogitlab.Ptr(source),
		TargetBranch: gogitlab.Ptr(target),
		OrderBy:      gogitlab.Ptr("updated_at"),
		Sort:         gogitlab.Ptr("desc"),
		ListOptions:  gogitlab.ListOptions{PerPage: 1},
	}

	mrs, _, err := c.api.MergeRequests.ListProjectMergeRequests(project.ID, opts)
	if err != nil {
		return nil, err
	}
	if len(mrs) == 0 {
		return nil, nil
	}

	mr := mrs[0]
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	item := &models.MRListItem{
		IID:          mr.IID,
		ProjectID:    mr.ProjectID,
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
		ProjectID:    mr.ProjectID,
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

// GetBranchDiffFull returns a full diff result (commits, files, diff content) between two branches.
func (c *Client) GetBranchDiffFull(projectPath, from, to string) (*models.DiffResult, error) {
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

		adds, dels := countDiffLinesFromPatch(d.Diff)
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

func countDiffLinesFromPatch(patch string) (additions, deletions int) {
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	return
}

// GetMRDiff returns the diff for a merge request by project path.
func (c *Client) GetMRDiff(projectPath string, mrIID int) (*models.DiffResult, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}
	return c.GetMRDiffByProjectID(project.ID, mrIID)
}

// GetMRDiffByProjectID returns the diff for a merge request using the numeric project ID.
// Uses the MR changes API first, falls back to SHA-based compare.
func (c *Client) GetMRDiffByProjectID(projectID, mrIID int) (*models.DiffResult, error) {
	changes, _, err := c.api.MergeRequests.GetMergeRequestChanges(projectID, mrIID, nil)
	if err == nil && len(changes.Changes) > 0 {
		return buildDiffFromChanges(changes), nil
	}

	mr, _, mrErr := c.api.MergeRequests.GetMergeRequest(projectID, mrIID, nil)
	if mrErr != nil {
		if err != nil {
			return nil, utils.NewGitLabError(fmt.Sprintf("failed to get changes for MR !%d", mrIID), err)
		}
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to get MR !%d", mrIID), mrErr)
	}

	baseSha := mr.DiffRefs.BaseSha
	headSha := mr.DiffRefs.HeadSha
	if baseSha == "" || headSha == "" {
		if err != nil {
			return nil, utils.NewGitLabError(fmt.Sprintf("failed to get changes for MR !%d", mrIID), err)
		}
		return nil, fmt.Errorf("MR !%d has no diff refs", mrIID)
	}

	compare, _, cmpErr := c.api.Repositories.Compare(projectID, &gogitlab.CompareOptions{
		From: gogitlab.Ptr(baseSha),
		To:   gogitlab.Ptr(headSha),
	})
	if cmpErr != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to compare SHAs for MR !%d", mrIID), cmpErr)
	}

	return buildDiffFromCompare(compare, mr.TargetBranch, mr.SourceBranch), nil
}

func buildDiffFromChanges(changes *gogitlab.MergeRequest) *models.DiffResult {
	var diffParts []string
	var fileChanges []models.DiffFile
	totalAdds, totalDels := 0, 0

	for _, ch := range changes.Changes {
		header := fmt.Sprintf("--- a/%s\n+++ b/%s", ch.OldPath, ch.NewPath)
		diffParts = append(diffParts, header+"\n"+ch.Diff)

		adds, dels := countDiffLinesFromPatch(ch.Diff)
		totalAdds += adds
		totalDels += dels

		fileChanges = append(fileChanges, models.DiffFile{
			OldPath:   ch.OldPath,
			NewPath:   ch.NewPath,
			NewFile:   ch.NewFile,
			Renamed:   ch.RenamedFile,
			Deleted:   ch.DeletedFile,
			Additions: adds,
			Deletions: dels,
		})
	}

	return &models.DiffResult{
		From:           changes.TargetBranch,
		To:             changes.SourceBranch,
		DiffContent:    strings.Join(diffParts, "\n\n"),
		Files:          fileChanges,
		TotalAdditions: totalAdds,
		TotalDeletions: totalDels,
	}
}

func buildDiffFromCompare(compare *gogitlab.Compare, from, to string) *models.DiffResult {
	var diffParts []string
	var fileChanges []models.DiffFile
	totalAdds, totalDels := 0, 0

	for _, d := range compare.Diffs {
		header := fmt.Sprintf("--- a/%s\n+++ b/%s", d.OldPath, d.NewPath)
		diffParts = append(diffParts, header+"\n"+d.Diff)

		adds, dels := countDiffLinesFromPatch(d.Diff)
		totalAdds += adds
		totalDels += dels

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

	var commits []string
	for _, c := range compare.Commits {
		commits = append(commits, c.Title)
	}

	return &models.DiffResult{
		From:           from,
		To:             to,
		DiffContent:    strings.Join(diffParts, "\n\n"),
		Files:          fileChanges,
		Commits:        commits,
		TotalAdditions: totalAdds,
		TotalDeletions: totalDels,
	}
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
