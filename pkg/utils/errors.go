package utils

import (
	"fmt"
)

// AppError represents a structured application error.
type AppError struct {
	Code       string
	Message    string
	Suggestion string
	Err        error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *AppError) Unwrap() error {
	return e.Err
}

// Error codes
const (
	ErrCodeAuth         = "AUTH_ERROR"
	ErrCodeGitLab       = "GITLAB_ERROR"
	ErrCodeMRNotFound   = "MR_NOT_FOUND"
	ErrCodeProjNotFound = "PROJECT_NOT_FOUND"
)

// NewAuthError creates an authentication error.
func NewAuthError(msg string, err error) *AppError {
	return &AppError{
		Code:       ErrCodeAuth,
		Message:    msg,
		Suggestion: "Check your .netrc file contains valid GitLab credentials.",
		Err:        err,
	}
}

// NewGitLabError creates a GitLab API error.
func NewGitLabError(msg string, err error) *AppError {
	return &AppError{
		Code:       ErrCodeGitLab,
		Message:    msg,
		Suggestion: "Check your GitLab connection and API token permissions.",
		Err:        err,
	}
}

// NewMRNotFoundError creates a merge request not found error.
func NewMRNotFoundError(project string, mrNumber int) *AppError {
	return &AppError{
		Code:    ErrCodeMRNotFound,
		Message: fmt.Sprintf("Merge request #%d not found in project '%s'", mrNumber, project),
		Err:     nil,
	}
}

// NewProjectNotFoundError creates a project not found error.
func NewProjectNotFoundError(project string) *AppError {
	return &AppError{
		Code:    ErrCodeProjNotFound,
		Message: fmt.Sprintf("Project '%s' not found", project),
		Err:     nil,
	}
}
