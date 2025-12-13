---
trigger: always_on
---

## Технологический стек 

- Go 1.22+ — конкурентность через goroutines (без async/await)
- net/http, context, goroutines — асинхронность
- go-telegram/bot — Telegram-клиент для бота
- Gin (+ swaggo/swag, gin-swagger) — создание REST API с автодокументацией
- net/http, resty, imroc/req — работа с HTTP
- rod или chromedp (+ goquery для разбора DOM) — парсинг/автоматизация браузера
- go-playground/validator (+ struct tags), строгая типизация — валидация и типизация данных
- gonum, go-gota/gota, gonum/plot или go-echarts — анализ и визуализация
- testing (stdlib), testify, ginkgo/gomega; покрытие: go test -cover — тестирование
- docker, go mod / go.sum — упаковка и управление зависимостями
- slog+tint+jsonhandler+lumberjack логирование централизованно

## Соглашение о структуре файлов и папок и разделении слоев

Следуй принципам Clean Architecture. Не делай крупных файлов. Вот пример структуры:

```text
yourbot/
├── go.mod

├── go.sum
├── .gitignore
├── .env.example
├── Makefile                 # сборка, тесты, линт
├── Dockerfile               # образ для деплоя
├── configs/                 # конфиги (yaml/json/toml)
│   └── config.example.yaml
├── migrations/              # SQL/инструмент миграций (golang-migrate и т.п.)
├── scripts/                 # локальные скрипты, генерация моков и т.д.
├── docs/                    # документация
├── cmd/
│   └── bot/
│       └── main.go          # composition root: загрузка конфигов, DI, запуск
├── internal/                # нереиспользуемый код (скрыт для внешних модулей)
│   ├── app/                 # сборка графа зависимостей (wire/fx/ручной DI)
│   │   └── app.go
│   ├── config/              # парсинг конфигов и .env
│   │   └── config.go
│   ├── domain/              # сущности, value-objects, доменные ошибки
│   │   ├── user.go
│   │   └── lesson.go
│   ├── usecase/             # приложение (бизнес-сценарии + ПОРТЫ = интерфейсы)
│   │   ├── conversation.go  # интерфейсы repo/gateway, юзкейсы
│   │   └── payment.go
│   ├── adapter/             # АДАПТЕРЫ (реализации портов и delivery)
│   │   ├── db/
│   │   │   └── postgres/    # реализации repo (инфраструктура БД)
│   │   │       ├── user_repo.go
│   │   │       └── lesson_repo.go
│   │   ├── external/
│   │   │   ├── openai/      # интеграции с внешними API
│   │   │   ├── qdrant/
│   │   │   ├── s3/
│   │   │   ├── stripe/
│   │   │   └── yookassa/
│   │   ├── telegram/        # delivery-слой: handlers, router, middleware
│   │   │   ├── bot.go
│   │   │   └── handlers/
│   │   │       ├── start.go
│   │   │       └── payments.go
│   │   └── scheduler/       # планировщик (cron/worker)
│   │       └── scheduler.go
│   ├── platform/            # технические детали (логгер, трейсинг, http, pg)
│   │   ├── logger/
│   │   │   └── logger.go
│   │   ├── http/
│   │   │   └── client.go
│   │   └── pg/
│   │       └── pool.go
│   └── shared/              # общие утилиты (минимум и без доменной логики)
│       └── errors.go
└── pkg/                     # опционально: реиспользуемые библиотеки (вне домена)
    └── retry/
        └── retry.go

# Тесты в тех же пакетах:
# internal/xxx/yyy_something_test.go
```