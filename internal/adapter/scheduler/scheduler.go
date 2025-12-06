package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// JobFunc представляет функцию задачи планировщика.
type JobFunc func(ctx context.Context) error

// CronJobID представляет идентификатор cron-задачи.
type CronJobID = cron.EntryID

// TickerJobID представляет идентификатор ticker-задачи.
type TickerJobID int

// OverlapPolicy определяет политику обработки перекрывающихся выполнений задач.
type OverlapPolicy int

const (
	// AllowOverlap разрешает параллельное выполнение задач (по умолчанию).
	AllowOverlap OverlapPolicy = iota
	// SkipIfRunning пропускает выполнение, если задача уже запущена.
	SkipIfRunning
	// DelayIfRunning ждет завершения предыдущего выполнения.
	DelayIfRunning
)

// JobOptions содержит опции для настройки задач.
type JobOptions struct {
	// Name - имя задачи для логирования (необязательно).
	Name string
	// Timeout - максимальное время выполнения задачи (необязательно).
	Timeout time.Duration
	// OverlapPolicy - политика обработки перекрывающихся выполнений.
	OverlapPolicy OverlapPolicy
}

// jobWrapper оборачивает задачу с её опциями.
type jobWrapper struct {
	job     JobFunc
	options JobOptions
	running sync.Mutex // для контроля перекрытий
}

// tickerJob содержит информацию о ticker-задаче.
type tickerJob struct {
	id      TickerJobID
	ticker  *time.Ticker
	cancel  context.CancelFunc
	wrapper *jobWrapper
}

// cronLogger адаптер для интеграции cron logger с slog.
type cronLogger struct {
	logger *slog.Logger
}

func (l cronLogger) Info(msg string, keysAndValues ...interface{}) {
	attrs := make([]slog.Attr, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := keysAndValues[i].(string)
			value := keysAndValues[i+1]
			attrs = append(attrs, slog.Any(key, value))
		}
	}
	l.logger.LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

func (l cronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	attrs := make([]slog.Attr, 0, len(keysAndValues)/2+1)
	attrs = append(attrs, slog.Any("error", err))
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := keysAndValues[i].(string)
			value := keysAndValues[i+1]
			attrs = append(attrs, slog.Any(key, value))
		}
	}
	l.logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
}

// Scheduler управляет периодическими задачами.
type Scheduler struct {
	cron         *cron.Cron
	logger       *slog.Logger
	hooks        JobHooks
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	tickerJobs   map[TickerJobID]*tickerJob
	nextTickerID TickerJobID
	mu           sync.Mutex
	stopOnce     sync.Once
	startOnce    sync.Once
}

// JobHooks содержит необязательные хуки для наблюдаемости.
type JobHooks struct {
	OnJobStart  func(jobName string)
	OnJobFinish func(jobName string, duration time.Duration, err error)
	OnJobError  func(jobName string, err error)
}

// Config содержит конфигурацию планировщика.
type Config struct {
	Logger   *slog.Logger
	JobHooks JobHooks
}

// New создает новый экземпляр планировщика с background контекстом.
func New(cfg Config) *Scheduler {
	return NewWithContext(context.Background(), cfg)
}

// NewWithContext создает новый экземпляр планировщика с указанным родительским контекстом.
func NewWithContext(parentCtx context.Context, cfg Config) *Scheduler {
	ctx, cancel := context.WithCancel(parentCtx)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Создаем cron с интегрированным логгером
	cronOpts := []cron.Option{
		cron.WithSeconds(),
		cron.WithLogger(cronLogger{logger: logger.With("component", "cron")}),
	}

	return &Scheduler{
		cron:         cron.New(cronOpts...),
		logger:       logger,
		hooks:        cfg.JobHooks,
		ctx:          ctx,
		cancel:       cancel,
		tickerJobs:   make(map[TickerJobID]*tickerJob),
		nextTickerID: 1,
	}
}

// AddCronJob добавляет задачу по cron-расписанию с опциями по умолчанию.
// Примеры расписаний:
//   - "0 30 * * * *" - каждые 30 минут
//   - "@hourly" - каждый час
//   - "@every 5m" - каждые 5 минут
func (s *Scheduler) AddCronJob(schedule string, job JobFunc) (CronJobID, error) {
	return s.AddCronJobWithOptions(schedule, job, JobOptions{})
}

// AddCronJobWithOptions добавляет задачу по cron-расписанию с указанными опциями.
func (s *Scheduler) AddCronJobWithOptions(schedule string, job JobFunc, opts JobOptions) (CronJobID, error) {
	wrapper := &jobWrapper{
		job:     job,
		options: opts,
	}

	// Создаем цепочку для обработки перекрытий
	var chain cron.Chain
	switch opts.OverlapPolicy {
	case SkipIfRunning:
		chain = cron.NewChain(cron.SkipIfStillRunning(cron.DefaultLogger))
	case DelayIfRunning:
		chain = cron.NewChain(cron.DelayIfStillRunning(cron.DefaultLogger))
	default: // AllowOverlap
		chain = cron.NewChain()
	}

	id, err := s.cron.AddJob(schedule, chain.Then(cron.FuncJob(func() {
		s.runJobWrapper(wrapper)
	})))
	if err != nil {
		s.logger.Error("failed to add cron job", "schedule", schedule, "name", opts.Name, "error", err)
		return 0, err
	}

	s.logger.Info("cron job added", "schedule", schedule, "name", opts.Name, "overlap_policy", opts.OverlapPolicy, "id", id)
	return id, nil
}

// AddTickerJob добавляет задачу с фиксированным интервалом с опциями по умолчанию.
func (s *Scheduler) AddTickerJob(interval time.Duration, job JobFunc) TickerJobID {
	return s.AddTickerJobWithOptions(interval, job, JobOptions{})
}

// AddTickerJobWithOptions добавляет задачу с фиксированным интервалом с указанными опциями.
func (s *Scheduler) AddTickerJobWithOptions(interval time.Duration, job JobFunc, opts JobOptions) TickerJobID {
	wrapper := &jobWrapper{
		job:     job,
		options: opts,
	}

	s.mu.Lock()
	id := s.nextTickerID
	s.nextTickerID++

	ticker := time.NewTicker(interval)
	ctx, cancel := context.WithCancel(s.ctx)

	tickerJob := &tickerJob{
		id:      id,
		ticker:  ticker,
		cancel:  cancel,
		wrapper: wrapper,
	}

	s.tickerJobs[id] = tickerJob
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer ticker.Stop()
		defer cancel()

		for {
			select {
			case <-ticker.C:
				s.runJobWrapper(wrapper)
			case <-ctx.Done():
				s.logger.Debug("ticker job stopped due to context cancellation", "name", opts.Name, "id", id)
				return
			}
		}
	}()

	s.logger.Info("ticker job added", "interval", interval, "name", opts.Name, "overlap_policy", opts.OverlapPolicy, "id", id)
	return id
}

// RemoveCronJob удаляет cron-задачу по ID.
func (s *Scheduler) RemoveCronJob(id CronJobID) {
	s.cron.Remove(id)
	s.logger.Info("cron job removed", "id", id)
}

// RemoveTickerJob удаляет ticker-задачу по ID.
func (s *Scheduler) RemoveTickerJob(id TickerJobID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, exists := s.tickerJobs[id]
	if !exists {
		return false
	}

	// Отменяем контекст задачи
	job.cancel()
	delete(s.tickerJobs, id)

	s.logger.Info("ticker job removed", "id", id, "name", job.wrapper.options.Name)
	return true
}

// Start запускает планировщик.
func (s *Scheduler) Start() {
	s.startOnce.Do(func() {
		s.logger.Info("starting scheduler")
		s.cron.Start()

		// Запускаем горутину для отслеживания контекста
		go func() {
			<-s.ctx.Done()
			s.logger.Info("stopping scheduler due to context cancellation")
			s.stopOnce.Do(s.stop)
		}()
	})
}

// Stop останавливает планировщик и ждет завершения всех задач.
func (s *Scheduler) Stop() {
	if !s.IsRunning() {
		return // Уже остановлен
	}
	s.logger.Info("stopping scheduler")
	s.cancel()
	s.stopOnce.Do(s.stop)
}

// StopContext останавливает планировщик с учетом контекста дедлайна.
// Если контекст истекает раньше, чем завершается graceful shutdown,
// планировщик все равно останавливается корректно.
func (s *Scheduler) StopContext(ctx context.Context) error {
	if !s.IsRunning() {
		return nil // Уже остановлен
	}

	s.logger.Info("stopping scheduler with deadline")
	s.cancel()

	// Запускаем остановку в отдельной горутине
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.stopOnce.Do(s.stop)
	}()

	// Ждем либо завершения остановки, либо истечения контекста
	select {
	case <-done:
		s.logger.Info("scheduler stopped gracefully within deadline")
		return nil
	case <-ctx.Done():
		s.logger.Warn("scheduler stop deadline exceeded, but shutdown will complete")
		// Все равно ждем завершения, но возвращаем ошибку
		<-done
		return ctx.Err()
	}
}

// stop выполняет фактическую остановку.
func (s *Scheduler) stop() {
	// Останавливаем cron
	ctx := s.cron.Stop()
	<-ctx.Done()

	// Останавливаем все ticker задачи
	s.mu.Lock()
	for _, job := range s.tickerJobs {
		job.cancel()
	}
	s.mu.Unlock()

	// Ждем завершения всех горутин
	s.wg.Wait()
	s.logger.Info("scheduler stopped")
}

// runJobWrapper выполняет задачу с учетом её опций.
func (s *Scheduler) runJobWrapper(wrapper *jobWrapper) {
	jobName := wrapper.options.Name
	if jobName == "" {
		jobName = "unnamed"
	}

	// Обработка политики перекрытий для ticker задач
	if wrapper.options.OverlapPolicy != AllowOverlap {
		if wrapper.options.OverlapPolicy == SkipIfRunning {
			if !wrapper.running.TryLock() {
				s.logger.Debug("skipping job execution, already running", "name", jobName)
				return
			}
			defer wrapper.running.Unlock()
		} else if wrapper.options.OverlapPolicy == DelayIfRunning {
			wrapper.running.Lock()
			defer wrapper.running.Unlock()
		}
	}

	// Вызываем хук начала задачи
	if s.hooks.OnJobStart != nil {
		s.hooks.OnJobStart(jobName)
	}

	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("panic: %v", r)
			s.logger.Error("job panicked", "name", jobName, "panic", r)
			if s.hooks.OnJobError != nil {
				s.hooks.OnJobError(jobName, panicErr)
			}
		}
	}()

	// Создаем контекст с таймаутом, если указан
	ctx := s.ctx
	var cancel context.CancelFunc
	if wrapper.options.Timeout > 0 {
		ctx, cancel = context.WithTimeout(s.ctx, wrapper.options.Timeout)
		defer cancel()
	}

	start := time.Now()
	err := wrapper.job(ctx)
	duration := time.Since(start)

	// Вызываем хук завершения задачи
	if s.hooks.OnJobFinish != nil {
		s.hooks.OnJobFinish(jobName, duration, err)
	}

	if err != nil {
		s.logger.Error("job failed", "name", jobName, "error", err, "duration", duration)
		if s.hooks.OnJobError != nil {
			s.hooks.OnJobError(jobName, err)
		}
	} else {
		s.logger.Debug("job completed successfully", "name", jobName, "duration", duration)
	}
}

// IsRunning возвращает true, если планировщик запущен.
func (s *Scheduler) IsRunning() bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
		return true
	}
}
