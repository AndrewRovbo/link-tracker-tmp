package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestE2E_ScrapperBotInteraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	networkName := "scrapper-bot-network"
	network, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name: networkName,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}
	defer func() {
		if err := network.Remove(ctx); err != nil {
			t.Logf("Failed to remove network: %v", err)
		}
	}()

	scrapperContainer, scrapperURL, err := startScrapperContainerWithNetwork(ctx, networkName)
	if err != nil {
		t.Fatalf("Failed to start Scrapper container: %v", err)
	}
	defer func() {
		if err := scrapperContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate Scrapper container: %v", err)
		}
	}()

	botContainer, botURL, err := startBotContainerWithNetwork(ctx, networkName, scrapperURL)
	if err != nil {
		t.Fatalf("Failed to start Bot container: %v", err)
	}
	defer func() {
		if err := botContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate Bot container: %v", err)
		}
	}()

	time.Sleep(2 * time.Second)

	t.Run("RegisterChat", func(t *testing.T) {
		testRegisterChat(t, scrapperURL)
	})

	t.Run("AddLink", func(t *testing.T) {
		testAddLink(t, scrapperURL)
	})

	t.Run("GetLinks", func(t *testing.T) {
		testGetLinks(t, scrapperURL)
	})

	t.Run("DeleteLink", func(t *testing.T) {
		testDeleteLink(t, scrapperURL)
	})

	t.Run("SendUpdateNotificationToBot", func(t *testing.T) {
		testSendUpdateNotification(t, botURL)
	})
}

func TestScrapperContainerStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	container, _, err := startScrapperContainer(ctx)
	if err != nil {
		t.Fatalf("Failed to start Scrapper container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate Scrapper container: %v", err)
		}
	}()

	if !container.IsRunning() {
		t.Fatalf("Scrapper container is not running")
	}
}

func TestBotContainerStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	networkName := "scrapper-bot-network-bot-test"
	network, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name: networkName,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}
	defer func() {
		if err := network.Remove(ctx); err != nil {
			t.Logf("Failed to remove network: %v", err)
		}
	}()

	scrapperContainer, _, err := startScrapperContainerWithNetwork(ctx, networkName)
	if err != nil {
		t.Fatalf("Failed to start Scrapper container: %v", err)
	}
	defer func() {
		if err := scrapperContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate Scrapper container: %v", err)
		}
	}()

	container, _, err := startBotContainerWithNetwork(ctx, networkName, "")
	if err != nil {
		t.Fatalf("Failed to start Bot container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate Bot container: %v", err)
		}
	}()

	if !container.IsRunning() {
		t.Fatalf("Bot container is not running")
	}
}

func startScrapperContainer(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "link-tracker:scrapper",
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor: wait.ForListeningPort("8080/tcp").
			WithStartupTimeout(30 * time.Second),
		Env: map[string]string{
			"SCRAPPER_SERVER_ADDR": ":8080",
			"BOT_SERVER_ADDR":      ":8081",
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Scrapper container: %w", err)
	}

	endpoint, err := container.Endpoint(ctx, "http")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get Scrapper endpoint: %w", err)
	}

	return container, endpoint, nil
}

func startScrapperContainerWithNetwork(ctx context.Context, networkName string) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "link-tracker:scrapper",
		ExposedPorts: []string{"8080/tcp", "50051/tcp"},
		WaitingFor: wait.ForListeningPort("8080/tcp").
			WithStartupTimeout(30 * time.Second),
		Networks: []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"scrapper"},
		},
		Env: map[string]string{
			"SCRAPPER_SERVER_ADDR": ":8080",
			"BOT_SERVER_ADDR":      ":8081",
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Scrapper container: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "8080/tcp")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get mapped port: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get host: %w", err)
	}

	endpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	return container, endpoint, nil
}

func startBotContainer(ctx context.Context, scrapperURL string) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "link-tracker:bot",
		ExposedPorts: []string{"8081/tcp"},
		WaitingFor: wait.ForListeningPort("8081/tcp").
			WithStartupTimeout(30 * time.Second),
		Env: map[string]string{
			"BOT_SERVER_ADDR":      ":8081",
			"SCRAPPER_SERVER_ADDR": scrapperURL,
			"APP_TELEGRAM_TOKEN":   "dummy_token",
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Bot container: %w", err)
	}

	endpoint, err := container.Endpoint(ctx, "http")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get Bot endpoint: %w", err)
	}

	return container, endpoint, nil
}

func startBotContainerWithNetwork(ctx context.Context, networkName string, scrapperURL string) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "link-tracker:bot",
		ExposedPorts: []string{"8081/tcp"},
		WaitingFor: wait.ForListeningPort("8081/tcp").
			WithStartupTimeout(30 * time.Second),
		Networks: []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"bot"},
		},
		Env: map[string]string{
			"BOT_SERVER_ADDR":      ":8081",
			"SCRAPPER_SERVER_ADDR": "scrapper:8080",
			"SCRAPPER_GRPC_ADDR":   "scrapper:50051",
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Bot container: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "8081/tcp")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get mapped port: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get host: %w", err)
	}

	endpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	return container, endpoint, nil
}

func testRegisterChat(t *testing.T, baseURL string) {
	resp, err := http.Post(baseURL+"/tg-chat/123", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to register chat: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Response: %s", resp.StatusCode, string(body))
	}
}

func testAddLink(t *testing.T, baseURL string) {
	http.Post(baseURL+"/tg-chat/124", "application/json", nil)

	payload := map[string]interface{}{
		"link":    "https://github.com/user/repo",
		"tags":    []string{"golang", "test"},
		"filters": []string{"filter1"},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "124")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to add link: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Response: %s", resp.StatusCode, string(body))
	}
}

func testGetLinks(t *testing.T, baseURL string) {
	http.Post(baseURL+"/tg-chat/125", "application/json", nil)

	payload := map[string]interface{}{
		"link":    "https://github.com/test/project",
		"tags":    []string{"test"},
		"filters": []string{},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "125")
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	req, _ = http.NewRequest(http.MethodGet, baseURL+"/links", nil)
	req.Header.Set("Tg-Chat-Id", "125")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to get links: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Response: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Links []interface{} `json:"links"`
		Size  int           `json:"size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Size != 1 {
		t.Fatalf("Expected 1 link, got %d", result.Size)
	}
}

func testDeleteLink(t *testing.T, baseURL string) {
	http.Post(baseURL+"/tg-chat/126", "application/json", nil)

	linkURL := "https://github.com/delete/test"
	payload := map[string]interface{}{
		"link":    linkURL,
		"tags":    []string{},
		"filters": []string{},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "126")
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	deletePayload := map[string]interface{}{
		"link": linkURL,
	}
	deleteBody, _ := json.Marshal(deletePayload)

	req, _ = http.NewRequest(http.MethodDelete, baseURL+"/links", bytes.NewBuffer(deleteBody))
	req.Header.Set("Tg-Chat-Id", "126")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete link: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Response: %s", resp.StatusCode, string(body))
	}
}

func testSendUpdateNotification(t *testing.T, botURL string) {
	updatePayload := map[string]interface{}{
		"id":          1,
		"url":         "https://github.com/test/repo",
		"description": "Test update",
		"tgChatIds":   []int64{999},
	}
	body, _ := json.Marshal(updatePayload)

	resp, err := http.Post(botURL+"/updates", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Failed to send update notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Response: %s", resp.StatusCode, string(body))
	}
}
