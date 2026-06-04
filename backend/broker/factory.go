package broker

import (
	"aqsystem/models"
	"fmt"

	"go.uber.org/zap"
)

// BrokerFactory 券商工厂 - 根据配置创建对应的券商实例
func NewBroker(cfg models.BrokerConfig, logger *zap.Logger) (Broker, error) {
	switch cfg.Type {
	case "simulated":
		return NewSimulatedBroker(cfg, logger), nil
	case "xtquant", "qmt":
		return NewXTQuantBroker(cfg, logger), nil
	case "csc", "中信建投":
		return NewCSCBroker(cfg, logger), nil
	case "cj", "长江证券", "changjiang":
		return NewCJBroker(cfg, logger), nil
	case "ths", "同花顺":
		return nil, fmt.Errorf("同花顺券商适配器暂未实现，请使用 simulated / xtquant / csc / cj")
	default:
		return nil, fmt.Errorf("不支持的券商类型: %s（支持: simulated, xtquant, csc, cj）", cfg.Type)
	}
}
