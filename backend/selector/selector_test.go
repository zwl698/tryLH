package selector

import (
	"aqsystem/models"
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

type fakeMarketData struct {
	klines map[string][]models.KLine
}

func (f fakeMarketData) GetKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error) {
	rows := f.klines[stockCode]
	if count > 0 && len(rows) > count {
		rows = rows[len(rows)-count:]
	}
	return rows, nil
}

func TestDefaultPlanForStrategy(t *testing.T) {
	cases := map[string]string{
		"double_ma":        "trend_breakout",
		"turtle":           "trend_breakout",
		"momentum":         "momentum_strength",
		"mean_reversion":   "oversold_rebound",
		"grid":             "grid_suitable",
		"unknown_strategy": "balanced_smart",
	}

	for strategyType, expectedPlan := range cases {
		plan := DefaultPlanForStrategy(strategyType)
		if plan.ID != expectedPlan {
			t.Fatalf("%s 默认选股方案应为 %s，实际 %s", strategyType, expectedPlan, plan.ID)
		}
	}
}

func TestEngineSelectMomentumRanksStrongStocks(t *testing.T) {
	engine := NewEngine(fakeMarketData{klines: map[string][]models.KLine{
		"000001": makeTrendKLines("000001", 10, 0.012, 90),
		"000002": makeTrendKLines("000002", 10, 0.002, 90),
		"000003": makeTrendKLines("000003", 20, -0.004, 90),
	}})

	result, err := engine.Select(context.Background(), SelectionRequest{
		StrategyType:   "momentum",
		CandidateCodes: []string{"000001", "000002", "000003"},
		TopN:           2,
		LookbackDays:   80,
	})
	if err != nil {
		t.Fatalf("选股失败: %v", err)
	}

	if result.Plan.ID != "momentum_strength" {
		t.Fatalf("动量策略应自动匹配 momentum_strength，实际 %s", result.Plan.ID)
	}
	if len(result.Picks) != 2 {
		t.Fatalf("应返回2只股票，实际 %d", len(result.Picks))
	}
	if result.Picks[0].StockCode != "000001" {
		t.Fatalf("强趋势股票应排第一，实际 %+v", result.Picks[0])
	}
	if result.Picks[0].Score <= result.Picks[1].Score {
		t.Fatalf("第一名得分应高于第二名: %+v", result.Picks)
	}
}

func TestEngineSelectGridPrefersRangeBoundStocks(t *testing.T) {
	engine := NewEngine(fakeMarketData{klines: map[string][]models.KLine{
		"000001": makeOscillatingKLines("000001", 20, 0.035, 90),
		"000002": makeTrendKLines("000002", 10, 0.018, 90),
		"000003": makeTrendKLines("000003", 20, -0.015, 90),
	}})

	result, err := engine.Select(context.Background(), SelectionRequest{
		StrategyType:   "grid",
		CandidateCodes: []string{"000001", "000002", "000003"},
		TopN:           1,
		LookbackDays:   80,
	})
	if err != nil {
		t.Fatalf("选股失败: %v", err)
	}

	if result.Plan.ID != "grid_suitable" {
		t.Fatalf("网格策略应自动匹配 grid_suitable，实际 %s", result.Plan.ID)
	}
	if len(result.Picks) != 1 || result.Picks[0].StockCode != "000001" {
		t.Fatalf("震荡股应最适合网格，实际 %+v", result.Picks)
	}
}

func makeTrendKLines(code string, start float64, dailyReturn float64, days int) []models.KLine {
	rows := make([]models.KLine, 0, days)
	price := start
	baseDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < days; i++ {
		open := price
		close := price * (1 + dailyReturn)
		high := maxFloat(open, close) * 1.01
		low := minFloat(open, close) * 0.99
		rows = append(rows, testKLine(code, open, high, low, close, int64(1000000+i*10000), baseDate.AddDate(0, 0, i)))
		price = close
	}
	return rows
}

func makeOscillatingKLines(code string, base float64, amplitude float64, days int) []models.KLine {
	rows := make([]models.KLine, 0, days)
	baseDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < days; i++ {
		offset := amplitude
		if i%2 == 1 {
			offset = -amplitude
		}
		close := base * (1 + offset)
		open := base * (1 - offset/2)
		high := maxFloat(open, close) * 1.01
		low := minFloat(open, close) * 0.99
		rows = append(rows, testKLine(code, open, high, low, close, int64(1200000+i%10*30000), baseDate.AddDate(0, 0, i)))
	}
	return rows
}

func testKLine(code string, open, high, low, close float64, volume int64, at time.Time) models.KLine {
	return models.KLine{
		StockCode: code,
		Period:    "day",
		Open:      decimal.NewFromFloat(open),
		High:      decimal.NewFromFloat(high),
		Low:       decimal.NewFromFloat(low),
		Close:     decimal.NewFromFloat(close),
		Volume:    volume,
		Amount:    decimal.NewFromFloat(close).Mul(decimal.NewFromInt(volume)),
		Timestamp: at,
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
