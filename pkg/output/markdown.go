package output

import (
	"fmt"
	"strings"

	"gitlab-ai/internal/models"
)

// GenerateGitLabComment formats a review for posting as a GitLab MR comment.
func GenerateGitLabComment(review *models.Review) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("🤖 Review generated on %s\n", review.ReviewDate.Format("2006-01-02 15:04 UTC")))
	sb.WriteString("---\n\n")

	for _, section := range review.Sections {
		sb.WriteString(fmt.Sprintf("## %s\n\n", section.Name))
		sb.WriteString(section.Content)
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n")

	return sb.String()
}
