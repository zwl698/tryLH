package backtest

import (
	"aqsystem/models"
	"aqsystem/strategy"
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func generateTestKLines(code string, days int, startPrice float64, trend float64) []models.KLine {
	var klines []models.KLine
	price := startPrice
	startDate := time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local)

	for i := 0; i < days; i++ {
		date := startDate.AddDate(0, 0, i)
		// 跳过周末
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}

		kline := models.KLine{
			StockCode: code,
			Period:    "day",
			Open:      decimal.NewFromFloat(price),
			High:      decimal.NewFromFloat(price + 0.2),
			Low:       decimal.NewFromFloat(price - 0.2),
			Close:     decimal.NewFromFloat(price + trend),
			Volume:    1000000,
			Amount:    decimal.NewFromFloat(price * 1000000),
			Timestamp: date,
		}
		klines = append(klines, kline)
		price += trend
	}
	return klines
}

func TestBacktestEngine_Run_DoubleMA(t *testing.T) {
	logger := zap.NewNop()
	btEngine := NewBacktestEngine(0.0003, 0.001, 0.001, logger)

	cfg := models.StrategyConfig{
		ID:     "bt_double_ma",
		Name:   "回测双均线",
		Type:   "double_ma",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"short_period": 5,
			"long_period":  10,
			"ma_type":      "SMA",
		},
		MaxPosition: decimal.NewFromFloat(50000),
		Status:      models.StrategyStatusPaused,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s := strategy.NewDoubleMAStrategy(cfg, logger)

	klines := map[string][]models.KLine{
		"600000": generateTestKLines("600000", 60, 10.0, 0.05),
	}

	startDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)
	endDate := time.Date(2024, 4, 1, 0, 0, 0, 0, time.Local)
	initialCash := decimal.NewFromInt(1000000)

	result, err := btEngine.Run(context.Background(), s, klines, startDate, endDate, initialCash)
	if err != nil {
		t.Fatalf("回测失败: %v", err)
	}

	if result == nil {
		t.Fatal("回测结果不应为 nil")
	}
	if result.StrategyID != "bt_double_ma" {
		t.Errorf("策略ID不正确: %s", result.StrategyID)
	}
	if result.InitialCapital != initialCash {
		t.Errorf("初始资金不正确: %s", result.InitialCapital.String())
	}
}

func TestBacktestEngine_Run_Turtle(t *testing.T) {
	logger := zap.NewNop()
	btEngine := NewBacktestEngine(0.0003, 0.001, 0.001, logger)

	cfg := models.StrategyConfig{
		ID:     "bt_turtle",
		Name:   "回测海龟",
		Type:   "turtle",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"entry_period": 10,
			"exit_period":  5,
			"atr_period":   10,
			"risk_pct":     0.01,
		},
		MaxPosition: decimal.NewFromFloat(50000),
		Status:      models.StrategyStatusPaused,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s := strategy.NewTurtleStrategy(cfg, logger)

	klines := map[string][]models.KLine{
		"600000": generateTestKLines("600000", 60, 10.0, 0.05),
	}

	startDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)
	endDate := time.Date(2024, 4, 1, 0, 0, 0, 0, time.Local)

	result, err := btEngine.Run(context.Background(), s, klines, startDate, endDate, decimal.NewFromInt(1000000))
	if err != nil {
		t.Fatalf("回测失败: %v", err)
	}
	if result == nil {
		t.Fatal("回测结果不应为 nil")
	}
}

func TestBacktestEngine_Run_MACDT(t *testing.T) {
	logger := zap.NewNop()
	btEngine := NewBacktestEngine(0.0003, 0.001, 0.001, logger)

	cfg := models.StrategyConfig{
		ID:     "bt_macd_t",
		Name:   "回测MACD做T",
		Type:   "macd_t",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{
			"fast_period":     12,
			"slow_period":     26,
			"signal_period":   9,
			"trend_period":    20,
			"hist_turn_days":  3,
			"max_hold_days":   5,
			"take_profit_pct": 0.018,
			"stop_loss_pct":   0.018,
		},
		MaxPosition: decimal.NewFromFloat(100000),
		Status:      models.StrategyStatusPaused,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s := strategy.NewMACDTStrategy(cfg, logger)
	klines := map[string][]models.KLine{
		"600000": generateMACDTBacktestKLines("600000"),
	}

	result, err := btEngine.Run(
		context.Background(),
		s,
		klines,
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local),
		time.Date(2024, 3, 31, 0, 0, 0, 0, time.Local),
		decimal.NewFromInt(1000000),
	)
	if err != nil {
		t.Fatalf("回测失败: %v", err)
	}
	if result == nil {
		t.Fatal("回测结果不应为 nil")
	}
	if result.StrategyID != "bt_macd_t" {
		t.Fatalf("策略ID不正确: %s", result.StrategyID)
	}
	if result.ExecutionModel != "next_open" {
		t.Fatalf("MACD做T回测应沿用下一交易日开盘成交模型，实际 %s", result.ExecutionModel)
	}
	if result.TotalTrades == 0 {
		t.Fatal("MACD做T回测应在柱线改善后完成至少一笔平仓交易")
	}
	if len(result.Trades) != result.TotalTrades {
		t.Fatalf("交易明细数量应等于完成交易数，明细 %d 完成 %d", len(result.Trades), result.TotalTrades)
	}
}

func TestBacktestEngine_CalcCommission(t *testing.T) {
	logger := zap.NewNop()
	btEngine := NewBacktestEngine(0.0003, 0.001, 0.001, logger)

	// 大额交易 - 正常佣金
	amount := decimal.NewFromInt(100000)
	commission := btEngine.calcCommission(amount, true)
	expectedCommission := amount.Mul(decimal.NewFromFloat(0.0003))
	if !commission.Equal(expectedCommission) {
		t.Errorf("佣金计算不正确: 期望 %s, 实际 %s", expectedCommission.String(), commission.String())
	}

	// 小额交易 - 最低5元
	smallAmount := decimal.NewFromInt(100)
	commission = btEngine.calcCommission(smallAmount, true)
	if !commission.Equal(decimal.NewFromInt(5)) {
		t.Errorf("最低佣金应为5元: 实际 %s", commission.String())
	}
}

func TestBacktestEngine_CompletedTradeStats(t *testing.T) {
	logger := zap.NewNop()
	btEngine := NewBacktestEngine(0.0003, 0.001, 0.001, logger)
	state := &BacktestState{
		Cash:       decimal.NewFromInt(1000000),
		Positions:  make(map[string]*backtestPosition),
		LastPrices: make(map[string]decimal.Decimal),
	}
	date := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)

	btEngine.executeSignal(state, models.Signal{
		StockCode: "600000",
		Side:      models.OrderSideBuy,
		Price:     decimal.NewFromInt(10),
		Volume:    1000,
	}, date)
	if state.TotalTrades != 0 {
		t.Fatalf("买入不应计入完成交易次数，实际 %d", state.TotalTrades)
	}

	btEngine.executeSignal(state, models.Signal{
		StockCode: "600000",
		Side:      models.OrderSideSell,
		Price:     decimal.NewFromInt(11),
		Volume:    1000,
	}, date.AddDate(0, 0, 1))
	if state.TotalTrades != 1 {
		t.Fatalf("卖出平仓应计入1笔完成交易，实际 %d", state.TotalTrades)
	}
	if state.WinTrades != 1 || state.LossTrades != 0 {
		t.Fatalf("胜负统计不正确，赢 %d 亏 %d", state.WinTrades, state.LossTrades)
	}
	if len(state.Trades) != state.TotalTrades {
		t.Fatalf("完成交易数应等于交易记录数，记录 %d 总数 %d", len(state.Trades), state.TotalTrades)
	}
}

type oneShotCloseSignalStrategy struct {
	strategy.BaseStrategy
	emitted bool
}

func newOneShotCloseSignalStrategy() *oneShotCloseSignalStrategy {
	cfg := models.StrategyConfig{
		ID:     "one_shot",
		Name:   "下一开盘测试策略",
		Type:   "one_shot",
		Stocks: []string{"600000"},
		Params: map[string]interface{}{},
		Status: models.StrategyStatusPaused,
	}
	return &oneShotCloseSignalStrategy{
		BaseStrategy: strategy.NewBaseStrategy(cfg, zap.NewNop()),
	}
}

func (s *oneShotCloseSignalStrategy) Type() string { return "one_shot" }

func (s *oneShotCloseSignalStrategy) Init(config models.StrategyConfig) error {
	s.emitted = false
	return nil
}

func (s *oneShotCloseSignalStrategy) OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error) {
	if s.emitted {
		return nil, nil
	}
	s.emitted = true
	return []models.Signal{{
		StrategyID: s.ID(),
		StockCode:  kline.StockCode,
		Side:       models.OrderSideBuy,
		Type:       models.OrderTypeLimit,
		Price:      kline.Close,
		Volume:     100,
		Timestamp:  kline.Timestamp,
	}}, nil
}

func (s *oneShotCloseSignalStrategy) OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *oneShotCloseSignalStrategy) OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func TestBacktestEngine_ExecutesDailySignalAtNextOpen(t *testing.T) {
	btEngine := NewBacktestEngine(0, 0, 0, zap.NewNop())
	klines := map[string][]models.KLine{
		"600000": {
			{
				StockCode: "600000",
				Period:    "day",
				Open:      decimal.NewFromInt(10),
				High:      decimal.NewFromInt(20),
				Low:       decimal.NewFromInt(10),
				Close:     decimal.NewFromInt(20),
				Volume:    1000000,
				Timestamp: time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local),
			},
			{
				StockCode: "600000",
				Period:    "day",
				Open:      decimal.NewFromInt(30),
				High:      decimal.NewFromInt(31),
				Low:       decimal.NewFromInt(29),
				Close:     decimal.NewFromInt(30),
				Volume:    1000000,
				Timestamp: time.Date(2024, 1, 3, 15, 0, 0, 0, time.Local),
			},
		},
	}

	result, err := btEngine.Run(
		context.Background(),
		newOneShotCloseSignalStrategy(),
		klines,
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.Local),
		decimal.NewFromInt(10000),
	)
	if err != nil {
		t.Fatalf("回测失败: %v", err)
	}
	if result.ExecutionModel != "next_open" {
		t.Fatalf("成交模型应标记为下一交易日开盘，实际 %s", result.ExecutionModel)
	}
	expectedFinal := decimal.NewFromInt(9995)
	if !result.FinalCapital.Equal(expectedFinal) {
		t.Fatalf("收盘信号应在下一交易日开盘30元成交并扣5元最低佣金，期望最终资金 %s，实际 %s", expectedFinal, result.FinalCapital)
	}
}

func TestBacktestEngine_Sqrt(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{4.0, 2.0},
		{9.0, 3.0},
		{0, 0},
		{-1, 0},
		{2.0, 1.4142},
	}

	for _, tt := range tests {
		result := sqrt(tt.input)
		if tt.input > 0 {
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("sqrt(%f) = %f, 期望约 %f", tt.input, result, tt.expected)
			}
		} else {
			if result != 0 {
				t.Errorf("sqrt(%f) 应返回 0", tt.input)
			}
		}
	}
}

func TestBacktestEngine_OrganizeByDate(t *testing.T) {
	logger := zap.NewNop()
	btEngine := NewBacktestEngine(0.0003, 0.001, 0.001, logger)

	klines := map[string][]models.KLine{
		"600000": {
			{StockCode: "600000", Timestamp: time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local)},
			{StockCode: "600000", Timestamp: time.Date(2024, 1, 3, 15, 0, 0, 0, time.Local)},
		},
	}

	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(2024, 1, 31, 0, 0, 0, 0, time.Local)

	result := btEngine.organizeByDate(klines, startDate, endDate)
	if len(result) != 2 {
		t.Errorf("应组织出2天的数据, 实际 %d 天", len(result))
	}

	expectedDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)
	if _, ok := result[expectedDate]; !ok {
		t.Errorf("应使用本地自然日作为回测日期: %s", expectedDate)
	}
}

func generateMACDTBacktestKLines(code string) []models.KLine {
	rows := make([]models.KLine, 0, 80)
	baseDate := time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local)

	for i := 0; i < 35; i++ {
		close := 20.0 - float64(i+1)*0.12
		rows = append(rows, backtestKLine(code, close, int64(1000000+i*10000), baseDate.AddDate(0, 0, i)))
	}
	for i := 35; i < 80; i++ {
		close := 15.8 + float64(i-34)*0.28
		rows = append(rows, backtestKLine(code, close, int64(1500000+(i%7)*50000), baseDate.AddDate(0, 0, i)))
	}

	return rows
}

func backtestKLine(code string, close float64, volume int64, at time.Time) models.KLine {
	return models.KLine{
		StockCode: code,
		Period:    "day",
		Open:      decimal.NewFromFloat(close),
		High:      decimal.NewFromFloat(close * 1.015),
		Low:       decimal.NewFromFloat(close * 0.985),
		Close:     decimal.NewFromFloat(close),
		Volume:    volume,
		Amount:    decimal.NewFromFloat(close * float64(volume)),
		Timestamp: at,
	}
}
