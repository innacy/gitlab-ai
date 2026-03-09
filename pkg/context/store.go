package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const contextDir = ".context"

// EnsureDir creates the .context directory if it doesn't exist.
func EnsureDir() error {
	return os.MkdirAll(contextDir, 0755)
}

// ContextPath returns the file path for a project's context file.
func ContextPath(project string) string {
	safe := strings.ReplaceAll(project, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	safe = strings.ReplaceAll(safe, " ", "_")
	return filepath.Join(contextDir, safe+".md")
}

// LoadContext reads the full context file for a project.
// Returns empty string if the file doesn't exist.
func LoadContext(project string) (string, error) {
	data, err := os.ReadFile(ContextPath(project))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// LoadContextTruncated reads context, truncating to maxBytes if it's too large.
func LoadContextTruncated(project string, maxBytes int) (string, error) {
	content, err := LoadContext(project)
	if err != nil {
		return "", err
	}
	if len(content) > maxBytes {
		content = content[:maxBytes] + "\n\n... [context truncated] ..."
	}
	return content, nil
}

// SaveIndex writes the code index section of the context file,
// preserving existing MR History and Tickets sections.
func SaveIndex(project, indexContent string) error {
	if err := EnsureDir(); err != nil {
		return err
	}

	existing, _ := LoadContext(project)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Project Context: %s\n\n", project))
	sb.WriteString(fmt.Sprintf("## Code Index\n\n_Indexed at: %s_\n\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString(indexContent)
	sb.WriteString("\n\n")

	// Preserve existing MR History section
	if section := extractSection(existing, "## MR Review History"); section != "" {
		sb.WriteString("## MR Review History\n\n")
		sb.WriteString(section)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("## MR Review History\n\n_No reviews yet._\n\n")
	}

	// Preserve existing Tickets section
	if section := extractSection(existing, "## Tickets"); section != "" {
		sb.WriteString("## Tickets\n\n")
		sb.WriteString(section)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("## Tickets\n\n_No tickets loaded yet._\n\n")
	}

	return os.WriteFile(ContextPath(project), []byte(sb.String()), 0644)
}

// AppendMRReview appends an MR review summary to the context file.
func AppendMRReview(project string, mrNumber int, title, summary string) error {
	if err := EnsureDir(); err != nil {
		return err
	}

	existing, _ := LoadContext(project)

	entry := fmt.Sprintf("### MR #%d: %s\n\n_Reviewed: %s_\n\n%s\n\n---\n\n",
		mrNumber, title, time.Now().Format("2006-01-02 15:04"), summary)

	if strings.Contains(existing, "## MR Review History") {
		// Insert new entry right after the header
		marker := "## MR Review History\n\n"
		idx := strings.Index(existing, marker)
		if idx >= 0 {
			insertAt := idx + len(marker)
			// Remove placeholder if present
			placeholder := "_No reviews yet._\n\n"
			if strings.HasPrefix(existing[insertAt:], placeholder) {
				existing = existing[:insertAt] + existing[insertAt+len(placeholder):]
			}
			existing = existing[:insertAt] + entry + existing[insertAt:]
		}
	} else {
		existing += "\n## MR Review History\n\n" + entry
	}

	return os.WriteFile(ContextPath(project), []byte(existing), 0644)
}

// UpdateTickets replaces the Tickets section in the context file.
func UpdateTickets(project, ticketsSummary string) error {
	if err := EnsureDir(); err != nil {
		return err
	}

	existing, _ := LoadContext(project)

	newSection := fmt.Sprintf("_Updated: %s_\n\n%s",
		time.Now().Format("2006-01-02 15:04"), ticketsSummary)

	if idx := strings.Index(existing, "## Tickets"); idx >= 0 {
		// Find the end of the Tickets section (next ## header or EOF)
		rest := existing[idx+len("## Tickets"):]
		// Skip any leading newlines
		for len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		nextSection := strings.Index(rest, "\n## ")
		if nextSection >= 0 {
			existing = existing[:idx] + "## Tickets\n\n" + newSection + "\n\n" + rest[nextSection+1:]
		} else {
			existing = existing[:idx] + "## Tickets\n\n" + newSection + "\n"
		}
	} else {
		if existing == "" {
			existing = fmt.Sprintf("# Project Context: %s\n\n", project)
		}
		existing += "\n## Tickets\n\n" + newSection + "\n"
	}

	return os.WriteFile(ContextPath(project), []byte(existing), 0644)
}

// HasIndex checks if a project already has a code index.
func HasIndex(project string) bool {
	content, _ := LoadContext(project)
	return strings.Contains(content, "## Code Index")
}

// extractSection extracts the content of a named section (between its header and the next ## header).
func extractSection(content, header string) string {
	idx := strings.Index(content, header)
	if idx < 0 {
		return ""
	}
	start := idx + len(header)
	// Skip leading newlines
	for start < len(content) && content[start] == '\n' {
		start++
	}
	if start >= len(content) {
		return ""
	}
	rest := content[start:]
	nextHeader := strings.Index(rest, "\n## ")
	if nextHeader >= 0 {
		return strings.TrimSpace(rest[:nextHeader])
	}
	return strings.TrimSpace(rest)
}
