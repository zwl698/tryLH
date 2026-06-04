package broker

import (
	"aqsystem/models"
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func newTestBroker() *SimulatedBroker {
	logger := zap.NewNop()
	cfg := models.BrokerConfig{
		ID:        "test_broker",
		Name:      "测试券商",
		Type:      "simulated",
		AccountID: "TEST001",
	}
	return NewSimulatedBroker(cfg, logger)
}

func TestSimulatedBroker_Login(t *testing.T) {
	b := newTestBroker()

	// 未登录时应返回 false
	if b.IsLoggedIn() {
		t.Error("未登录时 IsLoggedIn 应返回 false")
	}

	// 登录
	err := b.Login(context.Background())
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if !b.IsLoggedIn() {
		t.Error("登录后 IsLoggedIn 应返回 true")
	}

	// 重复登录应报错
	err = b.Login(context.Background())
	if err == nil {
		t.Error("重复登录应返回错误")
	}
}

func TestSimulatedBroker_Logout(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	err := b.Logout(context.Background())
	if err != nil {
		t.Fatalf("登出失败: %v", err)
	}
	if b.IsLoggedIn() {
		t.Error("登出后 IsLoggedIn 应返回 false")
	}
}

func TestSimulatedBroker_GetAccount(t *testing.T) {
	b := newTestBroker()

	// 未登录时获取账户应报错
	_, err := b.GetAccount(context.Background())
	if err == nil {
		t.Error("未登录获取账户应返回错误")
	}

	b.Login(context.Background())

	account, err := b.GetAccount(context.Background())
	if err != nil {
		t.Fatalf("获取账户失败: %v", err)
	}

	if account.ID != "TEST001" {
		t.Errorf("账户ID不正确: 期望 TEST001, 实际 %s", account.ID)
	}
	if !account.Cash.Equal(decimal.NewFromInt(1000000)) {
		t.Errorf("初始资金不正确: 期望 1000000, 实际 %s", account.Cash.String())
	}
}

func TestSimulatedBroker_SubmitOrder_Buy(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	order := &models.Order{
		StockCode: "600000",
		StockName: "浦发银行",
		Market:    models.MarketSH,
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    100,
	}

	result, err := b.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("买入下单失败: %v", err)
	}
	if result.Status != models.OrderStatusFilled {
		t.Errorf("模拟买入应立即成交, 状态: %s", result.Status)
	}
	if result.FilledVol != 100 {
		t.Errorf("成交量不正确: 期望 100, 实际 %d", result.FilledVol)
	}

	// 检查资金扣减
	account, _ := b.GetAccount(context.Background())
	expectedCash := decimal.NewFromInt(1000000).Sub(decimal.NewFromFloat(10.0).Mul(decimal.NewFromInt(100)))
	if !account.Cash.Equal(expectedCash) {
		t.Errorf("买入后资金不正确: 期望 %s, 实际 %s", expectedCash.String(), account.Cash.String())
	}
}

func TestSimulatedBroker_SubmitOrder_BuyInsufficientFunds(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	order := &models.Order{
		StockCode: "600000",
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromInt(999999),
		Volume:    100,
	}

	_, err := b.SubmitOrder(context.Background(), order)
	if err == nil {
		t.Error("资金不足时下单应返回错误")
	}
}

func TestSimulatedBroker_SubmitOrder_Sell(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	// 先买入
	buyOrder := &models.Order{
		StockCode: "600000",
		StockName: "浦发银行",
		Market:    models.MarketSH,
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    100,
	}
	b.SubmitOrder(context.Background(), buyOrder)

	// 模拟持仓变为可用 (直接修改)
	b.mu.Lock()
	if pos, ok := b.positions["600000"]; ok {
		pos.AvailableVol = 100
		b.positions["600000"] = pos
	}
	b.mu.Unlock()

	// 卖出
	sellOrder := &models.Order{
		StockCode: "600000",
		Market:    models.MarketSH,
		Side:      models.OrderSideSell,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(11.0),
		Volume:    100,
	}

	result, err := b.SubmitOrder(context.Background(), sellOrder)
	if err != nil {
		t.Fatalf("卖出下单失败: %v", err)
	}
	if result.Status != models.OrderStatusFilled {
		t.Errorf("模拟卖出应立即成交, 状态: %s", result.Status)
	}
}

func TestSimulatedBroker_SubmitOrder_SellInsufficientPosition(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	sellOrder := &models.Order{
		StockCode: "600000",
		Side:      models.OrderSideSell,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    100,
	}

	_, err := b.SubmitOrder(context.Background(), sellOrder)
	if err == nil {
		t.Error("持仓不足时卖出应返回错误")
	}
}

func TestSimulatedBroker_CancelOrder(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	// 取消不存在的订单
	err := b.CancelOrder(context.Background(), "nonexistent")
	if err == nil {
		t.Error("取消不存在的订单应返回错误")
	}
}

func TestSimulatedBroker_GetOrder(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	order := &models.Order{
		StockCode: "600000",
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    100,
	}
	result, _ := b.SubmitOrder(context.Background(), order)

	found, err := b.GetOrder(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("查询订单失败: %v", err)
	}
	if found.ID != result.ID {
		t.Errorf("订单ID不匹配: 期望 %s, 实际 %s", result.ID, found.ID)
	}
}

func TestSimulatedBroker_GetPositions(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	positions, err := b.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("获取持仓失败: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("初始持仓应为空, 实际 %d", len(positions))
	}

	// 买入后检查持仓
	order := &models.Order{
		StockCode: "600000",
		StockName: "浦发银行",
		Market:    models.MarketSH,
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    200,
	}
	b.SubmitOrder(context.Background(), order)

	positions, _ = b.GetPositions(context.Background())
	if len(positions) != 1 {
		t.Errorf("买入后持仓数量应为 1, 实际 %d", len(positions))
	}
}

func TestSimulatedBroker_UpdatePositionPrices(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	order := &models.Order{
		StockCode: "600000",
		StockName: "浦发银行",
		Market:    models.MarketSH,
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    100,
	}
	b.SubmitOrder(context.Background(), order)

	quotes := map[string]models.StockQuote{
		"600000": {
			StockCode: "600000",
			Close:     decimal.NewFromFloat(12.0),
		},
	}
	b.UpdatePositionPrices(quotes)

	positions, _ := b.GetPositions(context.Background())
	if len(positions) == 0 {
		t.Fatal("没有持仓")
	}
	if !positions[0].CurrentPrice.Equal(decimal.NewFromFloat(12.0)) {
		t.Errorf("更新后价格不正确: 期望 12.0, 实际 %s", positions[0].CurrentPrice.String())
	}
}

func TestNewBroker_Factory(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		brokerType string
		expectErr  bool
	}{
		{"simulated", false},
		{"xtquant", false},
		{"csc", false},
		{"cj", false},
		{"中信建投", false},
		{"长江证券", false},
		{"changjiang", false},
		{"unknown", true},
	}

	for _, tt := range tests {
		cfg := models.BrokerConfig{
			Type:      tt.brokerType,
			AccountID: "test",
			APIURL:    "http://localhost:8888",
		}
		_, err := NewBroker(cfg, logger)
		if tt.expectErr && err == nil {
			t.Errorf("券商类型 %s 应返回错误", tt.brokerType)
		}
		if !tt.expectErr && err != nil {
			t.Errorf("券商类型 %s 不应返回错误: %v", tt.brokerType, err)
		}
	}
}

func TestSimulatedBroker_AddPosition(t *testing.T) {
	b := newTestBroker()
	b.Login(context.Background())

	// 买入100股
	order1 := &models.Order{
		StockCode: "600000",
		StockName: "浦发银行",
		Market:    models.MarketSH,
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(10.0),
		Volume:    100,
	}
	b.SubmitOrder(context.Background(), order1)

	// 再买入100股（加仓）
	order2 := &models.Order{
		StockCode: "600000",
		StockName: "浦发银行",
		Market:    models.MarketSH,
		Side:      models.OrderSideBuy,
		Type:      models.OrderTypeLimit,
		Price:     decimal.NewFromFloat(12.0),
		Volume:    100,
	}
	b.SubmitOrder(context.Background(), order2)

	positions, _ := b.GetPositions(context.Background())
	if len(positions) != 1 {
		t.Fatalf("加仓后持仓数量应为 1, 实际 %d", len(positions))
	}
	if positions[0].Volume != 200 {
		t.Errorf("加仓后持仓数量不正确: 期望 200, 实际 %d", positions[0].Volume)
	}
	// 平均成本应为 (10*100 + 12*100) / 200 = 11
	expectedAvgCost := decimal.NewFromFloat(11.0)
	if !positions[0].AvgCost.Equal(expectedAvgCost) {
		t.Errorf("平均成本不正确: 期望 %s, 实际 %s", expectedAvgCost.String(), positions[0].AvgCost.String())
	}
}

