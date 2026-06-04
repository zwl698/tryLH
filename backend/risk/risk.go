package risk

import (
	"aqsystem/models"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// RiskManager 风控管理器
type RiskManager struct {
	mu        sync.RWMutex
	config    models.RiskConfig
	logger    *zap.Logger
	events    []models.RiskEvent
	dailyPL   decimal.Decimal    // 当日盈亏
	dailyTrades int              // 当日交易次数
	peakEquity decimal.Decimal   // 权益峰值
	account   *models.Account    // 账户快照
	callbacks []RiskCallback     // 风控回调
}

// RiskCallback 风控回调函数
type RiskCallback func(event models.RiskEvent)

// NewRiskManager 创建风控管理器
func NewRiskManager(config models.RiskConfig, logger *zap.Logger) *RiskManager {
	return &RiskManager{
		config:  config,
		logger:  logger,
		events:  make([]models.RiskEvent, 0),
	}
}

// CheckOrder 检查订单是否通过风控
func (r *RiskManager) CheckOrder(ctx context.Context, order *models.Order, account *models.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.account = account

	// 1. 检查黑名单
	if r.isBlacklisted(order.StockCode) {
		r.emitEvent(models.RiskLevelCritical, "BLACKLIST", fmt.Sprintf("股票 %s 在黑名单中", order.StockCode), order.StockCode, order.StrategyID)
		return fmt.Errorf("股票 %s 在交易黑名单中", order.StockCode)
	}

	// 2. 检查日交易次数
	if r.dailyTrades >= r.config.MaxDailyTrades {
		r.emitEvent(models.RiskLevelHigh, "MAX_TRADES", fmt.Sprintf("日交易次数已达上限 %d", r.config.MaxDailyTrades), order.StockCode, order.StrategyID)
		return fmt.Errorf("日交易次数已达上限 %d", r.config.MaxDailyTrades)
	}

	// 3. 买入风控
	if order.Side == models.OrderSideBuy {
		if err := r.checkBuyOrder(order, account); err != nil {
			return err
		}
	}

	// 4. 卖出风控
	if order.Side == models.OrderSideSell {
		if err := r.checkSellOrder(order, account); err != nil {
			return err
		}
	}

	// 5. 检查日亏损限制
	if !r.dailyPL.IsZero() {
		if !account.TotalAssets.IsZero() {
			dailyLossPct := r.dailyPL.Div(account.TotalAssets).Abs()
			if dailyLossPct.GreaterThan(r.config.MaxDailyLossPct) {
				r.emitEvent(models.RiskLevelCritical, "DAILY_LOSS", fmt.Sprintf("日亏损 %.2f%% 超过限制 %.2f%%", dailyLossPct.InexactFloat64()*100, r.config.MaxDailyLossPct.InexactFloat64()*100), order.StockCode, order.StrategyID)
				return fmt.Errorf("日亏损已超过限制 %.2f%%，暂停交易", r.config.MaxDailyLossPct.InexactFloat64()*100)
			}
		}
	}

	// 6. 检查最大回撤
	if !r.peakEquity.IsZero() && !account.TotalAssets.IsZero() {
		drawdown := decimal.NewFromInt(1).Sub(account.TotalAssets.Div(r.peakEquity))
		if drawdown.GreaterThan(r.config.MaxDrawdownPct) {
			r.emitEvent(models.RiskLevelCritical, "MAX_DRAWDOWN", fmt.Sprintf("回撤 %.2f%% 超过限制 %.2f%%", drawdown.InexactFloat64()*100, r.config.MaxDrawdownPct.InexactFloat64()*100), order.StockCode, order.StrategyID)
			return fmt.Errorf("账户回撤已超过限制 %.2f%%，暂停交易", r.config.MaxDrawdownPct.InexactFloat64()*100)
		}
	}

	return nil
}

// checkBuyOrder 买入风控检查
func (r *RiskManager) checkBuyOrder(order *models.Order, account *models.Account) error {
	// 检查单股仓位限制
	orderAmount := order.Price.Mul(decimal.NewFromInt(order.Volume))
	if !account.TotalAssets.IsZero() {
		positionPct := orderAmount.Div(account.TotalAssets)
		if positionPct.GreaterThan(r.config.MaxSinglePositionPct) {
			r.emitEvent(models.RiskLevelHigh, "SINGLE_POSITION", fmt.Sprintf("单股仓位 %.2f%% 超过限制 %.2f%%", positionPct.InexactFloat64()*100, r.config.MaxSinglePositionPct.InexactFloat64()*100), order.StockCode, order.StrategyID)
			return fmt.Errorf("单股仓位 %.2f%% 超过限制 %.2f%%", positionPct.InexactFloat64()*100, r.config.MaxSinglePositionPct.InexactFloat64()*100)
		}
	}

	// 检查总仓位限制
	totalPosition := account.MarketValue.Add(orderAmount)
	if !account.TotalAssets.IsZero() {
		totalPct := totalPosition.Div(account.TotalAssets)
		if totalPct.GreaterThan(r.config.MaxTotalPositionPct) {
			r.emitEvent(models.RiskLevelHigh, "TOTAL_POSITION", fmt.Sprintf("总仓位 %.2f%% 超过限制 %.2f%%", totalPct.InexactFloat64()*100, r.config.MaxTotalPositionPct.InexactFloat64()*100), order.StockCode, order.StrategyID)
			return fmt.Errorf("总仓位 %.2f%% 超过限制 %.2f%%", totalPct.InexactFloat64()*100, r.config.MaxTotalPositionPct.InexactFloat64()*100)
		}
	}

	// 检查资金是否充足
	if account.Cash.LessThan(orderAmount) {
		r.emitEvent(models.RiskLevelMedium, "INSUFFICIENT_CASH", fmt.Sprintf("可用资金不足，需要 %s，可用 %s", orderAmount.String(), account.Cash.String()), order.StockCode, order.StrategyID)
		return fmt.Errorf("可用资金不足")
	}

	return nil
}

// checkSellOrder 卖出风控检查
func (r *RiskManager) checkSellOrder(order *models.Order, account *models.Account) error {
	// 检查持仓是否充足
	for _, pos := range account.Positions {
		if pos.StockCode == order.StockCode {
			if pos.AvailableVol < order.Volume {
				r.emitEvent(models.RiskLevelMedium, "INSUFFICIENT_POSITION", fmt.Sprintf("可卖数量不足，需要 %d，可用 %d", order.Volume, pos.AvailableVol), order.StockCode, order.StrategyID)
				return fmt.Errorf("可卖数量不足")
			}
			return nil
		}
	}

	r.emitEvent(models.RiskLevelMedium, "NO_POSITION", fmt.Sprintf("无 %s 持仓", order.StockCode), order.StockCode, order.StrategyID)
	return fmt.Errorf("无 %s 持仓", order.StockCode)
}

// RecordTrade 记录交易
func (r *RiskManager) RecordTrade(order *models.Order) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.dailyTrades++

	if order.Side == models.OrderSideSell && r.account != nil {
		// 简化计算盈亏
		for _, pos := range r.account.Positions {
			if pos.StockCode == order.StockCode {
				profit := order.Price.Sub(pos.AvgCost).Mul(decimal.NewFromInt(order.FilledVol))
				r.dailyPL = r.dailyPL.Add(profit)
				break
			}
		}
	}
}

// UpdateEquity 更新权益（每日调用）
func (r *RiskManager) UpdateEquity(totalAssets decimal.Decimal) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if totalAssets.GreaterThan(r.peakEquity) {
		r.peakEquity = totalAssets
	}
}

// ResetDaily 每日重置
func (r *RiskManager) ResetDaily() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dailyPL = decimal.Zero
	r.dailyTrades = 0
}

// GetEvents 获取风控事件
func (r *RiskManager) GetEvents(limit int) []models.RiskEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > len(r.events) {
		return r.events
	}
	return r.events[len(r.events)-limit:]
}

// CheckStopLoss 检查止损
func (r *RiskManager) CheckStopLoss(positions []models.Position) []models.Signal {
	var signals []models.Signal

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, pos := range positions {
		if pos.ProfitPct.IsNegative() {
			lossPct := pos.ProfitPct.Abs()
			if lossPct.GreaterThan(r.config.StopLossPct.Mul(decimal.NewFromInt(100))) {
				r.emitEvent(models.RiskLevelHigh, "STOP_LOSS", fmt.Sprintf("股票 %s 亏损 %.2f%% 触发止损", pos.StockCode, lossPct.InexactFloat64()), pos.StockCode, "")
				signals = append(signals, models.Signal{
					StockCode: pos.StockCode,
					StockName: pos.StockName,
					Side:      models.OrderSideSell,
					Type:      models.OrderTypeMarket,
					Price:     pos.CurrentPrice,
					Volume:    pos.AvailableVol,
					Reason:    fmt.Sprintf("止损卖出: 亏损%.2f%%", lossPct.InexactFloat64()),
					Timestamp: time.Now(),
				})
			}
		}
	}

	return signals
}

// CheckTakeProfit 检查止盈
func (r *RiskManager) CheckTakeProfit(positions []models.Position) []models.Signal {
	var signals []models.Signal

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, pos := range positions {
		if pos.ProfitPct.IsPositive() {
			profitPct := pos.ProfitPct
			if profitPct.GreaterThan(r.config.TakeProfitPct.Mul(decimal.NewFromInt(100))) {
				r.emitEvent(models.RiskLevelMedium, "TAKE_PROFIT", fmt.Sprintf("股票 %s 盈利 %.2f%% 触发止盈", pos.StockCode, profitPct.InexactFloat64()), pos.StockCode, "")
				signals = append(signals, models.Signal{
					StockCode: pos.StockCode,
					StockName: pos.StockName,
					Side:      models.OrderSideSell,
					Type:      models.OrderTypeMarket,
					Price:     pos.CurrentPrice,
					Volume:    pos.AvailableVol,
					Reason:    fmt.Sprintf("止盈卖出: 盈利%.2f%%", profitPct.InexactFloat64()),
					Timestamp: time.Now(),
				})
			}
		}
	}

	return signals
}

// OnRiskEvent 注册风控回调
func (r *RiskManager) OnRiskEvent(callback RiskCallback) {
	r.callbacks = append(r.callbacks, callback)
}

// isBlacklisted 检查黑名单
func (r *RiskManager) isBlacklisted(stockCode string) bool {
	for _, code := range r.config.BlacklistStocks {
		if code == stockCode {
			return true
		}
	}
	return false
}

// emitEvent 发出风控事件
func (r *RiskManager) emitEvent(level models.RiskLevel, eventType, message, stockCode, strategyID string) {
	event := models.RiskEvent{
		ID:         fmt.Sprintf("RISK_%d", time.Now().UnixNano()),
		Level:      level,
		Type:       eventType,
		Message:    message,
		StockCode:  stockCode,
		StrategyID: strategyID,
		Timestamp:  time.Now(),
	}

	r.events = append(r.events, event)

	// 日志
	switch level {
	case models.RiskLevelCritical:
		r.logger.Error("风控事件", zap.String("type", eventType), zap.String("msg", message))
	case models.RiskLevelHigh:
		r.logger.Warn("风控事件", zap.String("type", eventType), zap.String("msg", message))
	default:
		r.logger.Info("风控事件", zap.String("type", eventType), zap.String("msg", message))
	}

	// 回调
	for _, cb := range r.callbacks {
		cb(event)
	}
}

// UpdateConfig 更新风控配置
func (r *RiskManager) UpdateConfig(config models.RiskConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = config
}

// GetConfig 获取风控配置
func (r *RiskManager) GetConfig() models.RiskConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

