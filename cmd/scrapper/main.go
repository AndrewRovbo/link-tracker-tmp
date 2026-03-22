package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/client"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/config"
	pb "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/grpc"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/repository"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newScrapperHandler(repo *repository.MemoryStorage, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	type apiError struct {
		Description      string   `json:"description"`
		Code             string   `json:"code,omitempty"`
		ExceptionName    string   `json:"exceptionName,omitempty"`
		ExceptionMessage string   `json:"exceptionMessage,omitempty"`
		Stacktrace       []string `json:"stacktrace,omitempty"`
	}

	writeError := func(w http.ResponseWriter, status int, description string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(apiError{Description: description, Code: http.StatusText(status)}); err != nil {
			logger.Error("failed to encode error response", "error", err)
		}
	}

	type linkResponse struct {
		ID      int64    `json:"id"`
		URL     string   `json:"url"`
		Tags    []string `json:"tags"`
		Filters []string `json:"filters"`
	}

	mux.HandleFunc("/tg-chat/", func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.TrimPrefix(r.URL.Path, "/tg-chat/")
		if idStr == "" {
			writeError(w, http.StatusBadRequest, "Некорректный ID чата")
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Некорректный ID чата")
			return
		}

		switch r.Method {
		case http.MethodPost:
			if !repo.AddChat(id) {
				writeError(w, http.StatusConflict, "Чат уже существует")
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			if !repo.RemoveChat(id) {
				writeError(w, http.StatusNotFound, "Чат не найден")
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/links", func(w http.ResponseWriter, r *http.Request) {
		chatIDStr := r.Header.Get("Tg-Chat-Id")
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Отсутствует или некорректен заголовок Tg-Chat-Id")
			return
		}

		links, exists := repo.GetLinks(chatID)
		if !exists {
			writeError(w, http.StatusNotFound, "Чат не найден")
			return
		}

		switch r.Method {
		case http.MethodGet:
			filterTag := r.URL.Query().Get("tag")

			response := struct {
				Links []linkResponse `json:"links"`
				Size  int            `json:"size"`
			}{
				Links: make([]linkResponse, 0, len(links)),
			}
			for _, l := range links {
				if filterTag != "" {
					found := false
					for _, t := range l.Tags {
						if t == filterTag {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}
				response.Links = append(response.Links, linkResponse{ID: time.Now().UnixNano(), URL: l.URL, Tags: l.Tags, Filters: l.Filters})
			}
			response.Size = len(response.Links)

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				logger.Error("failed to encode links response", "error", err)
			}
		case http.MethodPost:
			var req struct {
				Link    string   `json:"link"`
				Tags    []string `json:"tags"`
				Filters []string `json:"filters"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "Неверный формат запроса")
				return
			}
			if !repo.AddLink(chatID, req.Link, req.Tags, req.Filters) {
				writeError(w, http.StatusConflict, "Ссылка уже отслеживается")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(linkResponse{ID: time.Now().UnixNano(), URL: req.Link, Tags: req.Tags, Filters: req.Filters}); err != nil {
				logger.Error("failed to encode link response", "error", err)
			}
		case http.MethodDelete:
			var req struct {
				Link string `json:"link"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "Неверный формат запроса")
				return
			}
			if !repo.RemoveLink(chatID, req.Link) {
				writeError(w, http.StatusNotFound, "Ссылка не найдена")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(linkResponse{ID: time.Now().UnixNano(), URL: req.Link}); err != nil {
				logger.Error("failed to encode link response", "error", err)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return mux
}

type ScrapperGRPCServer struct {
	repo   *repository.MemoryStorage
	logger *slog.Logger
	pb.UnimplementedScrapperServiceServer
}

func (s *ScrapperGRPCServer) RegisterChat(ctx context.Context, req *pb.RegisterChatRequest) (*pb.RegisterChatResponse, error) {
	success := s.repo.AddChat(req.ChatId)
	return &pb.RegisterChatResponse{Success: success}, nil
}

func (s *ScrapperGRPCServer) UnregisterChat(ctx context.Context, req *pb.UnregisterChatRequest) (*pb.UnregisterChatResponse, error) {
	success := s.repo.RemoveChat(req.ChatId)
	return &pb.UnregisterChatResponse{Success: success}, nil
}

func (s *ScrapperGRPCServer) AddLink(ctx context.Context, req *pb.AddLinkRequest) (*pb.AddLinkResponse, error) {
	success := s.repo.AddLink(req.ChatId, req.Link, req.Tags, req.Filters)
	if !success {
		return nil, status.Error(codes.AlreadyExists, "Ссылка уже отслеживается")
	}
	resp := &pb.AddLinkResponse{
		Id:      time.Now().UnixNano(),
		Url:     req.Link,
		Tags:    req.Tags,
		Filters: req.Filters,
	}
	return resp, nil
}

func (s *ScrapperGRPCServer) RemoveLink(ctx context.Context, req *pb.RemoveLinkRequest) (*pb.RemoveLinkResponse, error) {
	success := s.repo.RemoveLink(req.ChatId, req.Link)
	if !success {
		return nil, status.Error(codes.NotFound, "Ссылка не найдена")
	}
	return &pb.RemoveLinkResponse{Url: req.Link}, nil
}

func (s *ScrapperGRPCServer) GetLinks(ctx context.Context, req *pb.GetLinksRequest) (*pb.GetLinksResponse, error) {
	links, exists := s.repo.GetLinks(req.ChatId)
	if !exists {
		return nil, status.Error(codes.NotFound, "Чат не найден")
	}

	response := &pb.GetLinksResponse{Links: make([]*pb.LinkData, 0, len(links))}
	for _, l := range links {
		if req.Tag != "" {
			found := false
			for _, t := range l.Tags {
				if t == req.Tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		response.Links = append(response.Links, &pb.LinkData{
			Id:      time.Now().UnixNano(),
			Url:     l.URL,
			Tags:    l.Tags,
			Filters: l.Filters,
		})
	}
	response.Size = int32(len(response.Links))
	return response, nil
}

func newScrapperGRPCServer(repo *repository.MemoryStorage, logger *slog.Logger, grpcAddr string) error {
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	s := grpc.NewServer()
	pb.RegisterScrapperServiceServer(s, &ScrapperGRPCServer{repo: repo, logger: logger})

	go func() {
		logger.Info("gRPC сервер Scrapper запущен", slog.String("addr", grpcAddr))
		if err := s.Serve(listener); err != nil {
			logger.Error("Ошибка gRPC сервера Scrapper", slog.String("error", err.Error()))
		}
	}()

	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	repo := repository.NewMemoryStorage()
	githubClient := client.NewGithubClient(cfg.GithubToken)
	stackOverflowClient := client.NewStackOverflowClient()
	botHTTPClient := &http.Client{Timeout: 5 * time.Second}

	mux := newScrapperHandler(repo, logger)

	go func() {
		logger.Info("HTTP сервер Scrapper запущен", slog.String("addr", cfg.ScrapperServerAddr))
		if err := http.ListenAndServe(cfg.ScrapperServerAddr, mux); err != nil {
			logger.Error("Ошибка HTTP сервера Scrapper", slog.String("error", err.Error()))
		}
	}()

	grpcAddr := ":50051"
	if err := newScrapperGRPCServer(repo, logger, grpcAddr); err != nil {
		logger.Error("Ошибка инициализации gRPC сервера", slog.String("error", err.Error()))
	}

	s, err := gocron.NewScheduler()
	if err != nil {
		logger.Error("Ошибка инициализации планировщика", slog.String("error", err.Error()))
		os.Exit(1)
	}

	botUpdateURL := "http://localhost" + cfg.BotServerAddr + "/updates"
	if !strings.HasPrefix(cfg.BotServerAddr, ":") {
		botUpdateURL = cfg.BotServerAddr + "/updates"
	}

	_, err = s.NewJob(
		gocron.DurationJob(30*time.Second),
		gocron.NewTask(func() {
			logger.Debug("Запуск проверки ссылок")
			chats := repo.GetAllChats()

			for chatID, chat := range chats {
				for url, link := range chat.Links {
					var (
						updatedAt time.Time
						err       error
					)

					switch {
					case strings.Contains(url, "github.com"):
						updatedAt, err = githubClient.FetchLastUpdate(url)
					case strings.Contains(url, "stackoverflow.com") || strings.Contains(url, "stackexchange.com"):
						updatedAt, err = stackOverflowClient.FetchLastUpdate(url)
					default:
						continue
					}

					if err != nil {
						logger.Error("Ошибка проверки ссылки", slog.String("url", url), slog.String("error", err.Error()))
						continue
					}

					if link.LastUpdated.IsZero() {
						repo.UpdateLinkLastUpdated(chatID, url, updatedAt)
						continue
					}

					if updatedAt.After(link.LastUpdated) {
						logger.Info("Изменение найдено", slog.String("url", url), slog.Time("prev", link.LastUpdated), slog.Time("now", updatedAt))
						repo.UpdateLinkLastUpdated(chatID, url, updatedAt)

						updateData := domain.LinkUpdate{
							ID:          time.Now().UnixNano(),
							URL:         url,
							Description: "Обнаружены новые изменения",
							TgChatIDs:   []int64{chatID},
						}

						body, err := json.Marshal(updateData)
						if err != nil {
							logger.Error("failed to marshal update data", "error", err, "url", url)
							continue
						}
						resp, err := botHTTPClient.Post(botUpdateURL, "application/json", bytes.NewBuffer(body))
						if err != nil || resp.StatusCode != http.StatusOK {
							logger.Error("Ошибка отправки уведомления боту", slog.String("url", url))
						} else {
							logger.Info("Уведомление успешно отправлено", slog.String("url", url))
						}
					}
				}
			}
		}),
	)

	if err != nil {
		logger.Error("Ошибка создания задачи планировщика", slog.String("error", err.Error()))
		os.Exit(1)
	}

	s.Start()
	select {}
}
