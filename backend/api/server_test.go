package api

import (
	"testing"
	"time"
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
