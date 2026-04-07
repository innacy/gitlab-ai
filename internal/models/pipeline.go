package models

type PipelineInfo struct {
	ID     int
	Status string
	Ref    string
	WebURL string
	Jobs   []JobInfo
}

type JobInfo struct {
	ID     int
	Name   string
	Stage  string
	Status string
	WebURL string
}
