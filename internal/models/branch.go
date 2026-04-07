package models

import "time"

type BranchInfo struct {
	Name        string
	Merged      bool
	Protected   bool
	Default     bool
	CommitDate  time.Time
	CommitTitle string
	AuthorName  string
}
