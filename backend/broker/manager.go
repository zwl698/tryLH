package broker

import (
	"aqsystem/models"
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// BrokerProvider 描述一个可在运行时选择的券商适配器。
type BrokerProvider struct {
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Requires     []string `json:"requires"`
	SupportsLive bool     `json:"supports_live"`
}

// BrokerRuntimeStatus 描述当前运行时券商连接状态。
type BrokerRuntimeStatus struct {
	LoggedIn    bool                `json:"logged_in"`
	LiveTrading bool                `json:"live_trading"`
	Current     models.BrokerConfig `json:"current"`
}

// BrokerManager 管理运行时券商实例，保证手动交易、策略交易和风控交易使用同一连接。
type BrokerManager struct {
	mu      sync.RWMutex
	current Broker
	config  models.BrokerConfig
	logger  *zap.Logger
}

// NewBrokerManager 创建券商管理器。
func NewBrokerManager(cfg models.BrokerConfig, logger *zap.Logger) (*BrokerManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	cfg = normalizeBrokerConfig(cfg)
	b, err := NewBroker(cfg, logger)
	if err != nil {
		return nil, err
	}

	return &BrokerManager{
		current: b,
		config:  cfg,
		logger:  logger,
	}, nil
}

// SupportedBrokerProviders 返回前端可展示的券商列表。
func SupportedBrokerProviders() []BrokerProvider {
	return []BrokerProvider{
		{
			Type:         "simulated",
			Name:         "模拟券商",
			Description:  "开发、演示和策略联调使用，不连接真实资金账户。",
			Requires:     []string{},
			SupportsLive: false,
		},
		{
			Type:         "csc",
			Name:         "中信建投证券",
			Description:  "通过已开通的中信建投量化/API交易网关接入真实或仿真账户。",
			Requires:     []string{"api_url", "account_id", "password", "app_key", "app_secret"},
			SupportsLive: true,
		},
		{
			Type:         "xtquant",
			Name:         "QMT / 迅投",
			Description:  "通过本地 MiniQMT/xtquant 交易服务接入。",
			Requires:     []string{"api_url", "account_id", "password"},
			SupportsLive: true,
		},
		{
			Type:         "cj",
			Name:         "长江证券",
			Description:  "通过已开通的长江证券量化交易网关接入。",
			Requires:     []string{"api_url", "account_id", "password", "app_key"},
			SupportsLive: true,
		},
	}
}

// Current 获取当前券商实例。
func (m *BrokerManager) Current() Broker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Status 获取脱敏后的运行时券商状态。
func (m *BrokerManager) Status() BrokerRuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	loggedIn := false
	if m.current != nil {
		loggedIn = m.current.IsLoggedIn()
	}
	safeConfig := sanitizeBrokerConfig(m.config)
	return BrokerRuntimeStatus{
		LoggedIn:    loggedIn,
		LiveTrading: loggedIn && safeConfig.Type != "simulated" && !safeConfig.IsDemo,
		Current:     safeConfig,
	}
}

// Select 切换当前券商但不登录，适合前端先选择券商再填写凭据。
func (m *BrokerManager) Select(ctx context.Context, cfg models.BrokerConfig) error {
	cfg = normalizeBrokerConfig(cfg)
	next, err := NewBroker(cfg, m.logger)
	if err != nil {
		return err
	}
	return m.swap(ctx, cfg, next)
}

// Login 使用传入配置登录。传空配置时登录当前券商。
func (m *BrokerManager) Login(ctx context.Context, cfg models.BrokerConfig) error {
	if strings.TrimSpace(cfg.Type) == "" {
		m.mu.RLock()
		current := m.current
		m.mu.RUnlock()
		if current == nil {
			return fmt.Errorf("当前未选择券商")
		}
		if current.IsLoggedIn() {
			return nil
		}
		return current.Login(ctx)
	}

	cfg = normalizeBrokerConfig(cfg)
	next, err := NewBroker(cfg, m.logger)
	if err != nil {
		return err
	}
	if err := next.Login(ctx); err != nil {
		return err
	}
	return m.swap(ctx, cfg, next)
}

// Logout 登出当前券商。
func (m *BrokerManager) Logout(ctx context.Context) error {
	m.mu.RLock()
	current := m.current
	m.mu.RUnlock()
	if current == nil {
		return nil
	}
	return current.Logout(ctx)
}

func (m *BrokerManager) swap(ctx context.Context, cfg models.BrokerConfig, next Broker) error {
	m.mu.Lock()
	previous := m.current
	m.current = next
	m.config = cfg
	m.mu.Unlock()

	if previous != nil && previous != next && previous.IsLoggedIn() {
		if err := previous.Logout(ctx); err != nil {
			m.logger.Warn("切换券商时登出旧连接失败", zap.Error(err))
		}
	}
	return nil
}

func normalizeBrokerConfig(cfg models.BrokerConfig) models.BrokerConfig {
	cfg.Type = strings.TrimSpace(strings.ToLower(cfg.Type))
	if cfg.Type == "" {
		cfg.Type = "simulated"
	}
	if cfg.ID == "" {
		cfg.ID = cfg.Type + "_runtime"
	}
	if cfg.Name == "" {
		for _, provider := range SupportedBrokerProviders() {
			if provider.Type == cfg.Type {
				cfg.Name = provider.Name
				break
			}
		}
	}
	if cfg.Name == "" {
		cfg.Name = cfg.Type
	}
	return cfg
}

func sanitizeBrokerConfig(cfg models.BrokerConfig) models.BrokerConfig {
	cfg.Password = ""
	cfg.AppSecret = ""
	cfg.CaPath = ""
	cfg.CertPath = ""
	if cfg.ExtConfig != nil {
		safeExt := make(map[string]string, len(cfg.ExtConfig))
		for key, value := range cfg.ExtConfig {
			if isSensitiveConfigKey(key) {
				safeExt[key] = ""
			} else {
				safeExt[key] = value
			}
		}
		cfg.ExtConfig = safeExt
	}
	return cfg
}

func isSensitiveConfigKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "password") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "passwd") ||
		strings.Contains(key, "pwd")
}
