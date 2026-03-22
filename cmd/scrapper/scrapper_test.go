package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"log/slog"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/repository"
)

func makeTestServer(t *testing.T) *httptest.Server {
	repo := repository.NewMemoryStorage()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	handler := newScrapperHandler(repo, logger)
	return httptest.NewServer(handler)
}

func TestScrapper_RegisterChatAndLinks(t *testing.T) {
	server := makeTestServer(t)
	defer server.Close()

	resp, err := http.Post(server.URL+"/tg-chat/1", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	payload := map[string]interface{}{"link": "https://github.com/user/repo", "tags": []string{"tag1"}, "filters": []string{"f1"}}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "1")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/links", nil)
	req.Header.Set("Tg-Chat-Id", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var data struct {
		Links []struct {
			URL     string   `json:"url"`
			Tags    []string `json:"tags"`
			Filters []string `json:"filters"`
		} `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(data.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(data.Links))
	}
	if data.Links[0].URL != "https://github.com/user/repo" {
		t.Fatalf("unexpected url: %s", data.Links[0].URL)
	}
	if len(data.Links[0].Filters) != 1 || data.Links[0].Filters[0] != "f1" {
		t.Fatalf("expected filters [f1], got %v", data.Links[0].Filters)
	}
}

func TestScrapper_LinkFiltering(t *testing.T) {
	server := makeTestServer(t)
	defer server.Close()

	_, _ = http.Post(server.URL+"/tg-chat/2", "", nil)

	for i, tag := range []string{"alpha", "beta"} {
		payload := map[string]interface{}{"link": "https://github.com/user/repo" + strconv.Itoa(i), "tags": []string{tag}}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/links", bytes.NewBuffer(body))
		req.Header.Set("Tg-Chat-Id", "2")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/links?tag=alpha", nil)
	req.Header.Set("Tg-Chat-Id", "2")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var data struct {
		Links []struct {
			URL     string   `json:"url"`
			Tags    []string `json:"tags"`
			Filters []string `json:"filters"`
		} `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(data.Links) != 1 {
		t.Fatalf("expected 1 link with tag filter, got %d", len(data.Links))
	}
}
