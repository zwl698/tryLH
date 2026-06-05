package strategy

import (
	"aqsystem/models"
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ==================== 双均线策略 ====================

// DoubleMAStrategy 双均线交叉策略
//
// 策略原理：
// 短期均线上穿长期均线形成金叉，买入信号
// 短期均线下穿长期均线形成死叉，卖出信号
//
// 这是最经典的技术分析策略之一，适合趋势明显的市场
type DoubleMAStrategy struct {
	BaseStrategy
	shortPeriod int                       // 短期均线周期
	longPeriod  int                       // 长期均线周期
	maType      string                    // 均线类型: SMA, EMA
	history     map[string][]models.KLine // 历史K线
	lastCross   map[string]string         // 上次交叉状态: golden, death, none
}

// NewDoubleMAStrategy 创建双均线策略
func NewDoubleMAStrategy(config models.StrategyConfig, logger *zap.Logger) *DoubleMAStrategy {
	s := &DoubleMAStrategy{
		BaseStrategy: NewBaseStrategy(config, logger),
		history:      make(map[string][]models.KLine),
		lastCross:    make(map[string]string),
	}
	s.shortPeriod = s.getIntParam("short_period", 5)
	s.longPeriod = s.getIntParam("long_period", 20)
	s.maType = s.getStringParam("ma_type", "SMA")
	return s
}

func (s *DoubleMAStrategy) Type() string { return "double_ma" }

func (s *DoubleMAStrategy) Init(config models.StrategyConfig) error {
	s.shortPeriod = s.getIntParam("short_period", 5)
	s.longPeriod = s.getIntParam("long_period", 20)
	s.maType = s.getStringParam("ma_type", "SMA")
	s.history = make(map[string][]models.KLine)
	s.lastCross = make(map[string]string)
	return nil
}

func (s *DoubleMAStrategy) OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error) {
	var signals []models.Signal

	code := kline.StockCode
	s.history[code] = append(s.history[code], kline)

	// 需要足够的数据计算均线
	if len(s.history[code]) < s.longPeriod {
		return signals, nil
	}

	// 保留最近2倍长周期数据即可
	if len(s.history[code]) > s.longPeriod*2 {
		s.history[code] = s.history[code][len(s.history[code])-s.longPeriod*2:]
	}

	// 计算短期和长期均线
	shortMA := s.calcMA(s.history[code], s.shortPeriod)
	longMA := s.calcMA(s.history[code], s.longPeriod)

	if len(shortMA) < 2 || len(longMA) < 2 {
		return signals, nil
	}

	// 当前和上一次的均线值
	currShort := shortMA[len(shortMA)-1]
	currLong := longMA[len(longMA)-1]
	prevShort := shortMA[len(shortMA)-2]
	prevLong := longMA[len(longMA)-2]

	// 检测交叉
	cross := "none"
	if prevShort.LessThanOrEqual(prevLong) && currShort.GreaterThan(currLong) {
		cross = "golden" // 金叉
	} else if prevShort.GreaterThanOrEqual(prevLong) && currShort.LessThan(currLong) {
		cross = "death" // 死叉
	}

	// 生成信号
	lastCross := s.lastCross[code]

	if cross == "golden" && lastCross != "golden" {
		// 金叉买入
		signals = append(signals, models.Signal{
			StrategyID: s.ID(),
			StockCode:  code,
			Side:       models.OrderSideBuy,
			Type:       models.OrderTypeLimit,
			Price:      kline.Close,
			Volume:     s.calcVolume(kline.Close),
			Reason:     fmt.Sprintf("金叉买入: 短期MA(%d)=%s 上穿 长期MA(%d)=%s", s.shortPeriod, currShort.StringFixed(2), s.longPeriod, currLong.StringFixed(2)),
			Timestamp:  time.Now(),
		})
		s.lastCross[code] = "golden"
	} else if cross == "death" && lastCross != "death" {
		// 死叉卖出
		signals = append(signals, models.Signal{
			StrategyID: s.ID(),
			StockCode:  code,
			Side:       models.OrderSideSell,
			Type:       models.OrderTypeLimit,
			Price:      kline.Close,
			Volume:     s.calcVolume(kline.Close), // 实际应查持仓量
			Reason:     fmt.Sprintf("死叉卖出: 短期MA(%d)=%s 下穿 长期MA(%d)=%s", s.shortPeriod, currShort.StringFixed(2), s.longPeriod, currLong.StringFixed(2)),
			Timestamp:  time.Now(),
		})
		s.lastCross[code] = "death"
	}

	return signals, nil
}

func (s *DoubleMAStrategy) OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil // 均线策略主要依赖K线
}

func (s *DoubleMAStrategy) OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *DoubleMAStrategy) GetParamDefs() []ParamDef {
	return []ParamDef{
		{Key: "short_period", Name: "短期均线周期", Type: "int", Default: 5, Min: 2, Max: 60, Description: "短期移动平均线周期"},
		{Key: "long_period", Name: "长期均线周期", Type: "int", Default: 20, Min: 10, Max: 250, Description: "长期移动平均线周期"},
		{Key: "ma_type", Name: "均线类型", Type: "string", Default: "SMA", Description: "SMA简单均线/EMA指数均线"},
	}
}

// calcMA 计算移动平均线
func (s *DoubleMAStrategy) calcMA(klines []models.KLine, period int) []decimal.Decimal {
	var result []decimal.Decimal

	if s.maType == "EMA" {
		// 指数移动平均
		k := decimal.NewFromFloat(2.0 / float64(period+1))
		var ema decimal.Decimal
		for i, kline := range klines {
			if i == 0 {
				ema = kline.Close
			} else {
				ema = kline.Close.Mul(k).Add(ema.Mul(decimal.NewFromInt(1).Sub(k)))
			}
			if i >= period-1 {
				result = append(result, ema)
			}
		}
	} else {
		// 简单移动平均
		for i := period - 1; i < len(klines); i++ {
			sum := decimal.Zero
			for j := i - period + 1; j <= i; j++ {
				sum = sum.Add(klines[j].Close)
			}
			result = append(result, sum.Div(decimal.NewFromInt(int64(period))))
		}
	}

	return result
}

// calcVolume 计算下单数量（按固定金额买入，100股整数倍）
func (s *DoubleMAStrategy) calcVolume(price decimal.Decimal) int64 {
	config := s.GetConfig()
	if !config.MaxPosition.IsZero() {
		volume := config.MaxPosition.Div(price).IntPart()
		return (volume / 100) * 100 // A股最小100股
	}
	return 100
}

// ==================== 海龟交易策略 ====================

// TurtleStrategy 海龟交易策略
//
// 策略原理：
// 基于唐奇安通道突破进行交易
// 价格突破N日最高价买入，突破N日最低价卖出
// 使用ATR进行仓位管理和止损
//
// 这是理查德·丹尼斯的海龟交易法，80年代最著名的交易系统
type TurtleStrategy struct {
	BaseStrategy
	entryPeriod int     // 入场通道周期
	exitPeriod  int     // 出场通道周期
	atrPeriod   int     // ATR周期
	riskPct     float64 // 每笔风险比例
	history     map[string][]models.KLine
	entries     map[string]decimal.Decimal // 入场价格
	stopLoss    map[string]decimal.Decimal // 止损价格
	units       map[string]int             // 持仓单位数
}

// NewTurtleStrategy 创建海龟策略
func NewTurtleStrategy(config models.StrategyConfig, logger *zap.Logger) *TurtleStrategy {
	s := &TurtleStrategy{
		BaseStrategy: NewBaseStrategy(config, logger),
		history:      make(map[string][]models.KLine),
		entries:      make(map[string]decimal.Decimal),
		stopLoss:     make(map[string]decimal.Decimal),
		units:        make(map[string]int),
	}
	s.entryPeriod = s.getIntParam("entry_period", 20)
	s.exitPeriod = s.getIntParam("exit_period", 10)
	s.atrPeriod = s.getIntParam("atr_period", 20)
	s.riskPct = s.getFloatParam("risk_pct", 0.01)
	return s
}

func (s *TurtleStrategy) Type() string { return "turtle" }

func (s *TurtleStrategy) Init(config models.StrategyConfig) error {
	s.entryPeriod = s.getIntParam("entry_period", 20)
	s.exitPeriod = s.getIntParam("exit_period", 10)
	s.atrPeriod = s.getIntParam("atr_period", 20)
	s.riskPct = s.getFloatParam("risk_pct", 0.01)
	s.history = make(map[string][]models.KLine)
	s.entries = make(map[string]decimal.Decimal)
	s.stopLoss = make(map[string]decimal.Decimal)
	s.units = make(map[string]int)
	return nil
}

func (s *TurtleStrategy) OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error) {
	var signals []models.Signal

	code := kline.StockCode
	s.history[code] = append(s.history[code], kline)

	if len(s.history[code]) < s.entryPeriod {
		return signals, nil
	}

	history := s.history[code]

	// 计算唐奇安通道
	entryHigh := s.highest(history, s.entryPeriod)
	_ = s.lowest(history, s.entryPeriod) // entryLow: 预留做多做空扩展
	_ = s.highest(history, s.exitPeriod) // exitHigh: 预留扩展
	exitLow := s.lowest(history, s.exitPeriod)
	atr := s.calcATR(history, s.atrPeriod)

	// 检查是否持仓
	_, hasEntry := s.entries[code]

	if !hasEntry {
		// 无仓位 - 检查入场信号
		if kline.Close.GreaterThanOrEqual(entryHigh) {
			// 突破N日高点买入
			stopLossPrice := kline.Close.Sub(atr.Mul(decimal.NewFromInt(2)))
			volume := s.calcTurtleVolume(atr)

			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideBuy,
				Type:       models.OrderTypeLimit,
				Price:      kline.Close,
				Volume:     volume,
				Reason:     fmt.Sprintf("海龟突破买入: 价格%s突破%d日高点%s, ATR=%s, 止损=%s", kline.Close.StringFixed(2), s.entryPeriod, entryHigh.StringFixed(2), atr.StringFixed(2), stopLossPrice.StringFixed(2)),
				Timestamp:  time.Now(),
			})

			s.entries[code] = kline.Close
			s.stopLoss[code] = stopLossPrice
			s.units[code] = 1
		}
	} else {
		// 有仓位 - 检查止损和出场信号
		// 检查止损
		if kline.Close.LessThanOrEqual(s.stopLoss[code]) {
			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideSell,
				Type:       models.OrderTypeLimit,
				Price:      kline.Close,
				Volume:     s.calcTurtleVolume(atr),
				Reason:     fmt.Sprintf("海龟止损卖出: 价格%s触及止损%s", kline.Close.StringFixed(2), s.stopLoss[code].StringFixed(2)),
				Timestamp:  time.Now(),
			})
			delete(s.entries, code)
			delete(s.stopLoss, code)
			delete(s.units, code)
		} else if kline.Close.LessThanOrEqual(exitLow) {
			// 突破M日低点卖出
			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideSell,
				Type:       models.OrderTypeLimit,
				Price:      kline.Close,
				Volume:     s.calcTurtleVolume(atr),
				Reason:     fmt.Sprintf("海龟通道卖出: 价格%s跌破%d日低点%s", kline.Close.StringFixed(2), s.exitPeriod, exitLow.StringFixed(2)),
				Timestamp:  time.Now(),
			})
			delete(s.entries, code)
			delete(s.stopLoss, code)
			delete(s.units, code)
		} else {
			// 加仓逻辑 - 价格每上涨0.5ATR加仓一次，最多4单位
			if s.units[code] < 4 {
				addPrice := s.entries[code].Add(atr.Mul(decimal.NewFromFloat(0.5)).Mul(decimal.NewFromInt(int64(s.units[code]))))
				if kline.Close.GreaterThanOrEqual(addPrice) {
					volume := s.calcTurtleVolume(atr)
					signals = append(signals, models.Signal{
						StrategyID: s.ID(),
						StockCode:  code,
						Side:       models.OrderSideBuy,
						Type:       models.OrderTypeLimit,
						Price:      kline.Close,
						Volume:     volume,
						Reason:     fmt.Sprintf("海龟加仓: 第%d单位, 价格%s", s.units[code]+1, kline.Close.StringFixed(2)),
						Timestamp:  time.Now(),
					})
					s.units[code]++
					s.stopLoss[code] = kline.Close.Sub(atr.Mul(decimal.NewFromInt(2)))
				}
			}
		}
	}

	return signals, nil
}

func (s *TurtleStrategy) OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *TurtleStrategy) OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *TurtleStrategy) GetParamDefs() []ParamDef {
	return []ParamDef{
		{Key: "entry_period", Name: "入场通道周期", Type: "int", Default: 20, Min: 5, Max: 100, Description: "突破入场的唐奇安通道周期"},
		{Key: "exit_period", Name: "出场通道周期", Type: "int", Default: 10, Min: 5, Max: 50, Description: "突破出场的唐奇安通道周期"},
		{Key: "atr_period", Name: "ATR周期", Type: "int", Default: 20, Min: 5, Max: 100, Description: "平均真实波幅周期"},
		{Key: "risk_pct", Name: "每笔风险比例", Type: "float", Default: 0.01, Min: 0.001, Max: 0.05, Description: "每笔交易的最大风险占总资金比例"},
	}
}

func (s *TurtleStrategy) highest(klines []models.KLine, period int) decimal.Decimal {
	start := len(klines) - period
	if start < 0 {
		start = 0
	}
	high := klines[start].High
	for i := start + 1; i < len(klines); i++ {
		if klines[i].High.GreaterThan(high) {
			high = klines[i].High
		}
	}
	return high
}

func (s *TurtleStrategy) lowest(klines []models.KLine, period int) decimal.Decimal {
	start := len(klines) - period
	if start < 0 {
		start = 0
	}
	low := klines[start].Low
	for i := start + 1; i < len(klines); i++ {
		if klines[i].Low.LessThan(low) {
			low = klines[i].Low
		}
	}
	return low
}

func (s *TurtleStrategy) calcATR(klines []models.KLine, period int) decimal.Decimal {
	if len(klines) < 2 {
		return decimal.Zero
	}

	var trSum decimal.Decimal
	count := 0
	for i := len(klines) - 1; i >= 1 && count < period; i-- {
		tr := trueRange(klines[i], klines[i-1])
		trSum = trSum.Add(tr)
		count++
	}

	if count == 0 {
		return decimal.Zero
	}
	return trSum.Div(decimal.NewFromInt(int64(count)))
}

func (s *TurtleStrategy) calcTurtleVolume(atr decimal.Decimal) int64 {
	if atr.IsZero() {
		return 100
	}
	// 简化的仓位计算：按风险比例计算
	config := s.GetConfig()
	if !config.MaxPosition.IsZero() {
		riskAmount := config.MaxPosition.Mul(decimal.NewFromFloat(s.riskPct))
		unitVolume := riskAmount.Div(atr.Mul(decimal.NewFromInt(2))).IntPart()
		return (unitVolume / 100) * 100
	}
	return 100
}

func trueRange(current, prev models.KLine) decimal.Decimal {
	tr1 := current.High.Sub(current.Low)
	tr2 := current.High.Sub(prev.Close).Abs()
	tr3 := current.Low.Sub(prev.Close).Abs()

	if tr1.GreaterThan(tr2) {
		if tr1.GreaterThan(tr3) {
			return tr1
		}
		return tr3
	}
	if tr2.GreaterThan(tr3) {
		return tr2
	}
	return tr3
}

// ==================== 动量策略 ====================

// MomentumStrategy 动量策略
//
// 策略原理：
// 选择过去N日涨幅最大的股票买入（强者恒强）
// 当动量减弱或反转时卖出
//
// 学术研究表明，3-12个月的动量效应在A股市场显著存在
type MomentumStrategy struct {
	BaseStrategy
	lookbackPeriod    int     // 回望期
	holdingPeriod     int     // 持有期
	topN              int     // 选择前N只
	momentumThreshold float64 // 动量阈值
	history           map[string][]models.KLine
	entryDates        map[string]int // 入场后的天数
	momentumScores    map[string]decimal.Decimal
}

// NewMomentumStrategy 创建动量策略
func NewMomentumStrategy(config models.StrategyConfig, logger *zap.Logger) *MomentumStrategy {
	s := &MomentumStrategy{
		BaseStrategy:   NewBaseStrategy(config, logger),
		history:        make(map[string][]models.KLine),
		entryDates:     make(map[string]int),
		momentumScores: make(map[string]decimal.Decimal),
	}
	s.lookbackPeriod = s.getIntParam("lookback_period", 20)
	s.holdingPeriod = s.getIntParam("holding_period", 10)
	s.topN = s.getIntParam("top_n", 3)
	s.momentumThreshold = s.getFloatParam("momentum_threshold", 0.05)
	return s
}

func (s *MomentumStrategy) Type() string { return "momentum" }

func (s *MomentumStrategy) Init(config models.StrategyConfig) error {
	s.lookbackPeriod = s.getIntParam("lookback_period", 20)
	s.holdingPeriod = s.getIntParam("holding_period", 10)
	s.topN = s.getIntParam("top_n", 3)
	s.momentumThreshold = s.getFloatParam("momentum_threshold", 0.05)
	s.history = make(map[string][]models.KLine)
	s.entryDates = make(map[string]int)
	s.momentumScores = make(map[string]decimal.Decimal)
	return nil
}

func (s *MomentumStrategy) OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error) {
	var signals []models.Signal

	code := kline.StockCode
	s.history[code] = append(s.history[code], kline)

	// 保留足够的历史数据
	if len(s.history[code]) > s.lookbackPeriod*2 {
		s.history[code] = s.history[code][len(s.history[code])-s.lookbackPeriod*2:]
	}

	// 计算动量得分
	if len(s.history[code]) >= s.lookbackPeriod {
		history := s.history[code]
		currentPrice := history[len(history)-1].Close
		pastPrice := history[len(history)-s.lookbackPeriod].Close

		if !pastPrice.IsZero() {
			momentum := currentPrice.Sub(pastPrice).Div(pastPrice)
			s.momentumScores[code] = momentum
		}
	}

	// 检查持有期
	if days, has := s.entryDates[code]; has {
		s.entryDates[code] = days + 1
		if days >= s.holdingPeriod {
			// 持有期结束，检查动量是否仍然强劲
			score, _ := s.momentumScores[code]
			if score.LessThan(decimal.NewFromFloat(s.momentumThreshold)) {
				signals = append(signals, models.Signal{
					StrategyID: s.ID(),
					StockCode:  code,
					Side:       models.OrderSideSell,
					Type:       models.OrderTypeLimit,
					Price:      kline.Close,
					Volume:     s.calcMomentumVolume(kline.Close),
					Reason:     fmt.Sprintf("动量减弱卖出: 动量得分%.2f%%, 持有%d日", score.Mul(decimal.NewFromInt(100)).InexactFloat64(), days),
					Timestamp:  time.Now(),
				})
				delete(s.entryDates, code)
			}
		}
	} else {
		// 无仓位 - 检查是否动量足够强
		score, _ := s.momentumScores[code]
		threshold := decimal.NewFromFloat(s.momentumThreshold)
		if score.GreaterThan(threshold) {
			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideBuy,
				Type:       models.OrderTypeLimit,
				Price:      kline.Close,
				Volume:     s.calcMomentumVolume(kline.Close),
				Reason:     fmt.Sprintf("动量买入: %d日动量得分%.2f%%, 超过阈值%.2f%%", s.lookbackPeriod, score.Mul(decimal.NewFromInt(100)).InexactFloat64(), threshold.Mul(decimal.NewFromInt(100)).InexactFloat64()),
				Timestamp:  time.Now(),
			})
			s.entryDates[code] = 0
		}
	}

	return signals, nil
}

func (s *MomentumStrategy) OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *MomentumStrategy) OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *MomentumStrategy) GetParamDefs() []ParamDef {
	return []ParamDef{
		{Key: "lookback_period", Name: "回望期", Type: "int", Default: 20, Min: 5, Max: 120, Description: "计算动量的回望天数"},
		{Key: "holding_period", Name: "持有期", Type: "int", Default: 10, Min: 1, Max: 60, Description: "最短持有天数"},
		{Key: "top_n", Name: "选股数量", Type: "int", Default: 3, Min: 1, Max: 20, Description: "选择动量最强的N只股票"},
		{Key: "momentum_threshold", Name: "动量阈值", Type: "float", Default: 0.05, Min: 0.0, Max: 0.5, Description: "买入的最低动量阈值"},
	}
}

func (s *MomentumStrategy) calcMomentumVolume(price decimal.Decimal) int64 {
	config := s.GetConfig()
	if !config.MaxPosition.IsZero() {
		volume := config.MaxPosition.Div(price).IntPart()
		return (volume / 100) * 100
	}
	return 100
}

// ==================== 均值回归策略 ====================

// MeanReversionStrategy 均值回归策略
//
// 策略原理：
// 价格偏离均值过大时，预期会回归均值
// 价格低于均值-2倍标准差时买入（超跌）
// 价格高于均值+2倍标准差时卖出（超涨）
//
// 这是量化投资中最经典的统计套利策略
type MeanReversionStrategy struct {
	BaseStrategy
	lookbackPeriod int     // 回望期
	entryZScore    float64 // 入场Z-score阈值
	exitZScore     float64 // 出场Z-score阈值
	history        map[string][]models.KLine
	positions      map[string]bool // 是否持仓
}

// NewMeanReversionStrategy 创建均值回归策略
func NewMeanReversionStrategy(config models.StrategyConfig, logger *zap.Logger) *MeanReversionStrategy {
	s := &MeanReversionStrategy{
		BaseStrategy: NewBaseStrategy(config, logger),
		history:      make(map[string][]models.KLine),
		positions:    make(map[string]bool),
	}
	s.lookbackPeriod = s.getIntParam("lookback_period", 20)
	s.entryZScore = s.getFloatParam("entry_zscore", 2.0)
	s.exitZScore = s.getFloatParam("exit_zscore", 0.5)
	return s
}

func (s *MeanReversionStrategy) Type() string { return "mean_reversion" }

func (s *MeanReversionStrategy) Init(config models.StrategyConfig) error {
	s.lookbackPeriod = s.getIntParam("lookback_period", 20)
	s.entryZScore = s.getFloatParam("entry_zscore", 2.0)
	s.exitZScore = s.getFloatParam("exit_zscore", 0.5)
	s.history = make(map[string][]models.KLine)
	s.positions = make(map[string]bool)
	return nil
}

func (s *MeanReversionStrategy) OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error) {
	var signals []models.Signal

	code := kline.StockCode
	s.history[code] = append(s.history[code], kline)

	if len(s.history[code]) > s.lookbackPeriod*2 {
		s.history[code] = s.history[code][len(s.history[code])-s.lookbackPeriod*2:]
	}

	if len(s.history[code]) < s.lookbackPeriod {
		return signals, nil
	}

	// 计算均值和标准差
	mean, std := s.calcStats(s.history[code], s.lookbackPeriod)
	if std.IsZero() {
		return signals, nil
	}

	// 计算Z-score
	zScore := kline.Close.Sub(mean).Div(std)
	zScoreFloat, _ := zScore.Float64()

	hasPosition := s.positions[code]

	if !hasPosition {
		// 无仓位 - 价格超跌时买入
		if zScoreFloat < -s.entryZScore {
			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideBuy,
				Type:       models.OrderTypeLimit,
				Price:      kline.Close,
				Volume:     s.calcMRVolume(kline.Close),
				Reason:     fmt.Sprintf("均值回归买入: Z-score=%.2f, 均值=%.2f, 标准差=%.2f, 价格超跌", zScoreFloat, mean.InexactFloat64(), std.InexactFloat64()),
				Timestamp:  time.Now(),
			})
			s.positions[code] = true
		}
	} else {
		// 有仓位 - 价格回归均值时卖出
		if zScoreFloat > -s.exitZScore {
			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideSell,
				Type:       models.OrderTypeLimit,
				Price:      kline.Close,
				Volume:     s.calcMRVolume(kline.Close),
				Reason:     fmt.Sprintf("均值回归卖出: Z-score=%.2f, 价格回归均值", zScoreFloat),
				Timestamp:  time.Now(),
			})
			delete(s.positions, code)
		}
	}

	return signals, nil
}

func (s *MeanReversionStrategy) OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *MeanReversionStrategy) OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return nil, nil
}

func (s *MeanReversionStrategy) GetParamDefs() []ParamDef {
	return []ParamDef{
		{Key: "lookback_period", Name: "回望期", Type: "int", Default: 20, Min: 5, Max: 120, Description: "计算均值和标准差的回望天数"},
		{Key: "entry_zscore", Name: "入场Z-score", Type: "float", Default: 2.0, Min: 1.0, Max: 4.0, Description: "Z-score低于此值时买入"},
		{Key: "exit_zscore", Name: "出场Z-score", Type: "float", Default: 0.5, Min: 0.0, Max: 2.0, Description: "Z-score高于此值时卖出"},
	}
}

func (s *MeanReversionStrategy) calcStats(klines []models.KLine, period int) (decimal.Decimal, decimal.Decimal) {
	n := len(klines)
	start := n - period
	if start < 0 {
		start = 0
	}

	// 计算均值
	sum := decimal.Zero
	for i := start; i < n; i++ {
		sum = sum.Add(klines[i].Close)
	}
	mean := sum.Div(decimal.NewFromInt(int64(period)))

	// 计算标准差
	varianceSum := decimal.Zero
	for i := start; i < n; i++ {
		diff := klines[i].Close.Sub(mean)
		varianceSum = varianceSum.Add(diff.Mul(diff))
	}
	variance := varianceSum.Div(decimal.NewFromInt(int64(period)))

	// 标准差 = sqrt(variance)
	stdFloat := variance.InexactFloat64()
	std := decimal.NewFromFloat(sqrt(stdFloat))

	return mean, std
}

func (s *MeanReversionStrategy) calcMRVolume(price decimal.Decimal) int64 {
	config := s.GetConfig()
	if !config.MaxPosition.IsZero() {
		volume := config.MaxPosition.Div(price).IntPart()
		return (volume / 100) * 100
	}
	return 100
}

// sqrt 简单的平方根计算
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

// ==================== 网格交易策略 ====================

// GridStrategy 网格交易策略
//
// 策略原理：
// 在价格区间内设置多个网格，每下降一格买入一定数量
// 每上涨一格卖出一定数量
// 适合震荡行情
//
// 这是A股散户最常用的自动化策略之一
type GridStrategy struct {
	BaseStrategy
	upperPrice float64                // 网格上限价
	lowerPrice float64                // 网格下限价
	gridCount  int                    // 网格数量
	gridVolume int64                  // 每格交易量
	grids      map[string][]gridLevel // 网格层级
	positions  map[string]int64       // 每只股票持仓量
}

type gridLevel struct {
	price    decimal.Decimal
	buyVol   int64
	sellVol  int64
	executed bool
}

// NewGridStrategy 创建网格策略
func NewGridStrategy(config models.StrategyConfig, logger *zap.Logger) *GridStrategy {
	s := &GridStrategy{
		BaseStrategy: NewBaseStrategy(config, logger),
		grids:        make(map[string][]gridLevel),
		positions:    make(map[string]int64),
	}
	s.upperPrice = s.getFloatParam("upper_price", 0)
	s.lowerPrice = s.getFloatParam("lower_price", 0)
	s.gridCount = s.getIntParam("grid_count", 10)
	s.gridVolume = int64(s.getIntParam("grid_volume", 100))
	for _, code := range config.Stocks {
		s.initGrid(code)
	}
	return s
}

func (s *GridStrategy) Type() string { return "grid" }

func (s *GridStrategy) Init(config models.StrategyConfig) error {
	s.upperPrice = s.getFloatParam("upper_price", 0)
	s.lowerPrice = s.getFloatParam("lower_price", 0)
	s.gridCount = s.getIntParam("grid_count", 10)
	s.gridVolume = int64(s.getIntParam("grid_volume", 100))
	s.grids = make(map[string][]gridLevel)
	s.positions = make(map[string]int64)

	// 为每只股票初始化网格
	for _, code := range config.Stocks {
		s.initGrid(code)
	}
	return nil
}

func (s *GridStrategy) OnBar(ctx context.Context, kline models.KLine) ([]models.Signal, error) {
	return s.checkGrid(kline.StockCode, kline.Close)
}

func (s *GridStrategy) OnQuote(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return s.checkGrid(quote.StockCode, quote.Close)
}

func (s *GridStrategy) OnTick(ctx context.Context, quote models.StockQuote) ([]models.Signal, error) {
	return s.checkGrid(quote.StockCode, quote.Close)
}

func (s *GridStrategy) GetParamDefs() []ParamDef {
	return []ParamDef{
		{Key: "upper_price", Name: "网格上限价", Type: "float", Default: 0, Description: "网格区间的上限价格（0表示自动）"},
		{Key: "lower_price", Name: "网格下限价", Type: "float", Default: 0, Description: "网格区间的下限价格（0表示自动）"},
		{Key: "grid_count", Name: "网格数量", Type: "int", Default: 10, Min: 3, Max: 50, Description: "网格的数量"},
		{Key: "grid_volume", Name: "每格交易量", Type: "int", Default: 100, Min: 100, Max: 10000, Description: "每个网格的交易数量（股）"},
	}
}

func (s *GridStrategy) initGrid(code string) {
	upper := decimal.NewFromFloat(s.upperPrice)
	lower := decimal.NewFromFloat(s.lowerPrice)

	if upper.IsZero() || lower.IsZero() {
		// 需要行情数据来初始化，暂时跳过
		return
	}

	gridStep := upper.Sub(lower).Div(decimal.NewFromInt(int64(s.gridCount)))
	grids := make([]gridLevel, s.gridCount+1)

	for i := 0; i <= s.gridCount; i++ {
		price := lower.Add(gridStep.Mul(decimal.NewFromInt(int64(i))))
		grids[i] = gridLevel{
			price:    price,
			buyVol:   s.gridVolume,
			sellVol:  s.gridVolume,
			executed: false,
		}
	}

	s.grids[code] = grids
	s.logger.Info("网格初始化完成",
		zap.String("stock", code),
		zap.String("upper", upper.String()),
		zap.String("lower", lower.String()),
		zap.Int("grids", s.gridCount),
	)
}

func (s *GridStrategy) checkGrid(code string, currentPrice decimal.Decimal) ([]models.Signal, error) {
	var signals []models.Signal

	grids, exists := s.grids[code]
	if !exists {
		return signals, nil
	}

	for i, grid := range grids {
		if grid.executed {
			continue
		}

		// 价格到达网格线 - 买入
		if currentPrice.LessThanOrEqual(grid.price) && grid.buyVol > 0 {
			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideBuy,
				Type:       models.OrderTypeLimit,
				Price:      grid.price,
				Volume:     grid.buyVol,
				Reason:     fmt.Sprintf("网格买入: 价格%s触及网格%d %s", currentPrice.StringFixed(2), i, grid.price.StringFixed(2)),
				Timestamp:  time.Now(),
			})
			grids[i].executed = true
			s.positions[code] += grid.buyVol
		}

		// 价格向上穿越网格线 - 卖出
		if currentPrice.GreaterThanOrEqual(grid.price) && s.positions[code] >= grid.sellVol && i > 0 && grids[i-1].executed {
			signals = append(signals, models.Signal{
				StrategyID: s.ID(),
				StockCode:  code,
				Side:       models.OrderSideSell,
				Type:       models.OrderTypeLimit,
				Price:      grid.price,
				Volume:     grid.sellVol,
				Reason:     fmt.Sprintf("网格卖出: 价格%s触及网格%d %s", currentPrice.StringFixed(2), i, grid.price.StringFixed(2)),
				Timestamp:  time.Now(),
			})
			s.positions[code] -= grid.sellVol
		}
	}

	s.grids[code] = grids
	return signals, nil
}
