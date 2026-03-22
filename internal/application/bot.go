package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	pb "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type UserState string

const (
	StateNone        UserState = ""
	StateWaitLink    UserState = "WAIT_LINK"
	StateWaitTags    UserState = "WAIT_TAGS"
	StateWaitUntrack UserState = "WAIT_UNTRACK"
)

type Bot struct {
	api          *tgbotapi.BotAPI
	sendFunc     func(tgbotapi.Chattable) (tgbotapi.Message, error)
	logger       *slog.Logger
	states       map[int64]UserState
	tempLinks    map[int64]string
	scrapperAddr string
	httpClient   *http.Client
	grpcClient   pb.ScrapperServiceClient
	useGRPC      bool
}

func NewBot(token string, scrapperAddr string, scrapperGRPCAddr string, logger *slog.Logger) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return NewBotWithAPI(api, scrapperAddr, scrapperGRPCAddr, logger), nil
}

func NewBotWithAPI(api *tgbotapi.BotAPI, scrapperAddr string, scrapperGRPCAddr string, logger *slog.Logger) *Bot {
	addr := scrapperAddr
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://localhost" + addr
	}

	var grpcClient pb.ScrapperServiceClient
	var useGRPC bool

	conn, err := grpc.NewClient(scrapperGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err == nil {
		grpcClient = pb.NewScrapperServiceClient(conn)
		useGRPC = true
		logger.Info("gRPC клиент инициализирован", slog.String("addr", scrapperGRPCAddr))
	} else {
		logger.Warn("Не удалось подключиться к gRPC сервису, используется HTTP", slog.String("error", err.Error()))
		useGRPC = false
	}

	if api == nil {
		api = &tgbotapi.BotAPI{Token: "dummy"}
	}

	return &Bot{
		api:          api,
		sendFunc:     api.Send,
		logger:       logger,
		states:       make(map[int64]UserState),
		tempLinks:    make(map[int64]string),
		scrapperAddr: addr,
		httpClient:   &http.Client{},
		grpcClient:   grpcClient,
		useGRPC:      useGRPC,
	}
}

func (b *Bot) Start(ctx context.Context) error {
	b.setupCommands()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return nil
		case update := <-updates:
			if update.Message != nil {
				b.handleMessage(update.Message)
			}
		}
	}
}

func generateResponse(command string) string {
	switch command {
	case "start":
		return "Добро пожаловать! Используйте /help, чтобы посмотреть доступные команды."
	case "help":
		return "Доступные команды:\n\n" +
			"/track — добавить новую ссылку для мониторинга.\n" +
			"/untrack — прекратить отслеживание ссылки.\n" +
			"/list — показать список ваших ссылок. Можно добавить тег (напр. `/list github`), чтобы отфильтровать список.\n" +
			"/cancel — прервать текущий ввод ссылки или тегов.\n" +
			"/help — показать это сообщение."
	default:
		return "Неизвестная команда. Воспользуйтесь /help, чтобы посмотреть список доступных команд."
	}
}

func (b *Bot) setupCommands() {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Начать работу с ботом"},
		{Command: "help", Description: "Показать справку"},
		{Command: "track", Description: "Добавить ссылку для отслеживания"},
		{Command: "untrack", Description: "Удалить ссылку из списка"},
		{Command: "list", Description: "Показать все отслеживаемые ссылки"},
		{Command: "cancel", Description: "Отменить текущую операцию"},
	}
	b.api.Request(tgbotapi.NewSetMyCommands(commands...))
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	text := msg.Text

	if msg.IsCommand() {
		b.states[chatID] = StateNone
		delete(b.tempLinks, chatID)
		cmd := msg.Command()

		switch cmd {
		case "start":
			b.registerChatInScrapper(chatID)
			b.SendMessage(chatID, generateResponse(cmd))
		case "help":
			b.SendMessage(chatID, generateResponse(cmd))
		case "track":
			b.states[chatID] = StateWaitLink
			b.SendMessage(chatID, "Введите ссылку для отслеживания:")
		case "untrack":
			b.states[chatID] = StateWaitUntrack
			b.SendMessage(chatID, "Введите ссылку для удаления:")
		case "list":
			b.handleListCommand(chatID, msg.CommandArguments())
		case "cancel":
			b.SendMessage(chatID, "Действие отменено.")
		default:
			b.SendMessage(chatID, generateResponse(cmd))
		}
		return
	}

	b.processState(chatID, text)
}

func (b *Bot) processState(chatID int64, text string) {
	switch b.states[chatID] {
	case StateWaitLink:
		if !strings.HasPrefix(text, "http") {
			b.SendMessage(chatID, "Некорректная ссылка. Попробуйте еще раз.")
			return
		}
		b.tempLinks[chatID] = text
		b.states[chatID] = StateWaitTags
		b.SendMessage(chatID, "Введите теги через запятую или '-' для пропуска:")
	case StateWaitTags:
		tags := []string{}
		if text != "-" {
			for _, t := range strings.Split(text, ",") {
				tags = append(tags, strings.TrimSpace(t))
			}
		}
		b.addLinkToScrapper(chatID, b.tempLinks[chatID], tags)
		b.states[chatID] = StateNone
	case StateWaitUntrack:
		b.removeLinkFromScrapper(chatID, text)
		b.states[chatID] = StateNone
	default:
		b.SendMessage(chatID, "Используйте /help для списка команд.")
	}
}

func (b *Bot) SendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.sendFunc(msg); err != nil {
		b.logger.Error("Ошибка отправки сообщения", slog.Int64("chatId", chatID), slog.String("error", err.Error()))
	}
}

func (b *Bot) registerChatInScrapper(chatID int64) bool {
	if b.useGRPC && b.grpcClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := b.grpcClient.RegisterChat(ctx, &pb.RegisterChatRequest{ChatId: chatID})
		if err == nil {
			b.logger.Debug("Чат зарегистрирован через gRPC", slog.Int64("chatId", chatID))
			return resp.Success
		}
		b.logger.Debug("Не удалось зарегистрировать чат через gRPC", slog.Int64("chatId", chatID), slog.String("error", err.Error()))
	}

	url := fmt.Sprintf("%s/tg-chat/%d", b.scrapperAddr, chatID)
	resp, err := b.httpClient.Post(url, "application/json", nil)
	if err != nil {
		b.logger.Warn("Не удалось зарегистрировать чат в Scrapper", slog.Int64("chatId", chatID), slog.String("error", err.Error()))
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusConflict
}

func (b *Bot) addLinkToScrapper(chatID int64, link string, tags []string) {
	if !b.registerChatInScrapper(chatID) {
		b.SendMessage(chatID, "Не удалось зарегистрировать вас в службе отслеживания. Пожалуйста, попробуйте позже.")
		return
	}

	tryAddGRPC := func() error {
		if !b.useGRPC || b.grpcClient == nil {
			return fmt.Errorf("gRPC не доступен")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := b.grpcClient.AddLink(ctx, &pb.AddLinkRequest{
			ChatId:  chatID,
			Link:    link,
			Tags:    tags,
			Filters: []string{},
		})

		if err == nil {
			b.logger.Debug("Ссылка добавлена через gRPC", slog.Int64("chatId", chatID), slog.String("link", link))
			return nil
		}

		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			return fmt.Errorf("already_exists")
		}

		return err
	}

	err := tryAddGRPC()
	if err == nil {
		b.SendMessage(chatID, "Ссылка добавлена!")
		return
	}

	if err.Error() == "already_exists" {
		b.SendMessage(chatID, "Ссылка уже отслеживается")
		return
	}

	tryAdd := func() (int, error) {
		body, err := json.Marshal(map[string]interface{}{"link": link, "tags": tags})
		if err != nil {
			b.logger.Error("failed to marshal link data", "error", err)
			return 0, err
		}
		req, err := http.NewRequest(http.MethodPost, b.scrapperAddr+"/links", bytes.NewBuffer(body))
		if err != nil {
			b.logger.Error("failed to create HTTP request", "error", err)
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Tg-Chat-Id", fmt.Sprint(chatID))
		resp, err := b.httpClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}

	status, err := tryAdd()
	if err != nil {
		b.SendMessage(chatID, "Ошибка: не удалось добавить ссылку.")
		return
	}

	if status == http.StatusNotFound {
		if b.registerChatInScrapper(chatID) {
			status, err = tryAdd()
			if err != nil {
				b.SendMessage(chatID, "Ошибка: не удалось добавить ссылку.")
				return
			}
		}
	}

	if status == http.StatusConflict {
		b.SendMessage(chatID, "Ссылка уже отслеживается")
		return
	}

	if status != http.StatusOK {
		b.SendMessage(chatID, "Ошибка: сервис недоступен.")
		return
	}

	b.SendMessage(chatID, "Ссылка добавлена!")
}

func (b *Bot) removeLinkFromScrapper(chatID int64, link string) {
	if !b.registerChatInScrapper(chatID) {
		b.SendMessage(chatID, "Не удалось зарегистрировать вас в службе отслеживания. Пожалуйста, попробуйте позже.")
		return
	}

	if b.useGRPC && b.grpcClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := b.grpcClient.RemoveLink(ctx, &pb.RemoveLinkRequest{ChatId: chatID, Link: link})
		if err == nil {
			b.logger.Debug("Ссылка удалена через gRPC", slog.Int64("chatId", chatID), slog.String("link", link))
			b.SendMessage(chatID, "Ссылка удалена.")
			return
		}

		st, ok := status.FromError(err)
		if ok && st.Code() == codes.NotFound {
			b.SendMessage(chatID, "Ссылка не найдена")
			return
		}

		b.logger.Debug("Не удалось удалить ссылку через gRPC", slog.Int64("chatId", chatID), slog.String("error", err.Error()))
	}

	body, err := json.Marshal(map[string]string{"link": link})
	if err != nil {
		b.logger.Error("failed to marshal link data", "error", err)
		b.SendMessage(chatID, "Ошибка: не удалось подготовить данные для удаления ссылки.")
		return
	}
	req, err := http.NewRequest(http.MethodDelete, b.scrapperAddr+"/links", bytes.NewBuffer(body))
	if err != nil {
		b.logger.Error("failed to create HTTP request", "error", err)
		b.SendMessage(chatID, "Ошибка: не удалось подготовить запрос для удаления ссылки.")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tg-Chat-Id", fmt.Sprint(chatID))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.SendMessage(chatID, "Ошибка: не удалось удалить ссылку.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		b.SendMessage(chatID, "Ссылка не найдена")
		return
	}
	if resp.StatusCode != http.StatusOK {
		b.SendMessage(chatID, "Ошибка: сервис недоступен.")
		return
	}

	b.SendMessage(chatID, "Ссылка удалена.")
}

func (b *Bot) handleListCommand(chatID int64, args string) {
	if !b.registerChatInScrapper(chatID) {
		b.SendMessage(chatID, "Не удалось зарегистрировать вас в службе отслеживания. Пожалуйста, попробуйте позже.")
		return
	}

	if b.useGRPC && b.grpcClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tag := strings.TrimSpace(args)
		resp, err := b.grpcClient.GetLinks(ctx, &pb.GetLinksRequest{ChatId: chatID, Tag: tag})
		if err == nil {
			if len(resp.Links) == 0 {
				b.SendMessage(chatID, "У вас нет активных подписок.")
				return
			}

			res := "Ваши ссылки:\n"
			for i, l := range resp.Links {
				res += fmt.Sprintf("%d. %s\n", i+1, l.Url)
			}
			b.SendMessage(chatID, res)
			return
		}

		st, ok := status.FromError(err)
		if ok && st.Code() == codes.NotFound {
			b.SendMessage(chatID, "Чат не найден на сервере отслеживания. Отправьте /start и повторите запрос.")
			return
		}

		b.logger.Debug("Не удалось получить список через gRPC", slog.Int64("chatId", chatID), slog.String("error", err.Error()))
	}

	url := fmt.Sprintf("%s/links", b.scrapperAddr)
	if tag := strings.TrimSpace(args); tag != "" {
		url = fmt.Sprintf("%s?tag=%s", url, tag)
	}

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Tg-Chat-Id", fmt.Sprint(chatID))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.SendMessage(chatID, "Не удалось получить список.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		b.SendMessage(chatID, "Чат не найден на сервере отслеживания. Отправьте /start и повторите запрос.")
		return
	}
	if resp.StatusCode != http.StatusOK {
		b.SendMessage(chatID, "Не удалось получить список.")
		return
	}

	var data struct {
		Links []struct {
			URL  string   `json:"url"`
			Tags []string `json:"tags"`
		} `json:"links"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	if len(data.Links) == 0 {
		b.SendMessage(chatID, "У вас нет активных подписок.")
		return
	}

	res := "Ваши ссылки:\n"
	for i, l := range data.Links {
		res += fmt.Sprintf("%d. %s\n", i+1, l.URL)
	}
	b.SendMessage(chatID, res)
}
