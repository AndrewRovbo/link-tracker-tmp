package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type GithubClient struct {
	client  *http.Client
	token   string
	baseURL string
}

func NewGithubClient(token string) *GithubClient {
	return &GithubClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		token:   token,
		baseURL: "https://api.github.com",
	}
}

func (c *GithubClient) FetchLastUpdate(url string) (time.Time, error) {
	parts := strings.Split(strings.TrimPrefix(url, "https://github.com/"), "/")
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("invalid github url")
	}
	owner, repo := parts[0], parts[1]

	apiURL := fmt.Sprintf("%s/repos/%s/%s", strings.TrimRight(c.baseURL, "/"), owner, repo)
	req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var result struct {
		UpdatedAt time.Time `json:"pushed_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, err
	}
	return result.UpdatedAt, nil
}
