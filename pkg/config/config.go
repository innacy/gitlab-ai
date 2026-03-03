package config

import (
	"fmt"

	"github.com/spf13/viper"

	"gitlab-ai/internal/models"
)

// AppConfig holds the complete application configuration.
type AppConfig struct {
	GitLab GitLabConfig `mapstructure:"gitlab" yaml:"gitlab"`
	AI     AIConfig     `mapstructure:"ai" yaml:"ai"`
	Review ReviewConfig `mapstructure:"review" yaml:"review"`
	Issues IssuesConfig `mapstructure:"issues" yaml:"issues"`
	CLI    CLIConfig    `mapstructure:"cli" yaml:"cli"`
}

// GitLabConfig holds GitLab connection settings.
type GitLabConfig struct {
	BaseURL        string `mapstructure:"base_url" yaml:"base_url"`
	APIVersion     string `mapstructure:"api_version" yaml:"api_version"`
	DefaultProject string `mapstructure:"default_project" yaml:"default_project"`
}

// AIConfig holds AI provider settings.
type AIConfig struct {
	Provider  string          `mapstructure:"provider" yaml:"provider"`
	Anthropic AnthropicConfig `mapstructure:"anthropic" yaml:"anthropic"`
}

// AnthropicConfig holds Anthropic-specific settings.
type AnthropicConfig struct {
	APIKey      string  `mapstructure:"api_key" yaml:"api_key"`           // direct key (takes priority)
	APIKeyEnv   string  `mapstructure:"api_key_env" yaml:"api_key_env"`   // env var name fallback
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

// CLIConfig holds CLI behavior settings.
type CLIConfig struct {
	ColorOutput       bool `mapstructure:"color_output" yaml:"color_output"`
	MarkdownRendering bool `mapstructure:"markdown_rendering" yaml:"markdown_rendering"`
	Verbose           bool `mapstructure:"verbose" yaml:"verbose"`
	ConfirmBeforePost bool `mapstructure:"confirm_before_post" yaml:"confirm_before_post"`
}

// Load reads configuration from file and environment.
func Load() (*AppConfig, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")          // project directory (primary)
	viper.AddConfigPath("./configs")  // configs subdirectory

	// Set defaults
	setDefaults()

	// Allow environment variable overrides
	viper.SetEnvPrefix("GITLAB_AI")
	viper.AutomaticEnv()

	// Read config file (it's OK if it doesn't exist)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	cfg := &AppConfig{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	return cfg, nil
}

