package config

import (
	"os"
	"sync"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// 创建临时配置文件
	content := `
server:
  host: "0.0.0.0"
  port: 9090
  mode: "debug"

database:
  type: "sqlite"
  name: "test.db"

log:
  level: "debug"
  format: "json"

broker:
  id: "test_broker"
  name: "测试券商"
  type: "simulated"
  is_demo: true

market:
  data_source: "sina"
  refresh_interval: 5

risk:
  max_single_position_pct: 0.3
  max_total_position_pct: 0.8
  max_daily_loss_pct: 0.05
  max_drawdown_pct: 0.15
  max_daily_trades: 50
  stop_loss_pct: 0.08
  take_profit_pct: 0.2

strategy:
  max_concurrent: 10
  backtest_init_capital: 1000000
  commission_rate: 0.0003
  stamp_tax_rate: 0.001
  slippage: 0.001
`

	tmpFile := "test_config.yaml"
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("创建临时配置文件失败: %v", err)
	}
	defer os.Remove(tmpFile)

	// 重置全局配置
	globalConfig = nil
	configOnce = sync.Once{} // 重置 sync.Once

	cfg, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("端口不正确: 期望 9090, 实际 %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("主机不正确: %s", cfg.Server.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("日志级别不正确: %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("日志格式不正确: %s", cfg.Log.Format)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	// 重置
	globalConfig = nil
	configOnce = sync.Once{} // 重置 sync.Once

	_, err := LoadConfig("nonexistent_config.yaml")
	if err == nil {
		t.Error("不存在的配置文件应返回错误")
	}
}

func TestInitLogger(t *testing.T) {
	cfg := &SystemConfig{
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}

	err := InitLogger(cfg)
	if err != nil {
		t.Fatalf("初始化日志失败: %v", err)
	}

	l := GetLogger()
	if l == nil {
		t.Error("GetLogger 不应返回 nil")
	}
}

func TestGetLogger_Default(t *testing.T) {
	// 重置 logger
	logger = nil

	l := GetLogger()
	if l == nil {
		t.Error("默认 GetLogger 不应返回 nil")
	}
}

func TestSetDefaults(t *testing.T) {
	cfg := &SystemConfig{}
	setDefaults(cfg)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("默认主机不正确: %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("默认端口不正确: %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "debug" {
		t.Errorf("默认模式不正确: %s", cfg.Server.Mode)
	}
	if cfg.Database.Type != "sqlite" {
		t.Errorf("默认数据库类型不正确: %s", cfg.Database.Type)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("默认日志级别不正确: %s", cfg.Log.Level)
	}
	if cfg.Market.DataSource != "sina" {
		t.Errorf("默认行情源不正确: %s", cfg.Market.DataSource)
	}
	if cfg.Strategy.MaxConcurrent != 10 {
		t.Errorf("默认并发策略数不正确: %d", cfg.Strategy.MaxConcurrent)
	}
	if cfg.Strategy.CommissionRate != 0.0003 {
		t.Errorf("默认佣金费率不正确: %f", cfg.Strategy.CommissionRate)
	}
}

func TestDecimalFromFloat(t *testing.T) {
	d := decimalFromFloat(0.3)
	if d.String() != "0.3" {
		t.Errorf("decimalFromFloat(0.3) = %s, 期望 0.3", d.String())
	}
}
