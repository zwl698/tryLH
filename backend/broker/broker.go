package broker

import (
	"aqsystem/models"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Broker 券商交易接口
type Broker interface {
	// 登录认证
	Login(ctx context.Context) error
	Logout(ctx context.Context) error
	IsLoggedIn() bool

	// 账户查询
	GetAccount(ctx context.Context) (*models.Account, error)
	GetPositions(ctx context.Context) ([]models.Position, error)
	GetOrders(ctx context.Context) ([]models.Order, error)

	// 交易下单
	SubmitOrder(ctx context.Context, order *models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, orderID string) error

	// 查询
	GetOrder(ctx context.Context, orderID string) (*models.Order, error)
}

// ==================== 模拟券商 ====================

// SimulatedBroker 模拟券商 - 用于开发测试和回测
type SimulatedBroker struct {
	mu       sync.RWMutex
	loggedIn bool
	account  *models.Account
	orders   map[string]*models.Order
	positions map[string]*models.Position
	config   models.BrokerConfig
	logger   *zap.Logger
}

// NewSimulatedBroker 创建模拟券商
func NewSimulatedBroker(cfg models.BrokerConfig, logger *zap.Logger) *SimulatedBroker {
	return &SimulatedBroker{
		config:   cfg,
		orders:   make(map[string]*models.Order),
		positions: make(map[string]*models.Position),
		logger:   logger,
	}
}

// Login 模拟登录
func (b *SimulatedBroker) Login(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.loggedIn {
		return fmt.Errorf("已经登录，请勿重复登录")
	}

	// 初始化模拟账户 - 100万初始资金
	initCash := decimal.NewFromInt(1000000)
	b.account = &models.Account{
		ID:          b.config.AccountID,
		Name:        "模拟账户",
		BrokerID:    b.config.ID,
		BrokerName:  b.config.Name,
		Cash:        initCash,
		TotalAssets: initCash,
		MarketValue: decimal.Zero,
		FrozenCash:  decimal.Zero,
		YesterdayPL: decimal.Zero,
		TodayPL:     decimal.Zero,
		TotalPL:     decimal.Zero,
		Positions:   []models.Position{},
		Orders:      []models.Order{},
		UpdatedAt:   time.Now(),
	}

	if b.account.ID == "" {
		b.account.ID = "SIM_ACCOUNT_001"
	}

	b.loggedIn = true
	b.logger.Info("模拟券商登录成功", zap.String("account", b.account.ID))
	return nil
}

// Logout 模拟登出
func (b *SimulatedBroker) Logout(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loggedIn = false
	b.logger.Info("模拟券商登出成功")
	return nil
}

// IsLoggedIn 检查登录状态
func (b *SimulatedBroker) IsLoggedIn() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.loggedIn
}

// GetAccount 获取账户信息
func (b *SimulatedBroker) GetAccount(ctx context.Context) (*models.Account, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	// 重新计算总资产
	totalMV := decimal.Zero
	for _, pos := range b.positions {
		totalMV = totalMV.Add(pos.MarketValue)
	}

	account := *b.account
	account.MarketValue = totalMV
	account.TotalAssets = account.Cash.Add(totalMV)
	account.Positions = b.getPositionList()
	account.Orders = b.getOrderList()

	return &account, nil
}

// GetPositions 获取持仓
func (b *SimulatedBroker) GetPositions(ctx context.Context) ([]models.Position, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}
	return b.getPositionList(), nil
}

// GetOrders 获取订单列表
func (b *SimulatedBroker) GetOrders(ctx context.Context) ([]models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}
	return b.getOrderList(), nil
}

// SubmitOrder 提交订单
func (b *SimulatedBroker) SubmitOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// 设置订单ID和时间
	if order.ID == "" {
		order.ID = fmt.Sprintf("ORD_%s_%d", order.StockCode, time.Now().UnixNano())
	}
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	// 模拟买入
	if order.Side == models.OrderSideBuy {
		requiredAmount := order.Price.Mul(decimal.NewFromInt(order.Volume))
		if b.account.Cash.LessThan(requiredAmount) {
			order.Status = models.OrderStatusRejected
			order.Reason = "资金不足"
			b.orders[order.ID] = order
			b.logger.Warn("订单被拒绝：资金不足",
				zap.String("orderID", order.ID),
				zap.String("stock", order.StockCode),
				zap.String("required", requiredAmount.String()),
				zap.String("available", b.account.Cash.String()),
			)
			return order, fmt.Errorf("资金不足，需要 %s，可用 %s", requiredAmount.String(), b.account.Cash.String())
		}

		// 扣除资金
		b.account.Cash = b.account.Cash.Sub(requiredAmount)
		b.account.FrozenCash = b.account.FrozenCash.Add(requiredAmount)

		// 模拟立即成交
		order.FilledVol = order.Volume
		order.FilledPrice = order.Price
		order.Status = models.OrderStatusFilled
		order.UpdatedAt = time.Now()

		// 更新持仓
		b.updatePositionOnBuy(order)

		b.logger.Info("买入成交",
			zap.String("orderID", order.ID),
			zap.String("stock", order.StockCode),
			zap.String("price", order.Price.String()),
			zap.Int64("volume", order.Volume),
		)
	} else if order.Side == models.OrderSideSell {
		// 检查持仓
		pos, exists := b.positions[order.StockCode]
		if !exists || pos.AvailableVol < order.Volume {
			order.Status = models.OrderStatusRejected
			order.Reason = "持仓不足"
			b.orders[order.ID] = order
			return order, fmt.Errorf("持仓不足")
		}

		// 模拟立即成交
		order.FilledVol = order.Volume
		order.FilledPrice = order.Price
		order.Status = models.OrderStatusFilled
		order.UpdatedAt = time.Now()

		// 更新持仓和资金
		b.updatePositionOnSell(order, pos)

		b.logger.Info("卖出成交",
			zap.String("orderID", order.ID),
			zap.String("stock", order.StockCode),
			zap.String("price", order.Price.String()),
			zap.Int64("volume", order.Volume),
		)
	}

	b.orders[order.ID] = order
	return order, nil
}

// CancelOrder 撤单
func (b *SimulatedBroker) CancelOrder(ctx context.Context, orderID string) error {
	if err := b.checkLogin(); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	order, exists := b.orders[orderID]
	if !exists {
		return fmt.Errorf("订单不存在: %s", orderID)
	}

	if order.Status != models.OrderStatusPending && order.Status != models.OrderStatusSubmitted {
		return fmt.Errorf("订单状态不允许撤单: %s", order.Status)
	}

	order.Status = models.OrderStatusCancelled
	order.UpdatedAt = time.Now()
	b.orders[orderID] = order

	b.logger.Info("订单已撤销", zap.String("orderID", orderID))
	return nil
}

// GetOrder 查询订单
func (b *SimulatedBroker) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	order, exists := b.orders[orderID]
	if !exists {
		return nil, fmt.Errorf("订单不存在: %s", orderID)
	}
	return order, nil
}

// UpdatePositionPrices 更新持仓价格（行情回调使用）
func (b *SimulatedBroker) UpdatePositionPrices(quotes map[string]models.StockQuote) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for code, pos := range b.positions {
		if quote, ok := quotes[code]; ok {
			pos.CurrentPrice = quote.Close
			pos.MarketValue = quote.Close.Mul(decimal.NewFromInt(pos.Volume))
			pos.ProfitLoss = pos.MarketValue.Sub(pos.AvgCost.Mul(decimal.NewFromInt(pos.Volume)))
			if !pos.AvgCost.IsZero() {
				pos.ProfitPct = pos.ProfitLoss.Div(pos.AvgCost.Mul(decimal.NewFromInt(pos.Volume))).Mul(decimal.NewFromInt(100))
			}
			pos.UpdatedAt = time.Now()
			b.positions[code] = pos
		}
	}
}

// 内部方法

func (b *SimulatedBroker) checkLogin() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.loggedIn {
		return fmt.Errorf("未登录，请先登录券商")
	}
	return nil
}

func (b *SimulatedBroker) updatePositionOnBuy(order *models.Order) {
	pos, exists := b.positions[order.StockCode]
	if !exists {
		pos = &models.Position{
			StockCode:    order.StockCode,
			StockName:    order.StockName,
			Market:       order.Market,
			Side:         models.PositionSideLong,
			Volume:       order.FilledVol,
			AvailableVol: 0, // T+1，当日买入不可卖
			AvgCost:      order.FilledPrice,
			CurrentPrice: order.FilledPrice,
			MarketValue:  order.FilledPrice.Mul(decimal.NewFromInt(order.FilledVol)),
			ProfitLoss:   decimal.Zero,
			ProfitPct:    decimal.Zero,
			UpdatedAt:    time.Now(),
		}
	} else {
		// 加仓：重新计算平均成本
		totalCost := pos.AvgCost.Mul(decimal.NewFromInt(pos.Volume)).Add(
			order.FilledPrice.Mul(decimal.NewFromInt(order.FilledVol)),
		)
		totalVol := pos.Volume + order.FilledVol
		pos.AvgCost = totalCost.Div(decimal.NewFromInt(totalVol))
		pos.Volume = totalVol
		pos.CurrentPrice = order.FilledPrice
		pos.MarketValue = order.FilledPrice.Mul(decimal.NewFromInt(totalVol))
		pos.UpdatedAt = time.Now()
	}
	b.positions[order.StockCode] = pos

	// 释放冻结资金
	requiredAmount := order.Price.Mul(decimal.NewFromInt(order.Volume))
	b.account.FrozenCash = b.account.FrozenCash.Sub(requiredAmount)
}

func (b *SimulatedBroker) updatePositionOnSell(order *models.Order, pos *models.Position) {
	sellAmount := order.FilledPrice.Mul(decimal.NewFromInt(order.FilledVol))
	b.account.Cash = b.account.Cash.Add(sellAmount)

	// 计算盈亏
	costAmount := pos.AvgCost.Mul(decimal.NewFromInt(order.FilledVol))
	profit := sellAmount.Sub(costAmount)
	b.account.TodayPL = b.account.TodayPL.Add(profit)
	b.account.TotalPL = b.account.TotalPL.Add(profit)

	pos.Volume -= order.FilledVol
	pos.AvailableVol -= order.FilledVol

	if pos.Volume <= 0 {
		delete(b.positions, order.StockCode)
	} else {
		pos.MarketValue = pos.CurrentPrice.Mul(decimal.NewFromInt(pos.Volume))
		pos.ProfitLoss = pos.MarketValue.Sub(pos.AvgCost.Mul(decimal.NewFromInt(pos.Volume)))
		pos.UpdatedAt = time.Now()
		b.positions[order.StockCode] = pos
	}
}

func (b *SimulatedBroker) getPositionList() []models.Position {
	result := make([]models.Position, 0, len(b.positions))
	for _, pos := range b.positions {
		result = append(result, *pos)
	}
	return result
}

func (b *SimulatedBroker) getOrderList() []models.Order {
	result := make([]models.Order, 0, len(b.orders))
	for _, order := range b.orders {
		result = append(result, *order)
	}
	return result
}

