package models

type UserInfo struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	WebURL   string `json:"web_url"`
}
