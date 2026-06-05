package selector

import (
	"aqsystem/models"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

// MarketData 是选股引擎依赖的最小行情接口。
type MarketData interface {
	GetKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error)
}

type SelectionPlan struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	StrategyTypes []string `json:"strategy_types"`
	DefaultTopN   int      `json:"default_top_n"`
	Metrics       []string `json:"metrics"`
}

type CandidateStock struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Sector string `json:"sector"`
}

type SelectionRequest struct {
	StrategyType   string   `json:"strategy_type"`
	PlanID         string   `json:"plan_id"`
	Universe       string   `json:"universe"`
	CandidateCodes []string `json:"candidate_codes"`
	TopN           int      `json:"top_n"`
	LookbackDays   int      `json:"lookback_days"`
}

type StockMetrics struct {
	LastClose          float64 `json:"last_close"`
	RecentHigh         float64 `json:"recent_high"`
	RecentLow          float64 `json:"recent_low"`
	Return20           float64 `json:"return_20"`
	Return60           float64 `json:"return_60"`
	TrendStrength      float64 `json:"trend_strength"`
	BreakoutStrength   float64 `json:"breakout_strength"`
	Volatility         float64 `json:"volatility"`
	MaxDrawdown        float64 `json:"max_drawdown"`
	VolumeTrend        float64 `json:"volume_trend"`
	MeanReversionScore float64 `json:"mean_reversion_score"`
	GridSuitability    float64 `json:"grid_suitability"`
}

type StockPick struct {
	Rank      int          `json:"rank"`
	StockCode string       `json:"stock_code"`
	StockName string       `json:"stock_name"`
	Sector    string       `json:"sector"`
	Score     float64      `json:"score"`
	Reasons   []string     `json:"reasons"`
	Metrics   StockMetrics `json:"metrics"`
}

type RejectedStock struct {
	StockCode string `json:"stock_code"`
	StockName string `json:"stock_name"`
	Reason    string `json:"reason"`
}

type SelectionResult struct {
	Plan         SelectionPlan   `json:"plan"`
	StrategyType string          `json:"strategy_type"`
	Universe     string          `json:"universe"`
	GeneratedAt  time.Time       `json:"generated_at"`
	Picks        []StockPick     `json:"picks"`
	Rejected     []RejectedStock `json:"rejected"`
	DataIssues   []string        `json:"data_issues"`
}

type Engine struct {
	market MarketData
}

func NewEngine(market MarketData) *Engine {
	return &Engine{market: market}
}

func BuiltInPlans() []SelectionPlan {
	return []SelectionPlan{
		{
			ID:            "trend_breakout",
			Name:          "趋势突破选股",
			Description:   "寻找均线向上、价格接近阶段新高、成交量温和放大的股票，适合双均线和海龟策略。",
			StrategyTypes: []string{"double_ma", "turtle"},
			DefaultTopN:   5,
			Metrics:       []string{"20日收益", "均线趋势", "阶段新高", "成交量放大", "最大回撤"},
		},
		{
			ID:            "momentum_strength",
			Name:          "动量强势选股",
			Description:   "寻找近20/60日收益靠前、趋势持续性较强且回撤可控的股票，适合动量策略。",
			StrategyTypes: []string{"momentum"},
			DefaultTopN:   5,
			Metrics:       []string{"20日收益", "60日收益", "趋势强度", "最大回撤"},
		},
		{
			ID:            "oversold_rebound",
			Name:          "超跌反弹选股",
			Description:   "寻找价格显著低于短期均值但近期波动可控的股票，适合均值回归策略。",
			StrategyTypes: []string{"mean_reversion"},
			DefaultTopN:   5,
			Metrics:       []string{"均值偏离", "短期跌幅", "波动率", "成交量"},
		},
		{
			ID:            "grid_suitable",
			Name:          "网格适配选股",
			Description:   "寻找区间震荡、波动适中、趋势不过分单边的股票，适合网格交易。",
			StrategyTypes: []string{"grid"},
			DefaultTopN:   3,
			Metrics:       []string{"区间波动", "趋势斜率", "价格区间", "成交量稳定性"},
		},
		{
			ID:            "balanced_smart",
			Name:          "综合智能选股",
			Description:   "综合趋势、动量、波动率、回撤和成交量，适合不确定策略或人工复核。",
			StrategyTypes: []string{},
			DefaultTopN:   5,
			Metrics:       []string{"趋势", "动量", "波动", "回撤", "成交量"},
		},
	}
}

func DefaultPlanForStrategy(strategyType string) SelectionPlan {
	strategyType = strings.TrimSpace(strategyType)
	for _, plan := range BuiltInPlans() {
		for _, t := range plan.StrategyTypes {
			if t == strategyType {
				return plan
			}
		}
	}
	return planByID("balanced_smart")
}

func DefaultUniverse() []CandidateStock {
	return []CandidateStock{
		{Code: "600519", Name: "贵州茅台", Sector: "白酒"},
		{Code: "000858", Name: "五粮液", Sector: "白酒"},
		{Code: "000001", Name: "平安银行", Sector: "银行"},
		{Code: "600036", Name: "招商银行", Sector: "银行"},
		{Code: "601318", Name: "中国平安", Sector: "保险"},
		{Code: "000333", Name: "美的集团", Sector: "家电"},
		{Code: "300750", Name: "宁德时代", Sector: "新能源"},
		{Code: "002475", Name: "立讯精密", Sector: "消费电子"},
		{Code: "600276", Name: "恒瑞医药", Sector: "医药"},
		{Code: "601888", Name: "中国中免", Sector: "消费"},
		{Code: "002594", Name: "比亚迪", Sector: "汽车"},
		{Code: "600030", Name: "中信证券", Sector: "证券"},
		{Code: "600887", Name: "伊利股份", Sector: "食品饮料"},
		{Code: "601012", Name: "隆基绿能", Sector: "光伏"},
		{Code: "300059", Name: "东方财富", Sector: "金融科技"},
		{Code: "600309", Name: "万华化学", Sector: "化工"},
		{Code: "002415", Name: "海康威视", Sector: "安防"},
		{Code: "600900", Name: "长江电力", Sector: "电力"},
		{Code: "601899", Name: "紫金矿业", Sector: "有色"},
		{Code: "300760", Name: "迈瑞医疗", Sector: "医疗器械"},
		{Code: "000651", Name: "格力电器", Sector: "家电"},
		{Code: "603259", Name: "药明康德", Sector: "医药服务"},
		{Code: "601166", Name: "兴业银行", Sector: "银行"},
		{Code: "600438", Name: "通威股份", Sector: "新能源"},
		{Code: "300124", Name: "汇川技术", Sector: "自动化"},
	}
}

func (e *Engine) Select(ctx context.Context, req SelectionRequest) (*SelectionResult, error) {
	if e == nil || e.market == nil {
		return nil, fmt.Errorf("选股引擎缺少行情服务")
	}

	plan := DefaultPlanForStrategy(req.StrategyType)
	if strings.TrimSpace(req.PlanID) != "" {
		plan = planByID(req.PlanID)
	}
	topN := req.TopN
	if topN <= 0 {
		topN = plan.DefaultTopN
	}
	if topN <= 0 {
		topN = 5
	}
	lookback := req.LookbackDays
	if lookback < 60 {
		lookback = 90
	}

	candidates := candidatesForRequest(req)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("候选股票池为空")
	}

	picks := make([]StockPick, 0, len(candidates))
	rejected := make([]RejectedStock, 0)
	for _, candidate := range candidates {
		klines, err := e.market.GetKLines(ctx, candidate.Code, "day", lookback+30)
		if err != nil {
			rejected = append(rejected, RejectedStock{StockCode: candidate.Code, StockName: candidate.Name, Reason: err.Error()})
			continue
		}
		if len(klines) < 30 {
			rejected = append(rejected, RejectedStock{StockCode: candidate.Code, StockName: candidate.Name, Reason: "K线数据不足"})
			continue
		}

		metrics := calculateMetrics(klines)
		score := scoreByPlan(plan.ID, metrics)
		picks = append(picks, StockPick{
			StockCode: candidate.Code,
			StockName: candidate.Name,
			Sector:    candidate.Sector,
			Score:     round2(score),
			Reasons:   explainPick(plan.ID, metrics),
			Metrics:   metrics,
		})
	}

	sort.SliceStable(picks, func(i, j int) bool {
		if picks[i].Score == picks[j].Score {
			return picks[i].StockCode < picks[j].StockCode
		}
		return picks[i].Score > picks[j].Score
	})

	if topN > len(picks) {
		topN = len(picks)
	}
	picks = picks[:topN]
	for i := range picks {
		picks[i].Rank = i + 1
	}

	dataIssues := make([]string, 0)
	if len(rejected) > 0 {
		dataIssues = append(dataIssues, fmt.Sprintf("%d只候选股票因行情不足或请求失败被排除", len(rejected)))
	}

	return &SelectionResult{
		Plan:         plan,
		StrategyType: req.StrategyType,
		Universe:     normalizedUniverse(req.Universe),
		GeneratedAt:  time.Now(),
		Picks:        picks,
		Rejected:     rejected,
		DataIssues:   dataIssues,
	}, nil
}

func RecommendedParams(strategyType string, picks []StockPick) map[string]interface{} {
	switch strategyType {
	case "double_ma":
		return map[string]interface{}{"short_period": 5, "long_period": 20, "ma_type": "SMA"}
	case "turtle":
		return map[string]interface{}{"entry_period": 20, "exit_period": 10, "atr_period": 20, "risk_pct": 0.01}
	case "momentum":
		topN := len(picks)
		if topN <= 0 {
			topN = 3
		}
		return map[string]interface{}{"lookback_period": 20, "holding_period": 10, "top_n": topN, "momentum_threshold": 0.05}
	case "mean_reversion":
		return map[string]interface{}{"lookback_period": 20, "entry_zscore": 2.0, "exit_zscore": 0.5}
	case "grid":
		params := map[string]interface{}{"upper_price": 0, "lower_price": 0, "grid_count": 10, "grid_volume": 100}
		if len(picks) > 0 && picks[0].Metrics.RecentHigh > picks[0].Metrics.RecentLow {
			params["upper_price"] = round2(picks[0].Metrics.RecentHigh)
			params["lower_price"] = round2(picks[0].Metrics.RecentLow)
		}
		return params
	default:
		return map[string]interface{}{}
	}
}

func planByID(id string) SelectionPlan {
	for _, plan := range BuiltInPlans() {
		if plan.ID == id {
			return plan
		}
	}
	return BuiltInPlans()[len(BuiltInPlans())-1]
}

func candidatesForRequest(req SelectionRequest) []CandidateStock {
	base := DefaultUniverse()
	nameByCode := make(map[string]CandidateStock, len(base))
	for _, candidate := range base {
		nameByCode[candidate.Code] = candidate
	}

	if len(req.CandidateCodes) == 0 {
		return base
	}

	seen := make(map[string]bool)
	result := make([]CandidateStock, 0, len(req.CandidateCodes))
	for _, raw := range req.CandidateCodes {
		code := normalizeCode(raw)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		if candidate, ok := nameByCode[code]; ok {
			result = append(result, candidate)
		} else {
			result = append(result, CandidateStock{Code: code, Name: code, Sector: "自选"})
		}
	}
	return result
}

func normalizeCode(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "sh")
	raw = strings.TrimPrefix(raw, "sz")
	var digits strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	code := digits.String()
	if len(code) != 6 {
		return ""
	}
	return code
}

func normalizedUniverse(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "core_a_share"
	}
	return strings.TrimSpace(raw)
}

func calculateMetrics(klines []models.KLine) StockMetrics {
	closes := make([]float64, 0, len(klines))
	volumes := make([]float64, 0, len(klines))
	highs := make([]float64, 0, len(klines))
	lows := make([]float64, 0, len(klines))
	for _, k := range klines {
		closes = append(closes, k.Close.InexactFloat64())
		volumes = append(volumes, float64(k.Volume))
		highs = append(highs, k.High.InexactFloat64())
		lows = append(lows, k.Low.InexactFloat64())
	}

	last := closes[len(closes)-1]
	recentHigh := maxSlice(tail(highs, 60))
	recentLow := minSlice(tail(lows, 60))
	return20 := pctChange(closes, 20)
	return60 := pctChange(closes, 60)
	ma5 := avg(tail(closes, 5))
	ma20 := avg(tail(closes, 20))
	trend := 0.0
	if ma20 > 0 {
		trend = (ma5/ma20 - 1) * 100
	}
	breakout := 0.0
	if recentHigh > 0 {
		breakout = last / recentHigh * 100
	}
	volatility := annualizedVolatility(closes)
	drawdown := maxDrawdown(closes)
	volumeTrend := 0.0
	priorVol := avg(tail(volumes[:maxInt(len(volumes)-5, 0)], 20))
	if priorVol > 0 {
		volumeTrend = avg(tail(volumes, 5)) / priorVol
	}
	mean := avg(tail(closes, 20))
	std := stddev(tail(closes, 20))
	meanReversion := 0.0
	if std > 0 {
		z := (last - mean) / std
		if z < 0 {
			meanReversion = -z
		}
	}
	rangePct := 0.0
	if last > 0 && recentHigh > recentLow {
		rangePct = (recentHigh - recentLow) / last * 100
	}
	gridSuitability := clamp(100-math.Abs(volatility-35)*1.1-math.Abs(return60)*1.2-drawdown*0.7+rangePct*0.25, 0, 100)

	return StockMetrics{
		LastClose:          round2(last),
		RecentHigh:         round2(recentHigh),
		RecentLow:          round2(recentLow),
		Return20:           round2(return20),
		Return60:           round2(return60),
		TrendStrength:      round2(trend),
		BreakoutStrength:   round2(breakout),
		Volatility:         round2(volatility),
		MaxDrawdown:        round2(drawdown),
		VolumeTrend:        round2(volumeTrend),
		MeanReversionScore: round2(meanReversion),
		GridSuitability:    round2(gridSuitability),
	}
}

func scoreByPlan(planID string, m StockMetrics) float64 {
	switch planID {
	case "trend_breakout":
		return clamp(45+m.Return20*1.1+m.TrendStrength*3+(m.BreakoutStrength-85)*1.3+m.VolumeTrend*6-m.MaxDrawdown*0.45, 0, 100)
	case "momentum_strength":
		return clamp(45+m.Return20*1.3+m.Return60*0.65+m.TrendStrength*2.5+m.VolumeTrend*5-m.MaxDrawdown*0.35, 0, 100)
	case "oversold_rebound":
		return clamp(45+m.MeanReversionScore*16-m.Return20*0.45-m.MaxDrawdown*0.2+volatilityBandScore(m.Volatility), 0, 100)
	case "grid_suitable":
		return clamp(m.GridSuitability, 0, 100)
	default:
		return clamp(45+m.Return20*0.6+m.Return60*0.3+m.TrendStrength*1.4+m.VolumeTrend*4-m.MaxDrawdown*0.3+volatilityBandScore(m.Volatility), 0, 100)
	}
}

func explainPick(planID string, m StockMetrics) []string {
	common := []string{
		fmt.Sprintf("20日收益 %.2f%%，60日收益 %.2f%%", m.Return20, m.Return60),
		fmt.Sprintf("最大回撤 %.2f%%，年化波动 %.2f%%", m.MaxDrawdown, m.Volatility),
	}
	switch planID {
	case "trend_breakout":
		return append([]string{
			fmt.Sprintf("价格处于近60日高点 %.2f%% 位置", m.BreakoutStrength),
			fmt.Sprintf("短均线相对长均线强度 %.2f%%", m.TrendStrength),
		}, common...)
	case "momentum_strength":
		return append([]string{
			fmt.Sprintf("动量延续强度 %.2f%%", m.Return20+m.Return60*0.5),
			fmt.Sprintf("成交量近5日/前20日比 %.2f", m.VolumeTrend),
		}, common...)
	case "oversold_rebound":
		return append([]string{
			fmt.Sprintf("低于20日均值的反弹潜力 %.2f", m.MeanReversionScore),
			fmt.Sprintf("短期跌幅越深得分越高，当前20日收益 %.2f%%", m.Return20),
		}, common...)
	case "grid_suitable":
		return append([]string{
			fmt.Sprintf("网格适配分 %.2f，近期区间 %.2f-%.2f", m.GridSuitability, m.RecentLow, m.RecentHigh),
			fmt.Sprintf("趋势不过分单边，60日收益 %.2f%%", m.Return60),
		}, common...)
	default:
		return common
	}
}

func pctChange(values []float64, period int) float64 {
	if len(values) <= period {
		period = len(values) - 1
	}
	if period <= 0 {
		return 0
	}
	base := values[len(values)-period-1]
	if base == 0 {
		return 0
	}
	return (values[len(values)-1]/base - 1) * 100
}

func annualizedVolatility(closes []float64) float64 {
	if len(closes) < 2 {
		return 0
	}
	returns := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		if closes[i-1] == 0 {
			continue
		}
		returns = append(returns, closes[i]/closes[i-1]-1)
	}
	return stddev(returns) * math.Sqrt(252) * 100
}

func maxDrawdown(closes []float64) float64 {
	if len(closes) == 0 {
		return 0
	}
	peak := closes[0]
	maxDD := 0.0
	for _, price := range closes {
		if price > peak {
			peak = price
		}
		if peak > 0 {
			dd := (peak - price) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}

func volatilityBandScore(volatility float64) float64 {
	return clamp(20-math.Abs(volatility-30)*0.5, -10, 20)
}

func tail(values []float64, n int) []float64 {
	if n <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) <= n {
		return values
	}
	return values[len(values)-n:]
}

func avg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func stddev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := avg(values)
	sum := 0.0
	for _, v := range values {
		d := v - mean
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(values)-1))
}

func maxSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func minSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
