package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/sync/interfaces"
)

// ConnectionMonitorConfig - конфигурация монитора подключения
type ConnectionMonitorConfig struct {
	// ReconnectInterval - начальный интервал между попытками переподключения
	ReconnectInterval time.Duration

	// MaxReconnectDelay - максимальная задержка между попытками переподключения
	MaxReconnectDelay time.Duration

	// BackoffMultiplier - множитель для exponential backoff
	BackoffMultiplier float64
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() ConnectionMonitorConfig {
	return ConnectionMonitorConfig{
		ReconnectInterval: 5 * time.Second,
		MaxReconnectDelay: 60 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// SGroupConnectionMonitor отслеживает состояние подключения к SGROUP
// через SyncStatuses stream и уведомляет подписчиков о событиях
type SGroupConnectionMonitor struct {
	client interfaces.SGroupGateway
	config ConnectionMonitorConfig
	logger *zap.Logger

	// State (защищено mu)
	mu            sync.RWMutex
	isConnected   bool
	lastTimestamp *timestamppb.Timestamp
	lastConnected time.Time

	// Listeners (защищено listenersMu)
	listenersMu sync.RWMutex
	listeners   map[string]ConnectionListener

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSGroupConnectionMonitor создает новый монитор подключения к SGROUP
func NewSGroupConnectionMonitor(
	client interfaces.SGroupGateway,
	config ConnectionMonitorConfig,
	logger *zap.Logger,
) *SGroupConnectionMonitor {
	return &SGroupConnectionMonitor{
		client:    client,
		config:    config,
		logger:    logger.With(zap.String("component", "SGroupConnectionMonitor")),
		listeners: make(map[string]ConnectionListener),
	}
}

// Start запускает мониторинг подключения (non-blocking)
// Возвращается сразу, мониторинг выполняется в фоновой goroutine
func (m *SGroupConnectionMonitor) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Предотвращаем повторный запуск
	if m.ctx != nil {
		m.logger.Warn("Monitor already started")
		return
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.wg.Add(1)

	go m.monitorLoop()

	m.logger.Info("Connection monitor started")
}

// Stop останавливает мониторинг подключения
// Блокируется до полной остановки всех goroutines
func (m *SGroupConnectionMonitor) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()

	m.wg.Wait()

	m.logger.Info("Connection monitor stopped")
}

// Subscribe добавляет подписчика на события подключения
func (m *SGroupConnectionMonitor) Subscribe(listener ConnectionListener) {
	m.listenersMu.Lock()
	defer m.listenersMu.Unlock()

	name := listener.GetListenerName()
	m.listeners[name] = listener

	m.logger.Info("Listener subscribed",
		zap.String("listener", name),
		zap.Int("total_listeners", len(m.listeners)),
	)
}

// Unsubscribe удаляет подписчика
func (m *SGroupConnectionMonitor) Unsubscribe(listenerName string) {
	m.listenersMu.Lock()
	defer m.listenersMu.Unlock()

	delete(m.listeners, listenerName)

	m.logger.Info("Listener unsubscribed",
		zap.String("listener", listenerName),
		zap.Int("total_listeners", len(m.listeners)),
	)
}

// IsConnected возвращает текущее состояние подключения к SGROUP
func (m *SGroupConnectionMonitor) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isConnected
}

// WaitForConnection ждет установки подключения с таймаутом
// Возвращает error если подключение не установлено за timeout
func (m *SGroupConnectionMonitor) WaitForConnection(timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)

	for {
		if m.IsConnected() {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for SGROUP connection after %v", timeout)
		}

		select {
		case <-ticker.C:
			continue
		case <-m.ctx.Done():
			return fmt.Errorf("monitor stopped while waiting for connection")
		}
	}
}

// GetLastTimestamp возвращает последний полученный timestamp
func (m *SGroupConnectionMonitor) GetLastTimestamp() *timestamppb.Timestamp {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastTimestamp
}

// Stats возвращает текущую статистику монитора
func (m *SGroupConnectionMonitor) Stats() ConnectionStats {
	m.mu.RLock()
	isConnected := m.isConnected
	lastConnected := m.lastConnected
	lastTimestamp := m.lastTimestamp
	m.mu.RUnlock()

	m.listenersMu.RLock()
	listenerCount := len(m.listeners)
	m.listenersMu.RUnlock()

	return ConnectionStats{
		IsConnected:   isConnected,
		LastConnected: lastConnected,
		LastTimestamp: lastTimestamp,
		ListenerCount: listenerCount,
	}
}

// monitorLoop - основной цикл мониторинга (выполняется в goroutine)
func (m *SGroupConnectionMonitor) monitorLoop() {
	defer m.wg.Done()

	retryDelay := m.config.ReconnectInterval

	m.logger.Info("Monitor loop started")

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info("Monitor loop stopped by context")
			return
		default:
		}

		// Попытка подключения и прослушивания stream
		err := m.connectAndListen()

		if err != nil {
			m.logger.Error("Stream connection failed",
				zap.Error(err),
				zap.Duration("retry_after", retryDelay),
			)

			// Exponential backoff
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(retryDelay):
			}

			// Увеличиваем задержку с учетом backoff multiplier
			retryDelay = time.Duration(float64(retryDelay) * m.config.BackoffMultiplier)
			if retryDelay > m.config.MaxReconnectDelay {
				retryDelay = m.config.MaxReconnectDelay
			}
		} else {
			// Успешное переподключение - сбрасываем delay
			retryDelay = m.config.ReconnectInterval
		}
	}
}

// connectAndListen устанавливает stream и слушает timestamps
func (m *SGroupConnectionMonitor) connectAndListen() error {
	// Открываем stream
	timestampChan, err := m.client.GetStatuses(m.ctx)
	if err != nil {
		m.setConnected(false)
		return fmt.Errorf("failed to open SyncStatuses stream: %w", err)
	}

	// Stream успешно открыт
	m.setConnected(true)
	m.broadcastEvent(ConnectionEvent{
		Type:       EventConnected,
		OccurredAt: time.Now(),
	})

	m.logger.Info("SyncStatuses stream established")

	// Читаем timestamps из stream
	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info("Stream reading stopped by context")
			return nil

		case ts, ok := <-timestampChan:
			if !ok {
				// Stream закрыт
				m.logger.Warn("SyncStatuses stream closed by server")
				m.setConnected(false)
				m.broadcastEvent(ConnectionEvent{
					Type:       EventDisconnected,
					Error:      fmt.Errorf("stream closed by server"),
					OccurredAt: time.Now(),
				})
				return fmt.Errorf("stream closed")
			}

			// Получен timestamp
			m.updateTimestamp(ts)
			m.broadcastEvent(ConnectionEvent{
				Type:       EventTimestamp,
				Timestamp:  ts,
				OccurredAt: time.Now(),
			})

			m.logger.Debug("Received timestamp from SGROUP",
				zap.Time("timestamp", ts.AsTime()),
			)
		}
	}
}

// setConnected обновляет статус подключения (thread-safe)
func (m *SGroupConnectionMonitor) setConnected(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isConnected = connected
	if connected {
		m.lastConnected = time.Now()
	}
}

// updateTimestamp обновляет последний timestamp (thread-safe)
func (m *SGroupConnectionMonitor) updateTimestamp(ts *timestamppb.Timestamp) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastTimestamp = ts
}

// broadcastEvent рассылает событие всем подписчикам (non-blocking)
func (m *SGroupConnectionMonitor) broadcastEvent(event ConnectionEvent) {
	// Копируем listeners под lock (избегаем deadlock)
	m.listenersMu.RLock()
	listeners := make([]ConnectionListener, 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	m.listenersMu.RUnlock()

	// Рассылаем событие каждому listener в отдельной goroutine
	for _, listener := range listeners {
		go m.notifyListener(listener, event)
	}

	m.logger.Debug("Event broadcasted",
		zap.String("event_type", event.Type.String()),
		zap.Int("listener_count", len(listeners)),
	)
}

// notifyListener уведомляет одного listener с защитой от panic
func (m *SGroupConnectionMonitor) notifyListener(listener ConnectionListener, event ConnectionEvent) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error("Listener panicked",
				zap.String("listener", listener.GetListenerName()),
				zap.Any("panic", r),
			)
		}
	}()

	listener.OnConnectionEvent(event)
}
