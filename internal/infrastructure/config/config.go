package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	TelegramToken        string
	BotServerAddr        string
	ScrapperServerAddr   string
	ScrapperGRPCAddr     string
	GithubToken          string
}

func loadEnv() {
	file, err := os.Open(".env")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			os.Setenv(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
}

func Load() *Config {
	loadEnv()
	return &Config{
		TelegramToken:        os.Getenv("APP_TELEGRAM_TOKEN"),
		BotServerAddr:        getEnvOrDefault("BOT_SERVER_ADDR", ":8081"),
		ScrapperServerAddr:   getEnvOrDefault("SCRAPPER_SERVER_ADDR", ":8080"),
		ScrapperGRPCAddr:     getEnvOrDefault("SCRAPPER_GRPC_ADDR", "localhost:50051"),
		GithubToken:          os.Getenv("GITHUB_TOKEN"),
	}
}

func getEnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
