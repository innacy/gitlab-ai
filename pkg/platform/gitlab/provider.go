package gitlab

import (
	"fmt"
	"strings"
	"time"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/config"
	rawgitlab "gitlab-ai/pkg/gitlab"
	"gitlab-ai/pkg/platform"
	"gitlab-ai/pkg/utils"
)

func init() {
	platform.Register("gitlab", func(cfg *config.AppConfig) (platform.Provider, error) {
		return New(cfg)
	})
}

type Provider struct {
	client *rawgitlab.Client
	cfg    *config.AppConfig
}

func New(cfg *config.AppConfig) (*Provider, error) {
	client, err := rawgitlab.NewClient(&cfg.GitLab)
	if err != nil {
		return nil, err
	}
	return &Provider{client: client, cfg: cfg}, nil
}

func (p *Provider) Name() string            { return "gitlab" }
func (p *Provider) Username() string         { return p.client.User().Username }
func (p *Provider) UserDisplayName() string  { return p.client.User().Name }
func (p *Provider) UserID() int              { return p.client.User().ID }
func (p *Provider) RawClient() *rawgitlab.Client { return p.client }

func (p *Provider) MRs() platform.MRService       { return &mrService{p} }
func (p *Provider) Issues() platform.IssueService  { return &issueService{p} }
func (p *Provider) Repos() platform.RepoService    { return &repoService{p} }
func (p *Provider) CI() platform.CIService         { return &ciService{p} }
func (p *Provider) Epics() platform.EpicService     { return &epicService{p} }

// ─── MR Service ──────────────────────────────────────────────────────────────

type mrService struct{ p *Provider }

func (s *mrService) GetMergeRequest(project string, mrIID int) (*models.MergeRequestInfo, error) {
	return s.p.client.GetMergeRequest(project, mrIID)
}

func (s *mrService) ListProjectMRs(project, state string, limit int) ([]models.MRListItem, error) {
	return s.p.client.ListProjectMRs(project, state, limit)
}

func (s *mrService) FindMRByBranches(project, source, target string) (*models.MRListItem, error) {
	return s.p.client.FindMRByBranches(project, source, target)
}

func (s *mrService) CreateMergeRequest(project, source, target, title, desc string) (*models.MRListItem, error) {
	return s.p.client.CreateMergeRequest(project, source, target, title, desc)
}

func (s *mrService) PostMRComment(project string, mrIID int, body string) (string, error) {
	return s.p.client.PostMRComment(project, mrIID, body)
}

func (s *mrService) GetMRPipeline(project string, mrIID int) (*models.PipelineInfo, error) {
	return s.p.client.GetMRPipeline(project, mrIID)
}

func (s *mrService) GetBranchDiff(project, from, to string) ([]string, error) {
	return s.p.client.GetBranchDiff(project, from, to)
}

func (s *mrService) GetBranchDiffFull(project, from, to string) (*models.DiffResult, error) {
	return s.p.client.GetBranchDiffFull(project, from, to)
}

func (s *mrService) CountMRChanges(changes []models.MRChange) (int, int) {
	return rawgitlab.CountMRChanges(changes)
}

func (s *mrService) MergeMR(project string, mrIID int, opts platform.MergeOptions) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	mergeOpts := &gogitlab.AcceptMergeRequestOptions{
		Squash:                 gogitlab.Ptr(opts.Squash),
		ShouldRemoveSourceBranch: gogitlab.Ptr(opts.RemoveSourceBranch),
		MergeWhenPipelineSucceeds: gogitlab.Ptr(opts.MergeWhenReady),
	}
	_, _, err = api.MergeRequests.AcceptMergeRequest(proj.ID, mrIID, mergeOpts)
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to merge MR #%d", mrIID), err)
	}
	return nil
}

func (s *mrService) ApproveMR(project string, mrIID int) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, _, err = api.MergeRequestApprovals.ApproveMergeRequest(proj.ID, mrIID, nil)
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to approve MR #%d", mrIID), err)
	}
	return nil
}

func (s *mrService) UnapproveMR(project string, mrIID int) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, err = api.MergeRequestApprovals.UnapproveMergeRequest(proj.ID, mrIID)
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to unapprove MR #%d", mrIID), err)
	}
	return nil
}

func (s *mrService) RebaseMR(project string, mrIID int) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, err = api.MergeRequests.RebaseMergeRequest(proj.ID, mrIID)
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to rebase MR #%d", mrIID), err)
	}
	return nil
}

func (s *mrService) UpdateMR(project string, mrIID int, opts platform.UpdateMROptions) (*models.MRListItem, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	updateOpts := &gogitlab.UpdateMergeRequestOptions{}
	if opts.Title != nil {
		updateOpts.Title = opts.Title
	}
	if opts.Description != nil {
		updateOpts.Description = opts.Description
	}
	if opts.Labels != nil {
		labels := gogitlab.LabelOptions(opts.Labels)
		updateOpts.Labels = &labels
	}
	mr, _, err := api.MergeRequests.UpdateMergeRequest(proj.ID, mrIID, updateOpts)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to update MR #%d", mrIID), err)
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

func (s *mrService) CloseMR(project string, mrIID int) error {
	_, err := s.UpdateMRState(project, mrIID, "close")
	return err
}

func (s *mrService) ReopenMR(project string, mrIID int) error {
	_, err := s.UpdateMRState(project, mrIID, "reopen")
	return err
}

func (s *mrService) UpdateMRState(project string, mrIID int, action string) (*models.MRListItem, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	stateEvent := action
	mr, _, err := api.MergeRequests.UpdateMergeRequest(proj.ID, mrIID, &gogitlab.UpdateMergeRequestOptions{
		StateEvent: gogitlab.Ptr(stateEvent),
	})
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to %s MR #%d", action, mrIID), err)
	}
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	return &models.MRListItem{
		IID:          mr.IID,
		ProjectID:    mr.ProjectID,
		Title:        mr.Title,
		State:        mr.State,
		Author:       author,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		WebURL:       mr.WebURL,
	}, nil
}

func (s *mrService) PostInlineComment(project string, mrIID int, comment platform.InlineComment) (string, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return "", err
	}
	position := &gogitlab.PositionOptions{
		PositionType: gogitlab.Ptr("text"),
		NewPath:      gogitlab.Ptr(comment.FilePath),
		OldPath:      gogitlab.Ptr(comment.FilePath),
	}
	if comment.NewLine > 0 {
		position.NewLine = gogitlab.Ptr(comment.NewLine)
	}
	if comment.OldLine > 0 {
		position.OldLine = gogitlab.Ptr(comment.OldLine)
	}

	disc, _, err := api.Discussions.CreateMergeRequestDiscussion(proj.ID, mrIID, &gogitlab.CreateMergeRequestDiscussionOptions{
		Body:     gogitlab.Ptr(comment.Body),
		Position: position,
	})
	if err != nil {
		return "", utils.NewGitLabError(fmt.Sprintf("failed to post inline comment on MR #%d", mrIID), err)
	}
	return disc.ID, nil
}

func (s *mrService) ListMRDiscussions(project string, mrIID int) ([]platform.Discussion, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	discussions, _, err := api.Discussions.ListMergeRequestDiscussions(proj.ID, mrIID, &gogitlab.ListMergeRequestDiscussionsOptions{
		PerPage: 50,
	})
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to list discussions for MR #%d", mrIID), err)
	}
	result := make([]platform.Discussion, 0, len(discussions))
	for _, d := range discussions {
		disc := platform.Discussion{
			ID:       d.ID,
		}
		for _, n := range d.Notes {
			resolved := false
			if n.Resolved {
				resolved = true
			}
			disc.Resolved = resolved
			author := ""
			if n.Author.Username != "" {
				author = n.Author.Username
			}
			createdAt := ""
			if n.CreatedAt != nil {
				createdAt = n.CreatedAt.Format("2006-01-02 15:04")
			}
			disc.Notes = append(disc.Notes, platform.DiscussionNote{
				ID:        n.ID,
				Author:    author,
				Body:      n.Body,
				CreatedAt: createdAt,
				System:    n.System,
			})
		}
		result = append(result, disc)
	}
	return result, nil
}

func (s *mrService) ResolveDiscussion(project string, mrIID int, discussionID string) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, _, err = api.Discussions.ResolveMergeRequestDiscussion(proj.ID, mrIID, discussionID, &gogitlab.ResolveMergeRequestDiscussionOptions{
		Resolved: gogitlab.Ptr(true),
	})
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to resolve discussion %s", discussionID), err)
	}
	return nil
}

func (s *mrService) UnresolveDiscussion(project string, mrIID int, discussionID string) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, _, err = api.Discussions.ResolveMergeRequestDiscussion(proj.ID, mrIID, discussionID, &gogitlab.ResolveMergeRequestDiscussionOptions{
		Resolved: gogitlab.Ptr(false),
	})
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to unresolve discussion %s", discussionID), err)
	}
	return nil
}

// ─── Issue Service ───────────────────────────────────────────────────────────

type issueService struct{ p *Provider }

func (s *issueService) ListProjectIssues(project string, filter models.IssueFilter) (*models.IssueListResult, error) {
	return s.p.client.ListProjectIssues(project, filter)
}

func (s *issueService) ListAssignedIssues(project string, filter models.IssueFilter) (*models.IssueListResult, error) {
	return s.p.client.ListAssignedIssues(project, filter)
}

func (s *issueService) CreateIssue(project, title, desc string, labels []string) (*models.Issue, error) {
	return s.p.client.CreateIssue(project, title, desc, labels)
}

func (s *issueService) ListProjectLabels(project string) ([]string, error) {
	return s.p.client.ListProjectLabels(project)
}

func (s *issueService) UpdateIssue(project string, issueIID int, opts platform.UpdateIssueOptions) (*models.Issue, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	updateOpts := &gogitlab.UpdateIssueOptions{}
	if opts.Title != nil {
		updateOpts.Title = opts.Title
	}
	if opts.Description != nil {
		updateOpts.Description = opts.Description
	}
	if opts.Labels != nil {
		labels := gogitlab.LabelOptions(opts.Labels)
		updateOpts.Labels = &labels
	}
	if opts.AssigneeID != nil {
		updateOpts.AssigneeIDs = &[]int{*opts.AssigneeID}
	}
	issue, _, err := api.Issues.UpdateIssue(proj.ID, issueIID, updateOpts)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to update issue #%d", issueIID), err)
	}
	converted := convertGitLabIssue(issue)
	return &converted, nil
}

func (s *issueService) CloseIssue(project string, issueIID int) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, _, err = api.Issues.UpdateIssue(proj.ID, issueIID, &gogitlab.UpdateIssueOptions{
		StateEvent: gogitlab.Ptr("close"),
	})
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to close issue #%d", issueIID), err)
	}
	return nil
}

func (s *issueService) ReopenIssue(project string, issueIID int) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, _, err = api.Issues.UpdateIssue(proj.ID, issueIID, &gogitlab.UpdateIssueOptions{
		StateEvent: gogitlab.Ptr("reopen"),
	})
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to reopen issue #%d", issueIID), err)
	}
	return nil
}

func (s *issueService) SearchIssues(project, query string) ([]models.Issue, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	issues, _, err := api.Issues.ListProjectIssues(proj.ID, &gogitlab.ListProjectIssuesOptions{
		Search: gogitlab.Ptr(query),
		ListOptions: gogitlab.ListOptions{PerPage: 20},
	})
	if err != nil {
		return nil, utils.NewGitLabError("failed to search issues", err)
	}
	result := make([]models.Issue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, convertGitLabIssue(issue))
	}
	return result, nil
}

func (s *issueService) ListRelatedMergeRequests(project string, issueIID int) ([]models.MRListItem, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}

	// Fetch MRs that close this issue (linked via "Closes #N" or the UI)
	closing, _, err := api.Issues.ListMergeRequestsClosingIssue(proj.ID, issueIID, nil)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to get MRs for issue #%d", issueIID), err)
	}

	// Also fetch related MRs (manually linked via the UI)
	related, _, err := api.Issues.ListMergeRequestsRelatedToIssue(proj.ID, issueIID, nil)
	if err != nil {
		related = nil
	}

	type mrKey struct{ project, iid int }
	seen := make(map[mrKey]bool)
	var result []models.MRListItem
	addMR := func(mr *gogitlab.MergeRequest) {
		k := mrKey{mr.ProjectID, mr.IID}
		if seen[k] {
			return
		}
		seen[k] = true
		author := ""
		if mr.Author != nil {
			author = mr.Author.Username
		}
		updatedAt := time.Time{}
		if mr.UpdatedAt != nil {
			updatedAt = *mr.UpdatedAt
		}
		result = append(result, models.MRListItem{
			IID:          mr.IID,
			ProjectID:    mr.ProjectID,
			Title:        mr.Title,
			State:        mr.State,
			Author:       author,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			WebURL:       mr.WebURL,
			UpdatedAt:    updatedAt,
		})
	}

	for _, mr := range closing {
		addMR(mr)
	}
	for _, mr := range related {
		addMR(mr)
	}
	return result, nil
}

func convertGitLabIssue(issue *gogitlab.Issue) models.Issue {
	assignee := ""
	if issue.Assignee != nil {
		assignee = issue.Assignee.Username
	}
	author := ""
	if issue.Author != nil {
		author = issue.Author.Username
	}
	milestone := ""
	if issue.Milestone != nil {
		milestone = issue.Milestone.Title
	}
	dueDate := ""
	if issue.DueDate != nil {
		dueDate = issue.DueDate.String()
	}
	i := models.Issue{
		ID:          issue.ID,
		IID:         issue.IID,
		ProjectID:   issue.ProjectID,
		Title:       issue.Title,
		Description: issue.Description,
		State:       issue.State,
		Labels:      issue.Labels,
		Assignee:    assignee,
		Author:      author,
		Milestone:   milestone,
		DueDate:     dueDate,
		WebURL:      issue.WebURL,
	}
	if issue.CreatedAt != nil {
		i.CreatedAt = *issue.CreatedAt
	}
	if issue.UpdatedAt != nil {
		i.UpdatedAt = *issue.UpdatedAt
	}
	if issue.ClosedAt != nil {
		i.ClosedAt = *issue.ClosedAt
	}
	return i
}

// ─── Repo Service ────────────────────────────────────────────────────────────

type repoService struct{ p *Provider }

func (s *repoService) ListProjects() ([]models.ProjectInfo, error) {
	return s.p.client.ListProjects()
}

func (s *repoService) ListProjectsSince(since time.Time, maxPages int) ([]models.ProjectInfo, error) {
	return s.p.client.ListProjectsSince(since, maxPages)
}

func (s *repoService) ListBranches(project string, limit int) ([]models.BranchInfo, error) {
	return s.p.client.ListBranches(project, limit)
}

func (s *repoService) ListActiveBranches(project string, limit int) ([]models.BranchInfo, error) {
	return s.p.client.ListActiveBranches(project, limit)
}

func (s *repoService) ListMergedBranches(project string) ([]models.BranchInfo, error) {
	return s.p.client.ListMergedBranches(project)
}

func (s *repoService) DeleteBranch(project, branchName string) error {
	return s.p.client.DeleteBranch(project, branchName)
}

func (s *repoService) GetBranch(project, branchName string) (*models.BranchInfo, error) {
	return s.p.client.GetBranch(project, branchName)
}

func (s *repoService) ListTags(project string, limit int) ([]models.TagInfo, error) {
	return s.p.client.ListTags(project, limit)
}

func (s *repoService) TagExists(project, tagName string) (bool, error) {
	return s.p.client.TagExists(project, tagName)
}

func (s *repoService) BranchExists(project, branchName string) (bool, error) {
	return s.p.client.BranchExists(project, branchName)
}

func (s *repoService) GetRefDiff(project, from, to string) (*models.DiffResult, error) {
	return s.p.client.GetRefDiff(project, from, to)
}

func (s *repoService) GetMRDiff(project string, mrIID int) (*models.DiffResult, error) {
	return s.p.client.GetMRDiff(project, mrIID)
}

func (s *repoService) GetMRDiffByProjectID(projectID, mrIID int) (*models.DiffResult, error) {
	return s.p.client.GetMRDiffByProjectID(projectID, mrIID)
}

func (s *repoService) GetFileContent(project, filePath, ref string) (string, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return "", err
	}

	opts := &gogitlab.GetRawFileOptions{Ref: gogitlab.Ptr(ref)}
	raw, _, err := api.RepositoryFiles.GetRawFile(proj.ID, filePath, opts)
	if err != nil {
		return "", fmt.Errorf("failed to get file %s@%s: %w", filePath, ref, err)
	}
	return string(raw), nil
}

func (s *repoService) CheckProjectRelease(projectPath string) models.ProjectReleaseInfo {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, projectPath)
	if err != nil {
		return models.ProjectReleaseInfo{
			Name:          extractProjectName(projectPath),
			Path:          projectPath,
			Status:        models.ReleaseInvalid,
			InvalidReason: fmt.Sprintf("Project not found: %v", err),
		}
	}
	return s.p.client.CheckProjectRelease(proj.ID, projectPath)
}

func extractProjectName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// ─── CI Service ──────────────────────────────────────────────────────────────

type ciService struct{ p *Provider }

func (s *ciService) ListPipelines(project string, limit int) ([]models.PipelineInfo, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	pipelines, _, err := api.Pipelines.ListProjectPipelines(proj.ID, &gogitlab.ListProjectPipelinesOptions{
		ListOptions: gogitlab.ListOptions{PerPage: limit},
		OrderBy:     gogitlab.Ptr("id"),
		Sort:        gogitlab.Ptr("desc"),
	})
	if err != nil {
		return nil, utils.NewGitLabError("failed to list pipelines", err)
	}
	result := make([]models.PipelineInfo, 0, len(pipelines))
	for _, pl := range pipelines {
		result = append(result, models.PipelineInfo{
			ID:     pl.ID,
			Status: pl.Status,
			Ref:    pl.Ref,
			WebURL: pl.WebURL,
		})
	}
	return result, nil
}

func (s *ciService) GetPipeline(project string, pipelineID int) (*models.PipelineInfo, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	pl, _, err := api.Pipelines.GetPipeline(proj.ID, pipelineID)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to get pipeline #%d", pipelineID), err)
	}
	info := &models.PipelineInfo{
		ID:     pl.ID,
		Status: pl.Status,
		Ref:    pl.Ref,
		WebURL: pl.WebURL,
	}
	jobs, _, err := api.Jobs.ListPipelineJobs(proj.ID, pipelineID, nil)
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

func (s *ciService) GetJobLog(project string, jobID int) (string, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return "", err
	}
	reader, _, err := api.Jobs.GetTraceFile(proj.ID, jobID)
	if err != nil {
		return "", utils.NewGitLabError(fmt.Sprintf("failed to get log for job #%d", jobID), err)
	}
	var buf strings.Builder
	p := make([]byte, 4096)
	for {
		n, readErr := reader.Read(p)
		if n > 0 {
			buf.Write(p[:n])
		}
		if readErr != nil {
			break
		}
	}
	return buf.String(), nil
}

func (s *ciService) RetryPipeline(project string, pipelineID int) (*models.PipelineInfo, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	pl, _, err := api.Pipelines.RetryPipelineBuild(proj.ID, pipelineID)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to retry pipeline #%d", pipelineID), err)
	}
	return &models.PipelineInfo{
		ID:     pl.ID,
		Status: pl.Status,
		Ref:    pl.Ref,
		WebURL: pl.WebURL,
	}, nil
}

func (s *ciService) CancelPipeline(project string, pipelineID int) (*models.PipelineInfo, error) {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return nil, err
	}
	pl, _, err := api.Pipelines.CancelPipelineBuild(proj.ID, pipelineID)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to cancel pipeline #%d", pipelineID), err)
	}
	return &models.PipelineInfo{
		ID:     pl.ID,
		Status: pl.Status,
		Ref:    pl.Ref,
		WebURL: pl.WebURL,
	}, nil
}

func (s *ciService) RetryJob(project string, jobID int) error {
	api := s.p.client.API()
	proj, err := rawgitlab.FindProject(api, project)
	if err != nil {
		return err
	}
	_, _, err = api.Jobs.RetryJob(proj.ID, jobID)
	if err != nil {
		return utils.NewGitLabError(fmt.Sprintf("failed to retry job #%d", jobID), err)
	}
	return nil
}

// ─── Epic Service ────────────────────────────────────────────────────────────

type epicService struct{ p *Provider }

func (s *epicService) CreateGroupEpic(groupPath, title, desc string) (*models.EpicResult, error) {
	api := s.p.client.API()
	epic, _, err := api.Epics.CreateEpic(groupPath, &gogitlab.CreateEpicOptions{
		Title:       gogitlab.Ptr(title),
		Description: gogitlab.Ptr(desc),
	})
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to create epic in group '%s'", groupPath), err)
	}
	return &models.EpicResult{
		IID:     epic.IID,
		Title:   epic.Title,
		WebURL:  epic.WebURL,
		GroupID: epic.GroupID,
	}, nil
}
