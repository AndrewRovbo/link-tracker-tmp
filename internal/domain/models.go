package domain

import "time"

type Link struct {
	URL         string
	Tags        []string
	Filters     []string
	LastUpdated time.Time
}

type Chat struct {
	ID    int64
	Links map[string]*Link
}

type LinkUpdate struct {
	ID          int64   `json:"id"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	TgChatIDs   []int64 `json:"tgChatIds"`
}
