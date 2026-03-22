package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGithubClient_FetchLastUpdate_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewGithubClient("")
	c.baseURL = server.URL
	c.client = server.Client()

	_, err := c.FetchLastUpdate("https://github.com/user/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGithubClient_FetchLastUpdate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	c := NewGithubClient("")
	c.baseURL = server.URL
	c.client = server.Client()

	_, err := c.FetchLastUpdate("https://github.com/user/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStackOverflowClient_FetchLastUpdate_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewStackOverflowClient()
	c.baseURL = server.URL
	c.client = server.Client()

	_, err := c.FetchLastUpdate("https://stackoverflow.com/questions/12345")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStackOverflowClient_FetchLastUpdate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	c := NewStackOverflowClient()
	c.baseURL = server.URL
	c.client = server.Client()

	_, err := c.FetchLastUpdate("https://stackoverflow.com/questions/12345")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
