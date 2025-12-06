package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitForAtLeast(t *testing.T, counter *int64, expected int64, timeout time.Duration) {
	t.Helper()

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(counter) >= expected
	}, timeout, 10*time.Millisecond, "значение счётчика не достигло ожидаемого уровня")
}

func ensureNoIncrement(t *testing.T, counter *int64, baseline int64, duration time.Duration) {
	t.Helper()

	assert.Never(t, func() bool {
		return atomic.LoadInt64(counter) > baseline
	}, duration, 10*time.Millisecond, "счётчик увеличился после ожидания")
}

func TestScheduler_New(t *testing.T) {
	logger := slog.Default()
	s := New(Config{Logger: logger})

	assert.NotNil(t, s)
	assert.NotNil(t, s.cron)
	assert.NotNil(t, s.logger)
	assert.True(t, s.IsRunning())
}

func TestScheduler_NewWithoutLogger(t *testing.T) {
	s := New(Config{})

	assert.NotNil(t, s)
	assert.NotNil(t, s.logger)
}

func TestScheduler_AddCronJob(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var counter int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&counter, 1)
		return nil
	}

	_, err := s.AddCronJob("@every 100ms", job)
	require.NoError(t, err)

	s.Start()

	waitForAtLeast(t, &counter, 1, 2*time.Second)
	count := atomic.LoadInt64(&counter)
	assert.GreaterOrEqual(t, count, int64(1), "задача по cron должна выполниться хотя бы один раз")
}

func TestScheduler_AddCronJobInvalidSchedule(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	job := func(ctx context.Context) error {
		return nil
	}

	_, err := s.AddCronJob("invalid schedule", job)
	assert.Error(t, err)
}

func TestScheduler_AddTickerJob(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var counter int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&counter, 1)
		return nil
	}

	s.AddTickerJob(50*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &counter, 2, time.Second)
	count := atomic.LoadInt64(&counter)
	assert.Greater(t, count, int64(1), "ticker-задача должна выполниться несколько раз")
}

func TestScheduler_JobWithError(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		return errors.New("test error")
	}

	s.AddTickerJob(50*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &runCount, 2, 2*time.Second)
	count := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, count, int64(2), "задача должна продолжать выполняться несмотря на ошибки")
}

func TestScheduler_JobWithPanic(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		count := atomic.AddInt64(&runCount, 1)
		if count == 1 {
			panic("test panic")
		}
		return nil
	}

	s.AddTickerJob(50*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &runCount, 2, 2*time.Second)
	count := atomic.LoadInt64(&runCount)
	assert.Greater(t, count, int64(1), "задача должна продолжить работу после паники")
}

func TestScheduler_Stop(t *testing.T) {
	s := New(Config{})

	var counter int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&counter, 1)
		return nil
	}

	s.AddTickerJob(30*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &counter, 1, time.Second)

	s.Stop()
	require.Eventually(t, func() bool {
		return !s.IsRunning()
	}, time.Second, 10*time.Millisecond, "планировщик не остановился")

	beforeStop := atomic.LoadInt64(&counter)
	ensureNoIncrement(t, &counter, beforeStop, 200*time.Millisecond)
}

func TestScheduler_ContextCancellation(t *testing.T) {
	s := New(Config{})

	var counter int64
	job := func(ctx context.Context) error {
		// Проверяем, что контекст передается в задачу
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			atomic.AddInt64(&counter, 1)
			return nil
		}
	}

	s.AddTickerJob(50*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &counter, 1, time.Second)
	s.cancel() // Отменяем контекст напрямую

	require.Eventually(t, func() bool {
		return !s.IsRunning()
	}, time.Second, 10*time.Millisecond, "планировщик не остановился после отмены контекста")

	beforeCancel := atomic.LoadInt64(&counter)
	ensureNoIncrement(t, &counter, beforeCancel, 200*time.Millisecond)
}

func TestScheduler_MultipleJobs(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var cronCounter, tickerCounter int64

	cronJob := func(ctx context.Context) error {
		atomic.AddInt64(&cronCounter, 1)
		return nil
	}

	tickerJob := func(ctx context.Context) error {
		atomic.AddInt64(&tickerCounter, 1)
		return nil
	}

	// Используем более частый интервал для cron
	_, err := s.AddCronJob("@every 50ms", cronJob)
	require.NoError(t, err)

	s.AddTickerJob(30*time.Millisecond, tickerJob)
	s.Start()

	waitForAtLeast(t, &cronCounter, 1, 2*time.Second)
	waitForAtLeast(t, &tickerCounter, 1, 2*time.Second)

	cronCount := atomic.LoadInt64(&cronCounter)
	tickerCount := atomic.LoadInt64(&tickerCounter)

	assert.GreaterOrEqual(t, cronCount, int64(1), "cron-задача должна выполниться хотя бы один раз")
	assert.GreaterOrEqual(t, tickerCount, int64(1), "ticker-задача должна выполниться хотя бы один раз")
}

func TestScheduler_MultipleStopCalls(t *testing.T) {
	s := New(Config{})

	s.Start()

	// Вызываем Stop() несколько раз - должно быть безопасно
	s.Stop()
	s.Stop()
	s.Stop()

	assert.False(t, s.IsRunning(), "планировщик должен быть остановлен")
}

func TestScheduler_JobWithTimeout(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		// Симулируем долгую работу
		select {
		case <-time.After(200 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	opts := JobOptions{
		Name:    "timeout-test",
		Timeout: 50 * time.Millisecond,
	}

	s.AddTickerJobWithOptions(100*time.Millisecond, job, opts)
	s.Start()

	waitForAtLeast(t, &runCount, 1, 2*time.Second)
	count := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, count, int64(1), "задача должна выполниться хотя бы один раз")
}

func TestScheduler_SkipIfRunning(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	var concurrentCount int64

	job := func(ctx context.Context) error {
		current := atomic.AddInt64(&runCount, 1)
		concurrent := atomic.AddInt64(&concurrentCount, 1)
		defer atomic.AddInt64(&concurrentCount, -1)

		// Проверяем, что не более одного выполнения одновременно
		assert.LessOrEqual(t, concurrent, int64(1), "не должно быть параллельных запусков")

		// Симулируем работу
		time.Sleep(150 * time.Millisecond)

		t.Logf("Job execution %d completed", current)
		return nil
	}

	opts := JobOptions{
		Name:          "skip-test",
		OverlapPolicy: SkipIfRunning,
	}

	s.AddTickerJobWithOptions(50*time.Millisecond, job, opts)
	s.Start()

	waitForAtLeast(t, &runCount, 1, time.Second)
	time.Sleep(250 * time.Millisecond)

	count := atomic.LoadInt64(&runCount)
	// При интервале 50ms и длительности 150ms число запусков должно быть заметно меньше числа тиков
	assert.GreaterOrEqual(t, count, int64(1), "задача должна выполниться хотя бы один раз")
	assert.LessOrEqual(t, count, int64(4), "задача должна пропускать часть запусков при пересечении")
}

func TestScheduler_JobWithName(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		return nil
	}

	opts := JobOptions{
		Name: "named-job",
	}

	s.AddTickerJobWithOptions(50*time.Millisecond, job, opts)
	s.Start()

	waitForAtLeast(t, &runCount, 1, time.Second)
	count := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, count, int64(1), "именованная задача должна выполниться")
}

func TestScheduler_RemoveCronJob(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		return nil
	}

	id, err := s.AddCronJob("@every 20ms", job)
	require.NoError(t, err)

	s.Start()

	waitForAtLeast(t, &runCount, 1, 2*time.Second)

	// Удаляем задачу
	s.RemoveCronJob(id)

	countBeforeRemoval := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, countBeforeRemoval, int64(1), "задача должна выполниться до удаления")

	ensureNoIncrement(t, &runCount, countBeforeRemoval, 300*time.Millisecond)
}

func TestScheduler_RemoveTickerJob(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		return nil
	}

	id := s.AddTickerJob(50*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &runCount, 1, time.Second)

	// Удаляем задачу
	removed := s.RemoveTickerJob(id)
	assert.True(t, removed, "задача должна удаляться без ошибок")

	countBeforeRemoval := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, countBeforeRemoval, int64(1), "задача должна выполниться до удаления")

	ensureNoIncrement(t, &runCount, countBeforeRemoval, 300*time.Millisecond)
}

func TestScheduler_RemoveNonExistentTickerJob(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	removed := s.RemoveTickerJob(999)
	assert.False(t, removed, "нужно вернуть false для несуществующей задачи")
}

func TestScheduler_NewWithContext(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	s := NewWithContext(parentCtx, Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		return nil
	}

	s.AddTickerJob(50*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &runCount, 1, time.Second)

	// Отменяем родительский контекст
	parentCancel()

	require.Eventually(t, func() bool {
		return !s.IsRunning()
	}, time.Second, 10*time.Millisecond, "планировщик должен остановиться после отмены родительского контекста")

	countBeforeCancel := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, countBeforeCancel, int64(1), "задача должна выполниться до отмены")

	ensureNoIncrement(t, &runCount, countBeforeCancel, 300*time.Millisecond)
}

func TestScheduler_MultipleStartCalls(t *testing.T) {
	s := New(Config{})
	defer s.Stop()

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		return nil
	}

	s.AddTickerJob(50*time.Millisecond, job)

	// Вызываем Start() несколько раз - должно быть безопасно
	s.Start()
	s.Start()
	s.Start()

	assert.True(t, s.IsRunning(), "планировщик должен быть запущен")

	waitForAtLeast(t, &runCount, 1, time.Second)

	count := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, count, int64(1), "задача должна выполниться")
}

func TestScheduler_StopContext(t *testing.T) {
	s := New(Config{})

	var runCount int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&runCount, 1)
		return nil
	}

	s.AddTickerJob(50*time.Millisecond, job)
	s.Start()

	waitForAtLeast(t, &runCount, 1, time.Second)

	// Останавливаем с достаточным дедлайном
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := s.StopContext(ctx)
	assert.NoError(t, err, "планировщик должен корректно остановиться в пределах дедлайна")
	require.Eventually(t, func() bool {
		return !s.IsRunning()
	}, time.Second, 10*time.Millisecond, "планировщик должен быть остановлен")

	count := atomic.LoadInt64(&runCount)
	assert.GreaterOrEqual(t, count, int64(1), "задача должна выполниться до остановки")

	ensureNoIncrement(t, &runCount, count, 300*time.Millisecond)
}

func TestScheduler_StopContextTimeout(t *testing.T) {
	s := New(Config{})

	// Создаем задачу, которая долго выполняется при остановке
	var activeJobs int64
	job := func(ctx context.Context) error {
		atomic.AddInt64(&activeJobs, 1)
		defer atomic.AddInt64(&activeJobs, -1)

		select {
		case <-time.After(200 * time.Millisecond):
			return nil
		case <-ctx.Done():
			// Симулируем медленную очистку ресурсов
			time.Sleep(100 * time.Millisecond)
			return ctx.Err()
		}
	}

	s.AddTickerJob(30*time.Millisecond, job)
	s.Start()

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&activeJobs) > 0
	}, time.Second, 10*time.Millisecond, "задача должна успеть стартовать до остановки")

	// Останавливаем с очень коротким дедлайном
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	err := s.StopContext(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded, "должна вернуться ошибка из-за превышения дедлайна")
	require.Eventually(t, func() bool {
		return !s.IsRunning()
	}, time.Second, 10*time.Millisecond, "планировщик всё равно должен остановиться")
}

func TestScheduler_JobHooks(t *testing.T) {
	var startCalls, finishCalls, errorCalls int64
	var finishDuration time.Duration
	var finishError error

	hooks := JobHooks{
		OnJobStart: func(jobName string) {
			atomic.AddInt64(&startCalls, 1)
			assert.Equal(t, "test-job", jobName)
		},
		OnJobFinish: func(jobName string, duration time.Duration, err error) {
			atomic.AddInt64(&finishCalls, 1)
			assert.Equal(t, "test-job", jobName)
			finishDuration = duration
			finishError = err
		},
		OnJobError: func(jobName string, err error) {
			atomic.AddInt64(&errorCalls, 1)
			assert.Equal(t, "test-job", jobName)
			assert.Error(t, err)
		},
	}

	s := New(Config{JobHooks: hooks})
	defer s.Stop()

	// Первая задача - успешная
	successJob := func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	opts := JobOptions{Name: "test-job"}
	s.AddTickerJobWithOptions(50*time.Millisecond, successJob, opts)
	s.Start()

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&startCalls) >= 1
	}, 2*time.Second, 10*time.Millisecond, "хуки запуска должны вызываться")

	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&finishCalls) >= 1
	}, 2*time.Second, 10*time.Millisecond, "хуки завершения должны вызываться")

	assert.Equal(t, int64(0), atomic.LoadInt64(&errorCalls), "хук ошибок не должен срабатывать для успешной задачи")
	assert.NoError(t, finishError, "ошибки завершения быть не должно")
	assert.Greater(t, finishDuration, time.Duration(0), "длительность выполнения должна быть положительной")
}
