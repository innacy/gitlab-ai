package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

var outputFormat = "text"

func SetOutputFormat(format string) {
	if format == "json" || format == "text" {
		outputFormat = format
	}
}

func IsJSON() bool {
	return outputFormat == "json"
}

func PrintJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

type Theme struct {
	Primary   *color.Color
	Accent    *color.Color
	Success   *color.Color
	Warning   *color.Color
	Error     *color.Color
	Muted     *color.Color
	Header    *color.Color
	Separator string
	BoxTop    string
	BoxBottom string
	BoxSide   string
}

var activeTheme = DefaultTheme()

func DefaultTheme() *Theme {
	return &Theme{
		Primary:   color.New(color.FgHiBlue, color.Bold),
		Accent:    color.New(color.FgHiMagenta),
		Success:   color.New(color.FgGreen),
		Warning:   color.New(color.FgYellow),
		Error:     color.New(color.FgRed),
		Muted:     color.New(color.Faint),
		Header:    color.New(color.FgHiBlue, color.Bold),
		Separator: "─",
		BoxTop:    "╭",
		BoxBottom: "╰",
		BoxSide:   "│",
	}
}

func SetTheme(t *Theme) {
	if t != nil {
		activeTheme = t
	}
}

func GetTheme() *Theme {
	return activeTheme
}

// ─── Themed output helpers ───────────────────────────────────────────────────

func ThemePrimary(format string, a ...interface{}) {
	activeTheme.Primary.Printf(format, a...)
}

func ThemeAccent(format string, a ...interface{}) {
	activeTheme.Accent.Printf(format, a...)
}

func ThemeMuted(format string, a ...interface{}) {
	activeTheme.Muted.Printf(format, a...)
}

func ThemeHeader(text string) {
	fmt.Println()
	activeTheme.Header.Printf("  %s\n", text)
	activeTheme.Muted.Printf("  %s\n", strings.Repeat(activeTheme.Separator, len(text)+2))
}

func ThemeSection(title string) {
	activeTheme.Accent.Printf("\n  %s %s\n", activeTheme.BoxSide, title)
}

func ThemeBox(lines []string) {
	if len(lines) == 0 {
		return
	}

	maxLen := 0
	for _, l := range lines {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	width := maxLen + 4

	activeTheme.Muted.Printf("  %s%s╮\n", activeTheme.BoxTop, strings.Repeat(activeTheme.Separator, width))
	for _, l := range lines {
		padded := l + strings.Repeat(" ", maxLen-len(l))
		activeTheme.Muted.Print("  │ ")
		fmt.Print(padded)
		activeTheme.Muted.Println("  │")
	}
	activeTheme.Muted.Printf("  %s%s╯\n", activeTheme.BoxBottom, strings.Repeat(activeTheme.Separator, width))
}
