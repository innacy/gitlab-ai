package config

import "github.com/spf13/viper"

func setDefaults() {
	viper.SetDefault("gitlab.base_url", "https://gitlab.example.com/")
	viper.SetDefault("gitlab.api_version", "v4")

	viper.SetDefault("ai.provider", "anthropic")
	viper.SetDefault("ai.anthropic.api_key_env", "ANTHROPIC_API_KEY")
	viper.SetDefault("ai.anthropic.model", "claude-sonnet-4-20250514")
	viper.SetDefault("ai.anthropic.max_tokens", 8192)
	viper.SetDefault("ai.anthropic.temperature", 0.7)
	viper.SetDefault("ai.gemini.api_key_env", "GEMINI_API_KEY")
	viper.SetDefault("ai.gemini.model", "gemini-2.5-flash")
	viper.SetDefault("ai.gemini.max_tokens", 8192)

	viper.SetDefault("review.output.directory", "./reviews")
	viper.SetDefault("review.output.filename_pattern", "{project}_{mr_number}.md")
	viper.SetDefault("review.output.include_metadata", true)
	viper.SetDefault("review.output.include_diff", false)

	viper.SetDefault("issues.output.directory", "./tickets")
	viper.SetDefault("issues.output.filename_pattern", "{project}_issues_{date}.md")

	viper.SetDefault("other.directory", "./zzz-Mds")

	viper.SetDefault("cli.color_output", true)
	viper.SetDefault("cli.markdown_rendering", true)
	viper.SetDefault("cli.verbose", false)
	viper.SetDefault("cli.confirm_before_post", true)

	viper.SetDefault("ticket_content.template", `## Title
One-line summary of the change

## Summary
Brief description (2-3 sentences max)

## Scope
- Bullet list of changed components

## Acceptance Criteria
- [ ] Criteria 1
- [ ] Criteria 2`)

	viper.SetDefault("epic_content.template", `## Title
Descriptive title of the epic

## Background
Detailed context and motivation for the changes

## Scope
### Sub-tasks
- [ ] Task 1
- [ ] Task 2

## Technical Details
Architecture changes, dependencies, migrations, implementation notes

## Acceptance Criteria
- [ ] Criteria 1
- [ ] Criteria 2

## Impact
Teams affected, breaking changes, rollback plan`)
}
