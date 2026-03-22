package repository

import (
	"testing"
	"time"
)

func TestUpdateLinkLastUpdated(t *testing.T) {
	repo := NewMemoryStorage()
	chatID := int64(42)
	repo.AddChat(chatID)
	url := "https://github.com/example/repo"
	if !repo.AddLink(chatID, url, nil, nil) {
		t.Fatalf("expected link to be added")
	}

	updatedAt := time.Unix(1234567890, 0).UTC()
	if !repo.UpdateLinkLastUpdated(chatID, url, updatedAt) {
		t.Fatalf("expected UpdateLinkLastUpdated to return true")
	}

	links, ok := repo.GetLinks(chatID)
	if !ok || len(links) != 1 {
		t.Fatalf("expected to get one link, got %v", links)
	}
	if !links[0].LastUpdated.Equal(updatedAt) {
		t.Fatalf("expected LastUpdated %v, got %v", updatedAt, links[0].LastUpdated)
	}
}
