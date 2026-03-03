package utils

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NetrcEntry represents a single machine entry in a .netrc file.
type NetrcEntry struct {
	Machine  string
	Login    string
	Password string
}

// ParseNetrc reads and parses the ~/.netrc file.
func ParseNetrc() ([]NetrcEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	netrcPath := filepath.Join(home, ".netrc")
	return ParseNetrcFile(netrcPath)
}

// ParseNetrcFile reads and parses a .netrc file at the given path.
func ParseNetrcFile(path string) ([]NetrcEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(".netrc file not found at %s", path)
		}
		return nil, fmt.Errorf("failed to stat .netrc: %w", err)
	}

	// Warn if file permissions are too open, but don't block
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: .netrc file has permissions %o (recommended 600): %s\n", mode, path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open .netrc: %w", err)
	}
	defer file.Close()

	var entries []NetrcEntry
	var current *NetrcEntry

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		tokens := tokenize(line)
		for i := 0; i < len(tokens); i++ {
			switch tokens[i] {
			case "machine":
				if current != nil {
					entries = append(entries, *current)
				}
				current = &NetrcEntry{}
				if i+1 < len(tokens) {
					current.Machine = tokens[i+1]
					i++
				}
			case "login":
				if current != nil && i+1 < len(tokens) {
					current.Login = tokens[i+1]
					i++
				}
			case "password":
				if current != nil && i+1 < len(tokens) {
					current.Password = tokens[i+1]
					i++
				}
			}
		}
	}

	if current != nil {
		entries = append(entries, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .netrc: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no entries found in .netrc file")
	}

	return entries, nil
}

// FindNetrcEntry finds a .netrc entry for the given machine hostname.
func FindNetrcEntry(machine string) (*NetrcEntry, error) {
	entries, err := ParseNetrc()
	if err != nil {
		return nil, err
	}

	// Normalize machine name (strip protocol and trailing slashes)
	machine = normalizeMachine(machine)

	for _, entry := range entries {
		if normalizeMachine(entry.Machine) == machine {
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("no .netrc entry found for machine: %s", machine)
}

// normalizeMachine strips protocol prefix and trailing slashes.
func normalizeMachine(machine string) string {
	machine = strings.TrimPrefix(machine, "https://")
	machine = strings.TrimPrefix(machine, "http://")
	machine = strings.TrimSuffix(machine, "/")
	return machine
}

// tokenize splits a line into whitespace-separated tokens.
func tokenize(line string) []string {
	return strings.Fields(line)
}
