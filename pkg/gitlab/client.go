package gitlab

import (
	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/pkg/config"
)

// Client wraps the go-gitlab client with additional functionality.
type Client struct {
	api    *gogitlab.Client
	config *config.GitLabConfig
	user   *gogitlab.User
}

// NewClient creates a new GitLab client wrapper.
func NewClient(cfg *config.GitLabConfig) (*Client, error) {
	api, err := Authenticate(cfg)
	if err != nil {
		return nil, err
	}

	user, err := GetCurrentUser(api)
	if err != nil {
		return nil, err
	}

	return &Client{
		api:    api,
		config: cfg,
		user:   user,
	}, nil
}

// API returns the underlying go-gitlab client.
func (c *Client) API() *gogitlab.Client {
	return c.api
}

// User returns the authenticated user.
func (c *Client) User() *gogitlab.User {
	return c.user
}

// Config returns the GitLab configuration.
func (c *Client) Config() *config.GitLabConfig {
	return c.config
}
