package application

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestGenerateResponse(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected string
	}{
		{
			name:     "positive: start command",
			command:  "start",
			expected: "Добро пожаловать! Используйте /help, чтобы посмотреть доступные команды.",
		},
		{
			name:     "positive: help command",
			command:  "help",
			expected: "Доступные команды:\n\n/track — добавить новую ссылку для мониторинга.\n/untrack — прекратить отслеживание ссылки.\n/list — показать список ваших ссылок. Можно добавить тег (напр. `/list github`), чтобы отфильтровать список.\n/cancel — прервать текущий ввод ссылки или тегов.\n/help — показать это сообщение.",
		},
		{
			name:     "negative: unknown command",
			command:  "qwerty",
			expected: "Неизвестная команда. Воспользуйтесь /help, чтобы посмотреть список доступных команд.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := generateResponse(tt.command)
			if actual != tt.expected {
				t.Errorf("test %q: expected %q, got %q", tt.name, tt.expected, actual)
			}
		})
	}
}

func makeScrapperMockServer() *httptest.Server {
	chats := map[int64]map[string]struct {
		tags    []string
		filters []string
	}{}

	mux := http.NewServeMux()
	mux.HandleFunc("/tg-chat/", func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.TrimPrefix(r.URL.Path, "/tg-chat/")
		if idStr == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPost:
			if _, ok := chats[id]; ok {
				w.WriteHeader(http.StatusConflict)
				return
			}
			chats[id] = make(map[string]struct{ tags, filters []string })
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			if _, ok := chats[id]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(chats, id)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/links", func(w http.ResponseWriter, r *http.Request) {
		chatIDStr := r.Header.Get("Tg-Chat-Id")
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		links, ok := chats[chatID]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		switch r.Method {
		case http.MethodPost:
			var body struct {
				Link    string   `json:"link"`
				Tags    []string `json:"tags"`
				Filters []string `json:"filters"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, exists := links[body.Link]; exists {
				w.WriteHeader(http.StatusConflict)
				return
			}
			links[body.Link] = struct{ tags, filters []string }{tags: body.Tags, filters: body.Filters}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			tag := r.URL.Query().Get("tag")
			resp := struct {
				Links []struct {
					URL     string   `json:"url"`
					Tags    []string `json:"tags"`
					Filters []string `json:"filters"`
				} `json:"links"`
			}{}
			for url, info := range links {
				if tag != "" {
					found := false
					for _, t := range info.tags {
						if t == tag {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}
				resp.Links = append(resp.Links, struct {
					URL     string   `json:"url"`
					Tags    []string `json:"tags"`
					Filters []string `json:"filters"`
				}{URL: url, Tags: info.tags, Filters: info.filters})
			}
			_ = json.NewEncoder(w).Encode(resp)
		case http.MethodDelete:
			var body struct {
				Link string `json:"link"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, exists := links[body.Link]; !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(links, body.Link)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return httptest.NewServer(mux)
}

func TestBot_TrackUntrackList_Commands(t *testing.T) {
	server := makeScrapperMockServer()
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	api := &tgbotapi.BotAPI{}
	b := NewBotWithAPI(api, server.URL, "localhost:50051", logger)
	var sent []string
	b.sendFunc = func(chattable tgbotapi.Chattable) (tgbotapi.Message, error) {
		switch msg := chattable.(type) {
		case tgbotapi.MessageConfig:
			sent = append(sent, msg.Text)
		default:
			sent = append(sent, "")
		}
		return tgbotapi.Message{}, nil
	}

	chatID := int64(1)

	b.handleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: "/track", Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 6}}})
	if len(sent) == 0 || sent[0] != "Введите ссылку для отслеживания:" {
		t.Fatalf("expected prompt for link, got %v", sent)
	}

	sent = nil
	b.handleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: "tbank://bad"})
	if len(sent) == 0 || sent[0] != "Некорректная ссылка. Попробуйте еще раз." {
		t.Fatalf("expected invalid link message, got %v", sent)
	}

	sent = nil
	b.handleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: "https://github.com/user/repo"})
	if len(sent) == 0 || sent[0] != "Введите теги через запятую или '-' для пропуска:" {
		t.Fatalf("expected prompt for tags, got %v", sent)
	}

	sent = nil
	b.handleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: "tag1,tag2"})
	if len(sent) == 0 || sent[0] != "Ссылка добавлена!" {
		t.Fatalf("expected confirmation, got %v", sent)
	}

	sent = nil
	b.handleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: "/list", Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 5}}})
	if len(sent) == 0 || !strings.Contains(sent[0], "https://github.com/user/repo") {
		t.Fatalf("expected list to contain the link, got %v", sent)
	}

	sent = nil
	b.handleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: "/untrack", Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 8}}})
	if len(sent) == 0 || sent[0] != "Введите ссылку для удаления:" {
		t.Fatalf("expected prompt for untrack, got %v", sent)
	}

	sent = nil
	b.handleMessage(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, Text: "https://github.com/user/repo"})
	if len(sent) == 0 || sent[0] != "Ссылка удалена." {
		t.Fatalf("expected untrack confirmation, got %v", sent)
	}
}
