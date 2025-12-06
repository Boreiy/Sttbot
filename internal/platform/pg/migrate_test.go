package pg

import (
	"testing"
	"testing/fstest"
)

func TestApplyMigrations_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		dsn            string
		migrationsPath string
		expectError    bool
		testDesc       string
	}{
		{
			name:           "invalid_migrations_path",
			dsn:            "postgres://user:pass@localhost:5432/test?sslmode=disable",
			migrationsPath: "file://nonexistent",
			expectError:    true,
			testDesc:       "should fail with nonexistent migrations path",
		},
		{
			name:           "invalid_dsn",
			dsn:            "invalid-dsn",
			migrationsPath: "file://migrations",
			expectError:    true,
			testDesc:       "should fail with invalid DSN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ApplyMigrations(tt.dsn, tt.migrationsPath)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got nil", tt.testDesc)
			} else if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.testDesc, err)
			}
		})
	}
}

func TestApplyMigrationsLegacy_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	dsn := "postgres://user:pass@localhost:5432/test?sslmode=disable"
	migrationsPath := "file://nonexistent"

	err := ApplyMigrationsLegacy(dsn, migrationsPath)
	if err == nil {
		t.Error("expected error for nonexistent migrations path, got nil")
	}
}

// Этот тест теперь включен в TestApplyMigrations_ErrorCases

func TestGetMigrationVersion_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		dsn            string
		migrationsPath string
		expectError    bool
		testDesc       string
	}{
		{
			name:           "invalid_dsn",
			dsn:            "invalid-dsn",
			migrationsPath: "file://migrations",
			expectError:    true,
			testDesc:       "should fail with invalid DSN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := GetMigrationVersion(tt.dsn, tt.migrationsPath)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got nil", tt.testDesc)
			} else if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.testDesc, err)
			}
		})
	}
}

func TestGetMigrationVersionFromFS_InvalidDSN(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"migrations/001_init.up.sql": &fstest.MapFile{Data: []byte("CREATE TABLE test (id INT);")},
	}

	_, _, err := GetMigrationVersionFromFS("invalid-dsn", fsys, "migrations")
	if err == nil {
		t.Error("expected error for invalid DSN, got nil")
	}
}

func TestApplyMigrationsFromFS_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		dsn         string
		setupFS     func() fstest.MapFS
		dirName     string
		expectError bool
		testDesc    string
	}{
		{
			name: "empty_filesystem",
			dsn:  "postgres://user:pass@localhost:5432/test?sslmode=disable",
			setupFS: func() fstest.MapFS {
				return fstest.MapFS{}
			},
			dirName:     "migrations",
			expectError: true,
			testDesc:    "should fail with empty filesystem",
		},
		{
			name: "invalid_dsn_valid_fs",
			dsn:  "invalid-dsn",
			setupFS: func() fstest.MapFS {
				return fstest.MapFS{
					"migrations/001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE test (id INT);")},
					"migrations/001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE test;")},
				}
			},
			dirName:     "migrations",
			expectError: true,
			testDesc:    "should fail due to invalid DSN even with valid FS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys := tt.setupFS()
			_, err := ApplyMigrationsFromFS(tt.dsn, fsys, tt.dirName)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got nil", tt.testDesc)
			} else if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.testDesc, err)
			}

			// Дополнительная проверка для случая с невалидным DSN
			if err != nil && err.Error() == "" {
				t.Error("expected non-empty error message")
			}
		})
	}
}

func TestMigrationInfo_Structure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		field    string
		expected interface{}
		actual   func(MigrationInfo) interface{}
	}{
		{
			field:    "Applied",
			expected: true,
			actual:   func(info MigrationInfo) interface{} { return info.Applied },
		},
		{
			field:    "CurrentVersion",
			expected: uint(1),
			actual:   func(info MigrationInfo) interface{} { return info.CurrentVersion },
		},
		{
			field:    "FinalVersion",
			expected: uint(2),
			actual:   func(info MigrationInfo) interface{} { return info.FinalVersion },
		},
		{
			field:    "Dirty",
			expected: false,
			actual:   func(info MigrationInfo) interface{} { return info.Dirty },
		},
	}

	info := MigrationInfo{
		Applied:        true,
		CurrentVersion: 1,
		FinalVersion:   2,
		Dirty:          false,
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			actual := tt.actual(info)
			if actual != tt.expected {
				t.Errorf("%s = %v, want %v", tt.field, actual, tt.expected)
			}
		})
	}
}

// Интеграционные тесты для миграций требуют реальной БД
func TestApplyMigrations_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Реализовать с использованием testcontainers для полной изоляции
	t.Skip("integration test requires real PostgreSQL database and migrations")

	// Пример для реального тестирования:
	// dsn := "postgres://test:test@localhost:5432/test?sslmode=disable"
	// migrationsPath := "file://../../migrations"
	//
	// // Применяем миграции
	// err := ApplyMigrations(dsn, migrationsPath)
	// if err != nil {
	//     t.Fatalf("failed to apply migrations: %v", err)
	// }
	//
	// // Проверяем версию
	// version, dirty, err := GetMigrationVersion(dsn, migrationsPath)
	// if err != nil {
	//     t.Fatalf("failed to get migration version: %v", err)
	// }
	// if dirty {
	//     t.Error("migrations are in dirty state")
	// }
	// if version == 0 {
	//     t.Error("expected non-zero migration version")
	// }
	//
	// // Повторное применение должно пройти без ошибок (ErrNoChange игнорируется)
	// err = ApplyMigrations(dsn, migrationsPath)
	// if err != nil {
	//     t.Fatalf("failed to apply migrations second time: %v", err)
	// }
}
