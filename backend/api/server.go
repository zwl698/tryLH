package api

import (
	"aqsystem/backtest"
	"aqsystem/broker"
	"aqsystem/config"
	"aqsystem/market"
	"aqsystem/models"
	"aqsystem/risk"
	"aqsystem/strategy"
	"fmt"
	"net/http"
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
	broker      broker.Broker
	riskManager *risk.RiskManager
	btEngine    *backtest.BacktestEngine
	logger      *zap.Logger
}

// NewServer 创建API服务器
func NewServer(
	engine *strategy.Engine,
	marketSvc *market.MarketService,
	b broker.Broker,
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
		broker:      b,
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

func (s *Server) brokerLogin(c *gin.Context) {
	if err := s.broker.Login(c.Request.Context()); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "登录成功"})
}

func (s *Server) brokerLogout(c *gin.Context) {
	if err := s.broker.Logout(c.Request.Context()); err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, gin.H{"message": "登出成功"})
}

func (s *Server) getAccount(c *gin.Context) {
	account, err := s.broker.GetAccount(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, account)
}

func (s *Server) getPositions(c *gin.Context) {
	positions, err := s.broker.GetPositions(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, positions)
}

func (s *Server) getOrders(c *gin.Context) {
	orders, err := s.broker.GetOrders(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}
	s.responseSuccess(c, orders)
}

// ==================== 交易接口 ====================

func (s *Server) submitOrder(c *gin.Context) {
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
	account, err := s.broker.GetAccount(c.Request.Context())
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.riskManager.CheckOrder(c.Request.Context(), order, account); err != nil {
		s.responseError(c, http.StatusForbidden, "风控拒绝: "+err.Error())
		return
	}

	result, err := s.broker.SubmitOrder(c.Request.Context(), order)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, err.Error())
		return
	}

	s.riskManager.RecordTrade(result)
	s.responseSuccess(c, result)
}

func (s *Server) cancelOrder(c *gin.Context) {
	orderID := c.Param("id")
	if err := s.broker.CancelOrder(c.Request.Context(), orderID); err != nil {
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
	}
	s.responseSuccess(c, templates)
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

	initCapital := decimal.NewFromFloat(1000000)
	if req.InitCapital > 0 {
		initCapital = decimal.NewFromFloat(req.InitCapital)
	}

	// 创建策略
	stratConfig := models.StrategyConfig{
		ID:          "backtest_" + req.StrategyType,
		Name:        req.StrategyType,
		Type:        req.StrategyType,
		Stocks:      stockCodes,
		Params:      req.Params,
		Status:      models.StrategyStatusPaused,
		MaxPosition: initCapital.Div(decimal.NewFromInt(int64(len(stockCodes)))),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	var strat strategy.Strategy
	switch req.StrategyType {
	case "double_ma":
		strat = strategy.NewDoubleMAStrategy(stratConfig, s.logger)
	case "turtle":
		strat = strategy.NewTurtleStrategy(stratConfig, s.logger)
	case "momentum":
		strat = strategy.NewMomentumStrategy(stratConfig, s.logger)
	case "mean_reversion":
		strat = strategy.NewMeanReversionStrategy(stratConfig, s.logger)
	case "grid":
		strat = strategy.NewGridStrategy(stratConfig, s.logger)
	default:
		s.responseError(c, http.StatusBadRequest, "不支持的策略类型: "+req.StrategyType)
		return
	}

	// 获取K线数据
	klines := make(map[string][]models.KLine)
	klineCount := estimateBacktestKLineCount(startDate, endDate)
	for _, code := range stockCodes {
		klineData, err := s.marketSvc.GetKLines(c.Request.Context(), code, "day", klineCount)
		if err != nil {
			s.responseError(c, http.StatusBadGateway, "获取真实K线数据失败: "+code+" "+err.Error())
			return
		}
		if len(klineData) == 0 {
			s.responseError(c, http.StatusBadGateway, "获取真实K线数据失败: "+code+" K线数据为空")
			return
		}
		klines[code] = klineData
	}

	// 执行回测
	result, err := s.btEngine.Run(c.Request.Context(), strat, klines, startDate, endDate, initCapital)
	if err != nil {
		s.responseError(c, http.StatusInternalServerError, "回测失败: "+err.Error())
		return
	}

	s.responseSuccess(c, result)
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
	s.responseSuccess(c, gin.H{
		"status":     "running",
		"broker":     s.broker.IsLoggedIn(),
		"strategies": len(s.engine.ListStrategies()),
		"timestamp":  time.Now().Format(time.RFC3339),
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
