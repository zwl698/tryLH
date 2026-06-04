package broker

import (
	"aqsystem/models"
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

// XTQuantBroker 迅投QMT券商接口
// 通过迅投xtquant的miniQmt进行交易
// 需要安装miniQmt客户端并开启交易服务
type XTQuantBroker struct {
	loggedIn  bool
	account   *models.Account
	orders    map[string]*models.Order
	positions map[string]*models.Position
	config    models.BrokerConfig
	client    *resty.Client
	logger    *zap.Logger
	sessionID string
}

// NewXTQuantBroker 创建迅投QMT券商
func NewXTQuantBroker(cfg models.BrokerConfig, logger *zap.Logger) *XTQuantBroker {
	client := resty.New()
	client.SetBaseURL(cfg.APIURL)
	client.SetTimeout(30 * time.Second)

	return &XTQuantBroker{
		config:    cfg,
		orders:    make(map[string]*models.Order),
		positions: make(map[string]*models.Position),
		client:    client,
		logger:    logger,
	}
}

// Login 登录券商
func (b *XTQuantBroker) Login(ctx context.Context) error {
	b.logger.Info("正在连接迅投QMT...", zap.String("url", b.config.APIURL))

	// 调用miniQmt的登录接口
	resp, err := b.client.R().
		SetBody(map[string]interface{}{
			"account_id": b.config.AccountID,
			"password":   b.config.Password,
			"type":       "stock",
		}).
		SetResult(map[string]interface{}{}).
		Post("/api/login")

	if err != nil {
		return fmt.Errorf("连接QMT失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("QMT登录失败，状态码: %d", resp.StatusCode())
	}

	result := resp.Result().(*map[string]interface{})
	if data, ok := (*result)["session_id"]; ok {
		b.sessionID = fmt.Sprintf("%v", data)
	}

	b.loggedIn = true
	b.logger.Info("迅投QMT登录成功", zap.String("account", b.config.AccountID))
	return nil
}

// Logout 登出
func (b *XTQuantBroker) Logout(ctx context.Context) error {
	if !b.loggedIn {
		return nil
	}

	_, err := b.client.R().
		SetHeader("X-Session-ID", b.sessionID).
		Post("/api/logout")

	b.loggedIn = false
	b.logger.Info("迅投QMT已登出")
	return err
}

// IsLoggedIn 检查登录状态
func (b *XTQuantBroker) IsLoggedIn() bool {
	return b.loggedIn
}

// GetAccount 获取账户信息
func (b *XTQuantBroker) GetAccount(ctx context.Context) (*models.Account, error) {
	if !b.loggedIn {
		return nil, fmt.Errorf("未登录")
	}

	resp, err := b.client.R().
		SetHeader("X-Session-ID", b.sessionID).
		SetResult(map[string]interface{}{}).
		Get("/api/account")

	if err != nil {
		return nil, fmt.Errorf("获取账户信息失败: %w", err)
	}

	_ = resp
	// 解析账户数据 - 根据实际QMT返回格式适配
	account := &models.Account{
		ID:         b.config.AccountID,
		BrokerID:   b.config.ID,
		BrokerName: b.config.Name,
	}
	return account, nil
}

// GetPositions 获取持仓
func (b *XTQuantBroker) GetPositions(ctx context.Context) ([]models.Position, error) {
	if !b.loggedIn {
		return nil, fmt.Errorf("未登录")
	}

	resp, err := b.client.R().
		SetHeader("X-Session-ID", b.sessionID).
		SetResult([]map[string]interface{}{}).
		Get("/api/positions")

	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	_ = resp
	return []models.Position{}, nil
}

// GetOrders 获取订单
func (b *XTQuantBroker) GetOrders(ctx context.Context) ([]models.Order, error) {
	if !b.loggedIn {
		return nil, fmt.Errorf("未登录")
	}

	resp, err := b.client.R().
		SetHeader("X-Session-ID", b.sessionID).
		SetResult([]map[string]interface{}{}).
		Get("/api/orders")

	if err != nil {
		return nil, fmt.Errorf("获取订单失败: %w", err)
	}

	_ = resp
	return []models.Order{}, nil
}

// SubmitOrder 提交订单
func (b *XTQuantBroker) SubmitOrder(ctx context.Context, order *models.Order) (*models.Order, error) {
	if !b.loggedIn {
		return nil, fmt.Errorf("未登录")
	}

	side := 23 // 买入
	if order.Side == models.OrderSideSell {
		side = 24 // 卖出
	}

	orderType := 11 // 限价
	if order.Type == models.OrderTypeMarket {
		orderType = 12 // 市价
	}

	resp, err := b.client.R().
		SetHeader("X-Session-ID", b.sessionID).
		SetBody(map[string]interface{}{
			"stock_code": order.StockCode,
			"market":     order.Market,
			"side":       side,
			"order_type": orderType,
			"price":      order.Price.String(),
			"volume":     order.Volume,
		}).
		SetResult(map[string]interface{}{}).
		Post("/api/order")

	if err != nil {
		return nil, fmt.Errorf("下单失败: %w", err)
	}

	_ = resp
	order.Status = models.OrderStatusSubmitted
	return order, nil
}

// CancelOrder 撤单
func (b *XTQuantBroker) CancelOrder(ctx context.Context, orderID string) error {
	if !b.loggedIn {
		return fmt.Errorf("未登录")
	}

	_, err := b.client.R().
		SetHeader("X-Session-ID", b.sessionID).
		SetBody(map[string]interface{}{
			"order_id": orderID,
		}).
		Post("/api/cancel_order")

	return err
}

// GetOrder 查询订单
func (b *XTQuantBroker) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	if !b.loggedIn {
		return nil, fmt.Errorf("未登录")
	}

	resp, err := b.client.R().
		SetHeader("X-Session-ID", b.sessionID).
		SetResult(map[string]interface{}{}).
		Get(fmt.Sprintf("/api/order/%s", orderID))

	if err != nil {
		return nil, fmt.Errorf("查询订单失败: %w", err)
	}

	_ = resp
	return nil, nil
}
