# LinkTracker

**LinkTracker** – Telegram-бот, который отслеживает изменения на веб-страницах и оперативно информирует пользователя о них.

## Как запустить (Windows PowerShell)

1. Создать файл `.env`.
2. Добавить в него:
```properties
APP_TELEGRAM_TOKEN=ваш_токен
BOT_SERVER_ADDR=:8081
SCRAPPER_SERVER_ADDR=:8080
GITHUB_TOKEN=ваш_токен
```
   GITHUB_TOKEN нужен, чтобы Scrapper корректно работал с GitHub API.
3. Запустить Scrapper (он должен работать до запуска бота):
   `go run cmd/scrapper/main.go`
4. В отдельном терминале запустить бота:
   `go run cmd/bot/main.go`

## Тестирование

Юнит-тесты:
`go test ./...`
Для детального покрытия:
`go test -v ./internal/application -cover`

Интеграционные тесты (требует Docker):
`go test -tags=integration -count=1 -timeout=15m -v ./testcontainers_test.go`
Или через make:
`make test-integration`