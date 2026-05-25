package config

import "github.com/spf13/viper"

func setDefaults() {
	viper.SetDefault("platform", "gitlab")

	viper.SetDefault("gitlab.base_url", "https://gitlab.example.com/")
	viper.SetDefault("gitlab.api_version", "v4")
	viper.SetDefault("gitlab.parent_folder", "")

	viper.SetDefault("ai.provider", "anthropic")
	viper.SetDefault("ai.timeout_seconds", 180)
	viper.SetDefault("ai.anthropic.api_key_env", "ANTHROPIC_API_KEY")
	viper.SetDefault("ai.anthropic.model", "claude-sonnet-4-20250514")
	viper.SetDefault("ai.anthropic.max_tokens", 8192)
	viper.SetDefault("ai.anthropic.temperature", 0.7)
	viper.SetDefault("ai.gemini.api_key_env", "GEMINI_API_KEY")
	viper.SetDefault("ai.gemini.model", "gemini-2.5-flash")
	viper.SetDefault("ai.gemini.max_tokens", 8192)
	viper.SetDefault("ai.nvidia.api_key_env", "NVIDIA_API_KEY")
	viper.SetDefault("ai.nvidia.model", "meta/llama-3.1-70b-instruct")
	viper.SetDefault("ai.nvidia.max_tokens", 8192)

	viper.SetDefault("review.output.directory", "./reviews")
	viper.SetDefault("review.output.filename_pattern", "{project}_{mr_number}.md")
	viper.SetDefault("review.output.include_metadata", true)
	viper.SetDefault("review.output.include_diff", false)
	viper.SetDefault("review.filters.exclude_files", []string{"*.lock", "vendor/*", "*.generated.go"})

	viper.SetDefault("issues.output.directory", "./tickets")
	viper.SetDefault("issues.output.filename_pattern", "{project}_issues_{date}.md")

	viper.SetDefault("other.directory", "./zzz-Mds")

	viper.SetDefault("cli.color_output", true)
	viper.SetDefault("cli.markdown_rendering", true)
	viper.SetDefault("cli.verbose", false)
	viper.SetDefault("cli.confirm_before_post", true)
	viper.SetDefault("cli.idle_timeout_minutes", 60)
	viper.SetDefault("cli.output_format", "text")
	viper.SetDefault("cli.theme", "default")

	viper.SetDefault("ticket_content.template", `# Ticket: <TITLE>

## Summary

<Create a concise summary of the work, objective, and expected outcome.>

---

# Problem Statement

Describe:
- the current problem
- existing gaps
- operational pain points
- technical limitations
- missing capabilities
- why this work is needed

Example:
- lack of documentation
- scalability issues
- missing observability
- inconsistent implementation patterns
- AI tooling lacking repository context
- operational inefficiencies

---

# Objective

Clearly define the primary goals.

Example:
- improve maintainability
- improve AI-assisted development
- standardize implementation
- reduce operational complexity
- improve scalability
- improve observability
- reduce onboarding effort

---

# Scope

## 1. <Section Name>

Describe:
- what needs to be implemented
- expected behavior
- required standards
- integrations
- constraints

### Deliverables

- item 1
- item 2
- item 3

---

## 2. <Section Name>

Describe:
- workflows
- integrations
- operational behavior
- architecture expectations

### Deliverables

- item 1
- item 2
- item 3

---

## 3. <Section Name>

Describe:
- implementation expectations
- validations
- edge cases
- failure handling
- operational requirements

### Deliverables

- item 1
- item 2
- item 3

---

# Functional Requirements

- Requirement 1
- Requirement 2
- Requirement 3
- Requirement 4

---

# Deliverables

- Deliverable 1
- Deliverable 2
- Deliverable 3
- Deliverable 4

---

# Acceptance Criteria

| Criteria | Definition of Done |
|---|---|
| Example 1 | Clear measurable expectation |
| Example 2 | Validation expectation |
| Example 3 | Operational expectation |

---`)

	viper.SetDefault("epic_content.template", `## Background
- <Why this epic exists>
- <Current limitations or business/technical motivation>
- <Any prior decisions or incidents that led to this work>

## Goals / Objectives
- <Primary goal>
- <Secondary goal>
- <Success definition (measurable if possible)>

## Scope
### In Scope
- <Major feature / system change 1>
- <Major feature / system change 2>

### Out of Scope
- <Explicit exclusions to prevent scope creep>

### Sub-tasks / Deliverables
- [ ] <Task 1 – short description>
- [ ] <Task 2 – short description>
- [ ] <Task 3 – short description>

## Architecture / Technical Approach
- <High-level design>
- <Key components involved (services, pipelines, DB, etc.)>
- <Data flow changes>
- <Trade-offs and design decisions>

## Dependencies
- <Upstream/downstream services>
- <External systems>
- <Team dependencies>

## Risks & Mitigations
- <Risk 1> → <Mitigation>
- <Risk 2> → <Mitigation>

## Acceptance Criteria
- [ ] All sub-tasks completed
- [ ] End-to-end flow works as expected
- [ ] No regression in existing systems
- [ ] Performance and scalability targets met

## Impact & Timeline
- Teams / services affected:
- Breaking changes (if any):
- Phase 1:
- Phase 2:
- Phase 3:`)
}
