package backtest

import (
	"aqsystem/models"
	"aqsystem/strategy"
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// BacktestEngine 回测引擎
type BacktestEngine struct {
	logger         *zap.Logger
	commissionRate float64 // 佣金费率
	stampTaxRate   float64 // 印花税率
	slippage       float64 // 滑点
}

// NewBacktestEngine 创建回测引擎
func NewBacktestEngine(commissionRate, stampTaxRate, slippage float64, logger *zap.Logger) *BacktestEngine {
	return &BacktestEngine{
		logger:         logger,
		commissionRate: commissionRate,
		stampTaxRate:   stampTaxRate,
		slippage:       slippage,
	}
}

// BacktestState 回测状态
type BacktestState struct {
	Cash        decimal.Decimal
	Positions   map[string]*backtestPosition
	Trades      []models.BacktestTrade
	DailyEquity []models.EquityPoint
	InitialCash decimal.Decimal
	PeakEquity  decimal.Decimal
	MaxDrawdown decimal.Decimal
	DateTrades  int
	TotalTrades int
	WinTrades   int
	LossTrades  int
}

type backtestPosition struct {
	StockCode  string
	Volume     int64
	AvgCost    decimal.Decimal
	EntryTime  time.Time
	EntryPrice decimal.Decimal
}

// Run 执行回测
func (e *BacktestEngine) Run(
	ctx context.Context,
	strategy strategy.Strategy,
	klines map[string][]models.KLine, // 每只股票的K线数据
	startDate, endDate time.Time,
	initialCash decimal.Decimal,
) (*models.BacktestResult, error) {

	if initialCash.IsZero() {
		initialCash = decimal.NewFromInt(1000000)
	}

	state := &BacktestState{
		Cash:        initialCash,
		InitialCash: initialCash,
		PeakEquity:  initialCash,
		Positions:   make(map[string]*backtestPosition),
		Trades:      []models.BacktestTrade{},
		DailyEquity: []models.EquityPoint{},
	}

	// 按日期排序所有K线
	dailyKLines := e.organizeByDate(klines, startDate, endDate)

	e.logger.Info("开始回测",
		zap.String("strategy", strategy.Name()),
		zap.String("start", startDate.Format("2006-01-02")),
		zap.String("end", endDate.Format("2006-01-02")),
		zap.String("initial_cash", initialCash.String()),
		zap.Int("trading_days", len(dailyKLines)),
	)

	// 初始化策略
	config := strategy.GetConfig()
	if err := strategy.Init(config); err != nil {
		return nil, fmt.Errorf("策略初始化失败: %w", err)
	}
	strategy.SetStatus(models.StrategyStatusActive)

	// 逐日回测
	for date, dayKlines := range dailyKLines {
		state.DateTrades = 0

		for _, kline := range dayKlines {
			// 将K线喂给策略
			signals, err := strategy.OnBar(ctx, kline)
			if err != nil {
				e.logger.Debug("策略处理K线出错", zap.Error(err))
				continue
			}

			// 处理信号
			for _, signal := range signals {
				e.executeSignal(state, signal, date)
			}
		}

		// 记录每日权益
		equity := e.calcEquity(state, dayKlines)
		e.recordDailyEquity(state, equity, date)
	}

	// 生成回测结果
	result := e.generateResult(strategy, state, startDate, endDate, initialCash)
	return result, nil
}

// executeSignal 执行交易信号
func (e *BacktestEngine) executeSignal(state *BacktestState, signal models.Signal, date time.Time) {
	// 考虑滑点
	execPrice := signal.Price
	if signal.Side == models.OrderSideBuy {
		execPrice = execPrice.Mul(decimal.NewFromFloat(1 + e.slippage))
	} else {
		execPrice = execPrice.Mul(decimal.NewFromFloat(1 - e.slippage))
	}

	// 确保合理的交易量（100的整数倍）
	volume := (signal.Volume / 100) * 100
	if volume <= 0 {
		volume = 100
	}

	if signal.Side == models.OrderSideBuy {
		// 买入
		totalCost := execPrice.Mul(decimal.NewFromInt(volume))
		commission := e.calcCommission(totalCost, true)

		if state.Cash.LessThan(totalCost.Add(commission)) {
			// 资金不足，调整数量
			maxVol := state.Cash.Sub(commission).Div(execPrice).IntPart()
			maxVol = (maxVol / 100) * 100
			if maxVol <= 0 {
				return
			}
			volume = maxVol
			totalCost = execPrice.Mul(decimal.NewFromInt(volume))
			commission = e.calcCommission(totalCost, true)
		}

		// 扣除资金
		state.Cash = state.Cash.Sub(totalCost).Sub(commission)

		// 更新持仓
		pos, exists := state.Positions[signal.StockCode]
		if !exists {
			state.Positions[signal.StockCode] = &backtestPosition{
				StockCode:  signal.StockCode,
				Volume:     volume,
				AvgCost:    execPrice,
				EntryTime:  date,
				EntryPrice: execPrice,
			}
		} else {
			// 加仓
			totalVol := pos.Volume + volume
			totalCostOld := pos.AvgCost.Mul(decimal.NewFromInt(pos.Volume))
			pos.AvgCost = totalCostOld.Add(totalCost).Div(decimal.NewFromInt(totalVol))
			pos.Volume = totalVol
		}

		state.DateTrades++
		state.TotalTrades++

	} else if signal.Side == models.OrderSideSell {
		// 卖出
		pos, exists := state.Positions[signal.StockCode]
		if !exists || pos.Volume <= 0 {
			return
		}

		sellVol := volume
		if sellVol > pos.Volume {
			sellVol = pos.Volume
		}

		totalAmount := execPrice.Mul(decimal.NewFromInt(sellVol))
		commission := e.calcCommission(totalAmount, false)
		stampTax := totalAmount.Mul(decimal.NewFromFloat(e.stampTaxRate))

		// 增加资金
		state.Cash = state.Cash.Add(totalAmount).Sub(commission).Sub(stampTax)

		// 记录交易
		profit := totalAmount.Sub(pos.AvgCost.Mul(decimal.NewFromInt(sellVol))).Sub(commission).Sub(stampTax)
		if profit.GreaterThanOrEqual(decimal.Zero) {
			state.WinTrades++
		} else {
			state.LossTrades++
		}

		state.Trades = append(state.Trades, models.BacktestTrade{
			StockCode:  signal.StockCode,
			Side:       models.OrderSideSell,
			EntryPrice: pos.EntryPrice,
			ExitPrice:  execPrice,
			Volume:     sellVol,
			Profit:     profit,
			EntryTime:  pos.EntryTime,
			ExitTime:   date,
		})

		// 更新持仓
		pos.Volume -= sellVol
		if pos.Volume <= 0 {
			delete(state.Positions, signal.StockCode)
		}

		state.DateTrades++
		state.TotalTrades++
	}
}

// calcCommission 计算佣金
func (e *BacktestEngine) calcCommission(amount decimal.Decimal, isBuy bool) decimal.Decimal {
	commission := amount.Mul(decimal.NewFromFloat(e.commissionRate))
	// 最低佣金5元
	minCommission := decimal.NewFromInt(5)
	if commission.LessThan(minCommission) {
		commission = minCommission
	}
	return commission
}

// calcEquity 计算当前权益
func (e *BacktestEngine) calcEquity(state *BacktestState, klines []models.KLine) decimal.Decimal {
	equity := state.Cash

	// 用当日收盘价计算持仓市值
	priceMap := make(map[string]decimal.Decimal)
	for _, k := range klines {
		priceMap[k.StockCode] = k.Close
	}

	for _, pos := range state.Positions {
		if price, ok := priceMap[pos.StockCode]; ok {
			equity = equity.Add(price.Mul(decimal.NewFromInt(pos.Volume)))
		} else {
			equity = equity.Add(pos.AvgCost.Mul(decimal.NewFromInt(pos.Volume)))
		}
	}

	return equity
}

// recordDailyEquity 记录每日权益
func (e *BacktestEngine) recordDailyEquity(state *BacktestState, equity decimal.Decimal, date time.Time) {
	// 更新最大回撤
	if equity.GreaterThan(state.PeakEquity) {
		state.PeakEquity = equity
	}

	if !state.PeakEquity.IsZero() {
		drawdown := decimal.NewFromInt(1).Sub(equity.Div(state.PeakEquity))
		if drawdown.GreaterThan(state.MaxDrawdown) {
			state.MaxDrawdown = drawdown
		}
	}

	state.DailyEquity = append(state.DailyEquity, models.EquityPoint{
		Date:   date,
		Equity: equity,
	})
}

// organizeByDate 按日期组织K线
func (e *BacktestEngine) organizeByDate(klines map[string][]models.KLine, startDate, endDate time.Time) map[time.Time][]models.KLine {
	result := make(map[time.Time][]models.KLine)

	for _, stockKlines := range klines {
		for _, kline := range stockKlines {
			date := kline.Timestamp.Truncate(24 * time.Hour)
			if date.Before(startDate) || date.After(endDate) {
				continue
			}
			result[date] = append(result[date], kline)
		}
	}

	return result
}

// generateResult 生成回测结果
func (e *BacktestEngine) generateResult(
	s strategy.Strategy,
	state *BacktestState,
	startDate, endDate time.Time,
	initialCash decimal.Decimal,
) *models.BacktestResult {
	finalEquity := state.Cash
	for _, pos := range state.Positions {
		finalEquity = finalEquity.Add(pos.AvgCost.Mul(decimal.NewFromInt(pos.Volume)))
	}

	// 总收益率
	totalReturn := decimal.Zero
	if !initialCash.IsZero() {
		totalReturn = finalEquity.Sub(initialCash).Div(initialCash)
	}

	// 年化收益率
	days := endDate.Sub(startDate).Hours() / 24
	years := days / 365.0
	annualReturn := decimal.Zero
	if years > 0 && !initialCash.IsZero() {
		// (1 + totalReturn)^(1/years) - 1
		trFloat, _ := totalReturn.Float64()
		annualReturnFloat := pow(1+trFloat, 1/years) - 1
		annualReturn = decimal.NewFromFloat(annualReturnFloat)
	}

	// 胜率
	winRate := decimal.Zero
	if state.TotalTrades > 0 {
		winRate = decimal.NewFromInt(int64(state.WinTrades)).Div(decimal.NewFromInt(int64(state.TotalTrades)))
	}

	// 夏普比率
	sharpeRatio := e.calcSharpeRatio(state.DailyEquity, initialCash)

	// 盈亏比
	profitFactor := e.calcProfitFactor(state.Trades)

	return &models.BacktestResult{
		StrategyID:     s.ID(),
		StrategyName:   s.Name(),
		StartDate:      startDate,
		EndDate:        endDate,
		InitialCapital: initialCash,
		FinalCapital:   finalEquity,
		TotalReturn:    totalReturn.Mul(decimal.NewFromInt(100)), // 百分比
		AnnualReturn:   annualReturn.Mul(decimal.NewFromInt(100)),
		MaxDrawdown:    state.MaxDrawdown.Mul(decimal.NewFromInt(100)),
		SharpeRatio:    sharpeRatio,
		WinRate:        winRate.Mul(decimal.NewFromInt(100)),
		ProfitFactor:   profitFactor,
		TotalTrades:    state.TotalTrades,
		WinTrades:      state.WinTrades,
		LossTrades:     state.LossTrades,
		Trades:         state.Trades,
		DailyEquity:    state.DailyEquity,
	}
}

// calcSharpeRatio 计算夏普比率
func (e *BacktestEngine) calcSharpeRatio(equityPoints []models.EquityPoint, initialCash decimal.Decimal) decimal.Decimal {
	if len(equityPoints) < 2 {
		return decimal.Zero
	}

	// 计算每日收益率
	var returns []float64
	for i := 1; i < len(equityPoints); i++ {
		if equityPoints[i-1].Equity.IsZero() {
			continue
		}
		dailyReturn := equityPoints[i].Equity.Sub(equityPoints[i-1].Equity).Div(equityPoints[i-1].Equity)
		r, _ := dailyReturn.Float64()
		returns = append(returns, r)
	}

	if len(returns) == 0 {
		return decimal.Zero
	}

	// 平均收益率
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// 标准差
	var varianceSum float64
	for _, r := range returns {
		varianceSum += (r - mean) * (r - mean)
	}
	std := sqrt(varianceSum/float64(len(returns))) * sqrt(252) // 年化标准差

	if std == 0 {
		return decimal.Zero
	}

	// 年化夏普比率 = (年化收益率 - 无风险利率) / 年化标准差
	// 假设无风险利率为3%
	riskFreeRate := 0.03
	sharpe := (mean*252 - riskFreeRate) / std

	return decimal.NewFromFloat(sharpe)
}

// calcProfitFactor 计算盈亏比
func (e *BacktestEngine) calcProfitFactor(trades []models.BacktestTrade) decimal.Decimal {
	var totalProfit, totalLoss decimal.Decimal

	for _, trade := range trades {
		if trade.Profit.GreaterThanOrEqual(decimal.Zero) {
			totalProfit = totalProfit.Add(trade.Profit)
		} else {
			totalLoss = totalLoss.Add(trade.Profit.Abs())
		}
	}

	if totalLoss.IsZero() {
		if totalProfit.GreaterThan(decimal.Zero) {
			return decimal.NewFromInt(999) // 无亏损
		}
		return decimal.Zero
	}

	return totalProfit.Div(totalLoss)
}

// sqrt 平方根
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 20; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// pow 幂运算
func pow(x, y float64) float64 {
	if y == 0 {
		return 1
	}
	if y == 1 {
		return x
	}
	// 使用对数: x^y = e^(y*ln(x))
	if x <= 0 {
		return 0
	}
	lnX := ln(x)
	result := exp(y * lnX)
	return result
}

// ln 自然对数 (泰勒展开)
func ln(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// 凑到接近1的值
	n := 0
	for x > 2 {
		x /= 2
		n++
	}
	for x < 0.5 {
		x *= 2
		n--
	}

	// ln(x) = ln((1+t)/(1-t)) = 2*(t + t^3/3 + t^5/5 + ...)
	t := (x - 1) / (x + 1)
	result := 0.0
	tPower := t
	for i := 1; i <= 100; i += 2 {
		result += tPower / float64(i)
		tPower *= t * t
	}
	result *= 2

	return result + float64(n)*0.6931471805599453 // ln(2)
}

// exp 指数函数
func exp(x float64) float64 {
	result := 1.0
	term := 1.0
	for i := 1; i <= 100; i++ {
		term *= x / float64(i)
		result += term
		if term < 1e-15 {
			break
		}
	}
	return result
}
