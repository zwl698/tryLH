package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// ==================== 基础类型 ====================

// MarketType 市场类型
type MarketType string

const (
	MarketSH MarketType = "SH" // 上海
	MarketSZ MarketType = "SZ" // 深圳
)

// OrderSide 买卖方向
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

// OrderType 订单类型
type OrderType string

const (
	OrderTypeLimit  OrderType = "LIMIT"  // 限价单
	OrderTypeMarket OrderType = "MARKET" // 市价单
)

// OrderStatus 订单状态
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"   // 待提交
	OrderStatusSubmitted OrderStatus = "SUBMITTED" // 已提交
	OrderStatusPartial   OrderStatus = "PARTIAL"   // 部分成交
	OrderStatusFilled    OrderStatus = "FILLED"    // 完全成交
	OrderStatusCancelled OrderStatus = "CANCELLED" // 已撤销
	OrderStatusRejected  OrderStatus = "REJECTED"  // 已拒绝
)

// PositionSide 持仓方向
type PositionSide string

const (
	PositionSideLong  PositionSide = "LONG"
	PositionSideShort PositionSide = "SHORT"
)

// ==================== 账户相关 ====================

// Account 账户信息
type Account struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	BrokerID    string          `json:"broker_id"`
	BrokerName  string          `json:"broker_name"`
	Cash        decimal.Decimal `json:"cash"`         // 可用资金
	TotalAssets decimal.Decimal `json:"total_assets"` // 总资产
	MarketValue decimal.Decimal `json:"market_value"` // 持仓市值
	FrozenCash  decimal.Decimal `json:"frozen_cash"`  // 冻结资金
	YesterdayPL decimal.Decimal `json:"yesterday_pl"` // 昨日盈亏
	TodayPL     decimal.Decimal `json:"today_pl"`     // 今日盈亏
	TotalPL     decimal.Decimal `json:"total_pl"`     // 累计盈亏
	Positions   []Position      `json:"positions"`
	Orders      []Order         `json:"orders"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ==================== 持仓相关 ====================

// Position 持仓信息
type Position struct {
	StockCode    string          `json:"stock_code"`
	StockName    string          `json:"stock_name"`
	Market       MarketType      `json:"market"`
	Side         PositionSide    `json:"side"`
	Volume       int64           `json:"volume"`        // 持仓数量
	AvailableVol int64           `json:"available_vol"` // 可用数量
	AvgCost      decimal.Decimal `json:"avg_cost"`      // 持仓成本
	CurrentPrice decimal.Decimal `json:"current_price"` // 当前价格
	MarketValue  decimal.Decimal `json:"market_value"`  // 持仓市值
	ProfitLoss   decimal.Decimal `json:"profit_loss"`   // 浮动盈亏
	ProfitPct    decimal.Decimal `json:"profit_pct"`    // 盈亏比例
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ==================== 订单相关 ====================

// Order 订单信息
type Order struct {
	ID          string          `json:"id"`
	StockCode   string          `json:"stock_code"`
	StockName   string          `json:"stock_name"`
	Market      MarketType      `json:"market"`
	Side        OrderSide       `json:"side"`
	Type        OrderType       `json:"type"`
	Price       decimal.Decimal `json:"price"`
	Volume      int64           `json:"volume"`
	FilledVol   int64           `json:"filled_vol"`
	FilledPrice decimal.Decimal `json:"filled_price"`
	Status      OrderStatus     `json:"status"`
	StrategyID  string          `json:"strategy_id"`
	Reason      string          `json:"reason"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ==================== 行情相关 ====================

// StockQuote 实时行情
type StockQuote struct {
	StockCode  string             `json:"stock_code"`
	StockName  string             `json:"stock_name"`
	Market     MarketType         `json:"market"`
	Open       decimal.Decimal    `json:"open"`
	High       decimal.Decimal    `json:"high"`
	Low        decimal.Decimal    `json:"low"`
	Close      decimal.Decimal    `json:"close"`       // 最新价
	PreClose   decimal.Decimal    `json:"pre_close"`   // 昨收价
	Volume     int64              `json:"volume"`      // 成交量
	Amount     decimal.Decimal    `json:"amount"`      // 成交额
	Turnover   decimal.Decimal    `json:"turnover"`    // 换手率
	PE         decimal.Decimal    `json:"pe"`          // 市盈率
	PB         decimal.Decimal    `json:"pb"`          // 市净率
	TotalMV    decimal.Decimal    `json:"total_mv"`    // 总市值
	CircMV     decimal.Decimal    `json:"circ_mv"`     // 流通市值
	BidPrices  [5]decimal.Decimal `json:"bid_prices"`  // 买1-5价
	BidVolumes [5]int64           `json:"bid_volumes"` // 买1-5量
	AskPrices  [5]decimal.Decimal `json:"ask_prices"`  // 卖1-5价
	AskVolumes [5]int64           `json:"ask_volumes"` // 卖1-5量
	Timestamp  time.Time          `json:"timestamp"`
}

// KLine K线数据
type KLine struct {
	StockCode string          `json:"stock_code"`
	Market    MarketType      `json:"market"`
	Period    string          `json:"period"` // 1m,5m,15m,30m,60m,day,week,month
	Open      decimal.Decimal `json:"open"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Close     decimal.Decimal `json:"close"`
	Volume    int64           `json:"volume"`
	Amount    decimal.Decimal `json:"amount"`
	Timestamp time.Time       `json:"timestamp"`
}

// ==================== 策略相关 ====================

// StrategyStatus 策略状态
type StrategyStatus string

const (
	StrategyStatusActive  StrategyStatus = "ACTIVE"
	StrategyStatusPaused  StrategyStatus = "PAUSED"
	StrategyStatusStopped StrategyStatus = "STOPPED"
	StrategyStatusError   StrategyStatus = "ERROR"
)

// StrategyConfig 策略配置
type StrategyConfig struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // 策略类型标识
	Description string                 `json:"description"`
	Stocks      []string               `json:"stocks"` // 策略关注的股票
	Params      map[string]interface{} `json:"params"` // 策略参数
	Status      StrategyStatus         `json:"status"`
	MaxPosition decimal.Decimal        `json:"max_position"` // 单股最大持仓金额
	StopLoss    decimal.Decimal        `json:"stop_loss"`    // 止损比例
	TakeProfit  decimal.Decimal        `json:"take_profit"`  // 止盈比例
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// Signal 交易信号
type Signal struct {
	StrategyID string          `json:"strategy_id"`
	StockCode  string          `json:"stock_code"`
	StockName  string          `json:"stock_name"`
	Side       OrderSide       `json:"side"`
	Type       OrderType       `json:"type"`
	Price      decimal.Decimal `json:"price"`
	Volume     int64           `json:"volume"`
	Reason     string          `json:"reason"`
	Timestamp  time.Time       `json:"timestamp"`
}

// ==================== 回测相关 ====================

// BacktestResult 回测结果
type BacktestResult struct {
	StrategyID     string          `json:"strategy_id"`
	StrategyName   string          `json:"strategy_name"`
	StartDate      time.Time       `json:"start_date"`
	EndDate        time.Time       `json:"end_date"`
	InitialCapital decimal.Decimal `json:"initial_capital"`
	FinalCapital   decimal.Decimal `json:"final_capital"`
	TotalReturn    decimal.Decimal `json:"total_return"`
	AnnualReturn   decimal.Decimal `json:"annual_return"`
	MaxDrawdown    decimal.Decimal `json:"max_drawdown"`
	SharpeRatio    decimal.Decimal `json:"sharpe_ratio"`
	WinRate        decimal.Decimal `json:"win_rate"`
	ProfitFactor   decimal.Decimal `json:"profit_factor"`
	TotalTrades    int             `json:"total_trades"`
	WinTrades      int             `json:"win_trades"`
	LossTrades     int             `json:"loss_trades"`
	Trades         []BacktestTrade `json:"trades"`
	DailyEquity    []EquityPoint   `json:"daily_equity"`
}

// BacktestTrade 回测交易记录
type BacktestTrade struct {
	StockCode  string          `json:"stock_code"`
	Side       OrderSide       `json:"side"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	ExitPrice  decimal.Decimal `json:"exit_price"`
	Volume     int64           `json:"volume"`
	Profit     decimal.Decimal `json:"profit"`
	EntryTime  time.Time       `json:"entry_time"`
	ExitTime   time.Time       `json:"exit_time"`
}

// EquityPoint 权益曲线点
type EquityPoint struct {
	Date   time.Time       `json:"date"`
	Equity decimal.Decimal `json:"equity"`
}

// ==================== 风控相关 ====================

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "LOW"
	RiskLevelMedium   RiskLevel = "MEDIUM"
	RiskLevelHigh     RiskLevel = "HIGH"
	RiskLevelCritical RiskLevel = "CRITICAL"
)

// RiskEvent 风控事件
type RiskEvent struct {
	ID         string          `json:"id"`
	Level      RiskLevel       `json:"level"`
	Type       string          `json:"type"`
	Message    string          `json:"message"`
	StockCode  string          `json:"stock_code"`
	StrategyID string          `json:"strategy_id"`
	Value      decimal.Decimal `json:"value"`
	Threshold  decimal.Decimal `json:"threshold"`
	Timestamp  time.Time       `json:"timestamp"`
}

// RiskConfig 风控配置
type RiskConfig struct {
	MaxSinglePositionPct decimal.Decimal `json:"max_single_position_pct"` // 单股最大仓位比例
	MaxTotalPositionPct  decimal.Decimal `json:"max_total_position_pct"`  // 总仓位上限
	MaxDailyLossPct      decimal.Decimal `json:"max_daily_loss_pct"`      // 日最大亏损比例
	MaxDrawdownPct       decimal.Decimal `json:"max_drawdown_pct"`        // 最大回撤比例
	MaxDailyTrades       int             `json:"max_daily_trades"`        // 日最大交易次数
	StopLossPct          decimal.Decimal `json:"stop_loss_pct"`           // 默认止损比例
	TakeProfitPct        decimal.Decimal `json:"take_profit_pct"`         // 默认止盈比例
	BlacklistStocks      []string        `json:"blacklist_stocks"`        // 黑名单股票
	AllowMarginTrade     bool            `json:"allow_margin_trade"`      // 是否允许融资融券
	AllowT0Trade         bool            `json:"allow_t0_trade"`          // 是否允许T+0（仅供回测）
}

// ==================== 券商配置 ====================

// BrokerConfig 券商配置
type BrokerConfig struct {
	ID        string            `json:"id" yaml:"id"`
	Name      string            `json:"name" yaml:"name"`
	Type      string            `json:"type" yaml:"type"` // simulated, xtquant, csc, cj, etc.
	APIURL    string            `json:"api_url" yaml:"api_url"`
	AccountID string            `json:"account_id" yaml:"account_id"`
	Password  string            `json:"password" yaml:"password"`
	CaPath    string            `json:"ca_path" yaml:"ca_path"`
	CertPath  string            `json:"cert_path" yaml:"cert_path"`
	IsDemo    bool              `json:"is_demo" yaml:"is_demo"`
	CommType  string            `json:"comm_type" yaml:"comm_type"`   // 通信方式: http, tcp, dll
	AppKey    string            `json:"app_key" yaml:"app_key"`       // 券商应用Key
	AppSecret string            `json:"app_secret" yaml:"app_secret"` // 券商应用Secret
	ExtConfig map[string]string `json:"ext_config" yaml:"ext_config"` // 扩展配置
}
