package main

import (
	"aqsystem/api"
	"aqsystem/broker"
	"aqsystem/config"
	"aqsystem/market"
	"aqsystem/models"
	"aqsystem/risk"
	"aqsystem/strategy"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	cfgPath := resolveConfigPath()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		fmt.Println("使用默认配置继续...")
		cfg = getDefaultConfig()
	}

	// 2. 初始化日志
	if err := config.InitLogger(cfg); err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	logger := config.GetLogger()

	logger.Info("========================================")
	logger.Info("    清北协同学的A股量化交易系统 启动中...")
	logger.Info("========================================")

	// 3. 创建券商运行时管理器
	brokerMgr, err := broker.NewBrokerManager(cfg.Broker, logger)
	if err != nil {
		logger.Fatal("创建券商管理器失败", zap.Error(err))
	}

	// 4. 创建行情服务
	marketSvc := market.NewMarketService(cfg.Market.DataSource, logger)

	// 5. 创建风控管理器
	riskMgr := risk.NewRiskManager(cfg.Risk, logger)

	// 6. 创建策略引擎
	strategyEngine := strategy.NewEngine(logger)

	// 7. 注册内置策略
	registerBuiltInStrategies(strategyEngine, logger)

	// 8. 创建API服务器
	server := api.NewServer(strategyEngine, marketSvc, brokerMgr, riskMgr, logger)

	// 9. 启动系统
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动行情轮询
	go marketSvc.StartPolling(ctx, time.Duration(cfg.Market.RefreshInterval)*time.Second)

	// 启动信号处理循环
	go processSignals(ctx, strategyEngine, brokerMgr, riskMgr, logger)

	// 启动行情到策略的实时分发
	go processMarketQuotes(ctx, marketSvc, strategyEngine, logger)

	// 启动风控监控
	go riskMonitor(ctx, brokerMgr, riskMgr, logger)

	// 10. 自动登录模拟券商；真实券商需要在Web端显式登录确认。
	if cfg.Broker.Type == "simulated" || cfg.Broker.Type == "" {
		if err := brokerMgr.Login(ctx, models.BrokerConfig{}); err != nil {
			logger.Error("模拟券商登录失败", zap.Error(err))
		} else {
			logger.Info("模拟券商登录成功")
		}
	} else {
		logger.Info("实盘券商已配置，等待Web端运行时登录确认", zap.String("broker", cfg.Broker.Type))
	}

	// 11. 启动HTTP服务
	router := server.SetupRouter()
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	logger.Info("========================================")
	logger.Info("系统启动完成！")
	logger.Info(fmt.Sprintf("API服务地址: http://%s", addr))
	logger.Info(fmt.Sprintf("行情数据源: %s", cfg.Market.DataSource))
	logger.Info(fmt.Sprintf("券商模式: %s", cfg.Broker.Type))
	logger.Info("========================================")

	// 优雅退出
	go func() {
		if err := router.Run(addr); err != nil {
			logger.Fatal("HTTP服务启动失败", zap.Error(err))
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("收到退出信号", zap.String("signal", sig.String()))
	logger.Info("正在关闭系统...")

	// 清理
	cancel()
	strategyEngine.Stop()
	marketSvc.Stop()
	brokerMgr.Logout(context.Background())

	logger.Info("系统已安全关闭")
}

func resolveConfigPath() string {
	if path := os.Getenv("AQ_CONFIG"); path != "" {
		return path
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if _, err := os.Stat("backend/config.yaml"); err == nil {
		return "backend/config.yaml"
	}
	return "config.yaml"
}

// registerBuiltInStrategies 注册内置策略
func registerBuiltInStrategies(engine *strategy.Engine, logger *zap.Logger) {
	// 双均线策略 - 默认配置
	doubleMACfg := models.StrategyConfig{
		ID:          "builtin_double_ma",
		Name:        "双均线交叉策略",
		Type:        "double_ma",
		Description: "短期均线上穿长期均线形成金叉买入，下穿形成死叉卖出。最经典的技术分析策略，适合趋势明显的市场。",
		Stocks:      []string{}, // 用户订阅后自动填充
		Params: map[string]interface{}{
			"short_period": 5,
			"long_period":  20,
			"ma_type":      "SMA",
		},
		Status:      models.StrategyStatusPaused,
		MaxPosition: decimal.NewFromFloat(100000),
		StopLoss:    decimal.NewFromFloat(0.08),
		TakeProfit:  decimal.NewFromFloat(0.2),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	doubleMA := strategy.NewDoubleMAStrategy(doubleMACfg, logger)
	engine.RegisterStrategy(doubleMA)

	// 海龟交易策略
	turtleCfg := models.StrategyConfig{
		ID:          "builtin_turtle",
		Name:        "海龟交易策略",
		Type:        "turtle",
		Description: "基于唐奇安通道突破的趋势跟踪策略，使用ATR进行仓位管理和止损。80年代最著名的交易系统。",
		Stocks:      []string{},
		Params: map[string]interface{}{
			"entry_period": 20,
			"exit_period":  10,
			"atr_period":   20,
			"risk_pct":     0.01,
		},
		Status:      models.StrategyStatusPaused,
		MaxPosition: decimal.NewFromFloat(100000),
		StopLoss:    decimal.NewFromFloat(0.1),
		TakeProfit:  decimal.NewFromFloat(0.3),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	turtle := strategy.NewTurtleStrategy(turtleCfg, logger)
	engine.RegisterStrategy(turtle)

	// 动量策略
	momentumCfg := models.StrategyConfig{
		ID:          "builtin_momentum",
		Name:        "动量策略",
		Type:        "momentum",
		Description: "选择过去N日涨幅最大的股票买入（强者恒强），当动量减弱时卖出。学术研究表明3-12个月动量效应在A股显著。",
		Stocks:      []string{},
		Params: map[string]interface{}{
			"lookback_period":    20,
			"holding_period":     10,
			"top_n":              3,
			"momentum_threshold": 0.05,
		},
		Status:      models.StrategyStatusPaused,
		MaxPosition: decimal.NewFromFloat(100000),
		StopLoss:    decimal.NewFromFloat(0.08),
		TakeProfit:  decimal.NewFromFloat(0.15),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	momentum := strategy.NewMomentumStrategy(momentumCfg, logger)
	engine.RegisterStrategy(momentum)

	// 均值回归策略
	mrCfg := models.StrategyConfig{
		ID:          "builtin_mean_reversion",
		Name:        "均值回归策略",
		Type:        "mean_reversion",
		Description: "价格偏离均值过大时预期会回归，低于均值-2倍标准差时买入，回归后卖出。最经典的统计套利策略。",
		Stocks:      []string{},
		Params: map[string]interface{}{
			"lookback_period": 20,
			"entry_zscore":    2.0,
			"exit_zscore":     0.5,
		},
		Status:      models.StrategyStatusPaused,
		MaxPosition: decimal.NewFromFloat(100000),
		StopLoss:    decimal.NewFromFloat(0.1),
		TakeProfit:  decimal.NewFromFloat(0.1),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	mr := strategy.NewMeanReversionStrategy(mrCfg, logger)
	engine.RegisterStrategy(mr)

	// 网格交易策略
	gridCfg := models.StrategyConfig{
		ID:          "builtin_grid",
		Name:        "网格交易策略",
		Type:        "grid",
		Description: "在价格区间内设置网格，每下降一格买入一定数量，每上涨一格卖出。适合震荡行情，A股最常用的自动化策略。",
		Stocks:      []string{},
		Params: map[string]interface{}{
			"upper_price": 0,
			"lower_price": 0,
			"grid_count":  10,
			"grid_volume": 100,
		},
		Status:      models.StrategyStatusPaused,
		MaxPosition: decimal.NewFromFloat(100000),
		StopLoss:    decimal.NewFromFloat(0.15),
		TakeProfit:  decimal.NewFromFloat(0.3),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	grid := strategy.NewGridStrategy(gridCfg, logger)
	engine.RegisterStrategy(grid)

	logger.Info("内置策略注册完成", zap.Int("count", 5))
}

// processSignals 处理交易信号
func processSignals(ctx context.Context, engine *strategy.Engine, brokerMgr *broker.BrokerManager, riskMgr *risk.RiskManager, logger *zap.Logger) {
	logger.Info("信号处理循环已启动")

	quoteChan := engine.SignalChannel()
	// 同时监听行情通道
	// marketQuoteChan := marketSvc.QuoteChannel()

	for {
		select {
		case <-ctx.Done():
			logger.Info("信号处理循环已停止")
			return

		case signal := <-quoteChan:
			b := brokerMgr.Current()
			if b == nil || !b.IsLoggedIn() {
				logger.Warn("策略信号未执行：当前未登录券商",
					zap.String("stock", signal.StockCode),
					zap.String("side", string(signal.Side)),
				)
				continue
			}

			// 将信号转换为订单
			order := &models.Order{
				StockCode:  signal.StockCode,
				StockName:  signal.StockName,
				Side:       signal.Side,
				Type:       signal.Type,
				Price:      signal.Price,
				Volume:     signal.Volume,
				StrategyID: signal.StrategyID,
			}

			// 风控检查
			account, err := b.GetAccount(ctx)
			if err != nil {
				logger.Error("获取账户信息失败", zap.Error(err))
				continue
			}

			if err := riskMgr.CheckOrder(ctx, order, account); err != nil {
				logger.Warn("订单被风控拒绝",
					zap.String("stock", signal.StockCode),
					zap.String("side", string(signal.Side)),
					zap.String("reason", err.Error()),
				)
				continue
			}

			// 提交订单
			result, err := b.SubmitOrder(ctx, order)
			if err != nil {
				logger.Error("下单失败",
					zap.String("stock", signal.StockCode),
					zap.Error(err),
				)
				continue
			}

			riskMgr.RecordTrade(result)
			logger.Info("订单已提交",
				zap.String("orderID", result.ID),
				zap.String("stock", signal.StockCode),
				zap.String("side", string(signal.Side)),
				zap.String("price", signal.Price.String()),
				zap.Int64("volume", signal.Volume),
				zap.String("reason", signal.Reason),
			)
		}
	}
}

// processMarketQuotes 将行情轮询结果分发给运行中的策略。
func processMarketQuotes(ctx context.Context, marketSvc *market.MarketService, engine *strategy.Engine, logger *zap.Logger) {
	logger.Info("实时行情策略分发已启动")
	quoteChan := marketSvc.QuoteChannel()

	for {
		select {
		case <-ctx.Done():
			logger.Info("实时行情策略分发已停止")
			return
		case quote := <-quoteChan:
			signals := engine.ProcessQuote(ctx, quote)
			for _, sig := range signals {
				engine.PushSignal(sig)
			}
		}
	}
}

// riskMonitor 风控监控
func riskMonitor(ctx context.Context, brokerMgr *broker.BrokerManager, riskMgr *risk.RiskManager, logger *zap.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b := brokerMgr.Current()
			if b == nil || !b.IsLoggedIn() {
				continue
			}

			// 获取账户信息
			account, err := b.GetAccount(ctx)
			if err != nil {
				continue
			}

			// 更新权益
			riskMgr.UpdateEquity(account.TotalAssets)

			// 检查止损止盈
			stopLossSignals := riskMgr.CheckStopLoss(account.Positions)
			takeProfitSignals := riskMgr.CheckTakeProfit(account.Positions)

			// 执行止损止盈
			for _, sig := range stopLossSignals {
				order := &models.Order{
					StockCode:  sig.StockCode,
					StockName:  sig.StockName,
					Side:       sig.Side,
					Type:       sig.Type,
					Price:      sig.Price,
					Volume:     sig.Volume,
					StrategyID: "risk_manager",
				}
				if _, err := b.SubmitOrder(ctx, order); err != nil {
					logger.Error("执行止损失败", zap.String("stock", sig.StockCode), zap.Error(err))
				} else {
					logger.Warn("已执行止损", zap.String("stock", sig.StockCode), zap.String("reason", sig.Reason))
				}
			}

			for _, sig := range takeProfitSignals {
				order := &models.Order{
					StockCode:  sig.StockCode,
					StockName:  sig.StockName,
					Side:       sig.Side,
					Type:       sig.Type,
					Price:      sig.Price,
					Volume:     sig.Volume,
					StrategyID: "risk_manager",
				}
				if _, err := b.SubmitOrder(ctx, order); err != nil {
					logger.Error("执行止盈失败", zap.String("stock", sig.StockCode), zap.Error(err))
				} else {
					logger.Info("已执行止盈", zap.String("stock", sig.StockCode), zap.String("reason", sig.Reason))
				}
			}

			// 每日重置（简化处理：每天9点重置）
			now := time.Now()
			if now.Hour() == 9 && now.Minute() < 1 {
				riskMgr.ResetDaily()
			}
		}
	}
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() *config.SystemConfig {
	return &config.SystemConfig{
		Server: config.ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "debug",
		},
		Database: config.DatabaseConfig{
			Type: "sqlite",
			Name: "data/aqsystem.db",
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "console",
		},
		Broker: models.BrokerConfig{
			ID:     "sim_broker_01",
			Name:   "模拟券商",
			Type:   "simulated",
			IsDemo: true,
		},
		Market: config.MarketConfig{
			DataSource:      "sina",
			RefreshInterval: 3,
		},
		Risk: models.RiskConfig{
			MaxSinglePositionPct: decimal.NewFromFloat(0.3),
			MaxTotalPositionPct:  decimal.NewFromFloat(0.8),
			MaxDailyLossPct:      decimal.NewFromFloat(0.05),
			MaxDrawdownPct:       decimal.NewFromFloat(0.15),
			MaxDailyTrades:       50,
			StopLossPct:          decimal.NewFromFloat(0.08),
			TakeProfitPct:        decimal.NewFromFloat(0.2),
		},
		Strategy: config.StrategyGlobalConfig{
			MaxConcurrent:       10,
			BacktestInitCapital: 1000000,
			CommissionRate:      0.0003,
			StampTaxRate:        0.001,
			Slippage:            0.001,
		},
	}
}
