package pg

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// DSNConfig содержит параметры для построения DSN PostgreSQL.
type DSNConfig struct {
	Host     string // Хост базы данных (по умолчанию localhost)
	Port     int    // Порт базы данных (по умолчанию 5432)
	User     string // Имя пользователя
	Password string // Пароль пользователя
	Database string // Имя базы данных
	SSLMode  string // Режим SSL (disable, require, verify-ca, verify-full)

	// Дополнительные параметры
	ApplicationName string // Имя приложения для логов PostgreSQL
	ConnectTimeout  int    // Таймаут подключения в секундах

	// Произвольные параметры подключения
	ExtraParams map[string]string
}

// DefaultDSNConfig возвращает конфигурацию DSN с параметрами по умолчанию.
func DefaultDSNConfig() DSNConfig {
	return DSNConfig{
		Host:    "localhost",
		Port:    5432,
		SSLMode: "disable",
	}
}

// BuildDSN формирует строку подключения PostgreSQL из структурированных параметров.
//
// Пример результата:
// postgres://user:pass@localhost:5432/dbname?sslmode=disable&application_name=myapp
func BuildDSN(config DSNConfig) string {
	// Базовые обязательные параметры
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == 0 {
		config.Port = 5432
	}
	if config.SSLMode == "" {
		config.SSLMode = "disable"
	}

	// Формируем базовый URL
	var dsn strings.Builder
	dsn.WriteString("postgres://")

	// Добавляем пользователя и пароль если указаны
	if config.User != "" {
		dsn.WriteString(url.QueryEscape(config.User))
		if config.Password != "" {
			dsn.WriteString(":")
			dsn.WriteString(url.QueryEscape(config.Password))
		}
		dsn.WriteString("@")
	}

	// Добавляем хост и порт
	dsn.WriteString(config.Host)
	dsn.WriteString(":")
	dsn.WriteString(strconv.Itoa(config.Port))

	// Добавляем базу данных
	if config.Database != "" {
		dsn.WriteString("/")
		dsn.WriteString(url.QueryEscape(config.Database))
	}

	// Формируем query параметры
	params := url.Values{}

	// SSL режим
	params.Set("sslmode", config.SSLMode)

	// Дополнительные стандартные параметры
	if config.ApplicationName != "" {
		params.Set("application_name", config.ApplicationName)
	}
	if config.ConnectTimeout > 0 {
		params.Set("connect_timeout", strconv.Itoa(config.ConnectTimeout))
	}

	// Дополнительные произвольные параметры
	for key, value := range config.ExtraParams {
		if key != "" && value != "" {
			params.Set(key, value)
		}
	}

	// Добавляем параметры к DSN
	if len(params) > 0 {
		dsn.WriteString("?")
		dsn.WriteString(params.Encode())
	}

	return dsn.String()
}

// ParseDSN разбирает строку подключения PostgreSQL в структуру DSNConfig.
// Полезно для чтения существующих DSN и их модификации.
func ParseDSN(dsn string) (DSNConfig, error) {
	config := DSNConfig{
		ExtraParams: make(map[string]string),
	}

	// Парсим URL
	u, err := url.Parse(dsn)
	if err != nil {
		return config, fmt.Errorf("invalid DSN format: %w", err)
	}

	// Проверяем схему
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return config, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	// Извлекаем хост и порт
	config.Host = u.Hostname()
	if u.Port() != "" {
		config.Port, err = strconv.Atoi(u.Port())
		if err != nil {
			return config, fmt.Errorf("invalid port: %s", u.Port())
		}
	} else {
		config.Port = 5432 // порт по умолчанию
	}

	// Извлекаем пользователя и пароль
	if u.User != nil {
		config.User = u.User.Username()
		if password, hasPassword := u.User.Password(); hasPassword {
			config.Password = password
		}
	}

	// Извлекаем базу данных
	if u.Path != "" && u.Path != "/" {
		config.Database = strings.TrimPrefix(u.Path, "/")
	}

	// Извлекаем параметры запроса
	query := u.Query()

	config.SSLMode = query.Get("sslmode")
	if config.SSLMode == "" {
		config.SSLMode = "disable" // по умолчанию
	}

	config.ApplicationName = query.Get("application_name")

	if connectTimeoutStr := query.Get("connect_timeout"); connectTimeoutStr != "" {
		config.ConnectTimeout, _ = strconv.Atoi(connectTimeoutStr)
	}

	// Все остальные параметры сохраняем в ExtraParams
	knownParams := map[string]bool{
		"sslmode":          true,
		"application_name": true,
		"connect_timeout":  true,
	}

	for key, values := range query {
		if !knownParams[key] && len(values) > 0 {
			config.ExtraParams[key] = values[0] // берем первое значение
		}
	}

	return config, nil
}

// ValidateConfig проверяет корректность конфигурации DSN.
func ValidateConfig(config DSNConfig) error {
	if config.User == "" {
		return fmt.Errorf("user is required")
	}
	if config.Database == "" {
		return fmt.Errorf("database is required")
	}
	if config.Host == "" {
		return fmt.Errorf("host is required")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", config.Port)
	}

	validSSLModes := map[string]bool{
		"disable":     true,
		"allow":       true,
		"prefer":      true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[config.SSLMode] {
		return fmt.Errorf("invalid sslmode: %s", config.SSLMode)
	}

	if config.ConnectTimeout < 0 {
		return fmt.Errorf("connect_timeout cannot be negative")
	}

	return nil
}
