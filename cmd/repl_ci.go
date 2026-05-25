package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"gitlab-ai/pkg/output"
)

// ─── Pipeline List ───────────────────────────────────────────────────────────

func (r *replState) handlePipeline(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	}
	if project == "" {
		project = r.promptForProject("Select project for pipelines")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching pipelines from '%s'...", project)
	s.Start()

	pipelines, err := r.provider.CI().ListPipelines(project, 15)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch pipelines: %v", err))
		return
	}

	if len(pipelines) == 0 {
		output.PrintSuccess("No pipelines found for this project.")
		return
	}

	output.PrintPipelinesTable(pipelines, fmt.Sprintf("Pipelines — %s", project))
	fmt.Println()
}

// ─── Pipeline View ───────────────────────────────────────────────────────────

func (r *replState) handlePipelineView(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var pipelineIDStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && pipelineIDStr == "" {
				pipelineIDStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else if len(remaining) > 0 {
		pipelineIDStr = remaining[0]
	}

	if project == "" {
		project = r.promptForProject("Select project for pipeline view")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	var pipelineID int
	if pipelineIDStr != "" {
		n, err := strconv.Atoi(pipelineIDStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid pipeline ID: %s", pipelineIDStr))
			return
		}
		pipelineID = n
	} else {
		pipelineID = r.promptForNumber("Pipeline ID")
		if pipelineID <= 0 {
			output.PrintError("Invalid pipeline ID.")
			return
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching pipeline #%d...", pipelineID)
	s.Start()

	pipeline, err := r.provider.CI().GetPipeline(project, pipelineID)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch pipeline: %v", err))
		return
	}

	output.PrintPipelineStatus(pipeline)
}

// ─── Pipeline Logs ───────────────────────────────────────────────────────────

func (r *replState) handlePipelineLogs(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var jobIDStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && jobIDStr == "" {
				jobIDStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else if len(remaining) > 0 {
		jobIDStr = remaining[0]
	}

	if project == "" {
		project = r.promptForProject("Select project for job logs")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	var jobID int
	if jobIDStr != "" {
		n, err := strconv.Atoi(jobIDStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid job ID: %s", jobIDStr))
			return
		}
		jobID = n
	} else {
		jobID = r.promptForNumber("Job ID")
		if jobID <= 0 {
			output.PrintError("Invalid job ID.")
			return
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching logs for job #%d...", jobID)
	s.Start()

	log, err := r.provider.CI().GetJobLog(project, jobID)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch job logs: %v", err))
		return
	}

	const maxLogDisplay = 5000
	if len(log) > maxLogDisplay {
		fmt.Printf("\n... (showing last %d chars of %d total) ...\n\n", maxLogDisplay, len(log))
		fmt.Println(log[len(log)-maxLogDisplay:])
	} else {
		fmt.Println()
		fmt.Println(log)
	}
	fmt.Println()
}

// ─── Pipeline Retry ──────────────────────────────────────────────────────────

func (r *replState) handlePipelineRetry(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var pipelineIDStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && pipelineIDStr == "" {
				pipelineIDStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else if len(remaining) > 0 {
		pipelineIDStr = remaining[0]
	}

	if project == "" {
		project = r.promptForProject("Select project for pipeline retry")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	var pipelineID int
	if pipelineIDStr != "" {
		n, err := strconv.Atoi(pipelineIDStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid pipeline ID: %s", pipelineIDStr))
			return
		}
		pipelineID = n
	} else {
		pipelineID = r.promptForNumber("Pipeline ID to retry")
		if pipelineID <= 0 {
			output.PrintError("Invalid pipeline ID.")
			return
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Retrying pipeline #%d...", pipelineID)
	s.Start()

	pl, err := r.provider.CI().RetryPipeline(project, pipelineID)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to retry pipeline: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Pipeline #%d retried — status: %s", pl.ID, pl.Status))
	if pl.WebURL != "" {
		output.PrintURL(pl.WebURL)
	}
	fmt.Println()
}

// ─── Pipeline Cancel ─────────────────────────────────────────────────────────

func (r *replState) handlePipelineCancel(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var pipelineIDStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && pipelineIDStr == "" {
				pipelineIDStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else if len(remaining) > 0 {
		pipelineIDStr = remaining[0]
	}

	if project == "" {
		project = r.promptForProject("Select project for pipeline cancel")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	var pipelineID int
	if pipelineIDStr != "" {
		n, err := strconv.Atoi(pipelineIDStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid pipeline ID: %s", pipelineIDStr))
			return
		}
		pipelineID = n
	} else {
		pipelineID = r.promptForNumber("Pipeline ID to cancel")
		if pipelineID <= 0 {
			output.PrintError("Invalid pipeline ID.")
			return
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Cancelling pipeline #%d...", pipelineID)
	s.Start()

	pl, err := r.provider.CI().CancelPipeline(project, pipelineID)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to cancel pipeline: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Pipeline #%d cancelled — status: %s", pl.ID, pl.Status))
	fmt.Println()
}
