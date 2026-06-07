package api

import (
	"aqsystem/backtest"
	"aqsystem/broker"
	"aqsystem/config"
	"aqsystem/market"
	"aqsystem/models"
	"aqsystem/risk"
	"aqsystem/selector"
	"aqsystem/strategy"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// corsMiddleware CORS中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// Server API服务器
type Server struct {
	engine      *strategy.Engine
	marketSvc   *market.MarketService
	brokerMgr   *broker.BrokerManager
	riskManager *risk.RiskManager
	btEngine    *backtest.BacktestEngine
	logger      *zap.Logger
}

// NewServer 创建API服务器
func NewServer(
	engine *strategy.Engine,
	marketSvc *market.MarketService,
	brokerMgr *broker.BrokerManager,
	riskMgr *risk.RiskManager,
	logger *zap.Logger,
) *Server {
	cfg := config.GetConfig()
	btEngine := backtest.NewBacktestEngine(
		cfg.Strategy.CommissionRate,
		cfg.Strategy.StampTaxRate,
		cfg.Strategy.Slippage,
		logger,
	)

	return &Server{
		engine:      engine,
		marketSvc:   marketSvc,
		brokerMgr:   brokerMgr,
		riskManager: riskMgr,
		btEngine:    btEngine,
		logger:      logger,
	}
}

// SetupRouter 设置路由
func (s *Server) SetupRouter() *gin.Engine {
	cfg := config.GetConfig()
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// CORS中间件
	r.Use(corsMiddleware())

	// 健康检查
	r.GET("/health", s.healthCheck)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 券商相关
		v1.GET("/broker/providers", s.getBrokerProviders)
		v1.GET("/broker/status", s.getBrokerStatus)
		v1.POST("/broker/switch", s.switchBroker)
		v1.POST("/broker/login", s.brokerLogin)
		v1.POST("/broker/logout", s.brokerLogout)
		v1.GET("/broker/account", s.getAccount)
		v1.GET("/broker/positions", s.getPositions)
		v1.GET("/broker/orders", s.getOrders)

		// 交易
		v1.POST("/trade/order", s.submitOrder)
		v1.DELETE("/trade/order/:id", s.cancelOrder)

		// 行情
		v1.GET("/market/quote/:code", s.getQuote)
		v1.POST("/market/quotes", s.getQuotes)
		v1.GET("/market/kline/:code", s.getKLines)
		v1.GET("/market/index/:code", s.getIndexQuote)
		v1.POST("/market/subscribe", s.subscribe)

		// 策略
		v1.GET("/strategies", s.listStrategies)
		v1.GET("/strategy/:id", s.getStrategy)
		v1.POST("/strategy/:id/start", s.startStrategy)
		v1.POST("/strategy/:id/stop", s.stopStrategy)
		v1.POST("/strategy/:id/pause", s.pauseStrategy)
		v1.PUT("/strategy/:id/params", s.updateStrategyParams)
		v1.GET("/strategy/:id/params-defs", s.getStrategyParamDefs)
		v1.GET("/strategy-templates", s.getStrategyTemplates)

		// 智能选股/智能交易
		v1.GET("/stock-selector/plans", s.getStockSelectionPlans)
		v1.GET("/stock-selector/universe", s.getStockUniverse)
		v1.POST("/stock-selector/run", s.runStockSelection)
		v1.GET("/smart-trade/benchmark", s.getSmartTradeBenchmark)
		v1.POST("/smart-trade/run", s.runSmartTrade)
		v1.POST("/smart-trade/apply", s.applySmartTrade)

		// 回测
		v1.POST("/backtest", s.runBacktest)

		// 风控
		v1.GET("/risk/config", s.getRiskConfig)
		v1.PUT("/risk/config", s.updateRiskConfig)
		v1.GET("/risk/events", s.getRiskEvents)

		// 系统状态
		v1.GET("/system/status", s.getSystemStatus)
	}

	return r
}

// ==================== 券商接口 ====================

func (s *Server) getBrokerProviders(c *gin.Context) {
	s.responseSuccess(c, broker.SupportedBrokerProviders())
}

func (s *Server) getBrokerStatus(c *gin.Context) {
	s.responseSuccess(c, s.brokerMgr.Status())
}

func (s *Server) switchBroker(c *gin.Context) {
	var req models.BrokerConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.brokerMgr.Select(c.Request.Context(), req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}
	s.responseSuccess(c, s.brokerMgr.Status())
}

func (s *Server) brokerLogin(c *gin.Context) {
	var req models.BrokerConfig
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
			s.responseError(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := s.brokerMgr.Login(c.Request.Context(), req); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "登录成功", "status": s.brokerMgr.Status()})
}

func (s *Server) brokerLogout(c *gin.Context) {
	if err := s.brokerMgr.Logout(c.Request.Context()); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "登出成功", "status": s.brokerMgr.Status()})
}

func (s *Server) getAccount(c *gin.Context) {
	b, ok := s.currentBroker(c)
	if !ok {
		return
	}
	account, err := b.GetAccount(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, account)
}

func (s *Server) getPositions(c *gin.Context) {
	b, ok := s.currentBroker(c)
	if !ok {
		return
	}
	positions, err := b.GetPositions(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, positions)
}

func (s *Server) getOrders(c *gin.Context) {
	b, ok := s.currentBroker(c)
	if !ok {
		return
	}
	orders, err := b.GetOrders(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, orders)
}

// ==================== 交易接口 ====================

func (s *Server) submitOrder(c *gin.Context) {
	b, ok := s.currentBroker(c)
	if !ok {
		return
	}

	var req struct {
		StockCode  string `json:"stock_code" binding:"required"`
		Side       string `json:"side" binding:"required"`
		Type       string `json:"type" binding:"required"`
		Price      string `json:"price" binding:"required"`
		Volume     int64  `json:"volume" binding:"required"`
		StrategyID string `json:"strategy_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		s.responseError(c, http.StatusBadRequest, "无效的价格")
		return
	}

	order := &models.Order{
		StockCode:  req.StockCode,
		Side:       models.OrderSide(req.Side),
		Type:       models.OrderType(req.Type),
		Price:      price,
		Volume:     req.Volume,
		StrategyID: req.StrategyID,
	}

	// 风控检查
	account, err := b.GetAccount(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.riskManager.CheckOrder(c.Request.Context(), order, account); err != nil {
		s.responseError(c, http.StatusForbidden, "风控拒绝: "+err.Error())
		return
	}

	result, err := b.SubmitOrder(c.Request.Context(), order)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}

	s.riskManager.RecordTrade(result)
	s.responseSuccess(c, result)
}

func (s *Server) cancelOrder(c *gin.Context) {
	b, ok := s.currentBroker(c)
	if !ok {
		return
	}
	orderID := c.Param("id")
	if err := b.CancelOrder(c.Request.Context(), orderID); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "撤单成功"})
}

// ==================== 行情接口 ====================

func (s *Server) getQuote(c *gin.Context) {
	code := c.Param("code")
	quote, err := s.marketSvc.GetQuote(c.Request.Context(), code)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, quote)
}

func (s *Server) getQuotes(c *gin.Context) {
	var req struct {
		Codes []string `json:"codes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}
	quotes, err := s.marketSvc.GetQuotes(c.Request.Context(), req.Codes)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, quotes)
}

func (s *Server) getKLines(c *gin.Context) {
	code := c.Param("code")
	period := c.DefaultQuery("period", "day")
	count := 100
	if period == "minute" || period == "1m" {
		count = 300
	}

	quotes, err := s.marketSvc.GetKLines(c.Request.Context(), code, period, count)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, quotes)
}

func (s *Server) getIndexQuote(c *gin.Context) {
	code := c.Param("code")
	quote, err := s.marketSvc.GetIndexQuote(c.Request.Context(), code)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, quote)
}

func (s *Server) subscribe(c *gin.Context) {
	var req struct {
		Codes []string `json:"codes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.marketSvc.Subscribe(c.Request.Context(), req.Codes); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "订阅成功"})
}

// ==================== 策略接口 ====================

func (s *Server) listStrategies(c *gin.Context) {
	strategies := s.engine.ListStrategies()
	result := make([]gin.H, 0, len(strategies))
	for _, strat := range strategies {
		result = append(result, gin.H{
			"id":          strat.ID(),
			"name":        strat.Name(),
			"description": strat.Description(),
			"type":        strat.Type(),
			"status":      strat.GetStatus(),
			"params":      strat.GetParams(),
		})
	}
	s.responseSuccess(c, result)
}

func (s *Server) getStrategy(c *gin.Context) {
	id := c.Param("id")
	strat, ok := s.engine.GetStrategy(id)
	if !ok {
		s.responseError(c, http.StatusNotFound, "策略不存在")
		return
	}
	s.responseSuccess(c, gin.H{
		"id":          strat.ID(),
		"name":        strat.Name(),
		"description": strat.Description(),
		"type":        strat.Type(),
		"status":      strat.GetStatus(),
		"params":      strat.GetParams(),
		"config":      strat.GetConfig(),
	})
}

func (s *Server) startStrategy(c *gin.Context) {
	id := c.Param("id")
	if err := s.engine.StartStrategy(id); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "策略已启动"})
}

func (s *Server) stopStrategy(c *gin.Context) {
	id := c.Param("id")
	if err := s.engine.StopStrategy(id); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "策略已停止"})
}

func (s *Server) pauseStrategy(c *gin.Context) {
	id := c.Param("id")
	if err := s.engine.PauseStrategy(id); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "策略已暂停"})
}

func (s *Server) updateStrategyParams(c *gin.Context) {
	id := c.Param("id")
	strat, ok := s.engine.GetStrategy(id)
	if !ok {
		s.responseError(c, http.StatusNotFound, "策略不存在")
		return
	}

	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := strat.SetParams(params); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}

	s.responseSuccess(c, gin.H{"message": "参数更新成功"})
}

func (s *Server) getStrategyParamDefs(c *gin.Context) {
	id := c.Param("id")
	strat, ok := s.engine.GetStrategy(id)
	if !ok {
		s.responseError(c, http.StatusNotFound, "策略不存在")
		return
	}
	s.responseSuccess(c, strat.GetParamDefs())
}

// getStrategyTemplates 获取策略模板
func (s *Server) getStrategyTemplates(c *gin.Context) {
	templates := []gin.H{
		{
			"type":        "double_ma",
			"name":        "双均线交叉策略",
			"description": "短期均线上穿长期均线形成金叉买入，下穿形成死叉卖出。最经典的技术分析策略，适合趋势明显的市场。",
			"params": []gin.H{
				{"key": "short_period", "name": "短期均线周期", "type": "int", "default": 5, "min": 2, "max": 60, "description": "用于捕捉短期趋势，数值越小越灵敏，默认5日。"},
				{"key": "long_period", "name": "长期均线周期", "type": "int", "default": 20, "min": 10, "max": 250, "description": "用于确认中长期趋势，必须大于短期周期，默认20日。"},
				{"key": "ma_type", "name": "均线类型", "type": "string", "default": "SMA", "description": "SMA为简单均线，EMA更重视最近价格。默认SMA。"},
			},
		},
		{
			"type":        "turtle",
			"name":        "海龟交易策略",
			"description": "基于唐奇安通道突破的趋势跟踪策略，使用ATR进行仓位管理和止损。80年代最著名的交易系统之一。",
			"params": []gin.H{
				{"key": "entry_period", "name": "入场通道周期", "type": "int", "default": 20, "min": 5, "max": 100, "description": "突破最近N日高点时入场，默认20日。"},
				{"key": "exit_period", "name": "出场通道周期", "type": "int", "default": 10, "min": 5, "max": 50, "description": "跌破最近N日低点时出场，默认10日。"},
				{"key": "atr_period", "name": "ATR周期", "type": "int", "default": 20, "min": 5, "max": 100, "description": "用于计算波动率和止损距离，默认20日。"},
				{"key": "risk_pct", "name": "每笔风险比例", "type": "float", "default": 0.01, "min": 0.001, "max": 0.05, "description": "单笔交易最大风险占资金比例，0.01表示1%。"},
			},
		},
		{
			"type":        "momentum",
			"name":        "动量策略",
			"description": "选择过去N日涨幅最大的股票买入（强者恒强），当动量减弱时卖出。学术研究表明3-12个月动量效应在A股显著存在。",
			"params": []gin.H{
				{"key": "lookback_period", "name": "回望期", "type": "int", "default": 20, "min": 5, "max": 120, "description": "计算过去N日涨跌幅，默认20日。"},
				{"key": "holding_period", "name": "持有期", "type": "int", "default": 10, "min": 1, "max": 60, "description": "买入后至少持有的交易日数量，默认10日。"},
				{"key": "top_n", "name": "选股数量", "type": "int", "default": 3, "min": 1, "max": 20, "description": "多股票回测时选择动量最强的前N只，默认3只。"},
				{"key": "momentum_threshold", "name": "动量阈值", "type": "float", "default": 0.05, "min": 0.0, "max": 0.5, "description": "达到该涨幅阈值才买入，0.05表示5%。"},
			},
		},
		{
			"type":        "mean_reversion",
			"name":        "均值回归策略",
			"description": "价格偏离均值过大时预期会回归，低于均值-2倍标准差时买入，回归后卖出。最经典的统计套利策略。",
			"params": []gin.H{
				{"key": "lookback_period", "name": "回望期", "type": "int", "default": 20, "min": 5, "max": 120, "description": "计算均值和标准差的窗口，默认20日。"},
				{"key": "entry_zscore", "name": "入场Z-score", "type": "float", "default": 2.0, "min": 1.0, "max": 4.0, "description": "价格低于均值若干倍标准差时买入，默认2.0。"},
				{"key": "exit_zscore", "name": "出场Z-score", "type": "float", "default": 0.5, "min": 0.0, "max": 2.0, "description": "价格回归到该偏离范围内时卖出，默认0.5。"},
			},
		},
		{
			"type":        "grid",
			"name":        "网格交易策略",
			"description": "在价格区间内设置网格，每下降一格买入一定数量，每上涨一格卖出。适合震荡行情，A股最常用的自动化策略。",
			"params": []gin.H{
				{"key": "upper_price", "name": "网格上限价", "type": "float", "default": 0, "description": "网格区间上沿，0表示由策略自动初始化。"},
				{"key": "lower_price", "name": "网格下限价", "type": "float", "default": 0, "description": "网格区间下沿，0表示由策略自动初始化。"},
				{"key": "grid_count", "name": "网格数量", "type": "int", "default": 10, "min": 3, "max": 50, "description": "区间切分数量，越多交易越频繁，默认10格。"},
				{"key": "grid_volume", "name": "每格交易量", "type": "int", "default": 100, "min": 100, "max": 10000, "description": "每次触发网格买卖的股数，A股默认最小100股。"},
			},
		},
		{
			"type":        "macd_t",
			"name":        "MACD短线做T策略",
			"description": "围绕MACD金叉、DIF强于DEA、柱线连续改善和量能不弱做短线交易，使用短线止盈、止损、最长持有天数和柱线转弱退出。",
			"params": []gin.H{
				{"key": "fast_period", "name": "快线EMA周期", "type": "int", "default": 12, "min": 5, "max": 30, "description": "MACD快线EMA周期，默认12。"},
				{"key": "slow_period", "name": "慢线EMA周期", "type": "int", "default": 26, "min": 10, "max": 60, "description": "MACD慢线EMA周期，默认26，需大于快线。"},
				{"key": "signal_period", "name": "信号线周期", "type": "int", "default": 9, "min": 3, "max": 20, "description": "DEA信号线EMA周期，默认9。"},
				{"key": "trend_period", "name": "趋势过滤周期", "type": "int", "default": 20, "min": 10, "max": 60, "description": "价格过度跌破趋势线时不追做T，默认20日。"},
				{"key": "hist_turn_days", "name": "柱线转强天数", "type": "int", "default": 3, "min": 2, "max": 8, "description": "要求MACD柱线连续改善的天数。"},
				{"key": "max_hold_days", "name": "最长持有天数", "type": "int", "default": 5, "min": 1, "max": 20, "description": "超过该天数且柱线转弱则退出。"},
				{"key": "take_profit_pct", "name": "短线止盈", "type": "float", "default": 0.025, "min": 0.005, "max": 0.15, "description": "达到该收益率卖出，0.025表示2.5%。"},
				{"key": "stop_loss_pct", "name": "短线止损", "type": "float", "default": 0.018, "min": 0.005, "max": 0.1, "description": "达到该亏损率卖出，0.018表示1.8%。"},
			},
		},
	}
	s.responseSuccess(c, templates)
}

// ==================== 智能选股/智能交易接口 ====================

type smartTradeRequest struct {
	StrategyType   string                 `json:"strategy_type" binding:"required"`
	PlanID         string                 `json:"plan_id"`
	Universe       string                 `json:"universe"`
	CandidateCodes []string               `json:"candidate_codes"`
	TopN           int                    `json:"top_n"`
	LookbackDays   int                    `json:"lookback_days"`
	Params         map[string]interface{} `json:"params"`
	StartDate      string                 `json:"start_date"`
	EndDate        string                 `json:"end_date"`
	InitCapital    float64                `json:"init_capital"`
}

type smartTradeApplyRequest struct {
	StrategyType string                 `json:"strategy_type" binding:"required"`
	Stocks       []string               `json:"stocks" binding:"required"`
	Params       map[string]interface{} `json:"params"`
	Mode         string                 `json:"mode"`
	InitCapital  float64                `json:"init_capital"`
	AutoStart    bool                   `json:"auto_start"`
}

type candidateBacktest struct {
	Rank         int     `json:"rank"`
	StockCode    string  `json:"stock_code"`
	StockName    string  `json:"stock_name"`
	TotalReturn  float64 `json:"total_return"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	SharpeRatio  float64 `json:"sharpe_ratio"`
	TotalTrades  int     `json:"total_trades"`
	FinalCapital float64 `json:"final_capital"`
	RankScore    float64 `json:"rank_score"`
	Outcome      string  `json:"outcome"`
	Selected     bool    `json:"selected"`
}

type validationSummary struct {
	CandidateCount int      `json:"candidate_count"`
	ValidatedCount int      `json:"validated_count"`
	PositiveCount  int      `json:"positive_count"`
	FlatCount      int      `json:"flat_count"`
	NegativeCount  int      `json:"negative_count"`
	PositiveRate   float64  `json:"positive_rate"`
	BestReturn     float64  `json:"best_return"`
	WorstDrawdown  float64  `json:"worst_drawdown"`
	Deployable     bool     `json:"deployable"`
	GateReason     string   `json:"gate_reason"`
	Method         string   `json:"method"`
	Warnings       []string `json:"warnings"`
}

func (s *Server) getStockSelectionPlans(c *gin.Context) {
	s.responseSuccess(c, selector.BuiltInPlans())
}

func (s *Server) getStockUniverse(c *gin.Context) {
	s.responseSuccess(c, selector.DefaultUniverse())
}

func (s *Server) runStockSelection(c *gin.Context) {
	var req selector.SelectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	result, err := selector.NewEngine(s.marketSvc).Select(c.Request.Context(), req)
	if err != nil {
		s.responseError(c, http.StatusBadGateway, err.Error())
		return
	}
	s.responseSuccess(c, result)
}

func (s *Server) getSmartTradeBenchmark(c *gin.Context) {
	s.responseSuccess(c, []gin.H{
		{
			"module":       "数据与因子",
			"benchmark":    "公开机构资料强调大规模数据、AI/多因子和持续因子挖掘。",
			"implemented":  true,
			"system_field": "核心A股池、行情K线、策略专属选股方案、机构对标多因子集成。",
		},
		{
			"module":       "策略验证",
			"benchmark":    "研究流程通常不会只看一次组合回测，而会先做候选标的验证和风险调整排序。",
			"implemented":  true,
			"system_field": "因子初筛后对全量候选逐只运行策略回测，展示盈利、亏损、未入选样本，再按收益、回撤、夏普和交易次数综合排序。",
		},
		{
			"module":       "组合与风控",
			"benchmark":    "成熟量化系统重视回撤、波动、风险敞口和分散度。",
			"implemented":  true,
			"system_field": "验证摘要输出正收益率、最佳收益、最坏回撤，并给出过拟合/高回撤提示。",
		},
		{
			"module":       "执行闭环",
			"benchmark":    "生产系统需要从研究、模拟到实盘执行的一体化链路。",
			"implemented":  true,
			"system_field": "一键选股、回测、参数生成、应用到模拟交易或当前已登录实盘券商。",
		},
	})
}

func (s *Server) runSmartTrade(c *gin.Context) {
	var req smartTradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	requestedTopN := req.TopN
	if requestedTopN <= 0 {
		requestedTopN = selector.DefaultPlanForStrategy(req.StrategyType).DefaultTopN
	}
	if requestedTopN <= 0 {
		requestedTopN = 5
	}

	selection, err := selector.NewEngine(s.marketSvc).Select(c.Request.Context(), selector.SelectionRequest{
		StrategyType:   req.StrategyType,
		PlanID:         req.PlanID,
		Universe:       req.Universe,
		CandidateCodes: req.CandidateCodes,
		TopN:           requestedTopN,
		LookbackDays:   req.LookbackDays,
	})
	if err != nil {
		s.responseError(c, http.StatusBadGateway, "智能选股失败: "+err.Error())
		return
	}
	if len(selection.Picks) == 0 {
		s.responseError(c, http.StatusBadGateway, "智能选股未选出可交易股票")
		return
	}

	startDate, endDate, err := parseSmartDateRange(req.StartDate, req.EndDate, time.Now())
	if err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	validationPicks := selection.Evaluated
	if len(validationPicks) == 0 {
		validationPicks = selection.Picks
	}

	candidateBacktests := s.validateCandidatesByBacktest(c.Request.Context(), req.StrategyType, validationPicks, req.Params, startDate, endDate, req.InitCapital)
	rankedAllBacktests := rankCandidateBacktests(candidateBacktests, 0)
	selectedBacktests := rankCandidateBacktests(candidateBacktests, requestedTopN)
	summary := buildValidationSummary(candidateBacktests, len(validationPicks))
	rankedAllBacktests = markSelectedBacktests(rankedAllBacktests, selectedBacktests)
	selectedBacktests = markSelectedBacktests(selectedBacktests, selectedBacktests)
	if len(selectedBacktests) > 0 {
		selection.Picks = reorderPicksByBacktest(validationPicks, selectedBacktests)
	}
	if len(selection.Picks) > requestedTopN {
		selection.Picks = selection.Picks[:requestedTopN]
		for i := range selection.Picks {
			selection.Picks[i].Rank = i + 1
		}
	}

	stocks := make([]string, 0, len(selection.Picks))
	for _, pick := range selection.Picks {
		stocks = append(stocks, pick.StockCode)
	}

	params := selector.RecommendedParams(req.StrategyType, selection.Picks)
	for k, v := range req.Params {
		params[k] = v
	}

	result, err := s.runBacktestInternal(c.Request.Context(), req.StrategyType, stocks, params, startDate, endDate, req.InitCapital)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, "智能回测失败: "+err.Error())
		return
	}

	s.responseSuccess(c, gin.H{
		"workflow": []gin.H{
			{"step": "select", "status": "done", "text": fmt.Sprintf("因子初筛%d只候选，保留全量验证样本", len(validationPicks))},
			{"step": "validate", "status": "done", "text": fmt.Sprintf("二次验证%d只，正收益%d只", summary.ValidatedCount, summary.PositiveCount)},
			{"step": "params", "status": "done", "text": "已按策略和股票特征生成参数"},
			{"step": "backtest", "status": "done", "text": "已完成真实K线回测"},
			{"step": "apply", "status": "ready", "text": "可一键应用到模拟交易或当前实盘连接"},
		},
		"selection":           selection,
		"stocks":              stocks,
		"recommended_params":  params,
		"candidate_backtests": rankedAllBacktests,
		"selected_backtests":  selectedBacktests,
		"validation_summary":  summary,
		"backtest":            result,
		"start_date":          startDate.Format("2006-01-02"),
		"end_date":            endDate.Format("2006-01-02"),
	})
}

func (s *Server) applySmartTrade(c *gin.Context) {
	var req smartTradeApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	stocks, err := normalizeBacktestStocks(req.Stocks)
	if err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "paper"
	}
	if mode == "paper" {
		if err := s.brokerMgr.Login(c.Request.Context(), models.BrokerConfig{
			ID:        "sim_broker_01",
			Name:      "模拟券商",
			Type:      "simulated",
			AccountID: "SIM_ACCOUNT_001",
			IsDemo:    true,
			CommType:  "http",
		}); err != nil {
			s.responseError(c, http.StatusInternalServerError, "切换模拟券商失败: "+err.Error())
			return
		}
	} else if mode == "live" && !s.brokerMgr.Status().LiveTrading {
		s.responseError(c, http.StatusBadRequest, "实盘模式需要先登录真实券商")
		return
	}

	params := selector.RecommendedParams(req.StrategyType, nil)
	for k, v := range req.Params {
		params[k] = v
	}

	initCapital := decimal.NewFromFloat(1000000)
	if req.InitCapital > 0 {
		initCapital = decimal.NewFromFloat(req.InitCapital)
	}

	config := models.StrategyConfig{
		ID:          "smart_" + req.StrategyType,
		Name:        "智能一键-" + strategyDisplayName(req.StrategyType),
		Type:        req.StrategyType,
		Description: "由智能交易模块自动选股、回测并应用的策略实例。",
		Stocks:      stocks,
		Params:      params,
		Status:      models.StrategyStatusPaused,
		MaxPosition: initCapital.Div(decimal.NewFromInt(int64(len(stocks)))),
		StopLoss:    decimal.NewFromFloat(0.08),
		TakeProfit:  decimal.NewFromFloat(0.2),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	strat, err := newStrategyFromConfig(config, s.logger)
	if err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	_ = s.engine.UnregisterStrategy(config.ID)
	if err := s.engine.RegisterStrategy(strat); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.marketSvc.Subscribe(c.Request.Context(), stocks); err != nil {
		s.responseError(c, http.StatusInternalServerError, "订阅行情失败: "+err.Error())
		return
	}
	if req.AutoStart {
		if err := s.engine.StartStrategy(config.ID); err != nil {
			s.responseError(c, http.StatusInternalServerError, "启动智能策略失败: "+err.Error())
			return
		}
	}

	s.responseSuccess(c, gin.H{
		"message":       "智能策略已应用",
		"strategy_id":   config.ID,
		"strategy_name": config.Name,
		"stocks":        stocks,
		"params":        params,
		"status":        strat.GetStatus(),
		"mode":          mode,
		"broker_status": s.brokerMgr.Status(),
	})
}

// ==================== 回测接口 ====================

func (s *Server) runBacktest(c *gin.Context) {
	var req struct {
		StrategyType string                 `json:"strategy_type" binding:"required"`
		Stocks       []string               `json:"stocks" binding:"required"`
		Params       map[string]interface{} `json:"params"`
		StartDate    string                 `json:"start_date" binding:"required"`
		EndDate      string                 `json:"end_date" binding:"required"`
		InitCapital  float64                `json:"init_capital"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	stockCodes, err := normalizeBacktestStocks(req.Stocks)
	if err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}

	startDate, err := time.ParseInLocation("2006-01-02", req.StartDate, time.Local)
	if err != nil {
		s.responseError(c, http.StatusBadRequest, "日期格式错误")
		return
	}

	endDate, err := time.ParseInLocation("2006-01-02", req.EndDate, time.Local)
	if err != nil {
		s.responseError(c, http.StatusBadRequest, "日期格式错误")
		return
	}
	if endDate.Before(startDate) {
		s.responseError(c, http.StatusBadRequest, "结束日期不能早于开始日期")
		return
	}

	result, err := s.runBacktestInternal(c.Request.Context(), req.StrategyType, stockCodes, req.Params, startDate, endDate, req.InitCapital)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, "回测失败: "+err.Error())
		return
	}

	s.responseSuccess(c, result)
}

func (s *Server) runBacktestInternal(ctx context.Context, strategyType string, stockCodes []string, params map[string]interface{}, startDate, endDate time.Time, initialCapital float64) (*models.BacktestResult, error) {
	initCapital := decimal.NewFromFloat(1000000)
	if initialCapital > 0 {
		initCapital = decimal.NewFromFloat(initialCapital)
	}
	if params == nil {
		params = map[string]interface{}{}
	}

	stratConfig := models.StrategyConfig{
		ID:          "backtest_" + strategyType,
		Name:        strategyType,
		Type:        strategyType,
		Stocks:      stockCodes,
		Params:      params,
		Status:      models.StrategyStatusPaused,
		MaxPosition: initCapital.Div(decimal.NewFromInt(int64(len(stockCodes)))),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	strat, err := newStrategyFromConfig(stratConfig, s.logger)
	if err != nil {
		return nil, err
	}

	klines := make(map[string][]models.KLine)
	klineCount := estimateBacktestKLineCount(startDate, endDate)
	for _, code := range stockCodes {
		klineData, err := s.marketSvc.GetKLines(ctx, code, "day", klineCount)
		if err != nil {
			return nil, fmt.Errorf("获取真实K线数据失败: %s %w", code, err)
		}
		if len(klineData) == 0 {
			return nil, fmt.Errorf("获取真实K线数据失败: %s K线数据为空", code)
		}
		klines[code] = klineData
	}

	return s.btEngine.Run(ctx, strat, klines, startDate, endDate, initCapital)
}

func (s *Server) validateCandidatesByBacktest(ctx context.Context, strategyType string, picks []selector.StockPick, extraParams map[string]interface{}, startDate, endDate time.Time, initialCapital float64) []candidateBacktest {
	results := make([]candidateBacktest, 0, len(picks))
	for _, pick := range picks {
		params := selector.RecommendedParams(strategyType, []selector.StockPick{pick})
		for k, v := range extraParams {
			params[k] = v
		}
		result, err := s.runBacktestInternal(ctx, strategyType, []string{pick.StockCode}, params, startDate, endDate, initialCapital)
		if err != nil || result == nil {
			continue
		}
		totalReturn := result.TotalReturn.InexactFloat64()
		maxDrawdown := result.MaxDrawdown.InexactFloat64()
		sharpe := result.SharpeRatio.InexactFloat64()
		finalCapital := result.FinalCapital.InexactFloat64()
		results = append(results, candidateBacktest{
			StockCode:    pick.StockCode,
			StockName:    pick.StockName,
			TotalReturn:  roundFloat(totalReturn, 2),
			MaxDrawdown:  roundFloat(maxDrawdown, 2),
			SharpeRatio:  roundFloat(sharpe, 2),
			TotalTrades:  result.TotalTrades,
			FinalCapital: roundFloat(finalCapital, 2),
			RankScore:    roundFloat(candidateRankScore(totalReturn, maxDrawdown, sharpe, result.TotalTrades), 2),
			Outcome:      candidateOutcome(totalReturn),
		})
	}
	return results
}

func rankCandidateBacktests(candidates []candidateBacktest, topN int) []candidateBacktest {
	ranked := append([]candidateBacktest(nil), candidates...)
	for i := range ranked {
		if ranked[i].RankScore == 0 {
			ranked[i].RankScore = roundFloat(candidateRankScore(ranked[i].TotalReturn, ranked[i].MaxDrawdown, ranked[i].SharpeRatio, ranked[i].TotalTrades), 2)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].RankScore == ranked[j].RankScore {
			return ranked[i].StockCode < ranked[j].StockCode
		}
		return ranked[i].RankScore > ranked[j].RankScore
	})
	if topN > 0 && topN < len(ranked) {
		ranked = ranked[:topN]
	}
	for i := range ranked {
		ranked[i].Rank = i + 1
	}
	return ranked
}

func buildValidationSummary(candidates []candidateBacktest, attemptedCount ...int) validationSummary {
	candidateCount := len(candidates)
	if len(attemptedCount) > 0 && attemptedCount[0] > candidateCount {
		candidateCount = attemptedCount[0]
	}
	summary := validationSummary{
		CandidateCount: candidateCount,
		ValidatedCount: len(candidates),
		Method:         "因子初筛 -> 全量候选单股回测 -> 收益/回撤/夏普综合排序；展示所有验证样本，含亏损和未入选股票",
		Warnings:       []string{},
	}
	if len(candidates) == 0 {
		summary.Warnings = append(summary.Warnings, "候选股票未完成二次验证，建议检查行情数据或缩小股票池。")
		summary.GateReason = "没有可验证候选，禁止视为可实盘策略。"
		return summary
	}
	if summary.ValidatedCount < summary.CandidateCount {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("%d只候选股票未完成回测验证，未纳入推荐排序。", summary.CandidateCount-summary.ValidatedCount))
	}

	bestReturn := candidates[0].TotalReturn
	worstDrawdown := candidates[0].MaxDrawdown
	for _, candidate := range candidates {
		if candidate.TotalReturn > 0 {
			summary.PositiveCount++
		} else if candidate.TotalReturn < 0 {
			summary.NegativeCount++
		} else {
			summary.FlatCount++
		}
		if candidate.TotalReturn > bestReturn {
			bestReturn = candidate.TotalReturn
		}
		if candidate.MaxDrawdown > worstDrawdown {
			worstDrawdown = candidate.MaxDrawdown
		}
	}

	summary.PositiveRate = roundFloat(float64(summary.PositiveCount)/float64(len(candidates))*100, 2)
	summary.BestReturn = roundFloat(bestReturn, 2)
	summary.WorstDrawdown = roundFloat(worstDrawdown, 2)
	summary.Deployable = summary.PositiveCount > 0 && summary.BestReturn > 0
	if summary.PositiveCount == 0 {
		summary.Warnings = append(summary.Warnings, "当前候选策略验证全部为非正收益，系统不会把这视为优质组合，建议换策略或扩大候选池。")
		summary.GateReason = "全量候选验证没有正收益样本，仅适合研究复盘，不适合应用到实盘。"
	} else if summary.PositiveRate < 50 {
		summary.Warnings = append(summary.Warnings, "二次验证正收益比例低于50%，结果可能依赖少数股票，建议谨慎模拟观察。")
	}
	if summary.WorstDrawdown >= 20 {
		summary.Warnings = append(summary.Warnings, "候选股中存在单股最大回撤超过20%的样本，已在排序中惩罚，但仍需关注仓位和止损。")
	}
	if summary.BestReturn <= 0 {
		summary.Warnings = append(summary.Warnings, "最佳单股验证收益仍未转正，这组参数不适合直接实盘。")
		if summary.GateReason == "" {
			summary.GateReason = "最佳候选仍未盈利，仅适合研究复盘，不适合应用到实盘。"
		}
	}
	return summary
}

func candidateRankScore(totalReturn, maxDrawdown, sharpe float64, totalTrades int) float64 {
	tradeBonus := 0.0
	if totalTrades > 0 {
		tradeBonus = 3
	}
	return totalReturn - maxDrawdown*0.65 + sharpe*4 + tradeBonus
}

func candidateOutcome(totalReturn float64) string {
	switch {
	case totalReturn > 0:
		return "profit"
	case totalReturn < 0:
		return "loss"
	default:
		return "flat"
	}
}

func markSelectedBacktests(all []candidateBacktest, selected []candidateBacktest) []candidateBacktest {
	selectedCodes := make(map[string]bool, len(selected))
	for _, candidate := range selected {
		selectedCodes[candidate.StockCode] = true
	}
	result := append([]candidateBacktest(nil), all...)
	for i := range result {
		result[i].Selected = selectedCodes[result[i].StockCode]
	}
	return result
}

func reorderPicksByBacktest(picks []selector.StockPick, ranked []candidateBacktest) []selector.StockPick {
	byCode := make(map[string]selector.StockPick, len(picks))
	for _, pick := range picks {
		byCode[pick.StockCode] = pick
	}
	result := make([]selector.StockPick, 0, len(ranked))
	for _, bt := range ranked {
		pick, ok := byCode[bt.StockCode]
		if !ok {
			continue
		}
		pick.Rank = len(result) + 1
		pick.Score = roundFloat((pick.Score*0.35)+(bt.RankScore*0.65), 2)
		pick.Reasons = append([]string{
			fmt.Sprintf("策略单股验证回测：收益 %.2f%%，回撤 %.2f%%，夏普 %.2f，交易 %d 次", bt.TotalReturn, bt.MaxDrawdown, bt.SharpeRatio, bt.TotalTrades),
		}, pick.Reasons...)
		result = append(result, pick)
	}
	return result
}

func roundFloat(v float64, places int) float64 {
	multiplier := 1.0
	for i := 0; i < places; i++ {
		multiplier *= 10
	}
	if v >= 0 {
		return float64(int(v*multiplier+0.5)) / multiplier
	}
	return float64(int(v*multiplier-0.5)) / multiplier
}

func newStrategyFromConfig(stratConfig models.StrategyConfig, logger *zap.Logger) (strategy.Strategy, error) {
	switch stratConfig.Type {
	case "double_ma":
		return strategy.NewDoubleMAStrategy(stratConfig, logger), nil
	case "turtle":
		return strategy.NewTurtleStrategy(stratConfig, logger), nil
	case "momentum":
		return strategy.NewMomentumStrategy(stratConfig, logger), nil
	case "mean_reversion":
		return strategy.NewMeanReversionStrategy(stratConfig, logger), nil
	case "grid":
		return strategy.NewGridStrategy(stratConfig, logger), nil
	case "macd_t":
		return strategy.NewMACDTStrategy(stratConfig, logger), nil
	default:
		return nil, fmt.Errorf("不支持的策略类型: %s", stratConfig.Type)
	}
}

func parseSmartDateRange(start, end string, now time.Time) (time.Time, time.Time, error) {
	endDate := now
	if strings.TrimSpace(end) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", end, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("结束日期格式错误")
		}
		endDate = parsed
	}

	startDate := endDate.AddDate(0, -3, 0)
	if strings.TrimSpace(start) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", start, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("开始日期格式错误")
		}
		startDate = parsed
	}
	if endDate.Before(startDate) {
		return time.Time{}, time.Time{}, fmt.Errorf("结束日期不能早于开始日期")
	}
	return startDate, endDate, nil
}

func strategyDisplayName(strategyType string) string {
	switch strategyType {
	case "double_ma":
		return "双均线交叉策略"
	case "turtle":
		return "海龟交易策略"
	case "momentum":
		return "动量策略"
	case "mean_reversion":
		return "均值回归策略"
	case "grid":
		return "网格交易策略"
	case "macd_t":
		return "MACD短线做T策略"
	default:
		return strategyType
	}
}

func normalizeBacktestStocks(stocks []string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(stocks))
	for _, stock := range stocks {
		code := strings.TrimSpace(strings.ToLower(stock))
		code = strings.TrimPrefix(code, "sh")
		code = strings.TrimPrefix(code, "sz")

		var digits strings.Builder
		for _, r := range code {
			if unicode.IsDigit(r) {
				digits.WriteRune(r)
			}
		}
		code = digits.String()
		if len(code) != 6 {
			return nil, fmt.Errorf("无效股票代码: %s", stock)
		}
		if !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("至少需要一个股票代码")
	}
	return result, nil
}

func estimateBacktestKLineCount(startDate, endDate time.Time) int {
	days := int(endDate.Sub(startDate).Hours()/24) + 1
	if days < 120 {
		return 120
	}
	return days*7/5 + 80
}

// ==================== 风控接口 ====================

func (s *Server) getRiskConfig(c *gin.Context) {
	s.responseSuccess(c, s.riskManager.GetConfig())
}

func (s *Server) updateRiskConfig(c *gin.Context) {
	var cfg models.RiskConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		s.responseError(c, http.StatusBadRequest, err.Error())
		return
	}
	s.riskManager.UpdateConfig(cfg)
	s.responseSuccess(c, gin.H{"message": "风控配置更新成功"})
}

func (s *Server) getRiskEvents(c *gin.Context) {
	events := s.riskManager.GetEvents(100)
	s.responseSuccess(c, events)
}

// ==================== 系统状态 ====================

func (s *Server) getSystemStatus(c *gin.Context) {
	brokerStatus := s.brokerMgr.Status()
	s.responseSuccess(c, gin.H{
		"status":        "running",
		"broker":        brokerStatus.LoggedIn,
		"broker_status": brokerStatus,
		"strategies":    len(s.engine.ListStrategies()),
		"timestamp":     time.Now().Format(time.RFC3339),
	})
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": "1.0.0",
	})
}

// ==================== 工具方法 ====================

func (s *Server) responseSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    data,
	})
}

func (s *Server) responseError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{
		"code":    status,
		"message": msg,
		"data":    nil,
	})
}

func (s *Server) currentBroker(c *gin.Context) (broker.Broker, bool) {
	b := s.brokerMgr.Current()
	if b == nil {
		s.responseError(c, http.StatusServiceUnavailable, "未选择券商")
		return nil, false
	}
	return b, true
}

// generateMockKLines 生成模拟K线数据（用于回测演示）
func generateMockKLines(stockCode string, startDate, endDate time.Time) []models.KLine {
	var klines []models.KLine

	// 简单的价格模型：随机游走
	price := decimal.NewFromFloat(10.0) // 起始价格10元
	date := startDate

	for date.Before(endDate) {
		// 跳过周末
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			date = date.AddDate(0, 0, 1)
			continue
		}

		// 生成随机价格变动
		change := decimal.NewFromFloat(0.02) // 2%波动
		open := price
		high := open.Add(change)
		low := open.Sub(change)
		close := open.Add(decimal.NewFromFloat(0.005)) // 微涨趋势
		volume := int64(1000000 + date.Unix()%500000)

		kline := models.KLine{
			StockCode: stockCode,
			Period:    "day",
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			Amount:    close.Mul(decimal.NewFromInt(volume)),
			Timestamp: date,
		}

		klines = append(klines, kline)
		price = close
		date = date.AddDate(0, 0, 1)
	}

	return klines
}
