package gitlab

import (
	"fmt"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/pkg/utils"
)

// EpicResult holds the essential fields after creating a group epic.
type EpicResult struct {
	IID     int
	Title   string
	WebURL  string
	GroupID int
}

// CreateGroupEpic creates a new epic under the specified group.
func (c *Client) CreateGroupEpic(groupPath, title, description string) (*EpicResult, error) {
	opts := &gogitlab.CreateEpicOptions{
		Title:       gogitlab.Ptr(title),
		Description: gogitlab.Ptr(description),
	}

	epic, _, err := c.api.Epics.CreateEpic(groupPath, opts)
	if err != nil {
		return nil, utils.NewGitLabError(fmt.Sprintf("failed to create epic in group '%s'", groupPath), err)
	}

	return &EpicResult{
		IID:     epic.IID,
		Title:   epic.Title,
		WebURL:  epic.WebURL,
		GroupID: epic.GroupID,
	}, nil
}
