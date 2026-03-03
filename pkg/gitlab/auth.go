package gitlab

import (
	"fmt"
	"strings"

	gogitlab "github.com/xanzy/go-gitlab"

	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/utils"
)

// Authenticate creates a new authenticated GitLab client.
// It reads credentials from the ~/.netrc file for the configured GitLab host.
func Authenticate(cfg *config.GitLabConfig) (*gogitlab.Client, error) {
	token := getToken(cfg)
	if token == "" {
		return nil, utils.NewAuthError("no GitLab token found", nil)
	}

	baseURL := cfg.BaseURL
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	apiURL := baseURL + "api/" + cfg.APIVersion

	client, err := gogitlab.NewClient(token, gogitlab.WithBaseURL(apiURL))
	if err != nil {
		return nil, utils.NewAuthError("failed to create GitLab client", err)
	}

	// Verify authentication by fetching current user
	_, _, err = client.Users.CurrentUser()
	if err != nil {
		return nil, utils.NewAuthError("authentication failed — invalid token or unreachable GitLab server", err)
	}

	return client, nil
}

// getToken retrieves the GitLab API token from ~/.netrc.
func getToken(cfg *config.GitLabConfig) string {
	entry, err := utils.FindNetrcEntry(cfg.BaseURL)
	if err != nil {
		utils.Debugf("Failed to read .netrc for %s: %v", cfg.BaseURL, err)
		return ""
	}

	utils.Debugf("Using .netrc credentials for %s", cfg.BaseURL)
	return entry.Password
}

// GetCurrentUser fetches the current authenticated user info.
func GetCurrentUser(client *gogitlab.Client) (*gogitlab.User, error) {
	user, _, err := client.Users.CurrentUser()
	if err != nil {
		return nil, utils.NewGitLabError("failed to get current user", err)
	}
	return user, nil
}

// FindProject finds a project by its path (e.g. "company/mgmt").
func FindProject(client *gogitlab.Client, projectPath string) (*gogitlab.Project, error) {
	project, _, err := client.Projects.GetProject(projectPath, nil)
	if err != nil {
		return nil, utils.NewProjectNotFoundError(fmt.Sprintf("%s (API error: %v)", projectPath, err))
	}
	return project, nil
}
