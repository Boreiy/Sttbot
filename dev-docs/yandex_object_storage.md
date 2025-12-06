## Yandex Object Storage (S3-совместимое хранилище)

Документ — практический и самодостаточный. Покрывает: подключение к Yandex Object Storage через AWS SDK for Go, настройку аутентификации, основные операции с объектами, многочастичную загрузку, оптимизацию производительности, обработку ошибок и лучшие практики безопасности. Все примеры готовы к использованию.

### 1) Обзор сервиса

Yandex Object Storage — это объектное хранилище, полностью совместимое с Amazon S3 API. Это позволяет использовать существующие инструменты и SDK без изменений. Основные возможности:

- Хранение объектов любого типа и размера (до 5 ТБ за объект)
- Высокая доступность и надёжность данных
- Интеграция с другими сервисами Yandex Cloud
- Гибкая система управления доступом
- Поддержка версионирования объектов

Endpoint: `https://storage.yandexcloud.net`
Регион: `ru-central1`

### 2) Установка и зависимости

```bash
go get -u github.com/aws/aws-sdk-go
```

Альтернативно для новых проектов можно использовать AWS SDK v2:

```bash
go get -u github.com/aws/aws-sdk-go-v2
go get -u github.com/aws/aws-sdk-go-v2/service/s3
go get -u github.com/aws/aws-sdk-go-v2/config
go get -u github.com/aws/aws-sdk-go-v2/credentials
```

В примерах используется AWS SDK v1 как более стабильный и широко используемый.

### 3) Переменные окружения

```env
# .env.example для Object Storage
YC_ACCESS_KEY_ID=AKIA...           # Access Key ID сервисного аккаунта
YC_SECRET_ACCESS_KEY=wJalr...      # Secret Access Key сервисного аккаунта
YC_STORAGE_ENDPOINT=https://storage.yandexcloud.net
YC_STORAGE_REGION=ru-central1
YC_STORAGE_DEFAULT_BUCKET=your-bucket-name

# Дополнительные настройки
YC_STORAGE_TIMEOUT=30s             # таймаут операций
YC_STORAGE_MAX_RETRIES=3          # количество ретраев
YC_STORAGE_TLS_INSECURE=false     # отключить проверку TLS (только для dev)
```

### 4) Создание сервисного аккаунта и ключей

Пошаговая инструкция:

1. **Создание сервисного аккаунта**:
   - Перейдите в консоль Yandex Cloud
   - Выберите каталог → Сервисные аккаунты → Создать сервисный аккаунт
   - Назначьте роли: `storage.editor` или `storage.admin`

2. **Создание статических ключей доступа**:
   - В карточке сервисного аккаунта → Создать новый ключ → Создать статический ключ доступа
   - Сохраните `Access Key ID` и `Secret Access Key`

3. **Создание бакета** (через консоль или CLI):
   ```bash
   yc storage bucket create --name your-bucket-name --default-storage-class standard
   ```

### 5) Инициализация клиента

```go
package s3client

import (
    "context"
    "errors"
    "os"
    "strconv"
    "time"

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/credentials"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/s3"
)

type Config struct {
    AccessKeyID     string
    SecretAccessKey string
    Endpoint        string
    Region          string
    DefaultBucket   string
    Timeout         time.Duration
    MaxRetries      int
    TLSInsecure     bool
}

func LoadConfigFromEnv() (Config, error) {
    timeout, _ := time.ParseDuration(getenv("YC_STORAGE_TIMEOUT", "30s"))
    maxRetries, _ := strconv.Atoi(getenv("YC_STORAGE_MAX_RETRIES", "3"))
    tlsInsecure, _ := strconv.ParseBool(getenv("YC_STORAGE_TLS_INSECURE", "false"))

    cfg := Config{
        AccessKeyID:     os.Getenv("YC_ACCESS_KEY_ID"),
        SecretAccessKey: os.Getenv("YC_SECRET_ACCESS_KEY"),
        Endpoint:        getenv("YC_STORAGE_ENDPOINT", "https://storage.yandexcloud.net"),
        Region:          getenv("YC_STORAGE_REGION", "ru-central1"),
        DefaultBucket:   os.Getenv("YC_STORAGE_DEFAULT_BUCKET"),
        Timeout:         timeout,
        MaxRetries:      maxRetries,
        TLSInsecure:     tlsInsecure,
    }

    if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
        return Config{}, errors.New("YC_ACCESS_KEY_ID and YC_SECRET_ACCESS_KEY must be set")
    }

    return cfg, nil
}

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

type Client struct {
    s3  *s3.S3
    cfg Config
}

func New(cfg Config) (*Client, error) {
    awsCfg := &aws.Config{
        Region:           aws.String(cfg.Region),
        Endpoint:         aws.String(cfg.Endpoint),
        Credentials:      credentials.NewStaticCredentials(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
        S3ForcePathStyle: aws.Bool(true), // важно для Yandex Object Storage
        MaxRetries:       aws.Int(cfg.MaxRetries),
    }

    // Настройка таймаутов
    if cfg.Timeout > 0 {
        awsCfg.HTTPClient = &http.Client{
            Timeout: cfg.Timeout,
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{
                    InsecureSkipVerify: cfg.TLSInsecure,
                },
                MaxIdleConns:        100,
                MaxConnsPerHost:     100,
                MaxIdleConnsPerHost: 100,
                IdleConnTimeout:     90 * time.Second,
            },
        }
    }

    sess, err := session.NewSession(awsCfg)
    if err != nil {
        return nil, fmt.Errorf("create AWS session: %w", err)
    }

    return &Client{
        s3:  s3.New(sess),
        cfg: cfg,
    }, nil
}

// Health проверяет подключение к Object Storage
func (c *Client) Health(ctx context.Context) error {
    _, err := c.s3.ListBucketsWithContext(ctx, &s3.ListBucketsInput{})
    if err != nil {
        return fmt.Errorf("health check failed: %w", err)
    }
    return nil
}
```

### 6) Основные операции с объектами

#### 6.1 Загрузка объекта

```go
import (
    "bytes"
    "context"
    "fmt"
    "io"
    "mime"
    "os"
    "path/filepath"

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/service/s3"
)

// UploadFile загружает файл в бакет
func (c *Client) UploadFile(ctx context.Context, bucket, key, filePath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return fmt.Errorf("open file %s: %w", filePath, err)
    }
    defer file.Close()

    // Определение Content-Type
    contentType := mime.TypeByExtension(filepath.Ext(filePath))
    if contentType == "" {
        contentType = "application/octet-stream"
    }

    _, err = c.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
        Bucket:      aws.String(bucket),
        Key:         aws.String(key),
        Body:        file,
        ContentType: aws.String(contentType),
        ACL:         aws.String("private"), // по умолчанию приватный доступ
    })

    if err != nil {
        return fmt.Errorf("upload file to s3: %w", err)
    }

    return nil
}

// UploadBytes загружает данные из байтового среза
func (c *Client) UploadBytes(ctx context.Context, bucket, key string, data []byte, contentType string) error {
    if contentType == "" {
        contentType = "application/octet-stream"
    }

    _, err := c.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
        Bucket:        aws.String(bucket),
        Key:           aws.String(key),
        Body:          bytes.NewReader(data),
        ContentType:   aws.String(contentType),
        ContentLength: aws.Int64(int64(len(data))),
        ACL:           aws.String("private"),
    })

    if err != nil {
        return fmt.Errorf("upload bytes to s3: %w", err)
    }

    return nil
}

// UploadStream загружает данные из потока
func (c *Client) UploadStream(ctx context.Context, bucket, key string, reader io.Reader, size int64) error {
    _, err := c.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
        Bucket:        aws.String(bucket),
        Key:           aws.String(key),
        Body:          reader,
        ContentLength: aws.Int64(size),
        ACL:           aws.String("private"),
    })

    if err != nil {
        return fmt.Errorf("upload stream to s3: %w", err)
    }

    return nil
}
```

#### 6.2 Скачивание объекта

```go
import (
    "context"
    "fmt"
    "io"
    "io/ioutil"
    "os"

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/service/s3"
)

// DownloadFile скачивает объект в файл
func (c *Client) DownloadFile(ctx context.Context, bucket, key, filePath string) error {
    result, err := c.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        return fmt.Errorf("get object from s3: %w", err)
    }
    defer result.Body.Close()

    // Создание файла с созданием директорий
    if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
        return fmt.Errorf("create directories: %w", err)
    }

    file, err := os.Create(filePath)
    if err != nil {
        return fmt.Errorf("create file %s: %w", filePath, err)
    }
    defer file.Close()

    _, err = io.Copy(file, result.Body)
    if err != nil {
        return fmt.Errorf("copy data to file: %w", err)
    }

    return nil
}

// DownloadBytes скачивает объект в память
func (c *Client) DownloadBytes(ctx context.Context, bucket, key string) ([]byte, error) {
    result, err := c.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        return nil, fmt.Errorf("get object from s3: %w", err)
    }
    defer result.Body.Close()

    data, err := ioutil.ReadAll(result.Body)
    if err != nil {
        return nil, fmt.Errorf("read object body: %w", err)
    }

    return data, nil
}

// GetObjectStream получает поток для чтения объекта
func (c *Client) GetObjectStream(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
    result, err := c.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        return nil, fmt.Errorf("get object from s3: %w", err)
    }

    return result.Body, nil
}
```

#### 6.3 Управление объектами

```go
// ObjectInfo содержит метаданные объекта
type ObjectInfo struct {
    Key          string
    Size         int64
    LastModified time.Time
    ContentType  string
    ETag         string
}

// ListObjects возвращает список объектов в бакете
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string, maxKeys int) ([]ObjectInfo, error) {
    if maxKeys == 0 {
        maxKeys = 1000
    }

    result, err := c.s3.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
        Bucket:  aws.String(bucket),
        Prefix:  aws.String(prefix),
        MaxKeys: aws.Int64(int64(maxKeys)),
    })
    if err != nil {
        return nil, fmt.Errorf("list objects in s3: %w", err)
    }

    objects := make([]ObjectInfo, 0, len(result.Contents))
    for _, obj := range result.Contents {
        objects = append(objects, ObjectInfo{
            Key:          aws.StringValue(obj.Key),
            Size:         aws.Int64Value(obj.Size),
            LastModified: aws.TimeValue(obj.LastModified),
            ETag:         aws.StringValue(obj.ETag),
        })
    }

    return objects, nil
}

// DeleteObject удаляет объект
func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
    _, err := c.s3.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        return fmt.Errorf("delete object from s3: %w", err)
    }

    return nil
}

// CopyObject копирует объект
func (c *Client) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
    _, err := c.s3.CopyObjectWithContext(ctx, &s3.CopyObjectInput{
        Bucket:     aws.String(dstBucket),
        Key:        aws.String(dstKey),
        CopySource: aws.String(fmt.Sprintf("%s/%s", srcBucket, srcKey)),
        ACL:        aws.String("private"),
    })
    if err != nil {
        return fmt.Errorf("copy object in s3: %w", err)
    }

    return nil
}

// ObjectExists проверяет существование объекта
func (c *Client) ObjectExists(ctx context.Context, bucket, key string) (bool, error) {
    _, err := c.s3.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        if isNotFoundError(err) {
            return false, nil
        }
        return false, fmt.Errorf("check object existence: %w", err)
    }

    return true, nil
}
```

### 7) Многочастичная загрузка (Multipart Upload)

Для больших файлов (> 100 МБ) рекомендуется использовать multipart upload:

```go
import (
    "context"
    "fmt"
    "io"
    "os"

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/service/s3"
    "github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// UploadLargeFile загружает большой файл через multipart upload
func (c *Client) UploadLargeFile(ctx context.Context, bucket, key, filePath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return fmt.Errorf("open file %s: %w", filePath, err)
    }
    defer file.Close()

    // Создание uploader с кастомными настройками
    uploader := s3manager.NewUploaderWithClient(c.s3, func(u *s3manager.Uploader) {
        u.PartSize = 64 * 1024 * 1024 // 64 МБ на часть
        u.Concurrency = 3             // параллельность загрузки частей
        u.LeavePartsOnError = false   // удалять части при ошибке
    })

    // Определение Content-Type
    contentType := mime.TypeByExtension(filepath.Ext(filePath))
    if contentType == "" {
        contentType = "application/octet-stream"
    }

    _, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
        Bucket:      aws.String(bucket),
        Key:         aws.String(key),
        Body:        file,
        ContentType: aws.String(contentType),
        ACL:         aws.String("private"),
    })

    if err != nil {
        return fmt.Errorf("multipart upload to s3: %w", err)
    }

    return nil
}

// UploadStreamMultipart загружает поток через multipart upload
func (c *Client) UploadStreamMultipart(ctx context.Context, bucket, key string, reader io.Reader) error {
    uploader := s3manager.NewUploaderWithClient(c.s3, func(u *s3manager.Uploader) {
        u.PartSize = 32 * 1024 * 1024 // 32 МБ на часть
        u.Concurrency = 5             // больше параллельности для потоков
    })

    _, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
        Body:   reader,
        ACL:    aws.String("private"),
    })

    if err != nil {
        return fmt.Errorf("multipart stream upload to s3: %w", err)
    }

    return nil
}

// DownloadLargeFile скачивает большой файл через multipart download
func (c *Client) DownloadLargeFile(ctx context.Context, bucket, key, filePath string) error {
    file, err := os.Create(filePath)
    if err != nil {
        return fmt.Errorf("create file %s: %w", filePath, err)
    }
    defer file.Close()

    // Создание downloader с кастомными настройками
    downloader := s3manager.NewDownloaderWithClient(c.s3, func(d *s3manager.Downloader) {
        d.PartSize = 64 * 1024 * 1024 // 64 МБ на часть
        d.Concurrency = 3             // параллельность скачивания частей
    })

    _, err = downloader.DownloadWithContext(ctx, file, &s3.GetObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })

    if err != nil {
        return fmt.Errorf("multipart download from s3: %w", err)
    }

    return nil
}
```

### 8) Пресайнеры (Presigned URLs)

Для временного доступа к объектам без передачи ключей:

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/service/s3"
)

// GeneratePresignedURL создаёт временную ссылку для скачивания
func (c *Client) GeneratePresignedURL(bucket, key string, expiration time.Duration) (string, error) {
    req, _ := c.s3.GetObjectRequest(&s3.GetObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })

    url, err := req.Presign(expiration)
    if err != nil {
        return "", fmt.Errorf("generate presigned URL: %w", err)
    }

    return url, nil
}

// GeneratePresignedUploadURL создаёт временную ссылку для загрузки
func (c *Client) GeneratePresignedUploadURL(bucket, key string, expiration time.Duration) (string, error) {
    req, _ := c.s3.PutObjectRequest(&s3.PutObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })

    url, err := req.Presign(expiration)
    if err != nil {
        return "", fmt.Errorf("generate presigned upload URL: %w", err)
    }

    return url, nil
}
```

### 9) Обработка ошибок

```go
import (
    "errors"
    "fmt"

    "github.com/aws/aws-sdk-go/aws/awserr"
    "github.com/aws/aws-sdk-go/service/s3"
)

// Стандартные ошибки Object Storage
var (
    ErrObjectNotFound = errors.New("object not found")
    ErrBucketNotFound = errors.New("bucket not found")
    ErrAccessDenied   = errors.New("access denied")
    ErrQuotaExceeded  = errors.New("quota exceeded")
)

// isNotFoundError проверяет, является ли ошибка "не найдено"
func isNotFoundError(err error) bool {
    if aerr, ok := err.(awserr.Error); ok {
        switch aerr.Code() {
        case s3.ErrCodeNoSuchKey, "NotFound":
            return true
        }
    }
    return false
}

// parseS3Error преобразует ошибки S3 в доменные ошибки
func parseS3Error(err error) error {
    if err == nil {
        return nil
    }

    if aerr, ok := err.(awserr.Error); ok {
        switch aerr.Code() {
        case s3.ErrCodeNoSuchKey, "NotFound":
            return fmt.Errorf("%w: %s", ErrObjectNotFound, aerr.Message())
        case s3.ErrCodeNoSuchBucket:
            return fmt.Errorf("%w: %s", ErrBucketNotFound, aerr.Message())
        case "AccessDenied":
            return fmt.Errorf("%w: %s", ErrAccessDenied, aerr.Message())
        case "QuotaExceeded":
            return fmt.Errorf("%w: %s", ErrQuotaExceeded, aerr.Message())
        default:
            return fmt.Errorf("s3 error %s: %s", aerr.Code(), aerr.Message())
        }
    }

    return err
}

// SafeUpload загружает с обработкой ошибок и ретраями
func (c *Client) SafeUpload(ctx context.Context, bucket, key string, data []byte, maxRetries int) error {
    var lastErr error

    for attempt := 0; attempt <= maxRetries; attempt++ {
        err := c.UploadBytes(ctx, bucket, key, data, "")
        if err == nil {
            return nil
        }

        lastErr = parseS3Error(err)

        // Не ретраим AccessDenied и подобные ошибки
        if errors.Is(lastErr, ErrAccessDenied) || errors.Is(lastErr, ErrBucketNotFound) {
            return lastErr
        }

        if attempt < maxRetries {
            time.Sleep(time.Duration(attempt+1) * time.Second)
        }
    }

    return fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
}
```

### 10) Интеграция в Clean Architecture

Размещение в структуре проекта:

```
internal/
├── adapter/
│   └── external/
│       └── yandex_s3/           # адаптер Object Storage
│           ├── client.go        # клиент S3
│           ├── operations.go    # CRUD операции
│           └── multipart.go     # большие файлы
├── usecase/
│   └── file_storage.go          # интерфейс хранилища файлов
└── domain/
    └── file.go                  # доменные сущности
```

Доменный интерфейс:

```go
package domain

import (
    "context"
    "io"
    "time"
)

// FileStorage - порт для работы с файловым хранилищем
type FileStorage interface {
    Upload(ctx context.Context, path string, data []byte) error
    UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error
    Download(ctx context.Context, path string) ([]byte, error)
    DownloadStream(ctx context.Context, path string) (io.ReadCloser, error)
    Delete(ctx context.Context, path string) error
    Exists(ctx context.Context, path string) (bool, error)
    GenerateDownloadURL(path string, expiration time.Duration) (string, error)
}

// FileInfo содержит метаданные файла
type FileInfo struct {
    Path         string
    Size         int64
    ContentType  string
    LastModified time.Time
}
```

Реализация адаптера:

```go
package yandex_s3

import (
    "context"
    "io"
    "time"

    "yourbot/internal/domain"
)

type FileStorageAdapter struct {
    client *Client
    bucket string
}

func NewFileStorageAdapter(client *Client, bucket string) *FileStorageAdapter {
    return &FileStorageAdapter{
        client: client,
        bucket: bucket,
    }
}

func (a *FileStorageAdapter) Upload(ctx context.Context, path string, data []byte) error {
    return a.client.UploadBytes(ctx, a.bucket, path, data, "")
}

func (a *FileStorageAdapter) UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error {
    return a.client.UploadStream(ctx, a.bucket, path, reader, size)
}

func (a *FileStorageAdapter) Download(ctx context.Context, path string) ([]byte, error) {
    return a.client.DownloadBytes(ctx, a.bucket, path)
}

func (a *FileStorageAdapter) DownloadStream(ctx context.Context, path string) (io.ReadCloser, error) {
    return a.client.GetObjectStream(ctx, a.bucket, path)
}

func (a *FileStorageAdapter) Delete(ctx context.Context, path string) error {
    return a.client.DeleteObject(ctx, a.bucket, path)
}

func (a *FileStorageAdapter) Exists(ctx context.Context, path string) (bool, error) {
    return a.client.ObjectExists(ctx, a.bucket, path)
}

func (a *FileStorageAdapter) GenerateDownloadURL(path string, expiration time.Duration) (string, error) {
    return a.client.GeneratePresignedURL(a.bucket, path, expiration)
}
```

### 11) Композиция в main.go

```go
func main() {
    // ... инициализация логгера и конфига

    // Инициализация S3 клиента
    s3cfg, err := yandex_s3.LoadConfigFromEnv()
    if err != nil {
        log.Fatal("load s3 config", slog.Any("error", err))
    }

    s3Client, err := yandex_s3.New(s3cfg)
    if err != nil {
        log.Fatal("create s3 client", slog.Any("error", err))
    }

    // Проверка подключения
    if err := s3Client.Health(ctx); err != nil {
        log.Fatal("s3 health check", slog.Any("error", err))
    }

    // Создание адаптера
    fileStorage := yandex_s3.NewFileStorageAdapter(s3Client, s3cfg.DefaultBucket)

    // Передача в use-cases
    fileUseCase := usecase.NewFileService(fileStorage, log)

    // ...
}
```

### 12) Тестирование

#### 12.1 Интеграционные тесты

```go
package yandex_s3_test

import (
    "context"
    "os"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "yourbot/internal/adapter/external/yandex_s3"
)

func TestS3Integration(t *testing.T) {
    if os.Getenv("YC_ACCESS_KEY_ID") == "" {
        t.Skip("YC_ACCESS_KEY_ID not set")
    }

    cfg, err := yandex_s3.LoadConfigFromEnv()
    require.NoError(t, err)

    client, err := yandex_s3.New(cfg)
    require.NoError(t, err)

    ctx := context.Background()

    // Проверка здоровья
    err = client.Health(ctx)
    require.NoError(t, err)

    // Тест загрузки и скачивания
    testData := []byte("test data for s3")
    testKey := "test/integration-test.txt"

    // Загрузка
    err = client.UploadBytes(ctx, cfg.DefaultBucket, testKey, testData, "text/plain")
    require.NoError(t, err)

    // Проверка существования
    exists, err := client.ObjectExists(ctx, cfg.DefaultBucket, testKey)
    require.NoError(t, err)
    assert.True(t, exists)

    // Скачивание
    downloadedData, err := client.DownloadBytes(ctx, cfg.DefaultBucket, testKey)
    require.NoError(t, err)
    assert.Equal(t, testData, downloadedData)

    // Удаление
    err = client.DeleteObject(ctx, cfg.DefaultBucket, testKey)
    require.NoError(t, err)

    // Проверка удаления
    exists, err = client.ObjectExists(ctx, cfg.DefaultBucket, testKey)
    require.NoError(t, err)
    assert.False(t, exists)
}
```

#### 12.2 Юнит-тесты с моками

```go
package usecase_test

import (
    "context"
    "errors"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"

    "yourbot/internal/domain"
    "yourbot/internal/usecase"
)

type MockFileStorage struct {
    mock.Mock
}

func (m *MockFileStorage) Upload(ctx context.Context, path string, data []byte) error {
    args := m.Called(ctx, path, data)
    return args.Error(0)
}

func (m *MockFileStorage) Download(ctx context.Context, path string) ([]byte, error) {
    args := m.Called(ctx, path)
    return args.Get(0).([]byte), args.Error(1)
}

// ... остальные методы

func TestFileService_ProcessUpload(t *testing.T) {
    mockStorage := new(MockFileStorage)
    service := usecase.NewFileService(mockStorage, nil)

    testData := []byte("test data")
    testPath := "uploads/test.txt"

    mockStorage.On("Upload", mock.Anything, testPath, testData).Return(nil)

    err := service.ProcessUpload(context.Background(), testPath, testData)
    assert.NoError(t, err)

    mockStorage.AssertExpectations(t)
}
```

### 13) Оптимизация производительности

#### 13.1 Пулы соединений и Keep-Alive

```go
import (
    "crypto/tls"
    "net/http"
    "time"
)

func optimizedHTTPClient() *http.Client {
    return &http.Client{
        Timeout: 60 * time.Second,
        Transport: &http.Transport{
            // Пулы соединений
            MaxIdleConns:        100,
            MaxConnsPerHost:     100,
            MaxIdleConnsPerHost: 100,
            IdleConnTimeout:     90 * time.Second,
            
            // TLS настройки
            TLSClientConfig: &tls.Config{
                InsecureSkipVerify: false,
                MinVersion:         tls.VersionTLS12,
            },
            TLSHandshakeTimeout: 10 * time.Second,
            
            // HTTP настройки
            ResponseHeaderTimeout: 30 * time.Second,
            ExpectContinueTimeout: 1 * time.Second,
            
            // Keep-Alive
            DisableKeepAlives: false,
        },
    }
}
```

#### 13.2 Параллельная обработка

```go
// BatchUpload загружает несколько файлов параллельно
func (c *Client) BatchUpload(ctx context.Context, bucket string, files map[string][]byte, maxWorkers int) error {
    if maxWorkers == 0 {
        maxWorkers = 5
    }

    type job struct {
        key  string
        data []byte
    }

    jobs := make(chan job, len(files))
    results := make(chan error, len(files))

    // Запуск воркеров
    for w := 0; w < maxWorkers; w++ {
        go func() {
            for j := range jobs {
                err := c.UploadBytes(ctx, bucket, j.key, j.data, "")
                results <- err
            }
        }()
    }

    // Отправка задач
    for key, data := range files {
        jobs <- job{key: key, data: data}
    }
    close(jobs)

    // Сбор результатов
    var errors []error
    for i := 0; i < len(files); i++ {
        if err := <-results; err != nil {
            errors = append(errors, err)
        }
    }

    if len(errors) > 0 {
        return fmt.Errorf("batch upload failed: %d errors, first: %w", len(errors), errors[0])
    }

    return nil
}
```

### 14) Безопасность и лучшие практики

#### 14.1 Управление ключами доступа

```go
// Безопасная загрузка конфигурации
func LoadSecureConfig() (Config, error) {
    cfg := Config{}

    // Приоритет: переменные окружения > файл конфигурации > значения по умолчанию
    if accessKey := os.Getenv("YC_ACCESS_KEY_ID"); accessKey != "" {
        cfg.AccessKeyID = accessKey
    } else {
        return Config{}, errors.New("YC_ACCESS_KEY_ID must be set")
    }

    if secretKey := os.Getenv("YC_SECRET_ACCESS_KEY"); secretKey != "" {
        cfg.SecretAccessKey = secretKey
    } else {
        return Config{}, errors.New("YC_SECRET_ACCESS_KEY must be set")
    }

    // Проверка на случайные коммиты ключей
    if strings.Contains(cfg.AccessKeyID, "AKIA") && len(cfg.AccessKeyID) < 20 {
        return Config{}, errors.New("access key looks invalid")
    }

    return cfg, nil
}
```

#### 14.2 Политики доступа

```go
// ACLType определяет уровень доступа к объекту
type ACLType string

const (
    ACLPrivate      ACLType = "private"
    ACLPublicRead   ACLType = "public-read"
    ACLPublicWrite  ACLType = "public-read-write"
)

// SecureUpload загружает с явным указанием ACL
func (c *Client) SecureUpload(ctx context.Context, bucket, key string, data []byte, acl ACLType) error {
    _, err := c.s3.PutObjectWithContext(ctx, &s3.PutObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
        Body:   bytes.NewReader(data),
        ACL:    aws.String(string(acl)),
        // Шифрование на стороне сервера
        ServerSideEncryption: aws.String("AES256"),
    })

    return parseS3Error(err)
}
```

#### 14.3 Валидация и санитизация

```go
import (
    "path/filepath"
    "regexp"
    "strings"
)

var (
    // Разрешённые символы в имени объекта
    validKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9\-_./]+$`)
    
    // Запрещённые расширения
    forbiddenExts = map[string]bool{
        ".exe": true, ".bat": true, ".cmd": true,
        ".com": true, ".scr": true, ".pif": true,
    }
)

// ValidateKey проверяет корректность ключа объекта
func ValidateKey(key string) error {
    if key == "" {
        return errors.New("key cannot be empty")
    }

    if len(key) > 1024 {
        return errors.New("key too long")
    }

    if !validKeyRegex.MatchString(key) {
        return errors.New("key contains invalid characters")
    }

    if strings.Contains(key, "..") {
        return errors.New("key cannot contain path traversal")
    }

    ext := strings.ToLower(filepath.Ext(key))
    if forbiddenExts[ext] {
        return fmt.Errorf("forbidden file extension: %s", ext)
    }

    return nil
}

// SanitizeKey очищает и нормализует ключ объекта
func SanitizeKey(key string) string {
    // Удаление небезопасных символов
    key = regexp.MustCompile(`[^\w\-_./]`).ReplaceAllString(key, "_")
    
    // Нормализация путей
    key = filepath.Clean(key)
    
    // Удаление ведущих слешей
    key = strings.TrimPrefix(key, "/")
    
    return key
}
```

### 15) Мониторинг и метрики

```go
import (
    "context"
    "log/slog"
    "time"
)

// MetricsWrapper оборачивает клиент для сбора метрик
type MetricsWrapper struct {
    client *Client
    logger *slog.Logger
}

func NewMetricsWrapper(client *Client, logger *slog.Logger) *MetricsWrapper {
    return &MetricsWrapper{
        client: client,
        logger: logger,
    }
}

func (m *MetricsWrapper) UploadBytes(ctx context.Context, bucket, key string, data []byte, contentType string) error {
    start := time.Now()
    
    err := m.client.UploadBytes(ctx, bucket, key, data, contentType)
    
    duration := time.Since(start)
    
    m.logger.Info("s3_upload",
        slog.String("bucket", bucket),
        slog.String("key", key),
        slog.Int("size", len(data)),
        slog.Duration("duration", duration),
        slog.Bool("success", err == nil),
    )
    
    if err != nil {
        m.logger.Error("s3_upload_failed",
            slog.String("bucket", bucket),
            slog.String("key", key),
            slog.Any("error", err),
        )
    }
    
    return err
}
```

### 16) Чек-лист

- [ ] Настроены переменные окружения (`YC_ACCESS_KEY_ID`, `YC_SECRET_ACCESS_KEY`)
- [ ] Создан сервисный аккаунт с соответствующими ролями
- [ ] Клиент настроен с правильными endpoint и регионом
- [ ] Используется `S3ForcePathStyle: true` для совместимости
- [ ] Настроены таймауты и ретраи
- [ ] Обработка ошибок мапит S3-ошибки в доменные
- [ ] Для больших файлов используется multipart upload
- [ ] Ключи доступа не хранятся в коде
- [ ] Объекты создаются с правильными ACL
- [ ] Покрытие интеграционными тестами (отключены по умолчанию)
- [ ] Мониторинг операций через логирование
- [ ] Валидация ключей объектов на безопасность

---

Готово: полная интеграция с Yandex Object Storage через AWS SDK for Go с примерами всех основных операций, оптимизацией производительности, безопасностью и тестированием.
