package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type StackOverflowClient struct {
	client  *http.Client
	baseURL string
}

func NewStackOverflowClient() *StackOverflowClient {
	return &StackOverflowClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://api.stackexchange.com/2.3",
	}
}

func (c *StackOverflowClient) FetchLastUpdate(url string) (time.Time, error) {
	parts := strings.Split(url, "/")
	var questionID string
	for i, part := range parts {
		if part == "questions" && i+1 < len(parts) {
			questionID = parts[i+1]
			break
		}
	}

	if questionID == "" {
		return time.Time{}, fmt.Errorf("could not extract question id from url: %s", url)
	}

	apiURL := fmt.Sprintf("%s/questions/%s?site=stackoverflow", strings.TrimRight(c.baseURL, "/"), questionID)

	resp, err := c.client.Get(apiURL)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("stackoverflow api returned status: %d", resp.StatusCode)
	}

	var result struct {
		Items []struct {
			LastActivityDate int64 `json:"last_activity_date"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, err
	}

	if len(result.Items) == 0 {
		return time.Time{}, fmt.Errorf("no items found for question id: %s", questionID)
	}

	return time.Unix(result.Items[0].LastActivityDate, 0), nil
}
