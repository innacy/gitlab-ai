package gitlab

import (
	"fmt"
	"time"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/utils"
)

// ListAssignedIssues fetches issues assigned to the current user.
func (c *Client) ListAssignedIssues(projectPath string, filter models.IssueFilter) (*models.IssueListResult, error) {
	project, err := FindProject(c.api, projectPath)
	if err != nil {
		return nil, err
	}

	opts := &gogitlab.ListProjectIssuesOptions{
		AssigneeID: gogitlab.AssigneeID(c.user.ID),
		ListOptions: gogitlab.ListOptions{
			PerPage: 50,
			Page:    1,
		},
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

	var allIssues []models.Issue
	for {
		issues, resp, err := c.api.Issues.ListProjectIssues(project.ID, opts)
		if err != nil {
			return nil, utils.NewGitLabError(fmt.Sprintf("failed to list issues for project '%s'", projectPath), err)
		}

		for _, issue := range issues {
			allIssues = append(allIssues, convertIssue(issue))
		}

		if resp.NextPage == 0 {
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
