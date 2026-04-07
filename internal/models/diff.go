package models

import "time"

type TagInfo struct {
	Name        string
	Message     string
	CommitTitle string
	CommitDate  time.Time
}

type DiffResult struct {
	From           string
	To             string
	DiffContent    string
	Files          []DiffFile
	Commits        []string
	TotalAdditions int
	TotalDeletions int
}

type DiffFile struct {
	OldPath   string
	NewPath   string
	NewFile   bool
	Renamed   bool
	Deleted   bool
	Additions int
	Deletions int
}
