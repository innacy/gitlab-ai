package models

type EpicResult struct {
	IID     int    `json:"iid"`
	Title   string `json:"title"`
	WebURL  string `json:"web_url"`
	GroupID int    `json:"group_id"`
}
