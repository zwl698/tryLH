package config

import (
	"fmt"
	"github.com/shopspring/decimal"
	"os"
	"sync"

	"aqsystem/models"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// SystemConfig 系统配置
type SystemConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Log      LogConfig      `yaml:"log"`
	Broker   models.BrokerConfig `yaml:"broker"`
	Market   MarketConfig   `yaml:"market"`
	Risk     models.RiskConfig   `yaml:"risk"`
	Strategy StrategyGlobalConfig `yaml:"strategy"`
}

// ServerConfig 服务配置
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // debug, release
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type     string `yaml:"type"` // sqlite, mysql
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level"` // debug, info, warn, error
	Format string `yaml:"format"` // json, console
	Path   string `yaml:"path"`
}

// MarketConfig 行情配置
type MarketConfig struct {
	DataSource string `yaml:"data_source"` // sina, tencent, eastmoney
	RefreshInterval int `yaml:"refresh_interval"` // 刷新间隔(秒)
	WSEnabled  bool   `yaml:"ws_enabled"`
	WSURL      string `yaml:"ws_url"`
}

// StrategyGlobalConfig 策略全局配置
type StrategyGlobalConfig struct {
	MaxConcurrent int `yaml:"max_concurrent"` // 最大并发策略数
	BacktestInitCapital float64 `yaml:"backtest_init_capital"`
	CommissionRate float64 `yaml:"commission_rate"` // 佣金费率
	StampTaxRate   float64 `yaml:"stamp_tax_rate"`  // 印花税费率
	Slippage       float64 `yaml:"slippage"`        // 滑点
}

var (
	globalConfig *SystemConfig
	configOnce   sync.Once
	logger       *zap.Logger
)

// LoadConfig 加载配置
func LoadConfig(path string) (*SystemConfig, error) {
	var err error
	configOnce.Do(func() {
		data, e := os.ReadFile(path)
		if e != nil {
			err = fmt.Errorf("读取配置文件失败: %w", e)
			return
		}

		var cfg SystemConfig
		if e = yaml.Unmarshal(data, &cfg); e != nil {
			err = fmt.Errorf("解析配置文件失败: %w", e)
			return
		}

		// 设置默认值
		setDefaults(&cfg)
		globalConfig = &cfg
	})
	return globalConfig, err
}

// GetConfig 获取全局配置
func GetConfig() *SystemConfig {
	if globalConfig == nil {
		panic("配置未初始化，请先调用 LoadConfig")
	}
	return globalConfig
}

// InitLogger 初始化日志
func InitLogger(cfg *SystemConfig) error {
	var zapCfg zap.Config
	if cfg.Log.Format == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
	}

	switch cfg.Log.Level {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	var err error
	logger, err = zapCfg.Build()
	if err != nil {
		return fmt.Errorf("初始化日志失败: %w", err)
	}
	zap.ReplaceGlobals(logger)
	return nil
}

// GetLogger 获取日志
func GetLogger() *zap.Logger {
	if logger == nil {
		l, _ := zap.NewDevelopment()
		logger = l
	}
	return logger
}

func setDefaults(cfg *SystemConfig) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "debug"
	}
	if cfg.Database.Type == "" {
		cfg.Database.Type = "sqlite"
		cfg.Database.Name = "data/aqsystem.db"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "console"
	}
	if cfg.Market.DataSource == "" {
		cfg.Market.DataSource = "sina"
	}
	if cfg.Market.RefreshInterval == 0 {
		cfg.Market.RefreshInterval = 3
	}
	if cfg.Strategy.MaxConcurrent == 0 {
		cfg.Strategy.MaxConcurrent = 10
	}
	if cfg.Strategy.BacktestInitCapital == 0 {
		cfg.Strategy.BacktestInitCapital = 1000000
	}
	if cfg.Strategy.CommissionRate == 0 {
		cfg.Strategy.CommissionRate = 0.0003
	}
	if cfg.Strategy.StampTaxRate == 0 {
		cfg.Strategy.StampTaxRate = 0.001
	}
	if cfg.Strategy.Slippage == 0 {
		cfg.Strategy.Slippage = 0.001
	}
	// 风控默认值
	if cfg.Risk.MaxSinglePositionPct.IsZero() {
		cfg.Risk.MaxSinglePositionPct = decimalFromFloat(0.3)
	}
	if cfg.Risk.MaxTotalPositionPct.IsZero() {
		cfg.Risk.MaxTotalPositionPct = decimalFromFloat(0.8)
	}
	if cfg.Risk.MaxDailyLossPct.IsZero() {
		cfg.Risk.MaxDailyLossPct = decimalFromFloat(0.05)
	}
	if cfg.Risk.MaxDrawdownPct.IsZero() {
		cfg.Risk.MaxDrawdownPct = decimalFromFloat(0.15)
	}
	if cfg.Risk.MaxDailyTrades == 0 {
		cfg.Risk.MaxDailyTrades = 50
	}
	if cfg.Risk.StopLossPct.IsZero() {
		cfg.Risk.StopLossPct = decimalFromFloat(0.08)
	}
	if cfg.Risk.TakeProfitPct.IsZero() {
		cfg.Risk.TakeProfitPct = decimalFromFloat(0.2)
	}
}

func decimalFromFloat(f float64) decimal.Decimal {
	return decimal.NewFromFloat(f)
}

