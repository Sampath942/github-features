package models

type ViewCount struct {
	ID       int64 `json:"id"`
	Username string `json:"username"`
	Views    int64    `json:"views_count"`
}
