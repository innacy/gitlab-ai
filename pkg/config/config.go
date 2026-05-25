package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"

	"gitlab-ai/internal/models"
)

// AppConfig holds the complete application configuration.
type AppConfig struct {
	Platform        string       `mapstructure:"platform" yaml:"platform"`
	GitLab          GitLabConfig `mapstructure:"gitlab" yaml:"gitlab"`
	AI              AIConfig     `mapstructure:"ai" yaml:"ai"`
	Review          ReviewConfig `mapstructure:"review" yaml:"review"`
	Issues          IssuesConfig `mapstructure:"issues" yaml:"issues"`
	CLI             CLIConfig             `mapstructure:"cli" yaml:"cli"`
	Other           OtherOutputConfig     `mapstructure:"other" yaml:"other"`
	TicketContent   ContentTemplateConfig `mapstructure:"ticket_content" yaml:"ticket_content"`
	EpicContent     ContentTemplateConfig `mapstructure:"epic_content" yaml:"epic_content"`
	Teams           []string              `mapstructure:"teams" yaml:"teams"`
	Projects        []string              `mapstructure:"projects" yaml:"projects"`
	IgnoredProjects []string              `mapstructure:"ignored_projects" yaml:"ignored_projects"`
}

// GitLabConfig holds GitLab connection settings.
type GitLabConfig struct {
	BaseURL        string `mapstructure:"base_url" yaml:"base_url"`
	APIVersion     string `mapstructure:"api_version" yaml:"api_version"`
	DefaultProject string `mapstructure:"default_project" yaml:"default_project"`
	ParentFolder   string `mapstructure:"parent_folder" yaml:"parent_folder"`
}

// AIConfig holds AI provider settings.
type AIConfig struct {
	Provider       string          `mapstructure:"provider" yaml:"provider"`
	TimeoutSeconds int             `mapstructure:"timeout_seconds" yaml:"timeout_seconds"`
	Anthropic      AnthropicConfig `mapstructure:"anthropic" yaml:"anthropic"`
	Gemini         GeminiConfig    `mapstructure:"gemini" yaml:"gemini"`
	Nvidia         NvidiaConfig    `mapstructure:"nvidia" yaml:"nvidia"`
}

// NvidiaConfig holds NVIDIA NIM-specific settings.
type NvidiaConfig struct {
	APIKey    string `mapstructure:"api_key" yaml:"api_key"`
	APIKeyEnv string `mapstructure:"api_key_env" yaml:"api_key_env"`
	Model     string `mapstructure:"model" yaml:"model"`
	MaxTokens int    `mapstructure:"max_tokens" yaml:"max_tokens"`
}

// GeminiConfig holds Google Gemini-specific settings.
type GeminiConfig struct {
	APIKey    string `mapstructure:"api_key" yaml:"api_key"`
	APIKeyEnv string `mapstructure:"api_key_env" yaml:"api_key_env"`
	Model     string `mapstructure:"model" yaml:"model"`
	MaxTokens int    `mapstructure:"max_tokens" yaml:"max_tokens"`
}

// AnthropicConfig holds Anthropic-specific settings.
type AnthropicConfig struct {
	APIKey      string  `mapstructure:"api_key" yaml:"api_key"`
	APIKeyEnv   string  `mapstructure:"api_key_env" yaml:"api_key_env"`
	Model       string  `mapstructure:"model" yaml:"model"`
	MaxTokens   int     `mapstructure:"max_tokens" yaml:"max_tokens"`
	Temperature float64 `mapstructure:"temperature" yaml:"temperature"`
}

// ReviewConfig holds review-related settings.
type ReviewConfig struct {
	Template ReviewTemplateConfig `mapstructure:"template" yaml:"template"`
	Output   ReviewOutputConfig   `mapstructure:"output" yaml:"output"`
	Filters  ReviewFiltersConfig  `mapstructure:"filters" yaml:"filters"`
}

// ReviewTemplateConfig holds the review template sections.
type ReviewTemplateConfig struct {
	Sections []models.ReviewTemplateSection `mapstructure:"sections" yaml:"sections"`
}

// ReviewOutputConfig holds review output settings.
type ReviewOutputConfig struct {
	Directory       string `mapstructure:"directory" yaml:"directory"`
	FilenamePattern string `mapstructure:"filename_pattern" yaml:"filename_pattern"`
	IncludeMetadata bool   `mapstructure:"include_metadata" yaml:"include_metadata"`
	IncludeDiff     bool   `mapstructure:"include_diff" yaml:"include_diff"`
}

// ReviewFiltersConfig holds review filter settings.
type ReviewFiltersConfig struct {
	ExcludeFiles []string `mapstructure:"exclude_files" yaml:"exclude_files"`
}

// IssuesConfig holds issue-related settings.
type IssuesConfig struct {
	Output IssuesOutputConfig `mapstructure:"output" yaml:"output"`
	Fields []string           `mapstructure:"fields" yaml:"fields"`
}

// IssuesOutputConfig holds issue output settings.
type IssuesOutputConfig struct {
	Directory       string `mapstructure:"directory" yaml:"directory"`
	FilenamePattern string `mapstructure:"filename_pattern" yaml:"filename_pattern"`
}

// OtherOutputConfig holds output settings for non-review, non-issue markdown files.
type OtherOutputConfig struct {
	Directory string `mapstructure:"directory" yaml:"directory"`
}

// ContentTemplateConfig holds a markdown template for AI-generated content.
type ContentTemplateConfig struct {
	Template string `mapstructure:"template" yaml:"template"`
}

// CLIConfig holds CLI behavior settings.
type CLIConfig struct {
	ColorOutput        bool   `mapstructure:"color_output" yaml:"color_output"`
	MarkdownRendering  bool   `mapstructure:"markdown_rendering" yaml:"markdown_rendering"`
	Verbose            bool   `mapstructure:"verbose" yaml:"verbose"`
	ConfirmBeforePost  bool   `mapstructure:"confirm_before_post" yaml:"confirm_before_post"`
	IdleTimeoutMinutes int    `mapstructure:"idle_timeout_minutes" yaml:"idle_timeout_minutes"`
	OutputFormat       string `mapstructure:"output_format" yaml:"output_format"`
	Theme              string `mapstructure:"theme" yaml:"theme"`
}

// Load reads configuration from YAML file and environment variables.
func Load() (*AppConfig, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")

	setDefaults()

	viper.SetEnvPrefix("GITLAB_AI")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	cfg := &AppConfig{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	if err := cfg.EnsureOutputDirs(); err != nil {
		return nil, fmt.Errorf("failed to create output directories: %w", err)
	}

	return cfg, nil
}

// EnsureOutputDirs creates all configured output directories if they don't exist.
func (c *AppConfig) EnsureOutputDirs() error {
	dirs := []string{
		c.Review.Output.Directory,
		c.Issues.Output.Directory,
		c.Other.Directory,
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create directory %q: %w", dir, err)
		}
	}
	return nil
}
