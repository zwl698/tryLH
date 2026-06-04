package strategy

import (
	"aqsystem/models"
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func makeKLine(code string, day int, closePrice float64) models.KLine {
	return models.KLine{
		StockCode: code,
		Period:    "day",
		Open:      decimal.NewFromFloat(closePrice - 0.1),
		High:      decimal.NewFromFloat(closePrice + 0.2),
		Low:       decimal.NewFromFloat(closePrice - 0.2),
		Close:     decimal.NewFromFloat(closePrice),
		Volume:    1000000,
		Amount:    decimal.NewFromFloat(closePrice * 1000000),
		Timestamp: time.Date(2024, 1, day, 15, 0, 0, 0, time.Local),
	}
}

// ==================== 双均线策略测试 ====================

func TestDoubleMAStrategy_Create(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_dma",
		Name:   "双均线测试",
		Type:   "double_ma",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"short_period": 5,
			"long_period":  20,
			"ma_type":      "SMA",
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewDoubleMAStrategy(cfg, zap.NewNop())
	if s.ID() != "test_dma" {
		t.Errorf("策略ID不正确: %s", s.ID())
	}
	if s.Type() != "double_ma" {
		t.Errorf("策略类型不正确: %s", s.Type())
	}
}

func TestDoubleMAStrategy_GoldenCross(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_dma_golden",
		Name:   "双均线金叉测试",
		Type:   "double_ma",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"short_period": 5,
			"long_period":  10,
			"ma_type":      "SMA",
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewDoubleMAStrategy(cfg, zap.NewNop())
	ctx := context.Background()

	// 先喂10天下跌的数据，让短均线在长均线下方
	for i := 1; i <= 10; i++ {
		kline := makeKLine("600000", i, 10.0-float64(i)*0.1)
		s.OnBar(ctx, kline)
	}

	// 然后喂上涨数据，制造金叉
	var signals []models.Signal
	for i := 11; i <= 25; i++ {
		kline := makeKLine("600000", i, 9.0+float64(i-10)*0.3)
		sigs, _ := s.OnBar(ctx, kline)
		signals = append(signals, sigs...)
	}

	// 应该产生了买入信号
	hasBuySignal := false
	for _, sig := range signals {
		if sig.Side == models.OrderSideBuy {
			hasBuySignal = true
			break
		}
	}
	if !hasBuySignal {
		t.Error("金叉应产生买入信号")
	}
}

func TestDoubleMAStrategy_NotEnoughData(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_dma_nodata",
		Name:   "双均线数据不足测试",
		Type:   "double_ma",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"short_period": 5,
			"long_period":  20,
			"ma_type":      "SMA",
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewDoubleMAStrategy(cfg, zap.NewNop())
	ctx := context.Background()

	// 只喂5天数据，不够计算长周期均线
	for i := 1; i <= 5; i++ {
		kline := makeKLine("600000", i, 10.0)
		sigs, _ := s.OnBar(ctx, kline)
		if len(sigs) != 0 {
			t.Error("数据不足时不应产生信号")
		}
	}
}

func TestDoubleMAStrategy_OnQuote(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_dma_quote",
		Name:   "双均线OnQuote测试",
		Type:   "double_ma",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewDoubleMAStrategy(cfg, zap.NewNop())
	sigs, err := s.OnQuote(context.Background(), models.StockQuote{})
	if err != nil {
		t.Fatalf("OnQuote 不应返回错误: %v", err)
	}
	if len(sigs) != 0 {
		t.Error("双均线策略 OnQuote 不应产生信号")
	}
}

func TestDoubleMAStrategy_GetParamDefs(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_dma_params",
		Name:   "双均线参数定义测试",
		Type:   "double_ma",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{},
	}
	s := NewDoubleMAStrategy(cfg, zap.NewNop())
	defs := s.GetParamDefs()
	if len(defs) != 3 {
		t.Errorf("参数定义数量不正确: 期望 3, 实际 %d", len(defs))
	}
}

// ==================== 海龟策略测试 ====================

func TestTurtleStrategy_Create(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_turtle",
		Name:   "海龟测试",
		Type:   "turtle",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"entry_period": 20,
			"exit_period":  10,
			"atr_period":   20,
			"risk_pct":     0.01,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewTurtleStrategy(cfg, zap.NewNop())
	if s.Type() != "turtle" {
		t.Errorf("策略类型不正确: %s", s.Type())
	}
}

func TestTurtleStrategy_Breakout(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_turtle_breakout",
		Name:   "海龟突破测试",
		Type:   "turtle",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"entry_period": 10,
			"exit_period":  5,
			"atr_period":   10,
			"risk_pct":     0.01,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewTurtleStrategy(cfg, zap.NewNop())
	ctx := context.Background()

	// 喂20天数据，其中最后一天突破最高价
	for i := 1; i <= 10; i++ {
		kline := makeKLine("600000", i, 10.0)
		s.OnBar(ctx, kline)
	}

	// 突破日
	breakoutKline := models.KLine{
		StockCode: "600000",
		Period:    "day",
		Open:      decimal.NewFromFloat(10.0),
		High:      decimal.NewFromFloat(10.5), // 突破前高
		Low:       decimal.NewFromFloat(9.8),
		Close:     decimal.NewFromFloat(10.5),
		Volume:    2000000,
		Timestamp: time.Date(2024, 1, 11, 15, 0, 0, 0, time.Local),
	}
	sigs, _ := s.OnBar(ctx, breakoutKline)

	if len(sigs) == 0 {
		t.Error("突破新高应产生买入信号")
	}
	if sigs[0].Side != models.OrderSideBuy {
		t.Error("突破信号应为买入")
	}
}

func TestTurtleStrategy_GetParamDefs(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_turtle_params",
		Name:   "海龟参数定义测试",
		Type:   "turtle",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{},
	}
	s := NewTurtleStrategy(cfg, zap.NewNop())
	defs := s.GetParamDefs()
	if len(defs) != 4 {
		t.Errorf("参数定义数量不正确: 期望 4, 实际 %d", len(defs))
	}
}

// ==================== 动量策略测试 ====================

func TestMomentumStrategy_Create(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_momentum",
		Name:   "动量测试",
		Type:   "momentum",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"lookback_period":    20,
			"holding_period":     10,
			"momentum_threshold": 0.05,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewMomentumStrategy(cfg, zap.NewNop())
	if s.Type() != "momentum" {
		t.Errorf("策略类型不正确: %s", s.Type())
	}
}

func TestMomentumStrategy_BuyOnHighMomentum(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_momentum_buy",
		Name:   "动量买入测试",
		Type:   "momentum",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"lookback_period":    10,
			"holding_period":     5,
			"top_n":              3,
			"momentum_threshold": 0.05,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewMomentumStrategy(cfg, zap.NewNop())
	ctx := context.Background()

	// 先喂10天低价格
	for i := 1; i <= 10; i++ {
		kline := makeKLine("600000", i, 10.0)
		s.OnBar(ctx, kline)
	}

	// 喂上涨数据，制造强动量
	var signals []models.Signal
	for i := 11; i <= 25; i++ {
		kline := makeKLine("600000", i, 10.0+float64(i-10)*0.15) // 涨幅超过5%
		sigs, _ := s.OnBar(ctx, kline)
		signals = append(signals, sigs...)
	}

	hasBuySignal := false
	for _, sig := range signals {
		if sig.Side == models.OrderSideBuy {
			hasBuySignal = true
			break
		}
	}
	if !hasBuySignal {
		t.Error("高动量应产生买入信号")
	}
}

// ==================== 均值回归策略测试 ====================

func TestMeanReversionStrategy_Create(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_mr",
		Name:   "均值回归测试",
		Type:   "mean_reversion",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"lookback_period": 20,
			"entry_zscore":    2.0,
			"exit_zscore":     0.5,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewMeanReversionStrategy(cfg, zap.NewNop())
	if s.Type() != "mean_reversion" {
		t.Errorf("策略类型不正确: %s", s.Type())
	}
}

func TestMeanReversionStrategy_OversoldBuy(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_mr_buy",
		Name:   "均值回归买入测试",
		Type:   "mean_reversion",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"lookback_period": 10,
			"entry_zscore":    1.5,
			"exit_zscore":     0.5,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewMeanReversionStrategy(cfg, zap.NewNop())
	ctx := context.Background()

	// 喂10天稳定价格
	for i := 1; i <= 10; i++ {
		kline := makeKLine("600000", i, 10.0)
		s.OnBar(ctx, kline)
	}

	// 价格突然大幅下跌（超跌）
	oversoldKline := makeKLine("600000", 11, 7.0) // 从10元跌到7元
	sigs, _ := s.OnBar(ctx, oversoldKline)

	if len(sigs) == 0 {
		t.Error("超跌应产生买入信号")
	}
	if sigs[0].Side != models.OrderSideBuy {
		t.Error("超跌信号应为买入")
	}
}

// ==================== 网格策略测试 ====================

func TestGridStrategy_Create(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_grid",
		Name:   "网格测试",
		Type:   "grid",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"upper_price": 12.0,
			"lower_price": 8.0,
			"grid_count":  10,
			"grid_volume": 100,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewGridStrategy(cfg, zap.NewNop())
	if s.Type() != "grid" {
		t.Errorf("策略类型不正确: %s", s.Type())
	}
}

func TestGridStrategy_Init(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_grid_init",
		Name:   "网格初始化测试",
		Type:   "grid",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"upper_price": 12.0,
			"lower_price": 8.0,
			"grid_count":  10,
			"grid_volume": 100,
		},
		MaxPosition: decimal.NewFromFloat(100000),
	}

	s := NewGridStrategy(cfg, zap.NewNop())
	err := s.Init(cfg)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
}

func TestGridStrategy_GetParamDefs(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_grid_params",
		Name:   "网格参数定义测试",
		Type:   "grid",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{},
	}
	s := NewGridStrategy(cfg, zap.NewNop())
	defs := s.GetParamDefs()
	if len(defs) != 4 {
		t.Errorf("参数定义数量不正确: 期望 4, 实际 %d", len(defs))
	}
}

// ==================== BaseStrategy 测试 ====================

func TestBaseStrategy_GetSetParams(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_base",
		Name:   "基础策略测试",
		Type:   "base",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"param1": "value1",
			"param2": 42,
		},
	}

	s := NewBaseStrategy(cfg, zap.NewNop())

	params := s.GetParams()
	if params["param1"] != "value1" {
		t.Error("参数 param1 不正确")
	}

	s.SetParams(map[string]interface{}{"param3": 3.14})
	params = s.GetParams()
	if params["param3"] != 3.14 {
		t.Error("更新参数 param3 不正确")
	}
}

func TestBaseStrategy_Status(t *testing.T) {
	cfg := models.StrategyConfig{
		ID:     "test_base_status",
		Name:   "基础策略状态测试",
		Type:   "base",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{},
	}

	s := NewBaseStrategy(cfg, zap.NewNop())

	if s.GetStatus() != models.StrategyStatusPaused {
		t.Error("初始状态应为 PAUSED")
	}

	s.SetStatus(models.StrategyStatusActive)
	if s.GetStatus() != models.StrategyStatusActive {
		t.Error("设置后状态应为 ACTIVE")
	}
}

