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
}

