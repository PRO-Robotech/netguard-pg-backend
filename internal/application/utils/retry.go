package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryConfig определяет параметры повторных попыток
type RetryConfig struct {
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	MinSyncInterval time.Duration
}

// DefaultRetryConfig возвращает стандартную конфигурацию для retry
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		InitialDelay:    2 * time.Second,
		MaxDelay:        30 * time.Second,
		MinSyncInterval: 1 * time.Second,
	}
}

// SyncTracker отслеживает последние синхронизации для предотвращения частых вызовов бэкенда
type SyncTracker struct {
	lastSyncTimes   map[string]time.Time
	syncStats       map[string]*SyncStats
	minSyncInterval time.Duration
}

// SyncStats содержит статистику синхронизаций для конкретного ресурса
type SyncStats struct {
	TotalAttempts   int
	SuccessfulSyncs int
	FailedSyncs     int
	LastSyncTime    time.Time
	LastError       error
}

// NewSyncTracker создает новый SyncTracker с указанным минимальным интервалом между синхронизациями
func NewSyncTracker(minInterval time.Duration) *SyncTracker {
	return &SyncTracker{
		lastSyncTimes:   make(map[string]time.Time),
		syncStats:       make(map[string]*SyncStats),
		minSyncInterval: minInterval,
	}
}

// ShouldSync проверяет, нужно ли выполнять синхронизацию для указанного ключа
func (t *SyncTracker) ShouldSync(key string) bool {
	lastSync, exists := t.lastSyncTimes[key]
	now := time.Now()

	if !exists || now.Sub(lastSync) >= t.minSyncInterval {
		t.lastSyncTimes[key] = now

		// Инициализируем статистику если нужно
		if _, ok := t.syncStats[key]; !ok {
			t.syncStats[key] = &SyncStats{}
		}
		t.syncStats[key].TotalAttempts++

		return true
	}

	return false
}

// RecordSuccess записывает успешную синхронизацию
func (t *SyncTracker) RecordSuccess(key string) {
	if stats, ok := t.syncStats[key]; ok {
		stats.SuccessfulSyncs++
		stats.LastSyncTime = time.Now()
		stats.LastError = nil
	}
}

// RecordFailure записывает неудачную синхронизацию
func (t *SyncTracker) RecordFailure(key string, err error) {
	if stats, ok := t.syncStats[key]; ok {
		stats.FailedSyncs++
		stats.LastError = err
	}
}

// GetStats возвращает копию статистики для ключа
func (t *SyncTracker) GetStats(key string) *SyncStats {
	if stats, ok := t.syncStats[key]; ok {
		// Возвращаем копию чтобы избежать race conditions
		return &SyncStats{
			TotalAttempts:   stats.TotalAttempts,
			SuccessfulSyncs: stats.SuccessfulSyncs,
			FailedSyncs:     stats.FailedSyncs,
			LastSyncTime:    stats.LastSyncTime,
			LastError:       stats.LastError,
		}
	}

	return nil
}

// CalculateRetryDelay вычисляет задержку для повторной попытки с экспоненциальным back-off
func CalculateRetryDelay(config RetryConfig, attemptCount int) time.Duration {
	delay := config.InitialDelay * time.Duration(1<<uint(attemptCount))
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}
	return delay
}

// IsRetryableError определяет, является ли ошибка временной и подходящей для retry
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Проверяем gRPC ошибки
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
			return true
		case codes.Internal:
			// Проверяем сообщение об ошибке для временных ошибок
			msg := strings.ToLower(st.Message())
			return strings.Contains(msg, "temporary") ||
				strings.Contains(msg, "timeout") ||
				strings.Contains(msg, "connection") ||
				strings.Contains(msg, "network")
		}
	}

	// Проверяем обычные ошибки
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "temporary") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "connection") ||
		strings.Contains(errMsg, "network") ||
		strings.Contains(errMsg, "unavailable") ||
		strings.Contains(errMsg, "deadline exceeded")
}

// ExecuteWithRetry выполняет функцию с retry логикой
func ExecuteWithRetry(ctx context.Context, config RetryConfig, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Проверяем, является ли ошибка временной
		if !IsRetryableError(err) {
			return err
		}

		// Проверяем контекст
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Вычисляем задержку
		delay := CalculateRetryDelay(config, attempt)

		// Ждем перед следующей попыткой
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("failed after 3 attempts, last error: %v", lastErr)
}
