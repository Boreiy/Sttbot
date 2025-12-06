package pg

import (
	"testing"
)

func TestDefaultDSNConfig(t *testing.T) {
	t.Parallel()

	config := DefaultDSNConfig()

	if config.Host != "localhost" {
		t.Errorf("expected Host=localhost, got %s", config.Host)
	}
	if config.Port != 5432 {
		t.Errorf("expected Port=5432, got %d", config.Port)
	}
	if config.SSLMode != "disable" {
		t.Errorf("expected SSLMode=disable, got %s", config.SSLMode)
	}
}

func TestBuildDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   DSNConfig
		expected string
	}{
		{
			name: "minimal_config",
			config: DSNConfig{
				User:     "testuser",
				Password: "testpass",
				Database: "testdb",
			},
			expected: "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "full_config",
			config: DSNConfig{
				Host:            "dbserver",
				Port:            5433,
				User:            "user",
				Password:        "pass",
				Database:        "mydb",
				SSLMode:         "require",
				ApplicationName: "myapp",
				ConnectTimeout:  30,
			},
			expected: "postgres://user:pass@dbserver:5433/mydb?application_name=myapp&connect_timeout=30&sslmode=require",
		},
		{
			name: "no_password",
			config: DSNConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Database: "testdb",
				SSLMode:  "disable",
			},
			expected: "postgres://user@localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "no_auth",
			config: DSNConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "testdb",
				SSLMode:  "disable",
			},
			expected: "postgres://localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "with_extra_params",
			config: DSNConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Database: "testdb",
				SSLMode:  "disable",
				ExtraParams: map[string]string{
					"search_path": "public,private",
					"timezone":    "UTC",
				},
			},
			expected: "postgres://user@localhost:5432/testdb?search_path=public%2Cprivate&sslmode=disable&timezone=UTC",
		},
		{
			name: "special_characters",
			config: DSNConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user@domain",
				Password: "p@ss w0rd!",
				Database: "test-db",
				SSLMode:  "disable",
			},
			expected: "postgres://user%40domain:p%40ss+w0rd%21@localhost:5432/test-db?sslmode=disable",
		},
		{
			name: "default_values_override",
			config: DSNConfig{
				User:     "user",
				Database: "db",
				// Host, Port, SSLMode не заданы - должны применяться defaults
			},
			expected: "postgres://user@localhost:5432/db?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := BuildDSN(tt.config)
			if result != tt.expected {
				t.Errorf("BuildDSN() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		dsn         string
		expected    DSNConfig
		expectError bool
	}{
		{
			name: "basic_dsn",
			dsn:  "postgres://user:pass@localhost:5432/testdb?sslmode=disable",
			expected: DSNConfig{
				Host:        "localhost",
				Port:        5432,
				User:        "user",
				Password:    "pass",
				Database:    "testdb",
				SSLMode:     "disable",
				ExtraParams: map[string]string{},
			},
			expectError: false,
		},
		{
			name: "with_extra_params",
			dsn:  "postgres://user@localhost:5432/db?sslmode=require&application_name=myapp&search_path=public",
			expected: DSNConfig{
				Host:            "localhost",
				Port:            5432,
				User:            "user",
				Database:        "db",
				SSLMode:         "require",
				ApplicationName: "myapp",
				ExtraParams: map[string]string{
					"search_path": "public",
				},
			},
			expectError: false,
		},
		{
			name: "no_port_uses_default",
			dsn:  "postgres://user@localhost/db?sslmode=disable",
			expected: DSNConfig{
				Host:        "localhost",
				Port:        5432, // default
				User:        "user",
				Database:    "db",
				SSLMode:     "disable",
				ExtraParams: map[string]string{},
			},
			expectError: false,
		},
		{
			name: "postgresql_scheme",
			dsn:  "postgresql://user@localhost/db",
			expected: DSNConfig{
				Host:        "localhost",
				Port:        5432,
				User:        "user",
				Database:    "db",
				SSLMode:     "disable", // default
				ExtraParams: map[string]string{},
			},
			expectError: false,
		},
		{
			name:        "invalid_scheme",
			dsn:         "mysql://user@localhost/db",
			expectError: true,
		},
		{
			name:        "invalid_url",
			dsn:         "not-a-url",
			expectError: true,
		},
		{
			name:        "invalid_port",
			dsn:         "postgres://user@localhost:abc/db",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ParseDSN(tt.dsn)

			if tt.expectError {
				if err == nil {
					t.Errorf("ParseDSN() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDSN() unexpected error: %v", err)
				return
			}

			if result.Host != tt.expected.Host {
				t.Errorf("Host = %q, want %q", result.Host, tt.expected.Host)
			}
			if result.Port != tt.expected.Port {
				t.Errorf("Port = %d, want %d", result.Port, tt.expected.Port)
			}
			if result.User != tt.expected.User {
				t.Errorf("User = %q, want %q", result.User, tt.expected.User)
			}
			if result.Password != tt.expected.Password {
				t.Errorf("Password = %q, want %q", result.Password, tt.expected.Password)
			}
			if result.Database != tt.expected.Database {
				t.Errorf("Database = %q, want %q", result.Database, tt.expected.Database)
			}
			if result.SSLMode != tt.expected.SSLMode {
				t.Errorf("SSLMode = %q, want %q", result.SSLMode, tt.expected.SSLMode)
			}
			if result.ApplicationName != tt.expected.ApplicationName {
				t.Errorf("ApplicationName = %q, want %q", result.ApplicationName, tt.expected.ApplicationName)
			}

			// Проверяем ExtraParams
			for key, expectedValue := range tt.expected.ExtraParams {
				if actualValue, exists := result.ExtraParams[key]; !exists || actualValue != expectedValue {
					t.Errorf("ExtraParams[%q] = %q, want %q", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      DSNConfig
		expectError bool
		errorText   string
	}{
		{
			name: "valid_config",
			config: DSNConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Database: "db",
				SSLMode:  "disable",
			},
			expectError: false,
		},
		{
			name: "missing_user",
			config: DSNConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "db",
				SSLMode:  "disable",
			},
			expectError: true,
			errorText:   "user is required",
		},
		{
			name: "missing_database",
			config: DSNConfig{
				Host:    "localhost",
				Port:    5432,
				User:    "user",
				SSLMode: "disable",
			},
			expectError: true,
			errorText:   "database is required",
		},
		{
			name: "missing_host",
			config: DSNConfig{
				Port:     5432,
				User:     "user",
				Database: "db",
				SSLMode:  "disable",
			},
			expectError: true,
			errorText:   "host is required",
		},
		{
			name: "invalid_port_zero",
			config: DSNConfig{
				Host:     "localhost",
				Port:     0,
				User:     "user",
				Database: "db",
				SSLMode:  "disable",
			},
			expectError: true,
			errorText:   "port must be between 1 and 65535",
		},
		{
			name: "invalid_port_too_high",
			config: DSNConfig{
				Host:     "localhost",
				Port:     65536,
				User:     "user",
				Database: "db",
				SSLMode:  "disable",
			},
			expectError: true,
			errorText:   "port must be between 1 and 65535",
		},
		{
			name: "invalid_sslmode",
			config: DSNConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Database: "db",
				SSLMode:  "invalid",
			},
			expectError: true,
			errorText:   "invalid sslmode",
		},
		{
			name: "negative_timeout",
			config: DSNConfig{
				Host:           "localhost",
				Port:           5432,
				User:           "user",
				Database:       "db",
				SSLMode:        "disable",
				ConnectTimeout: -1,
			},
			expectError: true,
			errorText:   "connect_timeout cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateConfig(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateConfig() expected error, got nil")
					return
				}
				if tt.errorText != "" && !contains(err.Error(), tt.errorText) {
					t.Errorf("ValidateConfig() error = %q, want to contain %q", err.Error(), tt.errorText)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateConfig() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBuildDSNParseRoundTrip(t *testing.T) {
	t.Parallel()

	// Проверяем, что Build -> Parse возвращает исходную конфигурацию
	originalConfig := DSNConfig{
		Host:            "testhost",
		Port:            5433,
		User:            "testuser",
		Password:        "testpass",
		Database:        "testdb",
		SSLMode:         "require",
		ApplicationName: "testapp",
		ConnectTimeout:  60,
		ExtraParams: map[string]string{
			"search_path": "public",
			"timezone":    "UTC",
		},
	}

	dsn := BuildDSN(originalConfig)
	parsedConfig, err := ParseDSN(dsn)

	if err != nil {
		t.Fatalf("ParseDSN() error: %v", err)
	}

	if parsedConfig.Host != originalConfig.Host {
		t.Errorf("Host mismatch: got %q, want %q", parsedConfig.Host, originalConfig.Host)
	}
	if parsedConfig.Port != originalConfig.Port {
		t.Errorf("Port mismatch: got %d, want %d", parsedConfig.Port, originalConfig.Port)
	}
	if parsedConfig.User != originalConfig.User {
		t.Errorf("User mismatch: got %q, want %q", parsedConfig.User, originalConfig.User)
	}
	if parsedConfig.Password != originalConfig.Password {
		t.Errorf("Password mismatch: got %q, want %q", parsedConfig.Password, originalConfig.Password)
	}
	if parsedConfig.Database != originalConfig.Database {
		t.Errorf("Database mismatch: got %q, want %q", parsedConfig.Database, originalConfig.Database)
	}
	if parsedConfig.SSLMode != originalConfig.SSLMode {
		t.Errorf("SSLMode mismatch: got %q, want %q", parsedConfig.SSLMode, originalConfig.SSLMode)
	}
	if parsedConfig.ApplicationName != originalConfig.ApplicationName {
		t.Errorf("ApplicationName mismatch: got %q, want %q", parsedConfig.ApplicationName, originalConfig.ApplicationName)
	}
	if parsedConfig.ConnectTimeout != originalConfig.ConnectTimeout {
		t.Errorf("ConnectTimeout mismatch: got %d, want %d", parsedConfig.ConnectTimeout, originalConfig.ConnectTimeout)
	}

	// Проверяем ExtraParams
	for key, expectedValue := range originalConfig.ExtraParams {
		if actualValue, exists := parsedConfig.ExtraParams[key]; !exists || actualValue != expectedValue {
			t.Errorf("ExtraParams[%q] mismatch: got %q, want %q", key, actualValue, expectedValue)
		}
	}
}

// contains проверяет, содержит ли строка подстроку
func contains(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) && (s == substr || s[0:len(substr)] == substr || contains(s[1:], substr))
}
