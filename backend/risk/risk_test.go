package risk

import (
	"aqsystem/models"
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func newTestRiskManager() *RiskManager {
	cfg := models.RiskConfig{
		MaxSinglePositionPct: decimal.NewFromFloat(0.3),
		MaxTotalPositionPct:  decimal.NewFromFloat(0.8),
		MaxDailyLossPct:      decimal.NewFromFloat(0.05),
		MaxDrawdownPct:       decimal.NewFromFloat(0.15),
		MaxDailyTrades:       50,
		StopLossPct:          decimal.NewFromFloat(0.08),
		TakeProfitPct:        decimal.NewFromFloat(0.2),
		BlacklistStocks:      []string{"688999"},
	}
	return NewRiskManager(cfg, zap.NewNop())
}

func TestRiskManager_BlacklistCheck(t *testing.T) {
	rm := newTestRiskManager()

	order := &models.Order{
		StockCode:  "688999",
		Side:       models.OrderSideBuy,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     100,
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(500000),
	}

	err := rm.CheckOrder(context.Background(), order, account)
	if err == nil {
		t.Error("黑名单股票应被风控拒绝")
	}
}

func TestRiskManager_MaxDailyTrades(t *testing.T) {
	rm := newTestRiskManager()

	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(500000),
		MarketValue: decimal.Zero,
	}

	// 模拟达到日交易次数上限
	for i := 0; i < 50; i++ {
		rm.RecordTrade(&models.Order{
			StockCode: "600000",
			Side:      models.OrderSideBuy,
			Price:     decimal.NewFromFloat(10.0),
			Volume:    100,
			FilledVol: 100,
		})
	}

	order := &models.Order{
		StockCode:  "600001",
		Side:       models.OrderSideBuy,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     100,
		StrategyID: "test",
	}

	err := rm.CheckOrder(context.Background(), order, account)
	if err == nil {
		t.Error("日交易次数达到上限应被风控拒绝")
	}
}

func TestRiskManager_BuyOrder_SinglePositionExceeded(t *testing.T) {
	rm := newTestRiskManager()

	// 订单金额超过总资产的30%
	order := &models.Order{
		StockCode:  "600000",
		Side:       models.OrderSideBuy,
		Price:      decimal.NewFromFloat(50.0),
		Volume:     10000, // 50*10000 = 500000, 超过30%的100万
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(800000),
		MarketValue: decimal.Zero,
	}

	err := rm.CheckOrder(context.Background(), order, account)
	if err == nil {
		t.Error("单股仓位超限应被风控拒绝")
	}
}

func TestRiskManager_BuyOrder_InsufficientCash(t *testing.T) {
	rm := newTestRiskManager()

	order := &models.Order{
		StockCode:  "600000",
		Side:       models.OrderSideBuy,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     1000, // 10*1000 = 10000
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(5000), // 资金不足
		MarketValue: decimal.Zero,
		Positions:   []models.Position{},
	}

	err := rm.CheckOrder(context.Background(), order, account)
	if err == nil {
		t.Error("资金不足应被风控拒绝")
	}
}

func TestRiskManager_SellOrder_NoPosition(t *testing.T) {
	rm := newTestRiskManager()

	order := &models.Order{
		StockCode:  "600000",
		Side:       models.OrderSideSell,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     100,
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(500000),
		Positions:   []models.Position{},
	}

	err := rm.CheckOrder(context.Background(), order, account)
	if err == nil {
		t.Error("无持仓卖出应被风控拒绝")
	}
}

func TestRiskManager_ValidBuyOrder(t *testing.T) {
	rm := newTestRiskManager()

	order := &models.Order{
		StockCode:  "600000",
		Side:       models.OrderSideBuy,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     100, // 10*100 = 1000
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(500000),
		MarketValue: decimal.Zero,
		Positions:   []models.Position{},
	}

	err := rm.CheckOrder(context.Background(), order, account)
	if err != nil {
		t.Errorf("有效买入订单不应被拒绝: %v", err)
	}
}

func TestRiskManager_ValidSellOrder(t *testing.T) {
	rm := newTestRiskManager()

	order := &models.Order{
		StockCode:  "600000",
		Side:       models.OrderSideSell,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     100,
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(500000),
		Positions: []models.Position{
			{
				StockCode:    "600000",
				AvailableVol: 200,
			},
		},
	}

	err := rm.CheckOrder(context.Background(), order, account)
	if err != nil {
		t.Errorf("有效卖出订单不应被拒绝: %v", err)
	}
}

func TestRiskManager_CheckStopLoss(t *testing.T) {
	rm := newTestRiskManager()

	positions := []models.Position{
		{
			StockCode:    "600000",
			StockName:    "浦发银行",
			CurrentPrice: decimal.NewFromFloat(9.0),
			AvgCost:      decimal.NewFromFloat(10.0),
			AvailableVol: 100,
			ProfitPct:    decimal.NewFromFloat(-10.0), // 亏损10%，超过8%止损线
		},
	}

	signals := rm.CheckStopLoss(positions)
	if len(signals) == 0 {
		t.Error("亏损超过止损线应产生止损信号")
	}
	if signals[0].Side != models.OrderSideSell {
		t.Error("止损信号应为卖出")
	}
}

func TestRiskManager_CheckTakeProfit(t *testing.T) {
	rm := newTestRiskManager()

	positions := []models.Position{
		{
			StockCode:    "600000",
			StockName:    "浦发银行",
			CurrentPrice: decimal.NewFromFloat(12.5),
			AvgCost:      decimal.NewFromFloat(10.0),
			AvailableVol: 100,
			ProfitPct:    decimal.NewFromFloat(25.0), // 盈利25%，超过20%止盈线
		},
	}

	signals := rm.CheckTakeProfit(positions)
	if len(signals) == 0 {
		t.Error("盈利超过止盈线应产生止盈信号")
	}
	if signals[0].Side != models.OrderSideSell {
		t.Error("止盈信号应为卖出")
	}
}

func TestRiskManager_ResetDaily(t *testing.T) {
	rm := newTestRiskManager()

	// 模拟交易
	rm.RecordTrade(&models.Order{
		StockCode: "600000",
		Side:      models.OrderSideBuy,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    100,
		FilledVol: 100,
	})

	rm.ResetDaily()
	events := rm.GetEvents(0)
	_ = events // 重置后不影响事件记录
}

func TestRiskManager_UpdateEquity(t *testing.T) {
	rm := newTestRiskManager()

	rm.UpdateEquity(decimal.NewFromInt(1000000))
	rm.UpdateEquity(decimal.NewFromInt(1100000))
	rm.UpdateEquity(decimal.NewFromInt(900000)) // 回撤
}

func TestRiskManager_GetEvents(t *testing.T) {
	rm := newTestRiskManager()

	// 触发一个风控事件
	order := &models.Order{
		StockCode:  "688999",
		Side:       models.OrderSideBuy,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     100,
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(500000),
	}

	rm.CheckOrder(context.Background(), order, account)

	events := rm.GetEvents(0)
	if len(events) == 0 {
		t.Error("应有风控事件记录")
	}

	// 限制数量
	limitedEvents := rm.GetEvents(1)
	if len(limitedEvents) != 1 {
		t.Errorf("限制数量应返回1条, 实际 %d", len(limitedEvents))
	}
}

func TestRiskManager_UpdateAndGetConfig(t *testing.T) {
	rm := newTestRiskManager()

	newCfg := models.RiskConfig{
		MaxSinglePositionPct: decimal.NewFromFloat(0.5),
		MaxTotalPositionPct:  decimal.NewFromFloat(0.9),
		MaxDailyLossPct:      decimal.NewFromFloat(0.03),
		MaxDrawdownPct:       decimal.NewFromFloat(0.1),
		MaxDailyTrades:       30,
		StopLossPct:          decimal.NewFromFloat(0.05),
		TakeProfitPct:        decimal.NewFromFloat(0.15),
	}

	rm.UpdateConfig(newCfg)
	config := rm.GetConfig()

	if !config.MaxSinglePositionPct.Equal(decimal.NewFromFloat(0.5)) {
		t.Error("更新风控配置失败")
	}
	if config.MaxDailyTrades != 30 {
		t.Error("更新日交易次数限制失败")
	}
}

func TestRiskManager_Callback(t *testing.T) {
	rm := newTestRiskManager()

	callbackCalled := false
	rm.OnRiskEvent(func(event models.RiskEvent) {
		callbackCalled = true
	})

	// 触发风控事件
	order := &models.Order{
		StockCode:  "688999",
		Side:       models.OrderSideBuy,
		Price:      decimal.NewFromFloat(10.0),
		Volume:     100,
		StrategyID: "test",
	}
	account := &models.Account{
		TotalAssets: decimal.NewFromInt(1000000),
		Cash:        decimal.NewFromInt(500000),
	}

	rm.CheckOrder(context.Background(), order, account)

	if !callbackCalled {
		t.Error("风控事件应触发回调")
	}
}

