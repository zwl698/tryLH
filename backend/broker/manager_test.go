package broker

import (
	"aqsystem/models"
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestSupportedBrokerProvidersIncludesCSC(t *testing.T) {
	providers := SupportedBrokerProviders()
	var found bool
	for _, provider := range providers {
		if provider.Type == "csc" {
			found = true
			if provider.Name == "" {
				t.Fatal("中信建投券商名称不应为空")
			}
			if !provider.SupportsLive {
				t.Fatal("中信建投应标记为支持实盘接入")
			}
		}
	}
	if !found {
		t.Fatal("支持券商列表应包含中信建投 csc")
	}
}

func TestBrokerManagerLoginSwitchesRuntimeBrokerAndRedactsSecrets(t *testing.T) {
	manager, err := NewBrokerManager(models.BrokerConfig{
		ID:        "sim_broker",
		Name:      "模拟券商",
		Type:      "simulated",
		AccountID: "SIM001",
		IsDemo:    true,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("创建券商管理器失败: %v", err)
	}

	err = manager.Login(context.Background(), models.BrokerConfig{
		ID:        "runtime_sim",
		Name:      "运行时模拟",
		Type:      "simulated",
		AccountID: "SIM_RUNTIME",
		Password:  "secret-password",
		AppSecret: "secret-app",
		IsDemo:    true,
		ExtConfig: map[string]string{
			"token": "secret-token",
			"note":  "keep-me",
		},
	})
	if err != nil {
		t.Fatalf("运行时登录失败: %v", err)
	}

	status := manager.Status()
	if !status.LoggedIn {
		t.Fatal("登录后状态应为已登录")
	}
	if status.Current.Type != "simulated" || status.Current.AccountID != "SIM_RUNTIME" {
		t.Fatalf("当前券商状态不正确: %+v", status.Current)
	}
	if status.Current.Password != "" || status.Current.AppSecret != "" {
		t.Fatalf("状态不应泄露密码或应用密钥: %+v", status.Current)
	}
	if status.Current.ExtConfig["token"] != "" {
		t.Fatalf("状态不应泄露扩展配置中的敏感 token: %+v", status.Current.ExtConfig)
	}
	if status.Current.ExtConfig["note"] != "keep-me" {
		t.Fatalf("非敏感扩展配置应保留: %+v", status.Current.ExtConfig)
	}
}
