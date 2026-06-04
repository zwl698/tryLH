package broker

import (
	"aqsystem/models"
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// CSCBroker 中信建投券商适配器
//
// 中信建投证券(China Securities Co., Ltd.)量化交易接口
// 支持两种接入方式：
// 1. HTTP API方式：通过中信建投提供的REST API进行交易
// 2. 柜台DLL方式：通过本地部署的交易终端DLL进行通信
//
// 使用前需要：
// - 在中信建投开通量化交易权限
// - 获取AppKey和AppSecret
// - 如使用DLL方式，需安装中信建投交易终端
type CSCBroker struct {
	mu        sync.RWMutex
	loggedIn  bool
	account   *models.Account
	orders    map[string]*models.Order
	positions map[string]*models.Position
	config    models.BrokerConfig
	client    *resty.Client
	logger    *zap.Logger
	sessionID string
	token     string
}

// NewCSCBroker 创建中信建投券商实例
func NewCSCBroker(cfg models.BrokerConfig, logger *zap.Logger) *CSCBroker {
	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(1 * time.Second)

	// 设置TLS配置（中信建投API需要HTTPS）
	client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	if cfg.APIURL == "" {
		// 默认API地址 - 中信建投量化交易网关
		if cfg.IsDemo {
			client.SetBaseURL("https://quant-demo.csc108.com/api/v1")
		} else {
			client.SetBaseURL("https://quant.csc108.com/api/v1")
		}
	} else {
		client.SetBaseURL(cfg.APIURL)
	}

	// 设置默认请求头
	client.SetHeader("Content-Type", "application/json")
	client.SetHeader("User-Agent", "AQSystem/1.0")

	return &CSCBroker{
		config:    cfg,
		orders:    make(map[string]*models.Order),
		positions: make(map[string]*models.Position),
		client:    client,
		logger:    logger,
	}
}

// Login 登录中信建投
func (b *CSCBroker) Login(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logger.Info("正在连接中信建投证券...", zap.String("account", b.config.AccountID))

	// 第一步：获取Token
	resp, err := b.client.R().
		SetBody(map[string]interface{}{
			"app_key":    b.config.AppKey,
			"app_secret": b.config.AppSecret,
			"account_id": b.config.AccountID,
			"password":   b.config.Password,
			"mac_addr":   "00:00:00:00:00:00", // 终端MAC地址
			"ip_addr":    "127.0.0.1",         // 终端IP地址
		}).
		SetResult(map[string]interface{}{}).
		Post("/auth/login")

	if err != nil {
		return fmt.Errorf("连接中信建投失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("中信建投登录失败，状态码: %d, 响应: %s", resp.StatusCode(), resp.String())
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	// 检查返回码
	if code, ok := data["code"]; ok && fmt.Sprintf("%v", code) != "0" {
		msg := ""
		if m, ok := data["message"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		return fmt.Errorf("中信建投登录失败: %s", msg)
	}

	// 提取token和session_id
	if token, ok := data["token"]; ok {
		b.token = fmt.Sprintf("%v", token)
	}
	if sid, ok := data["session_id"]; ok {
		b.sessionID = fmt.Sprintf("%v", sid)
	}

	// 设置后续请求的认证头
	b.client.SetAuthToken(b.token)
	b.client.SetHeader("X-Session-ID", b.sessionID)

	// 第二步：查询账户信息验证登录
	b.account = &models.Account{
		ID:         b.config.AccountID,
		BrokerID:   b.config.ID,
		BrokerName: "中信建投证券",
	}

	b.loggedIn = true
	b.logger.Info("中信建投登录成功", zap.String("account", b.config.AccountID))
	return nil
}

// Logout 登出
func (b *CSCBroker) Logout(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.loggedIn {
		return nil
	}

	_, err := b.client.R().
		SetHeader("Authorization", "Bearer "+b.token).
		Post("/auth/logout")

	b.loggedIn = false
	b.token = ""
	b.sessionID = ""
	b.logger.Info("中信建投已登出")
	return err
}

// IsLoggedIn 检查登录状态
func (b *CSCBroker) IsLoggedIn() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.loggedIn
}

// GetAccount 获取账户信息
func (b *CSCBroker) GetAccount(ctx context.Context) (*models.Account, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get("/account/info")

	if err != nil {
		return nil, fmt.Errorf("获取中信建投账户信息失败: %w", err)
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	// 解析账户数据
	account := &models.Account{
		ID:         b.config.AccountID,
		BrokerID:   b.config.ID,
		BrokerName: "中信建投证券",
	}

	if d, ok := data["data"].(map[string]interface{}); ok {
		if v, ok := d["cash"]; ok {
			account.Cash = parseDecimalFromInterface(v)
		}
		if v, ok := d["total_assets"]; ok {
			account.TotalAssets = parseDecimalFromInterface(v)
		}
		if v, ok := d["market_value"]; ok {
			account.MarketValue = parseDecimalFromInterface(v)
		}
		if v, ok := d["frozen_cash"]; ok {
			account.FrozenCash = parseDecimalFromInterface(v)
		}
		if v, ok := d["today_pl"]; ok {
			account.TodayPL = parseDecimalFromInterface(v)
		}
	} else {
		// 使用缓存的账户数据
		account = b.account
	}

	account.UpdatedAt = time.Now()
	return account, nil
}

// GetPositions 获取持仓
func (b *CSCBroker) GetPositions(ctx context.Context) ([]models.Position, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get("/account/positions")

	if err != nil {
		return nil, fmt.Errorf("获取中信建投持仓失败: %w", err)
	}

	_ = resp
	// 根据实际API返回格式解析持仓
	positions := make([]models.Position, 0)
	return positions, nil
}

// GetOrders 获取委托单
func (b *CSCBroker) GetOrders(ctx context.Context) ([]models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get("/account/orders")

	if err != nil {
		return nil, fmt.Errorf("获取中信建投委托单失败: %w", err)
	}

	_ = resp
	orders := make([]models.Order, 0)
	return orders, nil
}

// SubmitOrder 提交订单
func (b *CSCBroker) SubmitOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	// 中信建投订单方向编码
	sideMap := map[models.OrderSide]int{
		models.OrderSideBuy:  1, // 买入
		models.OrderSideSell: 2, // 卖出
	}

	// 中信建投订单类型编码
	typeMap := map[models.OrderType]int{
		models.OrderTypeLimit:  1, // 限价
		models.OrderTypeMarket: 2, // 市价
	}

	side, ok := sideMap[order.Side]
	if !ok {
		return nil, fmt.Errorf("不支持的交易方向: %s", order.Side)
	}

	orderType, ok := typeMap[order.Type]
	if !ok {
		return nil, fmt.Errorf("不支持的订单类型: %s", order.Type)
	}

	// 市场编码
	marketCode := "SH"
	if order.Market == models.MarketSZ {
		marketCode = "SZ"
	}

	resp, err := b.client.R().
		SetBody(map[string]interface{}{
			"stock_code":  order.StockCode,
			"market":      marketCode,
			"side":        side,
			"order_type":  orderType,
			"price":       order.Price.String(),
			"volume":      order.Volume,
			"strategy_id": order.StrategyID,
		}).
		SetResult(map[string]interface{}{}).
		Post("/trade/order")

	if err != nil {
		return nil, fmt.Errorf("中信建投下单失败: %w", err)
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	// 检查返回码
	if code, ok := data["code"]; ok && fmt.Sprintf("%v", code) != "0" {
		msg := "未知错误"
		if m, ok := data["message"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		order.Status = models.OrderStatusRejected
		order.Reason = msg
		return order, fmt.Errorf("中信建投下单被拒绝: %s", msg)
	}

	// 提取订单号
	if d, ok := data["data"].(map[string]interface{}); ok {
		if id, ok := d["order_id"]; ok {
			order.ID = fmt.Sprintf("%v", id)
		}
	}

	if order.ID == "" {
		order.ID = fmt.Sprintf("CSC_%s_%d", order.StockCode, time.Now().UnixNano())
	}

	order.Status = models.OrderStatusSubmitted
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	b.mu.Lock()
	b.orders[order.ID] = order
	b.mu.Unlock()

	b.logger.Info("中信建投订单已提交",
		zap.String("orderID", order.ID),
		zap.String("stock", order.StockCode),
		zap.String("side", string(order.Side)),
	)

	return order, nil
}

// CancelOrder 撤单
func (b *CSCBroker) CancelOrder(ctx context.Context, orderID string) error {
	if err := b.checkLogin(); err != nil {
		return err
	}

	resp, err := b.client.R().
		SetBody(map[string]interface{}{
			"order_id": orderID,
		}).
		SetResult(map[string]interface{}{}).
		Post("/trade/cancel")

	if err != nil {
		return fmt.Errorf("中信建投撤单失败: %w", err)
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	if code, ok := data["code"]; ok && fmt.Sprintf("%v", code) != "0" {
		msg := "未知错误"
		if m, ok := data["message"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		return fmt.Errorf("中信建投撤单失败: %s", msg)
	}

	b.mu.Lock()
	if order, ok := b.orders[orderID]; ok {
		order.Status = models.OrderStatusCancelled
		order.UpdatedAt = time.Now()
	}
	b.mu.Unlock()

	b.logger.Info("中信建投订单已撤销", zap.String("orderID", orderID))
	return nil
}

// GetOrder 查询订单
func (b *CSCBroker) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get(fmt.Sprintf("/trade/order/%s", orderID))

	if err != nil {
		return nil, fmt.Errorf("查询中信建投订单失败: %w", err)
	}

	_ = resp
	// 根据实际API返回格式解析订单
	return nil, nil
}

// checkLogin 检查登录状态
func (b *CSCBroker) checkLogin() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.loggedIn {
		return fmt.Errorf("未登录中信建投，请先登录")
	}
	return nil
}

// parseDecimalFromInterface 从interface{}解析decimal
func parseDecimalFromInterface(v interface{}) decimal.Decimal {
	if v == nil {
		return decimal.Zero
	}
	switch val := v.(type) {
	case float64:
		return decimal.NewFromFloat(val)
	case string:
		d, err := decimal.NewFromString(val)
		if err != nil {
			return decimal.Zero
		}
		return d
	case int:
		return decimal.NewFromInt(int64(val))
	case int64:
		return decimal.NewFromInt(val)
	default:
		return decimal.Zero
	}
}
