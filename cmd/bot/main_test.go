package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

func TestUpdatesHandler_ValidatesRequiredFields(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	h := newUpdatesHandler(func(int64, string) {}, logger)

	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Description == "" {
		t.Fatalf("expected error description, got empty")
	}
}

func TestUpdatesHandler_SendsMessageOnValidUpdate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	var sent struct {
		ChatID int64
		Text   string
	}
	stubSend := func(chatID int64, text string) {
		sent.ChatID = chatID
		sent.Text = text
	}

	h := newUpdatesHandler(stubSend, logger)

	update := domain.LinkUpdate{ID: 1, URL: "https://github.com/user/repo", Description: "desc", TgChatIDs: []int64{42}}
	b, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewBuffer(b))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if sent.ChatID != 42 {
		t.Fatalf("expected chatID 42, got %d", sent.ChatID)
	}
	if sent.Text == "" {
		t.Fatalf("expected message to be sent")
	}
}
