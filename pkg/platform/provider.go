package platform

import (
	"time"

	"gitlab-ai/internal/models"
)

type Provider interface {
	Name() string
	Username() string
	UserDisplayName() string
	UserID() int

	MRs() MRService
	Issues() IssueService
	Repos() RepoService
	CI() CIService
	Epics() EpicService
}

type MRService interface {
	GetMergeRequest(project string, mrIID int) (*models.MergeRequestInfo, error)
	ListProjectMRs(project, state string, limit int) ([]models.MRListItem, error)
	FindMRByBranches(project, source, target string) (*models.MRListItem, error)
	CreateMergeRequest(project, source, target, title, desc string) (*models.MRListItem, error)
	PostMRComment(project string, mrIID int, body string) (string, error)
	GetMRPipeline(project string, mrIID int) (*models.PipelineInfo, error)
	GetBranchDiff(project, from, to string) ([]string, error)
	GetBranchDiffFull(project, from, to string) (*models.DiffResult, error)

	MergeMR(project string, mrIID int, opts MergeOptions) error
	ApproveMR(project string, mrIID int) error
	UnapproveMR(project string, mrIID int) error
	RebaseMR(project string, mrIID int) error
	UpdateMR(project string, mrIID int, opts UpdateMROptions) (*models.MRListItem, error)
	CloseMR(project string, mrIID int) error
	ReopenMR(project string, mrIID int) error

	CountMRChanges(changes []models.MRChange) (additions, deletions int)

	// Code review
	PostInlineComment(project string, mrIID int, comment InlineComment) (string, error)
	ListMRDiscussions(project string, mrIID int) ([]Discussion, error)
	ResolveDiscussion(project string, mrIID int, discussionID string) error
	UnresolveDiscussion(project string, mrIID int, discussionID string) error
}

type InlineComment struct {
	FilePath    string
	NewLine     int
	OldLine     int
	Body        string
}

type Discussion struct {
	ID        string
	Resolved  bool
	Notes     []DiscussionNote
}

type DiscussionNote struct {
	ID        int
	Author    string
	Body      string
	CreatedAt string
	System    bool
}

type MergeOptions struct {
	Squash             bool
	RemoveSourceBranch bool
	MergeWhenReady     bool
}

type UpdateMROptions struct {
	Title       *string
	Description *string
	Assignees   []string
	Reviewers   []string
	Labels      []string
}

type IssueService interface {
	ListProjectIssues(project string, filter models.IssueFilter) (*models.IssueListResult, error)
	ListAssignedIssues(project string, filter models.IssueFilter) (*models.IssueListResult, error)
	CreateIssue(project, title, desc string, labels []string) (*models.Issue, error)
	ListProjectLabels(project string) ([]string, error)

	UpdateIssue(project string, issueIID int, opts UpdateIssueOptions) (*models.Issue, error)
	CloseIssue(project string, issueIID int) error
	ReopenIssue(project string, issueIID int) error
	SearchIssues(project, query string) ([]models.Issue, error)

	ListRelatedMergeRequests(project string, issueIID int) ([]models.MRListItem, error)
}

type UpdateIssueOptions struct {
	Title       *string
	Description *string
	Labels      []string
	Assignee    *string
	AssigneeID  *int
	Milestone   *string
}

type RepoService interface {
	ListProjects() ([]models.ProjectInfo, error)
	ListProjectsSince(since time.Time, maxPages int) ([]models.ProjectInfo, error)
	ListBranches(project string, limit int) ([]models.BranchInfo, error)
	ListActiveBranches(project string, limit int) ([]models.BranchInfo, error)
	ListMergedBranches(project string) ([]models.BranchInfo, error)
	DeleteBranch(project, branchName string) error
	GetBranch(project, branchName string) (*models.BranchInfo, error)
	ListTags(project string, limit int) ([]models.TagInfo, error)
	TagExists(project, tagName string) (bool, error)
	BranchExists(project, branchName string) (bool, error)
	GetRefDiff(project, from, to string) (*models.DiffResult, error)
	GetMRDiff(project string, mrIID int) (*models.DiffResult, error)
	GetMRDiffByProjectID(projectID, mrIID int) (*models.DiffResult, error)
	GetFileContent(project, filePath, ref string) (string, error)
	CheckProjectRelease(projectPath string) models.ProjectReleaseInfo
}

type CIService interface {
	ListPipelines(project string, limit int) ([]models.PipelineInfo, error)
	GetPipeline(project string, pipelineID int) (*models.PipelineInfo, error)
	GetJobLog(project string, jobID int) (string, error)
	RetryPipeline(project string, pipelineID int) (*models.PipelineInfo, error)
	CancelPipeline(project string, pipelineID int) (*models.PipelineInfo, error)
	RetryJob(project string, jobID int) error
}

type EpicService interface {
	CreateGroupEpic(groupPath, title, desc string) (*models.EpicResult, error)
}
