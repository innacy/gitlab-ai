package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"gitlab-ai/internal/models"
	projectctx "gitlab-ai/pkg/context"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

// ─── MR Review ───────────────────────────────────────────────────────────────

func (r *replState) handleMRReview(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var mrNumberStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && mrNumberStr == "" {
				mrNumberStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else {
		if len(remaining) > 0 {
			mrNumberStr = remaining[0]
		}
	}

	if project == "" {
		project = r.promptForProject("Select project for MR review")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}

	project = r.resolveProject(project)

	var mrNumber int
	if mrNumberStr != "" {
		n, err := strconv.Atoi(mrNumberStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid MR number: %s", mrNumberStr))
			return
		}
		mrNumber = n
	} else {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = fmt.Sprintf(" Fetching open MRs from '%s'...", project)
		s.Start()

		openMRs, err := r.provider.MRs().ListProjectMRs(project, "opened", 5)
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to fetch MRs: %v", err))
			return
		}

		if len(openMRs) == 0 {
			output.PrintWarning("No open MRs found for this project.")
			return
		}

		options := make([]string, len(openMRs))
		for i, mr := range openMRs {
			options[i] = fmt.Sprintf("!%d — %s (@%s, %s)", mr.IID, mr.Title, mr.Author, output.TimeAgo(mr.UpdatedAt))
		}

		choice := r.promptForChoice("Select MR to review", options)
		if choice < 0 {
			return
		}
		mrNumber = openMRs[choice].IID
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching MR #%d from '%s'...", mrNumber, project)
	s.Start()

	mrInfo, err := r.provider.MRs().GetMergeRequest(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch MR: %v", err))
		return
	}

	output.PrintMRInfo(mrInfo)

	projContext, _ := projectctx.LoadContextTruncated(project, 60000)
	if projContext != "" {
		output.PrintSuccess("Project context loaded for AI review")
	}

	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	suffix := fmt.Sprintf(" Generating AI review via %s", r.aiClient.ProviderName())
	if projContext != "" {
		suffix += " (with project context)"
	}
	s.Suffix = suffix + "..."
	s.Start()

	reviewText, err := r.reviewWithAI(mrInfo, projContext)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("AI review failed: %v", err))
		return
	}

	output.PrintSuccess("AI review generated")

	sections := parseReviewSections(reviewText)
	additions, deletions := r.provider.MRs().CountMRChanges(mrInfo.Changes)

	review := &models.Review{
		ProjectName:  project,
		MRNumber:     mrInfo.IID,
		MRTitle:      mrInfo.Title,
		MRURL:        mrInfo.WebURL,
		Author:       fmt.Sprintf("%s (@%s)", mrInfo.Author, mrInfo.AuthorUser),
		Reviewer:     r.username,
		ReviewDate:   time.Now().UTC(),
		Description:  mrInfo.Description,
		SourceBranch: mrInfo.SourceBranch,
		TargetBranch: mrInfo.TargetBranch,
		FilesChanged: len(mrInfo.Changes),
		Additions:    additions,
		Deletions:    deletions,
		Sections:     sections,
		RawResponse:  reviewText,
	}

	filename := filepath.Join(r.cfg.Review.Output.Directory, fmt.Sprintf("%s-%d.md", sanitizeProject(project), mrNumber))
	content := buildReviewMarkdown(review)

	if err := writeFile(filename, content); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save review: %v", err))
		return
	}

	output.PrintSuccess("Review saved to:")
	output.PrintFilePath(filename)

	if err := projectctx.AppendMRReview(project, mrNumber, mrInfo.Title, reviewText); err != nil {
		output.PrintWarning(fmt.Sprintf("Could not update project context: %v", err))
	} else {
		output.PrintSuccess("Project context updated with MR review")
	}
	fmt.Println()

	r.storeReview(project, mrNumber, mrInfo.Title, filename, output.GenerateGitLabComment(review))
	r.stats.mrsReviewed++
	r.stats.filesCreated++

	if r.promptForYesNo("Do you want to add this review as a comment to the MR?") {
		r.postReviewComment(project, mrNumber)
	}

	r.refreshCacheAsync()
}

// ─── MR Comment ──────────────────────────────────────────────────────────────

func (r *replState) handleMRComment(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var mrNumberStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && mrNumberStr == "" {
				mrNumberStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else {
		if len(remaining) > 0 {
			mrNumberStr = remaining[0]
		}
	}

	if project == "" {
		project = r.promptForProject("Select project for MR comment")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}

	project = r.resolveProject(project)

	var mrNumber int
	if mrNumberStr != "" {
		n, err := strconv.Atoi(mrNumberStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid MR number: %s", mrNumberStr))
			return
		}
		mrNumber = n
	} else {
		recent := r.recentReviewsForProject(project, 5)
		if len(recent) == 0 {
			output.PrintError("No reviews found for this project. Use 'mr-review' first.")
			return
		}

		options := make([]string, len(recent))
		for i, entry := range recent {
			options[i] = fmt.Sprintf("!%d — %s (reviewed %s)", entry.mrNumber, entry.mrTitle, output.TimeAgo(entry.reviewedAt))
		}

		choice := r.promptForChoice("Select MR to comment on", options)
		if choice < 0 {
			return
		}
		mrNumber = recent[choice].mrNumber
	}

	key := reviewKey(project, mrNumber)
	entry, exists := r.reviews[key]
	if !exists {
		output.PrintError(fmt.Sprintf("No review found for MR #%d. Review it first with 'mr-review'.", mrNumber))
		return
	}

	r.postReviewComment(entry.project, entry.mrNumber)
	r.refreshCacheAsync()
}

func (r *replState) postReviewComment(project string, mrNumber int) {
	key := reviewKey(project, mrNumber)
	entry, exists := r.reviews[key]
	if !exists {
		output.PrintError("No review found to post.")
		return
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Posting review to MR #%d...", mrNumber)
	s.Start()

	noteURL, err := r.provider.MRs().PostMRComment(project, mrNumber, entry.comment)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to post comment: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Review posted to MR #%d", mrNumber))
	output.PrintURL(noteURL)
	fmt.Println()
}

// ─── MR Status ───────────────────────────────────────────────────────────────

func (r *replState) handleMRStatus(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	}
	if project == "" {
		project = r.promptForProject("Select project for MR status")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}

	project = r.resolveProject(project)

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching MRs from '%s'...", project)
	s.Start()

	openMRs, err := r.provider.MRs().ListProjectMRs(project, "opened", 20)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to fetch MRs: %v", err))
		return
	}

	if len(openMRs) == 0 {
		output.PrintSuccess("No open merge requests found.")
	} else {
		output.PrintMRListTable(openMRs, fmt.Sprintf("Open MRs — %s", project))
	}
	fmt.Println()

	r.refreshCacheAsync()
}

// ─── MR Checks ───────────────────────────────────────────────────────────────

func (r *replState) handleMRChecks(args []string) {
	if !r.ensureSession() {
		return
	}

	project, remaining := parseProjectFlag(args)

	var mrNumberStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && mrNumberStr == "" {
				mrNumberStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else {
		if len(remaining) > 0 {
			mrNumberStr = remaining[0]
		}
	}

	if project == "" {
		project = r.promptForProject("Select project for MR checks")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	var mrNumber int
	if mrNumberStr != "" {
		n, err := strconv.Atoi(mrNumberStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid MR number: %s", mrNumberStr))
			return
		}
		mrNumber = n
	} else {
		mrNumber = r.promptForNumber("MR number")
		if mrNumber <= 0 {
			output.PrintError("Invalid MR number.")
			return
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Fetching pipeline for MR #%d...", mrNumber)
	s.Start()

	pipeline, err := r.provider.MRs().GetMRPipeline(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Pipeline check failed: %v", err))
		return
	}

	output.PrintPipelineStatus(pipeline)

	r.refreshCacheAsync()
}

// ─── MR Open (create new MR) ────────────────────────────────────────────────

func (r *replState) handleMROpen(args []string) {
	if !r.ensureSession() {
		return
	}

	var project, sourceBranch, targetBranch string

	switch len(args) {
	case 3:
		project, sourceBranch, targetBranch = args[0], args[1], args[2]
	case 2:
		project, sourceBranch = args[0], args[1]
	case 1:
		project = args[0]
	}

	if project == "" {
		project = r.promptForProject("Select project to open MR")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	if sourceBranch == "" {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = fmt.Sprintf(" Fetching branches from '%s'...", project)
		s.Start()

		branches, err := r.provider.Repos().ListActiveBranches(project, 5)
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("Failed to fetch branches: %v", err))
			return
		}
		if len(branches) == 0 {
			output.PrintWarning("No active branches found.")
			return
		}

		options := make([]string, len(branches))
		for i, b := range branches {
			options[i] = fmt.Sprintf("%s — %s (%s)", b.Name, b.CommitTitle, output.TimeAgo(b.CommitDate))
		}

		choice := r.promptForChoice("Select source branch", options)
		if choice < 0 {
			return
		}
		sourceBranch = branches[choice].Name
	}

	if targetBranch == "" {
		targetOptions := []string{"development", "master"}
		choice := r.promptForChoice("Select target branch", targetOptions)
		if choice < 0 {
			return
		}
		targetBranch = targetOptions[choice]
	}

	title := branchToMRTitle(sourceBranch)
	if bInfo, err := r.provider.Repos().GetBranch(project, sourceBranch); err == nil && bInfo.CommitTitle != "" {
		title = bInfo.CommitTitle
	}

	fmt.Println()
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Generating MR description..."
	s.Start()

	description, _, err := r.generateMRDescription(project, sourceBranch, targetBranch)
	s.Stop()

	if err != nil {
		output.PrintWarning(fmt.Sprintf("Could not generate description: %v", err))
		description = fmt.Sprintf("Merge %s into %s", sourceBranch, targetBranch)
	}

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Creating MR: %s → %s...", sourceBranch, targetBranch)
	s.Start()

	mr, err := r.provider.MRs().CreateMergeRequest(project, sourceBranch, targetBranch, title, description)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to create MR: %v", err))
		return
	}

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("MR #%d created: %s", mr.IID, mr.Title))
	output.PrintSuccess(fmt.Sprintf("Branch: %s → %s", sourceBranch, targetBranch))
	output.PrintURL(mr.WebURL)
	fmt.Println()

	r.stats.mrsCreated++
	r.refreshCacheAsync()
}

// ─── MR Merge ────────────────────────────────────────────────────────────────

func (r *replState) handleMRMerge(args []string) {
	if !r.ensureSession() {
		return
	}
	project, mrNumber := r.parseMRArgs(args)
	if project == "" || mrNumber <= 0 {
		return
	}

	squash := r.promptForYesNo("Squash commits?")
	removeSource := r.promptForYesNo("Remove source branch after merge?")

	s := newSpinner(fmt.Sprintf(" Merging MR #%d...", mrNumber))
	s.Start()
	err := r.provider.MRs().MergeMR(project, mrNumber, platform.MergeOptions{
		Squash:             squash,
		RemoveSourceBranch: removeSource,
	})
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to merge MR: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("MR #%d merged successfully", mrNumber))
	fmt.Println()
}

// ─── MR Approve / Unapprove ─────────────────────────────────────────────────

func (r *replState) handleMRApprove(args []string) {
	if !r.ensureSession() {
		return
	}
	project, mrNumber := r.parseMRArgs(args)
	if project == "" || mrNumber <= 0 {
		return
	}

	s := newSpinner(fmt.Sprintf(" Approving MR #%d...", mrNumber))
	s.Start()
	err := r.provider.MRs().ApproveMR(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to approve MR: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("MR #%d approved", mrNumber))
	fmt.Println()
}

func (r *replState) handleMRUnapprove(args []string) {
	if !r.ensureSession() {
		return
	}
	project, mrNumber := r.parseMRArgs(args)
	if project == "" || mrNumber <= 0 {
		return
	}

	s := newSpinner(fmt.Sprintf(" Removing approval from MR #%d...", mrNumber))
	s.Start()
	err := r.provider.MRs().UnapproveMR(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to unapprove MR: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("MR #%d approval removed", mrNumber))
	fmt.Println()
}

// ─── MR Rebase ───────────────────────────────────────────────────────────────

func (r *replState) handleMRRebase(args []string) {
	if !r.ensureSession() {
		return
	}
	project, mrNumber := r.parseMRArgs(args)
	if project == "" || mrNumber <= 0 {
		return
	}

	s := newSpinner(fmt.Sprintf(" Rebasing MR #%d...", mrNumber))
	s.Start()
	err := r.provider.MRs().RebaseMR(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to rebase MR: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("MR #%d rebase initiated", mrNumber))
	fmt.Println()
}

// ─── MR Update ───────────────────────────────────────────────────────────────

func (r *replState) handleMRUpdate(args []string) {
	if !r.ensureSession() {
		return
	}
	project, mrNumber := r.parseMRArgs(args)
	if project == "" || mrNumber <= 0 {
		return
	}

	fields := []string{"Title", "Description", "Labels", "Cancel"}
	choice := r.promptForChoice("What to update?", fields)
	if choice < 0 || choice == 3 {
		return
	}

	var opts platform.UpdateMROptions
	switch choice {
	case 0:
		title := r.promptForText("new-title")
		if title == "" {
			output.PrintError("Title cannot be empty.")
			return
		}
		opts.Title = &title
	case 1:
		desc := r.promptForText("new-description")
		opts.Description = &desc
	case 2:
		labelsStr := r.promptForText("labels (comma-separated)")
		if labelsStr != "" {
			labels := strings.Split(labelsStr, ",")
			for i := range labels {
				labels[i] = strings.TrimSpace(labels[i])
			}
			opts.Labels = labels
		}
	}

	s := newSpinner(fmt.Sprintf(" Updating MR #%d...", mrNumber))
	s.Start()
	mr, err := r.provider.MRs().UpdateMR(project, mrNumber, opts)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to update MR: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("MR #%d updated: %s", mr.IID, mr.Title))
	fmt.Println()
}

// ─── MR Close / Reopen ──────────────────────────────────────────────────────

func (r *replState) handleMRClose(args []string) {
	if !r.ensureSession() {
		return
	}
	project, mrNumber := r.parseMRArgs(args)
	if project == "" || mrNumber <= 0 {
		return
	}

	s := newSpinner(fmt.Sprintf(" Closing MR #%d...", mrNumber))
	s.Start()
	err := r.provider.MRs().CloseMR(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to close MR: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("MR #%d closed", mrNumber))
	fmt.Println()
}

func (r *replState) handleMRReopen(args []string) {
	if !r.ensureSession() {
		return
	}
	project, mrNumber := r.parseMRArgs(args)
	if project == "" || mrNumber <= 0 {
		return
	}

	s := newSpinner(fmt.Sprintf(" Reopening MR #%d...", mrNumber))
	s.Start()
	err := r.provider.MRs().ReopenMR(project, mrNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to reopen MR: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("MR #%d reopened", mrNumber))
	fmt.Println()
}

// ─── Shared MR Arg Parser ───────────────────────────────────────────────────

func (r *replState) parseMRArgs(args []string) (string, int) {
	project, remaining := parseProjectFlag(args)

	var mrNumberStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && mrNumberStr == "" {
				mrNumberStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else if len(remaining) > 0 {
		mrNumberStr = remaining[0]
	}

	if project == "" {
		project = r.promptForProject("Select project")
		if project == "" {
			output.PrintError("No project selected.")
			return "", 0
		}
	}
	project = r.resolveProject(project)

	var mrNumber int
	if mrNumberStr != "" {
		n, err := strconv.Atoi(mrNumberStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid MR number: %s", mrNumberStr))
			return "", 0
		}
		mrNumber = n
	} else {
		mrNumber = r.promptForNumber("MR number")
		if mrNumber <= 0 {
			output.PrintError("Invalid MR number.")
			return "", 0
		}
	}

	return project, mrNumber
}
