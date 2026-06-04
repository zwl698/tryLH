package market

import (
	"aqsystem/models"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func TestToSinaCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"600000", "sh600000"},
		{"000001", "sz000001"},
		{"300001", "sz300001"},
		{"900001", "sh900001"},
		{"sh600000", "sh600000"},
		{"sz000001", "sz000001"},
		{"SH600000", "sh600000"},
		{"SZ000001", "sz000001"},
	}

	for _, tt := range tests {
		result := toSinaCode(tt.input)
		if result != tt.expected {
			t.Errorf("toSinaCode(%s) = %s, 期望 %s", tt.input, result, tt.expected)
		}
	}
}

func TestSafeDecimal(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"10.50", "10.50"},
		{"0", "0"},
		{"", "0"},
		{"invalid", "0"},
		{"  15.23  ", "15.23"},
	}

	for _, tt := range tests {
		result := safeDecimal(tt.input)
		expected, _ := decimal.NewFromString(tt.expected)
		if !result.Equal(expected) {
			t.Errorf("safeDecimal(%s) = %s, 期望 %s", tt.input, result.String(), expected.String())
		}
	}
}

func TestSafeInt64(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1000", 1000},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
		{"  500  ", 500},
	}

	for _, tt := range tests {
		result := safeInt64(tt.input)
		if result != tt.expected {
			t.Errorf("safeInt64(%s) = %d, 期望 %d", tt.input, result, tt.expected)
		}
	}
}

func TestParseSinaQuote(t *testing.T) {
	// 模拟新浪行情数据格式（需要32个字段）
	// 字段: 0名称,1开盘,2昨收,3收盘,4最高,5最低,6买一,7卖一,8成交量,9成交额,
	// 10买一量,11买一价,12买二量,13买二价,14买三量,15买三价,16买四量,17买四价,18买五量,19买五价,
	// 20卖一量,21卖一价,22卖二量,23卖二价,24卖三量,25卖三价,26卖四量,27卖四价,28卖五量,29卖五价,
	// 30日期,31时间
	line := `var hq_str_sh600000="浦发银行,15.23,15.20,15.25,15.30,15.15,15.24,15.25,35678900,54321.00,100,15.24,200,15.23,300,15.22,400,15.21,500,15.20,600,15.26,700,15.27,800,15.28,900,15.29,1000,15.30,2024-01-15,15:00:00";`

	quote, err := parseSinaQuote(line)
	if err != nil {
		t.Fatalf("解析新浪行情失败: %v", err)
	}

	if quote.StockName != "浦发银行" {
		t.Errorf("股票名称不正确: %s", quote.StockName)
	}
	if quote.StockCode != "600000" {
		t.Errorf("股票代码不正确: %s", quote.StockCode)
	}
	if quote.Market != models.MarketSH {
		t.Errorf("市场不正确: %s", quote.Market)
	}
	if !quote.Open.Equal(decimal.NewFromFloat(15.23)) {
		t.Errorf("开盘价不正确: %s", quote.Open.String())
	}
	if !quote.PreClose.Equal(decimal.NewFromFloat(15.20)) {
		t.Errorf("昨收价不正确: %s", quote.PreClose.String())
	}
	if !quote.Close.Equal(decimal.NewFromFloat(15.25)) {
		t.Errorf("收盘价不正确: %s", quote.Close.String())
	}
}

func TestParseSinaQuote_EmptyData(t *testing.T) {
	line := `var hq_str_sh600000="";`
	_, err := parseSinaQuote(line)
	if err == nil {
		t.Error("空数据应返回错误")
	}
}

func TestParseSinaQuote_InvalidFormat(t *testing.T) {
	line := `invalid data`
	_, err := parseSinaQuote(line)
	if err == nil {
		t.Error("无效格式应返回错误")
	}
}

func TestExtractSinaRawCode_Index(t *testing.T) {
	line := `var hq_str_sh000001="上证指数,3000.00,2990.00,3010.00,3020.00,2980.00,0,0,123456,654321,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2024-01-15,15:00:00";`
	raw := extractSinaRawCode(line)
	if raw != "sh000001" {
		t.Fatalf("指数原始代码解析错误: %s", raw)
	}
}

func TestParseTencentQuote(t *testing.T) {
	// 构造腾讯行情格式的数据（字段间用~分隔，至少45个字段）
	// 字段: 0市场前缀,1股票名称,2股票代码,3收盘价,4昨收,5开盘,6成交量,
	// ... 33最高,34最低, ... 37成交额,38换手率,39市盈率, ...
	line := "v_sh600000~浦发银行~600000~15.25~15.20~15.23~35678900~~~~15.24~100~~~~15.25~200~~~~15.26~~~~~~0.00~0.00~15.30~15.15~0.00~0.00~0.00~15.30~15.15~0.00~0.00~54321.00~0.50~10.5~0.00~0.00~~~20240115000000"

	quote, err := parseTencentQuote(line)
	if err != nil {
		t.Fatalf("解析腾讯行情失败: %v", err)
	}

	if quote.StockName != "浦发银行" {
		t.Errorf("股票名称不正确: %s", quote.StockName)
	}
	if quote.StockCode != "600000" {
		t.Errorf("股票代码不正确: %s", quote.StockCode)
	}
	if quote.Market != models.MarketSH {
		t.Errorf("市场不正确: %s", quote.Market)
	}
	if !quote.Close.Equal(decimal.NewFromFloat(15.25)) {
		t.Errorf("收盘价不正确: %s", quote.Close.String())
	}
}

func TestNewMarketService(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		source string
	}{
		{"sina"},
		{"tencent"},
		{"unknown"},
	}

	for _, tt := range tests {
		svc := NewMarketService(tt.source, logger)
		if svc == nil {
			t.Errorf("NewMarketService(%s) 不应返回 nil", tt.source)
		}
	}
}

func TestMarketService_Subscribe(t *testing.T) {
	logger := zap.NewNop()
	svc := NewMarketService("sina", logger)

	err := svc.Subscribe(nil, []string{"600000", "000001"})
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}
}

func TestIsTradingTime(t *testing.T) {
	// 注意：此测试依赖当前时间，可能在不同时间段结果不同
	// 仅确保函数不会 panic
	_ = isTradingTime()
}

func TestParseSinaKLines(t *testing.T) {
	// 新浪K线CSV格式：日期,开盘,最高,最低,收盘,成交量,成交额
	data := `2024-01-15,15.23,15.30,15.15,15.25,35678900,54321.00
2024-01-16,15.25,15.35,15.20,15.30,25678900,44321.00`

	klines, err := parseSinaKLines(data, "600000", "day")
	if err != nil {
		t.Fatalf("解析K线失败: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("K线数量不正确: 期望 2, 实际 %d", len(klines))
	}
	if klines[0].StockCode != "600000" {
		t.Errorf("K线股票代码不正确: %s", klines[0].StockCode)
	}
	if !klines[0].Open.Equal(decimal.NewFromFloat(15.23)) {
		t.Errorf("K线开盘价不正确: %s", klines[0].Open.String())
	}
}

func TestParseSinaJSONKLines(t *testing.T) {
	data := `[{"day":"2024-01-15","open":"15.23","high":"15.30","low":"15.15","close":"15.25","volume":"35678900"},{"day":"2024-01-16","open":"15.25","high":"15.35","low":"15.20","close":"15.30","volume":"25678900"}]`

	klines, err := parseSinaKLines(data, "sh600000", "day")
	if err != nil {
		t.Fatalf("解析新浪JSON K线失败: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("K线数量不正确: 期望 2, 实际 %d", len(klines))
	}
	if klines[0].StockCode != "600000" {
		t.Errorf("K线股票代码不正确: %s", klines[0].StockCode)
	}
	if !klines[1].Close.Equal(decimal.NewFromFloat(15.30)) {
		t.Errorf("K线收盘价不正确: %s", klines[1].Close.String())
	}
}

func TestParseEastMoneyKLines(t *testing.T) {
	data := `{"data":{"klines":["2024-01-15,15.23,15.25,15.30,15.15,35678900,54321.00","2024-01-16,15.25,15.30,15.35,15.20,25678900,44321.00"]}}`

	klines, err := parseEastMoneyKLines(data, "600000", "day")
	if err != nil {
		t.Fatalf("解析东方财富K线失败: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("K线数量不正确: 期望 2, 实际 %d", len(klines))
	}
	if !klines[0].Close.Equal(decimal.NewFromFloat(15.25)) {
		t.Errorf("K线收盘价不正确: %s", klines[0].Close.String())
	}
	if !klines[0].High.Equal(decimal.NewFromFloat(15.30)) {
		t.Errorf("K线最高价不正确: %s", klines[0].High.String())
	}
	if !klines[0].Low.Equal(decimal.NewFromFloat(15.15)) {
		t.Errorf("K线最低价不正确: %s", klines[0].Low.String())
	}
}

func TestParseTencentKLines(t *testing.T) {
	data := `{"code":0,"msg":"","data":{"sh600519":{"qfqday":[["2026-06-03","1304.000","1281.910","1304.000","1276.000","52477.000"],["2026-06-04","1278.990","1277.750","1288.990","1276.000","13632.000"]]}}}`

	klines, err := parseTencentKLines(data, "sh600519", "600519", "day")
	if err != nil {
		t.Fatalf("解析腾讯K线失败: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("K线数量不正确: 期望 2, 实际 %d", len(klines))
	}
	if !klines[0].Open.Equal(decimal.NewFromFloat(1304)) {
		t.Errorf("K线开盘价不正确: %s", klines[0].Open.String())
	}
	if !klines[1].Close.Equal(decimal.NewFromFloat(1277.75)) {
		t.Errorf("K线收盘价不正确: %s", klines[1].Close.String())
	}
	if klines[1].Volume != 13632 {
		t.Errorf("K线成交量不正确: %d", klines[1].Volume)
	}
}

func TestParseTencentMinuteLines(t *testing.T) {
	data := `{"code":0,"msg":"","data":{"sz002475":{"data":{"data":["0930 73.30 15823 115982590.00","0931 73.01 74757 545881123.00","0932 73.08 76100 555696123.00"]}}}}`
	tradingDay := time.Date(2026, 6, 4, 0, 0, 0, 0, time.Local)

	klines, err := parseTencentMinuteLines(data, "sz002475", "002475", tradingDay)
	if err != nil {
		t.Fatalf("解析腾讯分时失败: %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("分时数量不正确: 期望 3, 实际 %d", len(klines))
	}
	if klines[0].Period != "minute" {
		t.Errorf("分时周期不正确: %s", klines[0].Period)
	}
	if !klines[0].Close.Equal(decimal.NewFromFloat(73.30)) {
		t.Errorf("分时价格不正确: %s", klines[0].Close.String())
	}
	if klines[0].Volume != 15823 {
		t.Errorf("首分钟成交量不正确: %d", klines[0].Volume)
	}
	if klines[1].Volume != 58934 {
		t.Errorf("第二分钟成交量应从累计量转成增量，实际 %d", klines[1].Volume)
	}
	if klines[2].Timestamp.Format("2006-01-02 15:04") != "2026-06-04 09:32" {
		t.Errorf("分时时间不正确: %s", klines[2].Timestamp.Format("2006-01-02 15:04"))
	}
}
