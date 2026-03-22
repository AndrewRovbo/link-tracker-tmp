package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/config"
)

type apiError struct {
	Description      string   `json:"description"`
	Code             string   `json:"code,omitempty"`
	ExceptionName    string   `json:"exceptionName,omitempty"`
	ExceptionMessage string   `json:"exceptionMessage,omitempty"`
	Stacktrace       []string `json:"stacktrace,omitempty"`
}

func writeAPIError(w http.ResponseWriter, status int, description string, logger *slog.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(apiError{Description: description, Code: http.StatusText(status)}); err != nil {
		logger.Error("failed to encode API error response", "error", err)
	}
}

func newUpdatesHandler(sendMessage func(int64, string), logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var update domain.LinkUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			logger.Error("Ошибка декодирования обновления", slog.String("error", err.Error()))
			writeAPIError(w, http.StatusBadRequest, "Неверный формат запроса", logger)
			return
		}

		if strings.TrimSpace(update.URL) == "" || len(update.TgChatIDs) == 0 {
			writeAPIError(w, http.StatusBadRequest, "Отсутствуют обязательные поля: url или tgChatIds", logger)
			return
		}

		logger.Info("Получено обновление", slog.String("url", update.URL))

		for _, chatID := range update.TgChatIDs {
			notification := fmt.Sprintf("Обновление по ссылке: %s\n%s", update.URL, update.Description)
			sendMessage(chatID, notification)
		}

		w.WriteHeader(http.StatusOK)
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	var api *tgbotapi.BotAPI
	var err error

	if cfg.TelegramToken != "" {
		api, err = tgbotapi.NewBotAPI(cfg.TelegramToken)
		if err != nil {
			logger.Error("Не удалось инициализировать бота", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}

	botApp := application.NewBotWithAPI(api, cfg.ScrapperServerAddr, cfg.ScrapperGRPCAddr, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/updates", newUpdatesHandler(botApp.SendMessage, logger))

	server := &http.Server{
		Addr:    cfg.BotServerAddr,
		Handler: mux,
	}

	go func() {
		logger.Info("HTTP сервер бота запущен", slog.String("addr", cfg.BotServerAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Ошибка HTTP сервера", slog.String("error", err.Error()))
		}
	}()

	go func() {
		if err := botApp.Start(ctx); err != nil {
			logger.Error("Критическая ошибка работы бота", slog.String("error", err.Error()))
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("Завершение работы бота...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Ошибка при остановке сервера", slog.String("error", err.Error()))
	}

	logger.Info("Бот успешно остановлен")
}
