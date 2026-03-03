package models

import "time"

// Review represents a complete AI-generated merge request review.
type Review struct {
	// Metadata
	ProjectName string    `json:"project_name"`
	MRNumber    int       `json:"mr_number"`
	MRTitle     string    `json:"mr_title"`
	MRURL       string    `json:"mr_url"`
	Author      string    `json:"author"`
	Reviewer    string    `json:"reviewer"`
	ReviewDate  time.Time `json:"review_date"`

	// MR Details
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	FilesChanged int    `json:"files_changed"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`

	// Review Content
	Sections []ReviewSection `json:"sections"`

	// Raw AI response
	RawResponse string `json:"raw_response,omitempty"`
}

// ReviewSection represents a single section of the review.
type ReviewSection struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// MergeRequestInfo holds the fetched MR data from GitLab.
type MergeRequestInfo struct {
	ID           int      `json:"id"`
	IID          int      `json:"iid"`
	ProjectID    int      `json:"project_id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	State        string   `json:"state"`
	Author       string   `json:"author"`
	AuthorUser   string   `json:"author_user"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	WebURL       string   `json:"web_url"`
	Labels       []string `json:"labels"`
	Changes      []MRChange `json:"changes"`
	DiffContent  string   `json:"diff_content"`
}

// MRChange represents a single file change in a merge request.
type MRChange struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

// ReviewTemplateSection defines a section in the review template configuration.
type ReviewTemplateSection struct {
	Name     string `yaml:"name" json:"name"`
	Prompt   string `yaml:"prompt" json:"prompt"`
	Required bool   `yaml:"required" json:"required"`
}
