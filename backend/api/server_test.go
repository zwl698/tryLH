package api

import (
	"aqsystem/models"
	"aqsystem/selector"
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

func TestRankCandidateBacktestsPrefersPositiveRiskAdjustedReturn(t *testing.T) {
	candidates := []candidateBacktest{
		{StockCode: "000001", TotalReturn: -3, MaxDrawdown: 5, SharpeRatio: -0.2, TotalTrades: 2},
		{StockCode: "000002", TotalReturn: 8, MaxDrawdown: 4, SharpeRatio: 1.1, TotalTrades: 3},
		{StockCode: "000003", TotalReturn: 10, MaxDrawdown: 18, SharpeRatio: 0.4, TotalTrades: 4},
	}

	ranked := rankCandidateBacktests(candidates, 2)
	if len(ranked) != 2 {
		t.Fatalf("应返回2个候选，实际 %d", len(ranked))
	}
	if ranked[0].StockCode != "000002" {
		t.Fatalf("应优先选择风险调整后更好的股票，实际 %+v", ranked[0])
	}
	if ranked[0].Rank != 1 || ranked[1].Rank != 2 {
		t.Fatalf("排序后应写入验证排名: %+v", ranked)
	}
	if ranked[0].RankScore <= ranked[1].RankScore {
		t.Fatalf("排序分数应递减: %+v", ranked)
	}
}

func TestRankCandidateBacktestsTopZeroReturnsAllCandidates(t *testing.T) {
	candidates := []candidateBacktest{
		{StockCode: "000001", TotalReturn: -3, MaxDrawdown: 5, SharpeRatio: -0.2, TotalTrades: 2},
		{StockCode: "000002", TotalReturn: 8, MaxDrawdown: 4, SharpeRatio: 1.1, TotalTrades: 3},
		{StockCode: "000003", TotalReturn: 0, MaxDrawdown: 2, SharpeRatio: 0, TotalTrades: 0},
	}

	ranked := rankCandidateBacktests(candidates, 0)
	if len(ranked) != len(candidates) {
		t.Fatalf("topN=0 应返回全量候选，实际 %+v", ranked)
	}
	for i := range ranked {
		if ranked[i].Rank != i+1 {
			t.Fatalf("全量候选应保留连续验证排名，实际 %+v", ranked)
		}
	}
}

func TestMarkSelectedBacktestsKeepsLossSamplesVisible(t *testing.T) {
	all := []candidateBacktest{
		{StockCode: "000001", TotalReturn: -3, Outcome: "loss"},
		{StockCode: "000002", TotalReturn: 8, Outcome: "profit"},
	}
	selected := []candidateBacktest{{StockCode: "000002"}}

	marked := markSelectedBacktests(all, selected)
	if len(marked) != 2 {
		t.Fatalf("标记入选时不应丢弃亏损样本: %+v", marked)
	}
	if marked[0].Selected || !marked[1].Selected {
		t.Fatalf("入选标记不正确: %+v", marked)
	}
}

func TestReorderPicksByBacktestResetsRanks(t *testing.T) {
	picks := []selector.StockPick{
		{Rank: 1, StockCode: "000001", Score: 70},
		{Rank: 2, StockCode: "000002", Score: 65},
	}
	ranked := []candidateBacktest{
		{StockCode: "000002", TotalReturn: 10, MaxDrawdown: 3, SharpeRatio: 1.2, TotalTrades: 2, RankScore: 15},
	}

	reordered := reorderPicksByBacktest(picks, ranked)
	if len(reordered) != 1 {
		t.Fatalf("应只保留回测排序股票，实际 %+v", reordered)
	}
	if reordered[0].StockCode != "000002" || reordered[0].Rank != 1 {
		t.Fatalf("二次验证重排后应重置排名，实际 %+v", reordered[0])
	}
}

func TestBuildValidationSummaryReportsPositiveRateAndWarnings(t *testing.T) {
	summary := buildValidationSummary([]candidateBacktest{
		{StockCode: "000001", TotalReturn: -2, MaxDrawdown: 6, SharpeRatio: -0.2},
		{StockCode: "000002", TotalReturn: 8, MaxDrawdown: 4, SharpeRatio: 1.1},
		{StockCode: "000003", TotalReturn: 4, MaxDrawdown: 24, SharpeRatio: 0.2},
	})

	if summary.ValidatedCount != 3 || summary.PositiveCount != 2 {
		t.Fatalf("验证摘要数量不正确: %+v", summary)
	}
	if summary.NegativeCount != 1 || summary.FlatCount != 0 {
		t.Fatalf("验证摘要应统计亏损/持平数量: %+v", summary)
	}
	if summary.PositiveRate != 66.67 {
		t.Fatalf("正收益率应按百分比四舍五入，实际 %.2f", summary.PositiveRate)
	}
	if summary.BestReturn != 8 || summary.WorstDrawdown != 24 {
		t.Fatalf("最佳收益/最坏回撤不正确: %+v", summary)
	}
	if len(summary.Warnings) == 0 {
		t.Fatalf("高回撤候选应产生风险提示: %+v", summary)
	}
	if !summary.Deployable {
		t.Fatalf("存在正收益候选时可进入模拟/人工复核流程: %+v", summary)
	}
}

func TestBuildValidationSummaryBlocksAllLosingCandidates(t *testing.T) {
	summary := buildValidationSummary([]candidateBacktest{
		{StockCode: "000001", TotalReturn: -2, MaxDrawdown: 6, SharpeRatio: -0.2},
		{StockCode: "000002", TotalReturn: -1, MaxDrawdown: 4, SharpeRatio: -0.1},
	})

	if summary.PositiveCount != 0 || summary.NegativeCount != 2 {
		t.Fatalf("全亏候选统计不正确: %+v", summary)
	}
	if summary.Deployable || summary.GateReason == "" {
		t.Fatalf("全亏候选应触发实盘门禁: %+v", summary)
	}
}
