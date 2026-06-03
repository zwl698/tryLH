# A股量化交易系统

一个基于 Go 语言实现的完整 A 股量化交易系统，支持多券商接入、实时行情获取、复杂策略配置和风险控制。

## 功能特性

- **多券商支持**：中信建投（CSC）、长江证券（CJ）、XTQuant、模拟券商
- **实时行情**：新浪行情源、腾讯行情源，支持轮询订阅
- **内置5大策略**：双均线交叉、海龟交易、动量策略、均值回归、网格交易
- **策略回测**：内置回测引擎，支持自定义初始资金、佣金费率
- **风险控制**：单票仓位限制、总仓位限制、每日亏损限制、止损止盈
- **RESTful API**：基于 Gin 框架，完整的 HTTP 接口
- **完整单元测试**：63 个测试用例，覆盖所有核心模块

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.21 |
| HTTP 框架 | Gin v1.9.1 |
| HTTP 客户端 | go-resty/resty v2 |
| 精度计算 | shopspring/decimal |
| 日志 | uber-go/zap |
| 配置 | gopkg.in/yaml.v3 |

## 项目结构

```
.
├── api/            # RESTful API 服务层（Gin）
├── backtest/       # 策略回测引擎
├── broker/         # 券商接口层
│   ├── broker.go   # 接口定义 & 模拟券商
│   ├── csc.go      # 中信建投
│   ├── cj.go       # 长江证券
│   ├── xtquant.go  # XTQuant
│   └── factory.go  # 券商工厂
├── config/         # 配置加载 & 日志初始化
├── market/         # 行情数据服务（新浪/腾讯）
├── models/         # 核心数据结构
├── risk/           # 风险管理器
├── strategy/       # 策略引擎 & 内置策略
├── config.yaml     # 系统配置文件
└── main.go         # 程序入口
```

## 快速开始

### 环境要求

- Go 1.21+

### 安装依赖

```bash
go mod tidy
```

### 运行系统

```bash
go run main.go
```

系统默认监听 `http://0.0.0.0:8080`，可在 `config.yaml` 中修改。

### 运行测试

```bash
go test ./... -v
```

## 配置说明

编辑 `config.yaml`：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"  # debug / release

broker:
  type: "simulated"  # simulated / csc / cj / xtquant
  # 接入真实券商时需填写 api_key / secret_key / host 等

market:
  data_source: "sina"  # sina / tencent
  refresh_interval: 3  # 行情刷新间隔（秒）

risk:
  max_single_position_pct: 0.3   # 单票最大仓位
  max_total_position_pct: 0.8    # 总仓位上限
  max_daily_loss_pct: 0.05       # 每日最大亏损
  stop_loss_pct: 0.08            # 止损比例
  take_profit_pct: 0.2           # 止盈比例
```

## API 接口

### 系统

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| GET | `/api/v1/system/status` | 系统状态 |

### 券商 & 账户

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/broker/login` | 券商登录 |
| POST | `/api/v1/broker/logout` | 退出登录 |
| GET | `/api/v1/broker/account` | 账户信息 |
| GET | `/api/v1/broker/positions` | 持仓列表 |
| GET | `/api/v1/broker/orders` | 委托记录 |

### 交易

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/trade/order` | 下单 |
| DELETE | `/api/v1/trade/order/:id` | 撤单 |

### 行情

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/market/quote/:code` | 单股行情 |
| POST | `/api/v1/market/quotes` | 批量行情 |
| GET | `/api/v1/market/kline/:code` | K线数据 |
| GET | `/api/v1/market/index/:code` | 大盘指数 |
| POST | `/api/v1/market/subscribe` | 订阅行情 |

### 策略

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/strategies` | 策略列表 |
| POST | `/api/v1/strategy/:id/start` | 启动策略 |
| POST | `/api/v1/strategy/:id/stop` | 停止策略 |
| PUT | `/api/v1/strategy/:id/params` | 更新参数 |
| GET | `/api/v1/strategy-templates` | 策略模板 |

### 回测

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/backtest` | 运行回测 |

### 风控

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/risk/config` | 查看风控配置 |
| PUT | `/api/v1/risk/config` | 更新风控配置 |
| GET | `/api/v1/risk/events` | 风控事件记录 |

## 内置策略

### 1. 双均线交叉策略（Double MA）
- **原理**：短期均线上穿长期均线买入（金叉），下穿卖出（死叉）
- **参数**：短期周期（默认5日）、长期周期（默认20日）、均线类型（SMA/EMA）
- **适用**：趋势明显的市场

### 2. 海龟交易策略（Turtle）
- **原理**：价格突破 N 日最高价买入，基于 ATR 管理仓位和止损
- **参数**：入场通道（默认20日）、出场通道（默认10日）、ATR 周期、风险比例
- **适用**：中长期趋势跟踪

### 3. 动量策略（Momentum）
- **原理**：买入过去 N 日涨幅最大的股票（强者恒强）
- **参数**：回望期（默认20日）、持有期（默认10日）、动量阈值
- **适用**：A 股 3-12 个月的动量效应

### 4. 均值回归策略（Mean Reversion）
- **原理**：价格偏离均值超过 2 倍标准差（Z-score）时买入，回归后卖出
- **参数**：回望期（默认20日）、入场 Z-score（默认2.0）、出场 Z-score（默认0.5）
- **适用**：震荡行情的统计套利

### 5. 网格交易策略（Grid）
- **原理**：在价格区间内设置网格，每下一格买入，每上一格卖出
- **参数**：上限价、下限价、网格数量（默认10）、每格交易量（默认100股）
- **适用**：震荡行情自动化交易

## 架构设计

```
main.go
  ├── Config（配置加载）
  ├── MarketService（行情服务）
  ├── Broker（券商接口）
  ├── RiskManager（风险管理）
  ├── StrategyEngine（策略引擎）
  ├── BacktestEngine（回测引擎）
  └── APIServer（HTTP服务）
         └── Gin Router
```

所有核心组件均基于接口设计，支持灵活替换：
- `Broker` 接口 → 可接入任意券商
- `DataProvider` 接口 → 可接入任意行情源
- `Strategy` 接口 → 可实现任意自定义策略

## 免责声明

本系统仅供学习和研究使用，不构成任何投资建议。量化交易存在亏损风险，请在充分了解风险的前提下谨慎使用。实盘交易前请确保已完成充分的回测验证。

