package repository

import (
	"sync"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type MemoryStorage struct {
	mu    sync.RWMutex
	chats map[int64]*domain.Chat
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		chats: make(map[int64]*domain.Chat),
	}
}

func (s *MemoryStorage) AddChat(chatID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.chats[chatID]; exists {
		return false
	}
	s.chats[chatID] = &domain.Chat{ID: chatID, Links: make(map[string]*domain.Link)}
	return true
}

func (s *MemoryStorage) RemoveChat(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.chats[id]; !exists {
		return false
	}

	delete(s.chats, id)
	return true
}

func (s *MemoryStorage) AddLink(chatID int64, url string, tags, filters []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, exists := s.chats[chatID]
	if !exists {
		return false
	}
	if _, linkExists := chat.Links[url]; linkExists {
		return false
	}
	chat.Links[url] = &domain.Link{URL: url, Tags: tags, Filters: filters}
	return true
}

func (s *MemoryStorage) RemoveLink(chatID int64, url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, exists := s.chats[chatID]
	if !exists {
		return false
	}
	if _, linkExists := chat.Links[url]; !linkExists {
		return false
	}
	delete(chat.Links, url)
	return true
}

func (s *MemoryStorage) GetLinks(chatID int64) ([]*domain.Link, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chat, exists := s.chats[chatID]
	if !exists {
		return nil, false
	}
	links := make([]*domain.Link, 0, len(chat.Links))
	for _, link := range chat.Links {
		links = append(links, link)
	}
	return links, true
}

func (s *MemoryStorage) GetAllChats() map[int64]*domain.Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make(map[int64]*domain.Chat, len(s.chats))
	for id, chat := range s.chats {
		linksCopy := make(map[string]*domain.Link, len(chat.Links))
		for url, link := range chat.Links {
			linksCopy[url] = &domain.Link{URL: link.URL, Tags: append([]string{}, link.Tags...), Filters: append([]string{}, link.Filters...), LastUpdated: link.LastUpdated}
		}
		copied[id] = &domain.Chat{ID: chat.ID, Links: linksCopy}
	}
	return copied
}

func (s *MemoryStorage) UpdateLinkLastUpdated(chatID int64, url string, updatedAt time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, exists := s.chats[chatID]
	if !exists {
		return false
	}
	link, exists := chat.Links[url]
	if !exists {
		return false
	}
	link.LastUpdated = updatedAt
	return true
}
