package config

import "github.com/spf13/viper"

// setDefaults sets all Viper default values.
func setDefaults() {
	// GitLab defaults
	viper.SetDefault("gitlab.base_url", "https://gitlab.widas.de/")
	viper.SetDefault("gitlab.api_version", "v4")
	viper.SetDefault("gitlab.default_project", "")

	// AI defaults
	viper.SetDefault("ai.provider", "anthropic")
	viper.SetDefault("ai.anthropic.api_key", "")
	viper.SetDefault("ai.anthropic.api_key_env", "ANTHROPIC_API_KEY")
	viper.SetDefault("ai.anthropic.model", "claude-sonnet-4-20250514")
	viper.SetDefault("ai.anthropic.max_tokens", 8192)
	viper.SetDefault("ai.anthropic.temperature", 0.7)

	// Review defaults
	viper.SetDefault("review.output.directory", "./reviews")
	viper.SetDefault("review.output.filename_pattern", "{project}_{mr_number}.md")
	viper.SetDefault("review.output.include_metadata", true)
	viper.SetDefault("review.output.include_diff", false)

	// Issue defaults
	viper.SetDefault("issues.output.directory", "./issues")
	viper.SetDefault("issues.output.filename_pattern", "{project}_issues_{date}.md")
	viper.SetDefault("issues.fields", []string{
		"id", "title", "description", "state", "labels",
		"assignee", "created_at", "updated_at", "due_date",
		"milestone", "web_url",
	})

	// CLI defaults
	viper.SetDefault("cli.color_output", true)
	viper.SetDefault("cli.markdown_rendering", true)
	viper.SetDefault("cli.verbose", false)
	viper.SetDefault("cli.confirm_before_post", true)
}
