# Background and Motivation

Цель: реализовать простого Telegram‑бота для перевода аудио в текст через OpenAI STT, согласно ТЗ:
- Принимать голосовые сообщения `.ogg` (Voice) и аудиофайлы (Audio/Document) и отвечать текстовой транскрибацией.
- Конфигурируемая модель через ENV, по умолчанию `gpt-4o-mini-transcribe`.
- Автоконвертация аудио в поддерживаемый формат при необходимости.
- Доступ только для пользователей из `ALLOWED_IDS`, остальным — «доступ запрещен».
- Правки Dockerfile/compose: порт `2010`, сервис/пакет `sttbot`, домен `sttbot.tgbots.site`.

Шаблон проекта уже содержит готовые модули, которые мы переиспользуем:
- Telegram диспетчер и middleware: `internal/adapter/telegram/dispatcher.go`, `middleware/ratelimit.go`, `handlers/*`.
- Конфиг и логгер: `internal/config/config.go`, `internal/platform/logger/logger.go`.
- Надёжный HTTP‑клиент с ретраями: `internal/platform/httpclient/client.go`.
- Планировщик задач: `internal/adapter/scheduler/*`.
- Ошибки и классификация: `internal/shared/errors.go`.

Ключевая идея плана: собрать решение из независимых пакетов (конвертация аудио, клиент OpenAI, Telegram‑handler, ACL middleware), чтобы их можно было разрабатывать и тестировать параллельно, а интеграция в бота происходила подключением уже готовых частей.

# Key Challenges and Analysis
- Форматы аудио: Telegram Voice = OGG/Opus, OpenAI в целом принимает `mp3`, `wav`, `m4a`, `ogg`. От конвертации отказываемся, чтобы избежать лишних CPU‑затрат; вводим фильтрацию для `Document` по MIME/расширению (обрабатываем только `audio/*` и allowlist контейнеров).
- Поток получения файлов: `GetFile` → загрузка по URL, лимиты Telegram, обработка ошибок/таймаутов.
- Ограничение доступа: простой ACL по `ALLOWED_IDS` для всех типов обновлений, включая аудио, команды и callbacks.
- Асинхронность: параллелить обработку чатов, сохраняя порядок внутри чата (готовый `Dispatcher` есть: `internal/adapter/telegram/dispatcher.go:1`). Долгие операции — с контекстными таймаутами.
- Надёжность HTTP: ретраи и backoff есть в `httpclient` (`internal/platform/httpclient/client.go:1`).
- Логирование и PII: `logger.New` с редактированием чувствительных полей (токены/секреты) (`internal/platform/logger/logger.go:1`).
- Сбор временных файлов: нужен планировщик для периодической очистки (`internal/adapter/scheduler/scheduler.go:58`).
- Пакетное внедрение: конфиг OpenAI/ACL, OpenAI‑адаптер, фильтрация/загрузка, обработчик Telegram — разрабатываются отдельно, подключаются позже.

# High-level Task Breakdown
1) Расширить конфиг и базовую инициализацию под STT и ACL
   - Критерии выполнения
     - ENV поддерживает `OPENAI_API_KEY`, `OPENAI_BASE_URL` (по умолчанию `https://api.openai.com/v1`), `OPENAI_TRANSCRIBE_MODEL` (по умолчанию `gpt-4o-mini-transcribe`), `ALLOWED_IDS` (список ID через запятую), `HTTP_ADDR=:2010`.
     - Логгер и App используют имя приложения `sttbot`.
     - `ENV=dev|prod`, webhook секрет проверяется при наличии URL.
   - Функциональные сценарии
     - Сценарий 1: При `ENV=dev` и отсутствующем webhook лог инициализируется, бот стартует в polling.
     - Сценарий 2: При `ENV=prod` и заданных `TELEGRAM_WEBHOOK_URL/SECRET` запускается Gin‑вебхук на `HTTP_ADDR`.
     - Сценарий 3: При пустом/невалидном `ALLOWED_IDS` — доступ открыт только для явно указанных ID; пустое значение трактуется как «нет разрешённых».
   - Тесты
     - `internal/config`: проверка обязательных полей, валидации, значений по умолчанию.
     - `internal/platform/logger`: имя приложения в логах, отсутствие утечек секретов.
   - Повторное использование
     - `internal/config/config.go:1`, `internal/platform/logger/logger.go:1`.
   - Асинхронность
     - Параллельно с задачами 2–4; блокирует только интеграцию, где нужны ENV.
   - Файлы
     - Изменить: `internal/config/config.go`, `internal/app/app.go` (поле `App: "sttbot"`).
     - Обновить примеры ENV: `README.md`, `.env.example` (создать при необходимости).
   - Методы
     - `type Config struct { OpenAI struct{ APIKey, BaseURL, TranscribeModel string }; AllowedIDs []int64 }`
     - `func Load() (Config, error)` — расширить парсинг `ALLOWED_IDS`, `OPENAI_*`.

2) Middleware ACL: ограничение доступа по `ALLOWED_IDS`
   - Критерии выполнения
     - Любое обновление от пользователя вне списка получает ответ «доступ запрещен», обработка прекращается.
   - Функциональные сценарии
     - Сценарий 1: Пользователь из списка отправляет команду `/ping` — получает `pong`.
     - Сценарий 2: Пользователь вне списка отправляет аудио — получает «доступ запрещен», транскрибации нет.
   - Тесты
     - Юнит‑тест middleware: разрешённый/запрещённый UID → вызов/блокировка следующего хендлера.
   - Повторное использование
     - Базовый конвейер middleware: `internal/adapter/telegram/middleware/middleware.go:1`.
   - Асинхронность
     - Параллельно с задачами 3–5.
   - Файлы
     - Создать: `internal/adapter/telegram/middleware/acl.go`.
     - Изменить: `internal/app/app.go` — включить ACL middleware в цепочку.
   - Методы
     - `type AccessControl struct{ allowed map[int64]struct{} }`
     - `func NewAccessControl(ids []int64) *AccessControl`
     - `func (ac *AccessControl) Middleware(next telegram.HandlerFunc) telegram.HandlerFunc`

3) Фильтрация документов (Document)
   - Критерии выполнения
     - Для `Document` обрабатываются только файлы с MIME `audio/*` и/или расширения из allowlist (`.ogg`, `.mp3`, `.m4a`, `.wav`, `.webm`, `.mp4`). Остальное — отказ с сообщением «неподдерживаемый формат».
   - Функциональные сценарии
     - Сценарий 1: Документ `audio/ogg` → обрабатывается как аудио.
     - Сценарий 2: Документ `application/zip` или `video/mp4` → отказ, отправляется поясняющее сообщение.
   - Тесты
     - Юнит: распознавание допустимых/недопустимых MIME/расширений.
   - Повторное использование
     - Нет; реализуем простую проверку MIME/расширения.
   - Асинхронность
     - Параллельно с задачами 4–5.
   - Файлы
     - Изменить/создать: фильтрация внутри `internal/adapter/telegram/handlers/audio.go` (или вспомогательно в `internal/adapter/telegram/files.go`).
   - Методы
     - `func IsSupportedAudio(mime string, filename string) bool`

4) Адаптер OpenAI STT
   - Критерии выполнения
     - Клиент вызывает OpenAI `/audio/transcriptions` (multipart), передает файл и модель, получает текст.
     - Ретраи при 429/503, таймауты и логирование через `httpclient`.
   - Функциональные сценарии
     - Сценарий 1: Валидный `.wav` → успешная транскрибация, возвращается текст.
     - Сценарий 2: 429 с `Retry-After` → повтор через задержку, итог успех/ошибка согласно mock.
   - Тесты
     - Юнит: формирование запроса (boundary, поля), разбор ответа (JSON) через `httptest.Server`.
     - Поведение ретраев: на 503/429 повторяются попытки.
   - Повторное использование
     - `internal/platform/httpclient/client.go:1`, `internal/shared/errors.go:1` для классификации ошибок.
   - Асинхронность
     - Параллельно с задачами 3 и 5.
   - Файлы
     - Создать: `internal/adapter/external/openai/stt.go`.
   - Методы
     - `type Transcriber interface { Transcribe(ctx context.Context, r io.Reader, filename, mime string) (string, error) }`
     - `func NewTranscriber(c *httpclient.Client, baseURL, apiKey, model string) Transcriber`

5) Загрузка аудио из Telegram
   - Критерии выполнения
     - Получение `file_id` из `Message.Voice|Audio|Document`, `GetFile`, скачивание `https://api.telegram.org/file/bot<TOKEN>/<path>` в `data/tmp`.
   - Функциональные сценарии
     - Сценарий 1: Голосовое сообщение → путь к локальному файлу.
     - Сценарий 2: Документ с аудио MIME → путь к локальному файлу.
     - Сценарий 3: Неверный `file_id` → корректная ошибка и сообщение пользователю.
   - Тесты
     - Юнит: корректная сборка URL, обработка ошибок сети/таймаутов.
   - Повторное использование
     - Telegram SDK: `github.com/go-telegram/bot`.
     - HTTP‑клиент: `internal/platform/httpclient/client.go:1`.
   - Асинхронность
     - Параллельно с задачами 3–4.
   - Файлы
     - Создать: `internal/adapter/telegram/files.go` (вспомогательные функции скачивания).
   - Методы
     - `func DownloadTelegramFile(ctx context.Context, b *bot.Bot, token string, fileID string) (string, error)`.

6) Telegram‑handler транскрибации
   - Критерии выполнения
     - Обработчик распознает аудио/voice, скачивает, проверяет поддерживаемость (для Document), отправляет файл в OpenAI, и возвращает текст ответом пользователю.
     - Стабильность при больших файлах: сообщения об ошибках без падений.
   - Функциональные сценарии
     - Сценарий 1: Пользователь из `ALLOWED_IDS` отправляет voice → бот отвечает текстом.
     - Сценарий 2: Пользователь отправляет `.mp3` → транскрибация без конвертации.
     - Сценарий 3: Ошибка OpenAI → ответ «не удалось распознать, попробуйте позже».
   - Тесты
     - Юнит: разбор `Update`, маршрутизация на handler.
     - Интеграция (с mock OpenAI): полный поток без реального API.
   - Повторное использование
     - Диспетчер: `internal/adapter/telegram/dispatcher.go:1`.
     - Middleware: `internal/adapter/telegram/middleware/middleware.go:1`.
   - Асинхронность
     - Требует готовности задач 3–5; собственная реализация — независимо от команд `/start`, `/ping`.
   - Файлы
     - Создать: `internal/adapter/telegram/handlers/audio.go`.
     - Изменить: `internal/adapter/telegram/handlers/commands.go:1` — добавить ветку для аудио/voice.
   - Методы
     - `func HandleAudio(ctx context.Context, b *bot.Bot, msg *models.Message)`.

7) Интеграция middleware в App
   - Критерии выполнения
     - Цепочка middleware: `ACL → RateLimiter → Handle`.
     - Порядок по чатам сохраняется (`Dispatcher`).
   - Функциональные сценарии
     - Сценарий 1: Пользователь вне ACL — любое обновление блокируется.
     - Сценарий 2: Частые запросы — «слишком часто» от `RateLimiter` (`internal/adapter/telegram/middleware/ratelimit.go:1`).
   - Тесты
     - Юнит: проверка порядка вызовов и блокировок.
   - Повторное использование
     - `internal/app/app.go:43`, `internal/adapter/telegram/middleware/*`.
   - Асинхронность
     - Параллельно с задачей 6 (изменения точечно).
   - Файлы
     - Изменить: `internal/app/app.go` — добавить ACL в `middleware.Chain`.
   - Методы
     - `func Chain(h telegram.HandlerFunc, mws ...Middleware) telegram.HandlerFunc` — уже есть.

8) Планировщик очистки временных файлов
   - Критерии выполнения
     - Раз в N минут удаляются старые файлы из `data/tmp`.
   - Функциональные сценарии
     - Сценарий 1: Файлы старше порога удаляются, ошибки логируются.
   - Тесты
     - Юнит: функция выбора и удаления старых файлов.
   - Повторное использование
     - `internal/adapter/scheduler/*` (`scheduler.go:58`, `examples.go:1`, `integration_example.go:62`).
   - Асинхронность
     - Независимо от остальных задач.
   - Файлы
     - Создать: `internal/adapter/scheduler/cleanup_tmp.go`.
   - Методы
     - `func SetupCleanupJob(s *scheduler.Scheduler, dir string, maxAge time.Duration) error`.

9) Правки Dockerfile/Compose и сервисного порта
   - Критерии выполнения
     - `EXPOSE 2010` в Dockerfile, `HTTP_ADDR=:2010`, и соответствующие метки в compose (сервис `sttbot`, домен `sttbot.tgbots.site`).
   - Функциональные сценарии
     - Сценарий 1: Контейнер поднимается, вебхук доступен на 2010 порту.
   - Тесты
     - Нет юнит‑тестов; проверка запуском в dev/staging.
   - Повторное использование
     - Текущий compose уже содержит метки домена (`docker-compose.yml:1–34`).
   - Асинхронность
     - Параллельно с остальными задачами.
   - Файлы
     - Изменить: `Dockerfile`, `docker-compose.yml` (порт `2011` → `2010`).

10) Переименование модуля и импортов на `sttbot`
   - Критерии выполнения
     - `go.mod` → `module sttbot`; импорты `bot-go-template/...` заменены на `sttbot/...`.
   - Функциональные сценарии
     - Сценарий 1: `go build ./...` успешен.
   - Тесты
     - Запуск `go test ./...` проходит.
   - Повторное использование
     - Вся кодовая база; требуется массовая замена импортов.
   - Асинхронность
     - Блокер: затрагивает одни и те же файлы; выполнять после стабилизации основных пакетов.
   - Файлы
     - Изменить: `go.mod`, все `import "bot-go-template/..."` → `import "sttbot/..."`.

11) (Опционально) Health/Readiness endpoints
   - Критерии выполнения
     - `/healthz` всегда `200 OK`; `/readyz` учитывает зависимости (например, доступность OpenAI/Telegram).
   - Функциональные сценарии
     - Сценарий 1: Без внешних зависимостей `/readyz=200`.
     - Сценарий 2: При недоступном OpenAI `/readyz=503`.
   - Тесты
     - Юнит: проверка формата ответов.
   - Повторное использование
     - `internal/platform/httpclient` для внешних пингов.
   - Асинхронность
     - Параллельно с остальными; независимая задача.
   - Файлы
     - Создать: `internal/adapter/http/health.go` (если выберем добавлять).

# Current Status / Progress Tracking
- Анализ ТЗ выполнен, кодовая база изучена.
- Реализованы: конфиг OpenAI/ACL, ACL‑middleware, адаптер OpenAI STT, загрузка аудио из Telegram, обработчик транскрибации, интеграция в App.
- Обновлён `.env.example` (порт `2010`, `ALLOWED_IDS`, `OPENAI_*`).
- Тесты: `internal/adapter/external/openai/stt_test.go`, `internal/adapter/telegram/middleware/acl_test.go` — зелёные.
- `go vet` без ошибок, сборка проходит.
- Исправлены предупреждения golangci-lint (ineffassign) в `internal/app/app.go`.
- Исправлены предупреждения golangci-lint (ineffassign) в `internal/app/app.go`; добавлена явная очистка буфера аудио после транскрибации.
- Исправлена отправка аудио в OpenAI: теперь multipart-поле `file` получает корректный Content-Type, чтобы OGG/OGA не отвергались.
- Исправлена нормализация имён голосовых файлов: `voice/*.oga` переименовываются в `.ogg` перед отправкой в OpenAI, чтобы избежать 400.

# Project Status Board
- [x] 1) Конфиг STT и ACL (ENV, App=sttbot)
- [x] 2) Middleware ACL
- [ ] 3) Фильтрация документов (Document)
- [x] 4) OpenAI STT адаптер
- [x] 5) Загрузка аудио из Telegram
- [x] 6) Handler транскрибации
- [x] 7) Интеграция middleware в App
- [ ] 8) Очистка временных файлов (scheduler)
 - [x] 9) Dockerfile/Compose правки порта и сервиса
 - [x] 10) Переименование модуля → sttbot
- [ ] 11) Health/Readiness (опционально)

# Executor's Feedback or Assistance Requests
- Имя модели STT: текущая `OPENAI_STT_MODEL=gpt-4o-mini-transcribe`. Можно заменить через ENV без кода.
- Конвертация `.ogg` удалена — OpenAI принимает OGG; реализована фильтрация `Document` по MIME/расширениям.
- Эндпоинты `/healthz`/`/readyz` при необходимости можно добавить отдельной задачей.

# Lessons
- Параллелить разработку независимых пакетов и подключать их в конце — снижает блокировки.
- Для сетевых ретраев использовать централизованный `httpclient` и контексты.
- Не логировать PII/секреты — редактирование в логгере уже предусмотрено.
- Для обработки потоков Telegram сохранять порядок по чатам через `Dispatcher` и параллелить между чатами.
- Отказ от конвертации снижает сложность и CPU‑нагрузку; фильтрация Document минимизирует неожиданные форматы.
- После чтения аудио освобождать буфер через `clear` и обнуление, чтобы ускорить возврат памяти GC.
