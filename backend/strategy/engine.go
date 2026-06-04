package strategy

import (
	"aqsystem/models"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// BrokerInterface 券商接口引用 - 策略引擎用于下单
type BrokerInterface interface {
	SubmitOrder(ctx context.Context, order *models.Order) (*models.Order, error)
	GetAccount(ctx context.Context) (*models.Account, error)
	GetPositions(ctx context.Context) ([]models.Position, error)
}

// Strategy 策略接口 - 所有策略必须实现
type Strategy interface {
	// 基础信息
	ID() string
	Name() string
	Description() string
	Type() string

	// 策略生命周期
	Init(config models.StrategyConfig) error
	OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error)
	OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error)
	OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error)

	// 参数管理
	GetParams() map[string]interface{}
	SetParams(params map[string]interface{}) error
	GetParamDefs() []ParamDef

	// 状态
	GetStatus() models.StrategyStatus
	SetStatus(status models.StrategyStatus)
	GetConfig() models.StrategyConfig
}

// ParamDef 策略参数定义
type ParamDef struct {
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	Type        string      `json:"type"` // int, float, string, bool
	Default     interface{} `json:"default"`
	Min         interface{} `json:"min,omitempty"`
	Max         interface{} `json:"max,omitempty"`
	Description string      `json:"description"`
}

// BaseStrategy 策略基类 - 提供通用实现
type BaseStrategy struct {
	mu     sync.RWMutex
	config models.StrategyConfig
	status models.StrategyStatus
	logger *zap.Logger
}

// NewBaseStrategy 创建策略基类
func NewBaseStrategy(config models.StrategyConfig, logger *zap.Logger) BaseStrategy {
	return BaseStrategy{
		config: config,
		status: models.StrategyStatusPaused,
		logger: logger,
	}
}

// ID 策略ID
func (s *BaseStrategy) ID() string {
	return s.config.ID
}

// Name 策略名称
func (s *BaseStrategy) Name() string {
	return s.config.Name
}

// Description 策略描述
func (s *BaseStrategy) Description() string {
	return s.config.Description
}

// GetStatus 获取状态
func (s *BaseStrategy) GetStatus() models.StrategyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetStatus 设置状态
func (s *BaseStrategy) SetStatus(status models.StrategyStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.config.Status = status
}

// GetConfig 获取配置
func (s *BaseStrategy) GetConfig() models.StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetParams 获取参数
func (s *BaseStrategy) GetParams() map[string]interface{} {
	return s.config.Params
}

// SetParams 设置参数
func (s *BaseStrategy) SetParams(params map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range params {
		s.config.Params[k] = v
	}
	s.config.UpdatedAt = time.Now()
	return nil
}

// GetParamDefs 默认参数定义（子类覆写）
func (s *BaseStrategy) GetParamDefs() []ParamDef {
	return nil
}

// getIntParam 获取整数参数
func (s *BaseStrategy) getIntParam(key string, defaultVal int) int {
	if val, ok := s.config.Params[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return defaultVal
}

// getFloatParam 获取浮点参数
func (s *BaseStrategy) getFloatParam(key string, defaultVal float64) float64 {
	if val, ok := s.config.Params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case decimal.Decimal:
			f, _ := v.Float64()
			return f
		}
	}
	return defaultVal
}

// getStringParam 获取字符串参数
func (s *BaseStrategy) getStringParam(key string, defaultVal string) string {
	if val, ok := s.config.Params[key]; ok {
		if v, ok := val.(string); ok {
			return v
		}
	}
	return defaultVal
}

// getBoolParam 获取布尔参数
func (s *BaseStrategy) getBoolParam(key string, defaultVal bool) bool {
	if val, ok := s.config.Params[key]; ok {
		if v, ok := val.(bool); ok {
			return v
		}
	}
	return defaultVal
}

// ==================== 策略引擎 ====================

// Engine 策略引擎 - 管理所有策略的运行
type Engine struct {
	mu         sync.RWMutex
	strategies map[string]Strategy
	broker     BrokerInterface // 券商引用接口
	logger     *zap.Logger
	signalChan chan models.Signal
	stopCh     chan struct{}
}

// NewEngine 创建策略引擎
func NewEngine(logger *zap.Logger) *Engine {
	return &Engine{
		strategies: make(map[string]Strategy),
		logger:     logger,
		signalChan: make(chan models.Signal, 1000),
		stopCh:     make(chan struct{}),
	}
}

// RegisterStrategy 注册策略
func (e *Engine) RegisterStrategy(s Strategy) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.strategies[s.ID()]; exists {
		return fmt.Errorf("策略已存在: %s", s.ID())
	}

	e.strategies[s.ID()] = s
	e.logger.Info("策略已注册",
		zap.String("id", s.ID()),
		zap.String("name", s.Name()),
		zap.String("type", s.Type()),
	)
	return nil
}

// UnregisterStrategy 注销策略
func (e *Engine) UnregisterStrategy(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if s, exists := e.strategies[id]; exists {
		s.SetStatus(models.StrategyStatusStopped)
		delete(e.strategies, id)
		e.logger.Info("策略已注销", zap.String("id", id))
	}
	return nil
}

// GetStrategy 获取策略
func (e *Engine) GetStrategy(id string) (Strategy, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.strategies[id]
	return s, ok
}

// ListStrategies 列出所有策略
func (e *Engine) ListStrategies() []Strategy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Strategy, 0, len(e.strategies))
	for _, s := range e.strategies {
		result = append(result, s)
	}
	return result
}

// StartStrategy 启动策略
func (e *Engine) StartStrategy(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, exists := e.strategies[id]
	if !exists {
		return fmt.Errorf("策略不存在: %s", id)
	}

	if s.GetStatus() == models.StrategyStatusActive {
		return fmt.Errorf("策略已在运行: %s", id)
	}

	s.SetStatus(models.StrategyStatusActive)
	e.logger.Info("策略已启动", zap.String("id", id), zap.String("name", s.Name()))
	return nil
}

// StopStrategy 停止策略
func (e *Engine) StopStrategy(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, exists := e.strategies[id]
	if !exists {
		return fmt.Errorf("策略不存在: %s", id)
	}

	s.SetStatus(models.StrategyStatusStopped)
	e.logger.Info("策略已停止", zap.String("id", id))
	return nil
}

// PauseStrategy 暂停策略
func (e *Engine) PauseStrategy(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, exists := e.strategies[id]
	if !exists {
		return fmt.Errorf("策略不存在: %s", id)
	}

	s.SetStatus(models.StrategyStatusPaused)
	e.logger.Info("策略已暂停", zap.String("id", id))
	return nil
}

// ProcessQuote 处理行情数据 - 分发给各策略
func (e *Engine) ProcessQuote(ctx context.Context, quote models.StockQuote) []models.Signal {
	var allSignals []models.Signal

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, s := range e.strategies {
		if s.GetStatus() != models.StrategyStatusActive {
			continue
		}

		// 检查策略是否关注该股票
		config := s.GetConfig()
		if !containsStock(config.Stocks, quote.StockCode) {
			continue
		}

		signals, err := s.OnQuote(ctx, quote)
		if err != nil {
			e.logger.Error("策略处理行情失败",
				zap.String("strategy", s.ID()),
				zap.String("stock", quote.StockCode),
				zap.Error(err),
			)
			continue
		}

		allSignals = append(allSignals, signals...)
	}

	return allSignals
}

// ProcessBar 处理K线数据
func (e *Engine) ProcessBar(ctx context.Context, kline models.KLine) []models.Signal {
	var allSignals []models.Signal

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, s := range e.strategies {
		if s.GetStatus() != models.StrategyStatusActive {
			continue
		}

		config := s.GetConfig()
		if !containsStock(config.Stocks, kline.StockCode) {
			continue
		}

		signals, err := s.OnBar(ctx, kline)
		if err != nil {
			e.logger.Error("策略处理K线失败",
				zap.String("strategy", s.ID()),
				zap.String("stock", kline.StockCode),
				zap.Error(err),
			)
			continue
		}

		allSignals = append(allSignals, signals...)
	}

	return allSignals
}

// SignalChannel 获取信号通道
func (e *Engine) SignalChannel() <-chan models.Signal {
	return e.signalChan
}

// PushSignal 推送信号
func (e *Engine) PushSignal(signal models.Signal) {
	select {
	case e.signalChan <- signal:
	default:
		e.logger.Warn("信号通道已满，丢弃信号", zap.String("strategy", signal.StrategyID))
	}
}

// Stop 停止引擎
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, s := range e.strategies {
		s.SetStatus(models.StrategyStatusStopped)
	}

	close(e.stopCh)
	e.logger.Info("策略引擎已停止")
}

// containsStock 检查策略是否关注该股票
func containsStock(stocks []string, code string) bool {
	for _, s := range stocks {
		if s == code {
			return true
		}
	}
	return false
}

