package models

import "time"

// Issue represents a GitLab issue with relevant fields.
type Issue struct {
	ID          int       `json:"id"`
	IID         int       `json:"iid"`
	ProjectID   int       `json:"project_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	Labels      []string  `json:"labels"`
	Assignee    string    `json:"assignee"`
	Author      string    `json:"author"`
	Milestone   string    `json:"milestone"`
	DueDate     string    `json:"due_date"`
	WebURL      string    `json:"web_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    time.Time `json:"closed_at,omitempty"`
}

// IssueListResult holds the result of listing issues.
type IssueListResult struct {
	ProjectName string    `json:"project_name"`
	Issues      []Issue   `json:"issues"`
	TotalCount  int       `json:"total_count"`
	GeneratedAt time.Time `json:"generated_at"`
	FilePath    string    `json:"file_path,omitempty"`
}

// IssueFilter defines filters for listing issues.
type IssueFilter struct {
	State     string   `json:"state,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Milestone string   `json:"milestone,omitempty"`
	Page      int      `json:"page,omitempty"`
	PerPage   int      `json:"per_page,omitempty"`
}
