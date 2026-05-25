package gitlab

import (
	"fmt"
	"strings"
	"time"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/utils"
)

// ListAssignedIssues fetches issues assigned to the current user.
func (c *Client) ListAssignedIssues(projectPath string, filter models.IssueFilter) (*models.IssueListResult, error) {
	return c.listProjectIssues(projectPath, filter, true)
}

// ListProjectIssues fetches all project issues using the provided filter.
func (c *Client) ListProjectIssues(projectPath string, filter models.IssueFilter) (*models.IssueListResult, error) {
	return c.listProjectIssues(projectPath, filter, false)
}

func (c *Client) listProjectIssues(projectPath string, filter models.IssueFilter, assignedOnly bool) (*models.IssueListResult, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	opts := &gogitlab.ListProjectIssuesOptions{
		ListOptions: gogitlab.ListOptions{
			PerPage: 50,
			Page:    1,
		},
	}
	if assignedOnly {
		opts.AssigneeID = gogitlab.AssigneeID(c.user.ID)
	}

	// Apply filters
	if filter.State != "" {
		opts.State = gogitlab.Ptr(filter.State)
	}
	if len(filter.Labels) > 0 {
		labels := gogitlab.LabelOptions(filter.Labels)
		opts.Labels = &labels
	}
	if filter.Milestone != "" {
		opts.Milestone = gogitlab.Ptr(filter.Milestone)
	}
	if filter.PerPage > 0 {
		opts.PerPage = filter.PerPage
	}
	if filter.Page > 0 {
		opts.Page = filter.Page
	}

	singlePage := filter.PerPage > 0

	var allIssues []models.Issue
	for {
		issues, resp, err := c.api.Issues.ListProjectIssues(project.ID, opts)
		if err != nil {
			return nil, utils.NewGitLabError(fmt.Sprintf("failed to list issues for project '%s'", projectPath), err)
		}

		for _, issue := range issues {
			allIssues = append(allIssues, convertIssue(issue))
		}

		if singlePage || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return &models.IssueListResult{
		ProjectName: projectPath,
		Issues:      allIssues,
		TotalCount:  len(allIssues),
		GeneratedAt: time.Now(),
	}, nil
}

// ListProjectLabels returns available labels for a project.
func (c *Client) ListProjectLabels(projectPath string) ([]string, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	opts := &gogitlab.ListLabelsOptions{
		ListOptions: gogitlab.ListOptions{
			PerPage: 100,
			Page:    1,
		},
	}

	labels := make([]string, 0, 100)
	for {
		items, resp, err := c.api.Labels.ListLabels(project.ID, opts)
		if err != nil {
			return nil, utils.NewGitLabError(fmt.Sprintf("failed to list labels for project '%s'", projectPath), err)
		}
		for _, label := range items {
			name := strings.TrimSpace(label.Name)
			if name != "" {
				labels = append(labels, name)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return labels, nil
}

// CreateIssue creates a new issue in the target project.
func (c *Client) CreateIssue(projectPath, title, description string, labels []string) (*models.Issue, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	opts := &gogitlab.CreateIssueOptions{
		Title:       gogitlab.Ptr(title),
		Description: gogitlab.Ptr(description),
	}
	if len(labels) > 0 {
		labelOptions := gogitlab.LabelOptions(labels)
		opts.Labels = &labelOptions
	}

	issue, _, err := c.api.Issues.CreateIssue(project.ID, opts)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to create issue for project '%s'", projectPath), err)
	}

	converted := convertIssue(issue)
	return &converted, nil
}

// convertIssue converts a go-gitlab Issue to our models.Issue.
func convertIssue(issue *gogitlab.Issue) models.Issue {
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
		dueDate = time.Time(*issue.DueDate).Format("2006-01-02")
	}

	var closedAt time.Time
	if issue.ClosedAt != nil {
		closedAt = *issue.ClosedAt
	}

	return models.Issue{
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
		CreatedAt:   *issue.CreatedAt,
		UpdatedAt:   *issue.UpdatedAt,
		ClosedAt:    closedAt,
	}
}
