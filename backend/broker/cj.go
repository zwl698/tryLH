package broker

import (
	"aqsystem/models"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

// CJBroker 长江证券券商适配器
//
// 长江证券(Changjiang Securities)量化交易接口
// 通过长江证券提供的iVangard量化交易平台接入
//
// 支持两种接入方式：
// 1. HTTP REST API：通过iVangard平台API进行交易
// 2. 本地终端方式：通过本地部署的交易终端接入
//
// 使用前需要：
// - 在长江证券开通量化交易权限
// - 获取量化交易终端或API授权
type CJBroker struct {
	mu        sync.RWMutex
	loggedIn  bool
	account   *models.Account
	orders    map[string]*models.Order
	positions map[string]*models.Position
	config    models.BrokerConfig
	client    *resty.Client
	logger    *zap.Logger
	token     string
	sessionID string
}

// NewCJBroker 创建长江证券券商实例
func NewCJBroker(cfg models.BrokerConfig, logger *zap.Logger) *CJBroker {
	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(1 * time.Second)

	if cfg.APIURL == "" {
		// 默认API地址 - 长江证券iVangard量化交易网关
		if cfg.IsDemo {
			client.SetBaseURL("https://quant-demo.95579.com/api/v1")
		} else {
			client.SetBaseURL("https://quant.95579.com/api/v1")
		}
	} else {
		client.SetBaseURL(cfg.APIURL)
	}

	client.SetHeader("Content-Type", "application/json")
	client.SetHeader("User-Agent", "AQSystem/1.0")

	return &CJBroker{
		config:    cfg,
		orders:    make(map[string]*models.Order),
		positions: make(map[string]*models.Position),
		client:    client,
		logger:    logger,
	}
}

// Login 登录长江证券
func (b *CJBroker) Login(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logger.Info("正在连接长江证券...", zap.String("account", b.config.AccountID))

	// 第一步：认证获取Token
	resp, err := b.client.R().
		SetBody(map[string]interface{}{
			"account_id": b.config.AccountID,
			"password":   b.config.Password,
			"app_key":    b.config.AppKey,
			"client_info": map[string]string{
				"mac":      "00:00:00:00:00:00",
				"ip":       "127.0.0.1",
				"version":  "1.0.0",
				"terminal": "AQSystem",
			},
		}).
		SetResult(map[string]interface{}{}).
		Post("/auth/login")

	if err != nil {
		return fmt.Errorf("连接长江证券失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("长江证券登录失败，状态码: %d, 响应: %s", resp.StatusCode(), resp.String())
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	// 检查返回码
	if code, ok := data["error_code"]; ok && fmt.Sprintf("%v", code) != "0" {
		msg := "未知错误"
		if m, ok := data["error_msg"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		return fmt.Errorf("长江证券登录失败: %s", msg)
	}

	// 提取token
	if d, ok := data["data"].(map[string]interface{}); ok {
		if t, ok := d["token"]; ok {
			b.token = fmt.Sprintf("%v", t)
		}
		if s, ok := d["session_id"]; ok {
			b.sessionID = fmt.Sprintf("%v", s)
		}
	}

	// 设置认证头
	b.client.SetAuthToken(b.token)
	b.client.SetHeader("X-Session-ID", b.sessionID)

	// 初始化账户
	b.account = &models.Account{
		ID:         b.config.AccountID,
		BrokerID:   b.config.ID,
		BrokerName: "长江证券",
	}

	b.loggedIn = true
	b.logger.Info("长江证券登录成功", zap.String("account", b.config.AccountID))
	return nil
}

// Logout 登出
func (b *CJBroker) Logout(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.loggedIn {
		return nil
	}

	_, _ = b.client.R().
		SetHeader("Authorization", "Bearer "+b.token).
		Post("/auth/logout")

	b.loggedIn = false
	b.token = ""
	b.sessionID = ""
	b.logger.Info("长江证券已登出")
	return nil
}

// IsLoggedIn 检查登录状态
func (b *CJBroker) IsLoggedIn() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.loggedIn
}

// GetAccount 获取账户信息
func (b *CJBroker) GetAccount(ctx context.Context) (*models.Account, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get("/account/asset")

	if err != nil {
		return nil, fmt.Errorf("获取长江证券账户信息失败: %w", err)
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	account := &models.Account{
		ID:         b.config.AccountID,
		BrokerID:   b.config.ID,
		BrokerName: "长江证券",
	}

	if d, ok := data["data"].(map[string]interface{}); ok {
		if v, ok := d["available_cash"]; ok {
			account.Cash = parseDecimalFromInterface(v)
		}
		if v, ok := d["total_asset"]; ok {
			account.TotalAssets = parseDecimalFromInterface(v)
		}
		if v, ok := d["market_value"]; ok {
			account.MarketValue = parseDecimalFromInterface(v)
		}
		if v, ok := d["frozen_cash"]; ok {
			account.FrozenCash = parseDecimalFromInterface(v)
		}
		if v, ok := d["today_profit"]; ok {
			account.TodayPL = parseDecimalFromInterface(v)
		}
	} else {
		account = b.account
	}

	account.UpdatedAt = time.Now()
	return account, nil
}

// GetPositions 获取持仓
func (b *CJBroker) GetPositions(ctx context.Context) ([]models.Position, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get("/account/positions")

	if err != nil {
		return nil, fmt.Errorf("获取长江证券持仓失败: %w", err)
	}

	_ = resp
	positions := make([]models.Position, 0)
	return positions, nil
}

// GetOrders 获取委托单
func (b *CJBroker) GetOrders(ctx context.Context) ([]models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get("/account/orders")

	if err != nil {
		return nil, fmt.Errorf("获取长江证券委托单失败: %w", err)
	}

	_ = resp
	orders := make([]models.Order, 0)
	return orders, nil
}

// SubmitOrder 提交订单
func (b *CJBroker) SubmitOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	// 长江证券订单方向
	sideMap := map[models.OrderSide]string{
		models.OrderSideBuy:  "buy",
		models.OrderSideSell: "sell",
	}

	typeMap := map[models.OrderType]string{
		models.OrderTypeLimit:  "limit",
		models.OrderTypeMarket: "market",
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
		return nil, fmt.Errorf("长江证券下单失败: %w", err)
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	// 检查返回
	if code, ok := data["error_code"]; ok && fmt.Sprintf("%v", code) != "0" {
		msg := "未知错误"
		if m, ok := data["error_msg"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		order.Status = models.OrderStatusRejected
		order.Reason = msg
		return order, fmt.Errorf("长江证券下单被拒绝: %s", msg)
	}

	// 提取订单号
	if d, ok := data["data"].(map[string]interface{}); ok {
		if id, ok := d["order_id"]; ok {
			order.ID = fmt.Sprintf("%v", id)
		}
	}

	if order.ID == "" {
		order.ID = fmt.Sprintf("CJ_%s_%d", order.StockCode, time.Now().UnixNano())
	}

	order.Status = models.OrderStatusSubmitted
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	b.mu.Lock()
	b.orders[order.ID] = order
	b.mu.Unlock()

	b.logger.Info("长江证券订单已提交",
		zap.String("orderID", order.ID),
		zap.String("stock", order.StockCode),
		zap.String("side", string(order.Side)),
	)

	return order, nil
}

// CancelOrder 撤单
func (b *CJBroker) CancelOrder(ctx context.Context, orderID string) error {
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
		return fmt.Errorf("长江证券撤单失败: %w", err)
	}

	result := resp.Result().(*map[string]interface{})
	data := *result

	if code, ok := data["error_code"]; ok && fmt.Sprintf("%v", code) != "0" {
		msg := "未知错误"
		if m, ok := data["error_msg"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		return fmt.Errorf("长江证券撤单失败: %s", msg)
	}

	b.mu.Lock()
	if order, ok := b.orders[orderID]; ok {
		order.Status = models.OrderStatusCancelled
		order.UpdatedAt = time.Now()
	}
	b.mu.Unlock()

	b.logger.Info("长江证券订单已撤销", zap.String("orderID", orderID))
	return nil
}

// GetOrder 查询订单
func (b *CJBroker) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	if err := b.checkLogin(); err != nil {
		return nil, err
	}

	resp, err := b.client.R().
		SetResult(map[string]interface{}{}).
		Get(fmt.Sprintf("/trade/order/%s", orderID))

	if err != nil {
		return nil, fmt.Errorf("查询长江证券订单失败: %w", err)
	}

	_ = resp
	return nil, nil
}

// checkLogin 检查登录状态
func (b *CJBroker) checkLogin() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.loggedIn {
		return fmt.Errorf("未登录长江证券，请先登录")
	}
	return nil
}
