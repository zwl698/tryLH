package strategy

import (
	"aqsystem/models"
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// mockStrategy 用于测试的模拟策略
type mockStrategy struct {
	BaseStrategy
	onBarCalled   bool
	onQuoteCalled bool
}

func newMockStrategy(id string) *mockStrategy {
	cfg := models.StrategyConfig{
		ID:     id,
		Name:   "模拟策略",
		Type:   "mock",
		Stocks: []string{"600000", "000001"},
		Params: map[string]interface{}{},
		Status: models.StrategyStatusPaused,
	}
	return &mockStrategy{
		BaseStrategy: NewBaseStrategy(cfg, zap.NewNop()),
	}
}

func (s *mockStrategy) Type() string { return "mock" }

func (s *mockStrategy) Init(config models.StrategyConfig) error { return nil }

func (s *mockStrategy) OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error) {
	s.onBarCalled = true
	return nil, nil
}

func (s *mockStrategy) OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	s.onQuoteCalled = true
	return []models.Signal{{
		StrategyID: s.ID(),
		StockCode:  quote.StockCode,
		Side:       models.OrderSideBuy,
		Type:       models.OrderTypeLimit,
		Price:      quote.Close,
		Volume:     100,
		Reason:     "模拟信号",
		Timestamp:  time.Now(),
	}}, nil
}

func (s *mockStrategy) OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func TestEngine_RegisterStrategy(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_1")
	err := engine.RegisterStrategy(s)
	if err != nil {
		t.Fatalf("注册策略失败: %v", err)
	}

	// 重复注册
	err = engine.RegisterStrategy(s)
	if err == nil {
		t.Error("重复注册策略应返回错误")
	}
}

func TestEngine_UnregisterStrategy(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_2")
	engine.RegisterStrategy(s)

	err := engine.UnregisterStrategy("test_2")
	if err != nil {
		t.Fatalf("注销策略失败: %v", err)
	}

	_, ok := engine.GetStrategy("test_2")
	if ok {
		t.Error("注销后策略不应存在")
	}
}

func TestEngine_StartStopStrategy(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_3")
	engine.RegisterStrategy(s)

	err := engine.StartStrategy("test_3")
	if err != nil {
		t.Fatalf("启动策略失败: %v", err)
	}
	if s.GetStatus() != models.StrategyStatusActive {
		t.Error("启动后策略状态应为 ACTIVE")
	}

	// 重复启动
	err = engine.StartStrategy("test_3")
	if err == nil {
		t.Error("重复启动策略应返回错误")
	}

	err = engine.StopStrategy("test_3")
	if err != nil {
		t.Fatalf("停止策略失败: %v", err)
	}
	if s.GetStatus() != models.StrategyStatusStopped {
		t.Error("停止后策略状态应为 STOPPED")
	}
}

func TestEngine_PauseStrategy(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_4")
	engine.RegisterStrategy(s)
	engine.StartStrategy("test_4")

	err := engine.PauseStrategy("test_4")
	if err != nil {
		t.Fatalf("暂停策略失败: %v", err)
	}
	if s.GetStatus() != models.StrategyStatusPaused {
		t.Error("暂停后策略状态应为 PAUSED")
	}
}

func TestEngine_ProcessQuote(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_5")
	engine.RegisterStrategy(s)
	engine.StartStrategy("test_5")

	quote := models.StockQuote{
		StockCode: "600000",
		Close:     decimal.NewFromFloat(10.0),
	}

	signals := engine.ProcessQuote(context.Background(), quote)
	if len(signals) == 0 {
		t.Error("处理行情应产生信号")
	}
	if !s.onQuoteCalled {
		t.Error("OnQuote 未被调用")
	}
}

func TestEngine_ProcessQuote_FilterByStock(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_6")
	engine.RegisterStrategy(s)
	engine.StartStrategy("test_6")

	quote := models.StockQuote{
		StockCode: "999999", // 不在策略关注列表中
		Close:     decimal.NewFromFloat(10.0),
	}

	signals := engine.ProcessQuote(context.Background(), quote)
	if len(signals) != 0 {
		t.Error("不在关注列表的股票不应产生信号")
	}
}

func TestEngine_ProcessQuote_InactiveStrategy(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_7")
	engine.RegisterStrategy(s)
	// 策略未启动（默认 PAUSED）

	quote := models.StockQuote{
		StockCode: "600000",
		Close:     decimal.NewFromFloat(10.0),
	}

	signals := engine.ProcessQuote(context.Background(), quote)
	if len(signals) != 0 {
		t.Error("未启动的策略不应产生信号")
	}
}

func TestEngine_ProcessBar(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_8")
	engine.RegisterStrategy(s)
	engine.StartStrategy("test_8")

	kline := models.KLine{
		StockCode: "600000",
		Close:     decimal.NewFromFloat(10.0),
	}

	engine.ProcessBar(context.Background(), kline)
	if !s.onBarCalled {
		t.Error("OnBar 未被调用")
	}
}

func TestEngine_ListStrategies(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s1 := newMockStrategy("list_1")
	s2 := newMockStrategy("list_2")
	engine.RegisterStrategy(s1)
	engine.RegisterStrategy(s2)

	list := engine.ListStrategies()
	if len(list) != 2 {
		t.Errorf("策略数量不正确: 期望 2, 实际 %d", len(list))
	}
}

func TestEngine_Stop(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	s := newMockStrategy("test_stop")
	engine.RegisterStrategy(s)
	engine.StartStrategy("test_stop")

	engine.Stop()
	if s.GetStatus() != models.StrategyStatusStopped {
		t.Error("引擎停止后策略应为 STOPPED")
	}
}

func TestEngine_PushSignal(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	sig := models.Signal{
		StrategyID: "test",
		StockCode:  "600000",
		Side:       models.OrderSideBuy,
	}

	engine.PushSignal(sig)

	// 从通道读取
	select {
	case received := <-engine.SignalChannel():
		if received.StrategyID != "test" {
			t.Errorf("信号策略ID不正确: %s", received.StrategyID)
		}
	default:
		t.Error("应能从通道读取信号")
	}
}

func TestContainsStock(t *testing.T) {
	stocks := []string{"600000", "000001", "300001"}

	if !containsStock(stocks, "600000") {
		t.Error("应包含 600000")
	}
	if containsStock(stocks, "999999") {
		t.Error("不应包含 999999")
	}
}
