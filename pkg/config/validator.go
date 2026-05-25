package config

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidationError holds a collection of validation errors.
type ValidationError struct {
	Errors []string
}

// Error implements the error interface.
func (v *ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed:\n  - %s", strings.Join(v.Errors, "\n  - "))
}

// HasErrors returns true if there are validation errors.
func (v *ValidationError) HasErrors() bool {
	return len(v.Errors) > 0
}

// Validate checks the configuration for errors.
func Validate(cfg *AppConfig) error {
	ve := &ValidationError{}

	// Validate GitLab config
	validateGitLab(&cfg.GitLab, ve)

	// Validate AI config
	validateAI(&cfg.AI, ve)

	// Validate Review config
	validateReview(&cfg.Review, ve)

	// Validate Issues config
	validateIssues(&cfg.Issues, ve)

	if ve.HasErrors() {
		return ve
	}
	return nil
}

func validateGitLab(cfg *GitLabConfig, ve *ValidationError) {
	if cfg.BaseURL == "" {
		ve.Errors = append(ve.Errors, "gitlab.base_url is required")
	} else {
		if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
			ve.Errors = append(ve.Errors, fmt.Sprintf("gitlab.base_url is not a valid URL: %s", cfg.BaseURL))
		}
	}

	if cfg.APIVersion != "" && cfg.APIVersion != "v4" {
		ve.Errors = append(ve.Errors, fmt.Sprintf("gitlab.api_version '%s' is not supported (use 'v4')", cfg.APIVersion))
	}
}

func validateAI(cfg *AIConfig, ve *ValidationError) {
	supported := map[string]bool{"anthropic": true, "claude": true, "gemini": true, "google": true, "nvidia": true, "": true}
	if !supported[strings.ToLower(cfg.Provider)] {
		ve.Errors = append(ve.Errors, fmt.Sprintf("ai.provider '%s' is not supported (use: anthropic, gemini, nvidia)", cfg.Provider))
	}
}

func validateReview(cfg *ReviewConfig, ve *ValidationError) {
	if len(cfg.Template.Sections) == 0 {
		ve.Errors = append(ve.Errors, "review.template.sections must have at least one section")
	}
	for i, section := range cfg.Template.Sections {
		if section.Name == "" {
			ve.Errors = append(ve.Errors, fmt.Sprintf("review.template.sections[%d].name is required", i))
		}
		if section.Prompt == "" {
			ve.Errors = append(ve.Errors, fmt.Sprintf("review.template.sections[%d].prompt is required", i))
		}
	}

	if cfg.Output.Directory == "" {
		ve.Errors = append(ve.Errors, "review.output.directory is required")
	}
	if cfg.Output.FilenamePattern == "" {
		ve.Errors = append(ve.Errors, "review.output.filename_pattern is required")
	}
}

func validateIssues(cfg *IssuesConfig, ve *ValidationError) {
	if cfg.Output.Directory == "" {
		ve.Errors = append(ve.Errors, "issues.output.directory is required")
	}
	if cfg.Output.FilenamePattern == "" {
		ve.Errors = append(ve.Errors, "issues.output.filename_pattern is required")
	}
}
