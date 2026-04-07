package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/output"
)

// ─── Interactive Selection ───────────────────────────────────────────────────

// interactiveSelect presents an arrow-key navigable menu.
// Returns the 0-based index of the selected option, or -1 on cancel.
func (r *replState) interactiveSelect(title string, options []string) int {
	if len(options) == 0 {
		return -1
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return r.fallbackChoice(title, options)
	}
	defer term.Restore(fd, oldState)

	selected := 0
	numOpts := len(options)

	write := func(s string) { os.Stdout.WriteString(s) }

	renderOption := func(i int, highlighted bool) {
		write("\r\033[K")
		if highlighted {
			write(fmt.Sprintf("  \033[1;32m❯ %d. %s\033[0m\r\n", i+1, options[i]))
		} else {
			write(fmt.Sprintf("    %d. %s\r\n", i+1, options[i]))
		}
	}

	write("\r\n")
	write(fmt.Sprintf("  \033[1;36m%s\033[0m\r\n", title))
	for i := range options {
		renderOption(i, i == selected)
	}

	maxQuick := numOpts
	if maxQuick > 9 {
		maxQuick = 9
	}
	write(fmt.Sprintf("\r\n  \033[2m↑/↓ navigate • Enter select • 1-%d quick pick • Esc cancel\033[0m", maxQuick))

	// Cursor is at end of hint line. Move back to first option line.
	write(fmt.Sprintf("\033[%dA\r", numOpts+1))

	renderAll := func() {
		write("\r")
		for i := range options {
			renderOption(i, i == selected)
		}
		write(fmt.Sprintf("\033[%dA\r", numOpts))
	}

	cleanup := func() {
		// Move from first option line down past all options, then clear blank+hint lines
		write(fmt.Sprintf("\033[%dB", numOpts))
		write("\r\033[K\r\n\033[K\r\n")
	}

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			cleanup()
			return -1
		}

		if n == 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up
				if selected > 0 {
					selected--
					renderAll()
				}
			case 66: // Down
				if selected < numOpts-1 {
					selected++
					renderAll()
				}
			}
			continue
		}

		if n >= 1 {
			switch buf[0] {
			case 13, 10: // Enter
				cleanup()
				return selected
			case 27, 3: // Esc, Ctrl+C
				cleanup()
				return -1
			default:
				if buf[0] >= '1' && buf[0] <= '9' {
					idx := int(buf[0]-'0') - 1
					if idx < numOpts {
						selected = idx
						renderAll()
						cleanup()
						return idx
					}
				}
			}
		}
	}
}

// fallbackChoice is used when the terminal doesn't support raw mode.
func (r *replState) fallbackChoice(title string, options []string) int {
	fmt.Println()
	color.New(color.FgCyan, color.Bold).Println(title)
	for i, opt := range options {
		fmt.Printf("  %d. %s\n", i+1, opt)
	}
	fmt.Println()

	r.rl.SetPrompt("choice> ")
	defer r.rl.SetPrompt("gitlab-ai> ")

	line, err := r.rl.Readline()
	r.resetIdle()
	if err != nil {
		return -1
	}

	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(options) {
		output.PrintError("Invalid choice.")
		return -1
	}
	return n - 1
}

// ─── Interactive Prompts ─────────────────────────────────────────────────────

func (r *replState) recentProjects(n int) []models.ProjectInfo {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	if len(r.projectCache) == 0 {
		return nil
	}

	sorted := make([]models.ProjectInfo, len(r.projectCache))
	copy(sorted, r.projectCache)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LastActivity.After(sorted[j].LastActivity)
	})

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

func (r *replState) promptForProject(title string) string {
	if r.cacheReady != nil {
		select {
		case <-r.cacheReady:
		case <-time.After(3 * time.Second):
		}
	}

	for {
		recent := r.recentProjects(5)

		if len(recent) > 0 {
			options := make([]string, len(recent)+2)
			for i, p := range recent {
				options[i] = fmt.Sprintf("%s (%s)", p.Path, output.TimeAgo(p.LastActivity))
			}
			options[len(recent)] = "Type project name manually..."
			options[len(recent)+1] = "↻ Sync projects"

			choice := r.interactiveSelect(title, options)
			if choice < 0 {
				return ""
			}
			if choice < len(recent) {
				return recent[choice].Path
			}
			if choice == len(recent)+1 {
				r.syncProjectCache()
				continue
			}
		}

		fmt.Println()
		color.New(color.FgCyan, color.Bold).Println("  Enter project name or path (Tab to auto-complete):")

		r.rl.SetPrompt("project> ")
		defer r.rl.SetPrompt("gitlab-ai> ")

		line, err := r.rl.Readline()
		r.resetIdle()
		if err != nil {
			return ""
		}

		return strings.TrimSpace(line)
	}
}

func (r *replState) promptForNumber(label string) int {
	r.rl.SetPrompt(fmt.Sprintf("%s> ", label))
	defer r.rl.SetPrompt("gitlab-ai> ")

	line, err := r.rl.Readline()
	r.resetIdle()

	if err != nil {
		return 0
	}

	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		return 0
	}
	return n
}

func (r *replState) promptForChoice(title string, options []string) int {
	return r.interactiveSelect(title, options)
}

func (r *replState) promptForYesNo(question string) bool {
	choice := r.interactiveSelect(question, []string{"Yes", "No"})
	return choice == 0
}

func (r *replState) promptForText(label string) string {
	r.rl.SetPrompt(fmt.Sprintf("%s> ", label))
	defer r.rl.SetPrompt("gitlab-ai> ")

	line, err := r.rl.Readline()
	r.resetIdle()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

func (r *replState) selectProjectWithConfig(title string) string {
	configured := make([]string, 0, 4)
	for _, p := range r.cfg.Projects {
		project := strings.TrimSpace(p)
		if project != "" {
			configured = append(configured, project)
		}
	}
	if len(configured) == 0 {
		dp := strings.TrimSpace(r.cfg.GitLab.DefaultProject)
		if dp != "" {
			configured = append(configured, dp)
		}
	}

	switch len(configured) {
	case 0:
		return r.promptForProject(title)
	case 1:
		output.PrintSuccess(fmt.Sprintf("Using configured project: %s", configured[0]))
		return configured[0]
	default:
		choice := r.promptForChoice("Select configured project", configured)
		if choice < 0 {
			return ""
		}
		return configured[choice]
	}
}
