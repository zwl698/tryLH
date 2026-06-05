package api

import (
	"aqsystem/models"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func TestNormalizeBacktestStocks(t *testing.T) {
	stocks, err := normalizeBacktestStocks([]string{"sh600519", "600519", "SZ000001", " 300750 "})
	if err != nil {
		t.Fatalf("股票代码规范化失败: %v", err)
	}

	expected := []string{"600519", "000001", "300750"}
	if len(stocks) != len(expected) {
		t.Fatalf("股票数量不正确: 期望 %d 实际 %d", len(expected), len(stocks))
	}
	for i := range expected {
		if stocks[i] != expected[i] {
			t.Fatalf("第%d个股票不正确: 期望 %s 实际 %s", i, expected[i], stocks[i])
		}
	}
}

func TestNormalizeBacktestStocks_Invalid(t *testing.T) {
	if _, err := normalizeBacktestStocks([]string{"60051"}); err == nil {
		t.Fatal("无效股票代码应返回错误")
	}
	if _, err := normalizeBacktestStocks(nil); err == nil {
		t.Fatal("空股票列表应返回错误")
	}
}

func TestEstimateBacktestKLineCount(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 6, 4, 0, 0, 0, 0, time.Local)

	count := estimateBacktestKLineCount(start, end)
	if count < 120 {
		t.Fatalf("估算K线数量过小: %d", count)
	}
	if count <= int(end.Sub(start).Hours()/24) {
		t.Fatalf("估算K线数量应覆盖非交易日缓冲: %d", count)
	}
}

func TestParseSmartDateRangeDefaultsToRecentThreeMonths(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.Local)

	start, end, err := parseSmartDateRange("", "", now)
	if err != nil {
		t.Fatalf("默认智能日期不应报错: %v", err)
	}
	if !end.Equal(now) {
		t.Fatalf("默认结束日期应为当前时间: %v", end)
	}
	expectedStart := time.Date(2026, 3, 5, 10, 0, 0, 0, time.Local)
	if !start.Equal(expectedStart) {
		t.Fatalf("默认开始日期应为最近3个月: %v", start)
	}
}

func TestNewStrategyFromConfigCreatesSmartStrategy(t *testing.T) {
	strat, err := newStrategyFromConfig(models.StrategyConfig{
		ID:          "smart_momentum",
		Name:        "智能动量策略",
		Type:        "momentum",
		Stocks:      []string{"002475", "300750"},
		Params:      map[string]interface{}{"lookback_period": 20, "holding_period": 10, "top_n": 2, "momentum_threshold": 0.05},
		Status:      models.StrategyStatusPaused,
		MaxPosition: decimal.NewFromInt(100000),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("创建智能策略失败: %v", err)
	}
	if strat.ID() != "smart_momentum" || strat.Type() != "momentum" {
		t.Fatalf("智能策略信息不正确: id=%s type=%s", strat.ID(), strat.Type())
	}
	if len(strat.GetConfig().Stocks) != 2 {
		t.Fatalf("智能策略应保留选股结果: %+v", strat.GetConfig().Stocks)
	}
}
