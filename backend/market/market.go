package market

import (
	"aqsystem/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-resty/resty/v2"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// DataProvider 行情数据提供者接口
type DataProvider interface {
	// 获取实时行情
	GetQuote(ctx context.Context, stockCode string) (*models.StockQuote, error)
	GetQuotes(ctx context.Context, stockCodes []string) (map[string]models.StockQuote, error)

	// 获取K线数据
	GetKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error)

	// 获取大盘指数
	GetIndexQuote(ctx context.Context, indexCode string) (*models.StockQuote, error)

	// 订阅行情变动
	Subscribe(ctx context.Context, stockCodes []string) error
}

// QuoteCallback 行情回调函数
type QuoteCallback func(quote models.StockQuote)

// ==================== 新浪行情源 ====================

// SinaProvider 新浪行情数据源
type SinaProvider struct {
	client    *resty.Client
	logger    *zap.Logger
	cache     map[string]models.StockQuote
	mu        sync.RWMutex
	callbacks []QuoteCallback
}

// NewSinaProvider 创建新浪行情源
func NewSinaProvider(logger *zap.Logger) *SinaProvider {
	client := resty.New()
	client.SetTimeout(10 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(1 * time.Second)

	return &SinaProvider{
		client:    client,
		logger:    logger,
		cache:     make(map[string]models.StockQuote),
		callbacks: make([]QuoteCallback, 0),
	}
}

// GetQuote 获取单只股票实时行情
func (p *SinaProvider) GetQuote(ctx context.Context, stockCode string) (*models.StockQuote, error) {
	quotes, err := p.GetQuotes(ctx, []string{stockCode})
	if err != nil {
		return nil, err
	}
	quote, ok := quotes[stockCode]
	if !ok {
		return nil, fmt.Errorf("未获取到 %s 的行情数据", stockCode)
	}
	return &quote, nil
}

// GetQuotes 批量获取实时行情
func (p *SinaProvider) GetQuotes(ctx context.Context, stockCodes []string) (map[string]models.StockQuote, error) {
	result := make(map[string]models.StockQuote)

	// 转换股票代码为新浪格式: sh600000, sz000001
	sinaCodes := make([]string, 0, len(stockCodes))
	codeMap := make(map[string]string) // sina_code -> original_code
	for _, code := range stockCodes {
		sinaCode := toSinaCode(code)
		sinaCodes = append(sinaCodes, sinaCode)
		codeMap[sinaCode] = code
	}

	// 新浪行情API
	url := "https://hq.sinajs.cn/list=" + strings.Join(sinaCodes, ",")

	resp, err := p.client.R().
		SetHeader("Referer", "https://finance.sina.com.cn").
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("请求新浪行情失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("新浪行情返回异常，状态码: %d", resp.StatusCode())
	}

	// 解析新浪行情数据
	body := decodeMarketResponse(resp.Body())
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		quote, err := parseSinaQuote(line)
		if err != nil {
			p.logger.Debug("解析行情数据失败", zap.String("line", line), zap.Error(err))
			continue
		}

		// 映射回原始代码。指数代码如 sh000001 解析后也是 000001，
		// 不能再按普通股票代码推断市场，否则会误映射成 sz000001。
		originalCode := quote.StockCode
		if rawCode := extractSinaRawCode(line); rawCode != "" {
			if mapped, ok := codeMap[rawCode]; ok {
				originalCode = mapped
			}
		} else if mapped, ok := codeMap[toSinaCode(quote.StockCode)]; ok {
			originalCode = mapped
		}
		quote.StockCode = originalCode

		result[originalCode] = *quote

		// 更新缓存
		p.mu.Lock()
		p.cache[originalCode] = *quote
		p.mu.Unlock()

		// 触发回调
		for _, cb := range p.callbacks {
			cb(*quote)
		}
	}

	return result, nil
}

// GetKLines 获取K线数据
func (p *SinaProvider) GetKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error) {
	if period == "minute" || period == "1m" {
		return p.getTencentMinuteLines(ctx, stockCode, count)
	}

	klines, err := p.getEastMoneyKLines(ctx, stockCode, period, count)
	if err == nil && len(klines) > 0 {
		return klines, nil
	}
	if err != nil {
		p.logger.Warn("东方财富K线获取失败，尝试新浪K线接口", zap.String("stock", stockCode), zap.Error(err))
	}

	klines, err = p.getTencentKLines(ctx, stockCode, period, count)
	if err == nil && len(klines) > 0 {
		return klines, nil
	}
	if err != nil {
		p.logger.Warn("腾讯K线获取失败，尝试新浪K线接口", zap.String("stock", stockCode), zap.Error(err))
	}

	sinaPeriod, ok := map[string]string{
		"1m":    "5",
		"5m":    "15",
		"15m":   "30",
		"30m":   "60",
		"60m":   "120",
		"day":   "1440",
		"week":  "10080",
		"month": "43200",
	}[period]
	if !ok {
		return nil, fmt.Errorf("不支持的K线周期: %s", period)
	}

	sinaCode := toSinaCode(stockCode)
	url := fmt.Sprintf("https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData?symbol=%s&scale=%s&ma=no&datalen=%d",
		sinaCode, sinaPeriod, count)

	resp, err := p.client.R().Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取K线数据失败: %w", err)
	}

	klines, err = parseSinaKLines(decodeMarketResponse(resp.Body()), stockCode, period)
	if err != nil {
		return nil, err
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("新浪K线数据为空")
	}

	return klines, nil
}

func (p *SinaProvider) getEastMoneyKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error) {
	klt, ok := map[string]string{
		"1m":    "1",
		"5m":    "5",
		"15m":   "15",
		"30m":   "30",
		"60m":   "60",
		"day":   "101",
		"week":  "102",
		"month": "103",
	}[period]
	if !ok {
		return nil, fmt.Errorf("不支持的K线周期: %s", period)
	}

	if count <= 0 {
		count = 100
	}

	end := "20500101"
	allKLines := make([]models.KLine, 0, count)
	seen := make(map[string]bool)

	for len(allKLines) < count {
		batchSize := min(100, count-len(allKLines))
		batch, err := p.fetchEastMoneyKLineBatch(ctx, stockCode, period, klt, end, batchSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		for _, kline := range batch {
			key := kline.Timestamp.Format("2006-01-02")
			if seen[key] {
				continue
			}
			seen[key] = true
			allKLines = append(allKLines, kline)
		}

		earliest := batch[0].Timestamp
		for _, kline := range batch[1:] {
			if kline.Timestamp.Before(earliest) {
				earliest = kline.Timestamp
			}
		}
		nextEnd := earliest.AddDate(0, 0, -1).Format("20060102")
		if nextEnd == end {
			break
		}
		end = nextEnd
	}

	sort.Slice(allKLines, func(i, j int) bool {
		return allKLines[i].Timestamp.Before(allKLines[j].Timestamp)
	})

	if len(allKLines) > count {
		allKLines = allKLines[len(allKLines)-count:]
	}
	if len(allKLines) == 0 {
		return nil, fmt.Errorf("东方财富K线数据为空")
	}
	return allKLines, nil
}

func (p *SinaProvider) fetchEastMoneyKLineBatch(ctx context.Context, stockCode string, period string, klt string, end string, count int) ([]models.KLine, error) {
	url := "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "Mozilla/5.0").
		SetHeader("Referer", "https://quote.eastmoney.com/").
		SetQueryParams(map[string]string{
			"secid":   toEastMoneySecID(stockCode),
			"fields1": "f1,f2,f3,f4,f5,f6",
			"fields2": "f51,f52,f53,f54,f55,f56,f57",
			"klt":     klt,
			"fqt":     "1",
			"end":     end,
			"lmt":     strconv.Itoa(count),
		}).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求东方财富K线失败: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("东方财富K线返回异常，状态码: %d", resp.StatusCode())
	}

	return parseEastMoneyKLines(resp.String(), normalizeStockCode(stockCode), period)
}

func (p *SinaProvider) getTencentKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error) {
	txPeriod, ok := map[string]string{
		"day":   "day",
		"week":  "week",
		"month": "month",
	}[period]
	if !ok {
		return nil, fmt.Errorf("腾讯K线不支持周期: %s", period)
	}
	if count <= 0 {
		count = 100
	}

	txCode := toSinaCode(stockCode)
	url := "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "Mozilla/5.0").
		SetHeader("Referer", "https://gu.qq.com/").
		SetQueryParam("param", fmt.Sprintf("%s,%s,,,%d,qfq", txCode, txPeriod, count)).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求腾讯K线失败: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("腾讯K线返回异常，状态码: %d", resp.StatusCode())
	}

	return parseTencentKLines(resp.String(), txCode, normalizeStockCode(stockCode), period)
}

func (p *SinaProvider) getTencentMinuteLines(ctx context.Context, stockCode string, count int) ([]models.KLine, error) {
	txCode := toSinaCode(stockCode)
	url := "https://web.ifzq.gtimg.cn/appstock/app/minute/query"
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "Mozilla/5.0").
		SetHeader("Referer", "https://gu.qq.com/").
		SetQueryParam("code", txCode).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求腾讯分时失败: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("腾讯分时返回异常，状态码: %d", resp.StatusCode())
	}

	klines, err := parseTencentMinuteLines(resp.String(), txCode, normalizeStockCode(stockCode), time.Now())
	if err != nil {
		return nil, err
	}
	if count > 0 && len(klines) > count {
		klines = klines[len(klines)-count:]
	}
	return klines, nil
}

// GetIndexQuote 获取大盘指数行情
func (p *SinaProvider) GetIndexQuote(ctx context.Context, indexCode string) (*models.StockQuote, error) {
	// 主要指数代码
	indexCodes := map[string]string{
		"000001": "sh000001", // 上证指数
		"399001": "sz399001", // 深证成指
		"399006": "sz399006", // 创业板指
	}

	sinaCode, ok := indexCodes[indexCode]
	if !ok {
		sinaCode = toSinaCode(indexCode)
	}

	quotes, err := p.GetQuotes(ctx, []string{sinaCode})
	if err != nil {
		return nil, err
	}

	quote, ok := quotes[sinaCode]
	if !ok {
		return nil, fmt.Errorf("未获取到指数 %s 的行情", indexCode)
	}
	return &quote, nil
}

// Subscribe 订阅行情
func (p *SinaProvider) Subscribe(ctx context.Context, stockCodes []string) error {
	p.logger.Info("订阅行情（轮询模式）", zap.Strings("stocks", stockCodes))
	return nil
}

// OnQuote 注册行情回调
func (p *SinaProvider) OnQuote(callback QuoteCallback) {
	p.callbacks = append(p.callbacks, callback)
}

// GetCachedQuote 获取缓存的行情
func (p *SinaProvider) GetCachedQuote(stockCode string) (models.StockQuote, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	quote, ok := p.cache[stockCode]
	return quote, ok
}

// ==================== 腾讯行情源 ====================

// TencentProvider 腾讯行情数据源
type TencentProvider struct {
	client *resty.Client
	logger *zap.Logger
	cache  map[string]models.StockQuote
	mu     sync.RWMutex
}

// NewTencentProvider 创建腾讯行情源
func NewTencentProvider(logger *zap.Logger) *TencentProvider {
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	return &TencentProvider{
		client: client,
		logger: logger,
		cache:  make(map[string]models.StockQuote),
	}
}

// GetQuote 获取单只股票行情
func (p *TencentProvider) GetQuote(ctx context.Context, stockCode string) (*models.StockQuote, error) {
	quotes, err := p.GetQuotes(ctx, []string{stockCode})
	if err != nil {
		return nil, err
	}
	quote, ok := quotes[stockCode]
	if !ok {
		return nil, fmt.Errorf("未获取到 %s 的行情数据", stockCode)
	}
	return &quote, nil
}

// GetQuotes 批量获取行情
func (p *TencentProvider) GetQuotes(ctx context.Context, stockCodes []string) (map[string]models.StockQuote, error) {
	result := make(map[string]models.StockQuote)

	// 转换为腾讯格式: sh600000, sz000001
	txCodes := make([]string, 0, len(stockCodes))
	codeMap := make(map[string]string)
	for _, code := range stockCodes {
		txCode := toSinaCode(code)
		txCodes = append(txCodes, txCode) // 格式与新浪相同
		codeMap[txCode] = code
	}

	url := "https://qt.gtimg.cn/q=" + strings.Join(txCodes, ",")

	resp, err := p.client.R().Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求腾讯行情失败: %w", err)
	}

	body := decodeMarketResponse(resp.Body())
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "~") {
			continue
		}

		quote, err := parseTencentQuote(line)
		if err != nil {
			p.logger.Debug("解析腾讯行情失败", zap.Error(err))
			continue
		}

		originalCode := quote.StockCode
		if rawCode := extractTencentRawCode(line); rawCode != "" {
			if mapped, ok := codeMap[rawCode]; ok {
				originalCode = mapped
			}
		}
		quote.StockCode = originalCode

		result[originalCode] = *quote
		p.mu.Lock()
		p.cache[originalCode] = *quote
		p.mu.Unlock()
	}

	return result, nil
}

// GetKLines 获取K线
func (p *TencentProvider) GetKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error) {
	// 使用新浪的K线接口作为备用
	sinaProvider := NewSinaProvider(p.logger)
	return sinaProvider.GetKLines(ctx, stockCode, period, count)
}

// GetIndexQuote 获取指数行情
func (p *TencentProvider) GetIndexQuote(ctx context.Context, indexCode string) (*models.StockQuote, error) {
	indexCodes := map[string]string{
		"000001": "sh000001",
		"399001": "sz399001",
		"399006": "sz399006",
	}

	sinaCode, ok := indexCodes[indexCode]
	if !ok {
		sinaCode = toSinaCode(indexCode)
	}
	quotes, err := p.GetQuotes(ctx, []string{sinaCode})
	if err != nil {
		return nil, err
	}
	quote, ok := quotes[sinaCode]
	if !ok {
		return nil, fmt.Errorf("未获取到指数行情")
	}
	return &quote, nil
}

// Subscribe 订阅行情
func (p *TencentProvider) Subscribe(ctx context.Context, stockCodes []string) error {
	return nil
}

// ==================== 行情服务 ====================

// MarketService 行情服务 - 统一管理行情数据
type MarketService struct {
	provider      DataProvider
	logger        *zap.Logger
	subscriptions map[string]bool
	klineCache    map[string]cachedKLines
	quoteChan     chan models.StockQuote
	mu            sync.RWMutex
	stopCh        chan struct{}
}

type cachedKLines struct {
	rows      []models.KLine
	expiresAt time.Time
}

// NewMarketService 创建行情服务
func NewMarketService(dataSource string, logger *zap.Logger) *MarketService {
	var provider DataProvider
	switch dataSource {
	case "sina":
		provider = NewSinaProvider(logger)
	case "tencent":
		provider = NewTencentProvider(logger)
	default:
		logger.Warn("未知行情源，使用新浪", zap.String("source", dataSource))
		provider = NewSinaProvider(logger)
	}

	return &MarketService{
		provider:      provider,
		logger:        logger,
		subscriptions: make(map[string]bool),
		klineCache:    make(map[string]cachedKLines),
		quoteChan:     make(chan models.StockQuote, 1000),
		stopCh:        make(chan struct{}),
	}
}

// GetQuote 获取实时行情
func (s *MarketService) GetQuote(ctx context.Context, stockCode string) (*models.StockQuote, error) {
	return s.provider.GetQuote(ctx, stockCode)
}

// GetQuotes 批量获取行情
func (s *MarketService) GetQuotes(ctx context.Context, stockCodes []string) (map[string]models.StockQuote, error) {
	return s.provider.GetQuotes(ctx, stockCodes)
}

// GetKLines 获取K线
func (s *MarketService) GetKLines(ctx context.Context, stockCode string, period string, count int) ([]models.KLine, error) {
	key := klineCacheKey(stockCode, period, count)
	now := time.Now()
	s.mu.RLock()
	if cached, ok := s.klineCache[key]; ok && now.Before(cached.expiresAt) {
		rows := cloneKLines(cached.rows)
		s.mu.RUnlock()
		return rows, nil
	}
	s.mu.RUnlock()

	rows, err := s.provider.GetKLines(ctx, stockCode, period, count)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.klineCache[key] = cachedKLines{
		rows:      cloneKLines(rows),
		expiresAt: now.Add(2 * time.Minute),
	}
	s.mu.Unlock()
	return rows, nil
}

func klineCacheKey(stockCode string, period string, count int) string {
	return normalizeStockCode(stockCode) + "|" + strings.TrimSpace(period) + "|" + strconv.Itoa(count)
}

func cloneKLines(rows []models.KLine) []models.KLine {
	if len(rows) == 0 {
		return nil
	}
	cloned := make([]models.KLine, len(rows))
	copy(cloned, rows)
	return cloned
}

// GetIndexQuote 获取指数行情
func (s *MarketService) GetIndexQuote(ctx context.Context, indexCode string) (*models.StockQuote, error) {
	return s.provider.GetIndexQuote(ctx, indexCode)
}

// Subscribe 订阅行情
func (s *MarketService) Subscribe(ctx context.Context, stockCodes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, code := range stockCodes {
		s.subscriptions[code] = true
	}

	return s.provider.Subscribe(ctx, stockCodes)
}

// QuoteChannel 获取行情推送通道
func (s *MarketService) QuoteChannel() <-chan models.StockQuote {
	return s.quoteChan
}

// StartPolling 启动轮询行情
func (s *MarketService) StartPolling(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info("行情轮询已启动", zap.Duration("interval", interval))

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("行情轮询已停止")
			return
		case <-s.stopCh:
			s.logger.Info("行情轮询已停止")
			return
		case <-ticker.C:
			s.pollQuotes(ctx)
		}
	}
}

// Stop 停止行情服务
func (s *MarketService) Stop() {
	close(s.stopCh)
}

func (s *MarketService) pollQuotes(ctx context.Context) {
	s.mu.RLock()
	codes := make([]string, 0, len(s.subscriptions))
	for code := range s.subscriptions {
		codes = append(codes, code)
	}
	s.mu.RUnlock()

	if len(codes) == 0 {
		return
	}

	// 检查是否在交易时间
	if !isTradingTime() {
		return
	}

	quotes, err := s.provider.GetQuotes(ctx, codes)
	if err != nil {
		s.logger.Error("轮询行情失败", zap.Error(err))
		return
	}

	// 推送到通道
	for _, quote := range quotes {
		select {
		case s.quoteChan <- quote:
		default:
			// 通道满了丢弃旧数据
			s.logger.Debug("行情通道已满，丢弃数据", zap.String("stock", quote.StockCode))
		}
	}
}

// ==================== 工具函数 ====================

// toSinaCode 将股票代码转换为新浪格式
func toSinaCode(code string) string {
	code = strings.TrimSpace(code)
	code = strings.ToLower(code)

	// 已经是新浪格式
	if strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") {
		return code
	}

	// 纯数字代码
	code = strings.TrimPrefix(code, "sh")
	code = strings.TrimPrefix(code, "sz")

	if len(code) != 6 {
		return code
	}

	// 上海: 6开头
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") {
		return "sh" + code
	}
	// 深圳: 0开头, 3开头
	return "sz" + code
}

func normalizeStockCode(code string) string {
	code = strings.TrimSpace(strings.ToLower(code))
	code = strings.TrimPrefix(code, "sh")
	code = strings.TrimPrefix(code, "sz")
	return code
}

func toEastMoneySecID(code string) string {
	sinaCode := toSinaCode(code)
	if strings.HasPrefix(sinaCode, "sh") {
		return "1." + strings.TrimPrefix(sinaCode, "sh")
	}
	return "0." + strings.TrimPrefix(sinaCode, "sz")
}

func extractSinaRawCode(line string) string {
	parts := strings.SplitN(line, "=\"", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(parts[0]), "var hq_str_")
}

func extractTencentRawCode(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) == 2 {
		raw := strings.TrimSpace(parts[0])
		raw = strings.TrimPrefix(raw, "v_")
		return raw
	}

	fields := strings.Split(line, "~")
	if len(fields) > 0 {
		return strings.TrimPrefix(fields[0], "v_")
	}
	return ""
}

// parseSinaQuote 解析新浪行情数据
// 格式: var hq_str_sh600000="浦发银行,15.23,15.20,..."
func parseSinaQuote(line string) (*models.StockQuote, error) {
	parts := strings.SplitN(line, "=\"", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("无效的新浪行情格式")
	}

	// 提取股票代码
	rawCode := strings.TrimPrefix(parts[0], "var hq_str_")

	// 提取数据部分
	data := strings.TrimRight(parts[1], "\";")
	if data == "" {
		return nil, fmt.Errorf("空数据")
	}

	fields := strings.Split(data, ",")
	if len(fields) < 32 {
		return nil, fmt.Errorf("行情数据字段不足: 需要32个字段，实际%d个", len(fields))
	}

	quote := &models.StockQuote{
		StockName: fields[0],
		Open:      safeDecimal(fields[1]),
		PreClose:  safeDecimal(fields[2]),
		Close:     safeDecimal(fields[3]),
		High:      safeDecimal(fields[4]),
		Low:       safeDecimal(fields[5]),
		Volume:    safeInt64(fields[8]),
		Amount:    safeDecimal(fields[9]),
	}

	// 解析市场
	if strings.HasPrefix(rawCode, "sh") {
		quote.Market = models.MarketSH
		quote.StockCode = strings.TrimPrefix(rawCode, "sh")
	} else if strings.HasPrefix(rawCode, "sz") {
		quote.Market = models.MarketSZ
		quote.StockCode = strings.TrimPrefix(rawCode, "sz")
	} else {
		quote.StockCode = rawCode
	}

	// 解析买卖盘
	if len(fields) > 29 {
		for i := 0; i < 5; i++ {
			quote.BidPrices[i] = safeDecimal(fields[11+i*2])
			quote.BidVolumes[i] = safeInt64(fields[10+i*2])
			quote.AskPrices[i] = safeDecimal(fields[21+i*2])
			quote.AskVolumes[i] = safeInt64(fields[20+i*2])
		}
	}

	// 解析时间
	if len(fields) >= 32 {
		dateStr := fields[30] + " " + fields[31]
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateStr, time.Local); err == nil {
			quote.Timestamp = t
		} else {
			quote.Timestamp = time.Now()
		}
	} else {
		quote.Timestamp = time.Now()
	}

	return quote, nil
}

// parseTencentQuote 解析腾讯行情数据
func parseTencentQuote(line string) (*models.StockQuote, error) {
	parts := strings.Split(line, "~")
	if len(parts) < 45 {
		return nil, fmt.Errorf("腾讯行情数据字段不足")
	}

	quote := &models.StockQuote{
		StockCode: parts[2],
		StockName: parts[1],
		Close:     safeDecimal(parts[3]),
		PreClose:  safeDecimal(parts[4]),
		Open:      safeDecimal(parts[5]),
		Volume:    safeInt64(parts[6]),
		High:      safeDecimal(parts[33]),
		Low:       safeDecimal(parts[34]),
		Amount:    safeDecimal(parts[37]),
		Turnover:  safeDecimal(parts[38]),
		PE:        safeDecimal(parts[39]),
	}

	if parts[0] != "" {
		codePart := parts[2]
		if strings.HasPrefix(parts[0], "sh") || strings.HasPrefix(codePart, "6") || strings.HasPrefix(codePart, "9") {
			quote.Market = models.MarketSH
		} else {
			quote.Market = models.MarketSZ
		}
	}

	quote.Timestamp = time.Now()
	return quote, nil
}

// parseSinaKLines 解析新浪K线数据
func parseSinaKLines(data string, stockCode string, period string) ([]models.KLine, error) {
	if strings.HasPrefix(strings.TrimSpace(data), "[") {
		return parseSinaJSONKLines(data, stockCode, period)
	}

	var klines []models.KLine

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 新浪K线JSON格式
		// 简化处理：按CSV格式解析
		fields := strings.Split(line, ",")
		if len(fields) < 7 {
			continue
		}

		kline := models.KLine{
			StockCode: stockCode,
			Period:    period,
			Open:      safeDecimal(fields[1]),
			High:      safeDecimal(fields[2]),
			Low:       safeDecimal(fields[3]),
			Close:     safeDecimal(fields[4]),
			Volume:    safeInt64(fields[5]),
		}

		if t, err := time.Parse("2006-01-02", fields[0]); err == nil {
			kline.Timestamp = t
		}

		klines = append(klines, kline)
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("新浪K线数据为空")
	}
	return klines, nil
}

type sinaKLineItem struct {
	Day    string `json:"day"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

func parseSinaJSONKLines(data string, stockCode string, period string) ([]models.KLine, error) {
	var rows []sinaKLineItem
	if err := json.Unmarshal([]byte(data), &rows); err != nil {
		return nil, fmt.Errorf("解析新浪K线JSON失败: %w", err)
	}

	klines := make([]models.KLine, 0, len(rows))
	for _, row := range rows {
		t, err := parseKLineTime(row.Day)
		if err != nil {
			continue
		}
		klines = append(klines, models.KLine{
			StockCode: normalizeStockCode(stockCode),
			Period:    period,
			Open:      safeDecimal(row.Open),
			High:      safeDecimal(row.High),
			Low:       safeDecimal(row.Low),
			Close:     safeDecimal(row.Close),
			Volume:    safeInt64(row.Volume),
			Timestamp: t,
		})
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("新浪K线数据为空")
	}
	return klines, nil
}

type tencentKLineResponse struct {
	Code int                        `json:"code"`
	Msg  string                     `json:"msg"`
	Data map[string]json.RawMessage `json:"data"`
}

func parseTencentKLines(data string, txCode string, stockCode string, period string) ([]models.KLine, error) {
	var resp tencentKLineResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("解析腾讯K线JSON失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("腾讯K线返回异常: %s", resp.Msg)
	}

	periodKey := map[string]string{
		"day":   "qfqday",
		"week":  "qfqweek",
		"month": "qfqmonth",
	}[period]
	rawStock, ok := resp.Data[txCode]
	if !ok {
		return nil, fmt.Errorf("腾讯K线数据为空")
	}

	var stockData map[string]json.RawMessage
	if err := json.Unmarshal(rawStock, &stockData); err != nil {
		return nil, fmt.Errorf("解析腾讯K线股票数据失败: %w", err)
	}

	var rawRows []json.RawMessage
	if err := json.Unmarshal(stockData[periodKey], &rawRows); err != nil {
		return nil, fmt.Errorf("解析腾讯K线%s失败: %w", periodKey, err)
	}
	if len(rawRows) == 0 {
		return nil, fmt.Errorf("腾讯K线数据为空")
	}

	klines := make([]models.KLine, 0, len(rawRows))
	for _, rawRow := range rawRows {
		var fields []interface{}
		if err := json.Unmarshal(rawRow, &fields); err != nil {
			continue
		}
		if len(fields) < 6 {
			continue
		}

		t, err := parseKLineTime(jsonValueToString(fields[0]))
		if err != nil {
			continue
		}
		klines = append(klines, models.KLine{
			StockCode: stockCode,
			Period:    period,
			Open:      safeDecimal(jsonValueToString(fields[1])),
			Close:     safeDecimal(jsonValueToString(fields[2])),
			High:      safeDecimal(jsonValueToString(fields[3])),
			Low:       safeDecimal(jsonValueToString(fields[4])),
			Volume:    safeDecimal(jsonValueToString(fields[5])).IntPart(),
			Timestamp: t,
		})
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("腾讯K线数据为空")
	}
	return klines, nil
}

type tencentMinuteStockData struct {
	Data struct {
		Rows []string `json:"data"`
	} `json:"data"`
}

func parseTencentMinuteLines(data string, txCode string, stockCode string, tradingDay time.Time) ([]models.KLine, error) {
	var resp tencentKLineResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("解析腾讯分时JSON失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("腾讯分时返回异常: %s", resp.Msg)
	}

	rawStock, ok := resp.Data[txCode]
	if !ok {
		return nil, fmt.Errorf("腾讯分时数据为空")
	}

	var stockData tencentMinuteStockData
	if err := json.Unmarshal(rawStock, &stockData); err != nil {
		return nil, fmt.Errorf("解析腾讯分时股票数据失败: %w", err)
	}
	if len(stockData.Data.Rows) == 0 {
		return nil, fmt.Errorf("腾讯分时数据为空")
	}

	day := localDate(tradingDay)
	klines := make([]models.KLine, 0, len(stockData.Data.Rows))
	var prevVolume int64
	prevAmount := decimal.Zero

	for _, row := range stockData.Data.Rows {
		fields := strings.Fields(row)
		if len(fields) < 3 {
			continue
		}

		t, err := parseMinuteTime(day, fields[0])
		if err != nil {
			continue
		}

		price := safeDecimal(fields[1])
		cumulativeVolume := safeInt64(fields[2])
		volume := cumulativeVolume - prevVolume
		if volume < 0 {
			volume = cumulativeVolume
		}
		prevVolume = cumulativeVolume

		amount := decimal.Zero
		if len(fields) > 3 {
			cumulativeAmount := safeDecimal(fields[3])
			amount = cumulativeAmount.Sub(prevAmount)
			if amount.LessThan(decimal.Zero) {
				amount = cumulativeAmount
			}
			prevAmount = cumulativeAmount
		}

		klines = append(klines, models.KLine{
			StockCode: stockCode,
			Period:    "minute",
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    volume,
			Amount:    amount,
			Timestamp: t,
		})
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("腾讯分时数据为空")
	}
	return klines, nil
}

func parseMinuteTime(day time.Time, value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if len(value) != 4 {
		return time.Time{}, fmt.Errorf("无效分时时间: %s", value)
	}
	hour, err := strconv.Atoi(value[:2])
	if err != nil {
		return time.Time{}, err
	}
	minute, err := strconv.Atoi(value[2:])
	if err != nil {
		return time.Time{}, err
	}

	y, m, d := day.In(time.Local).Date()
	return time.Date(y, m, d, hour, minute, 0, 0, time.Local), nil
}

func localDate(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func jsonValueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

type eastMoneyKLineResponse struct {
	Data *struct {
		KLines []string `json:"klines"`
	} `json:"data"`
}

func parseEastMoneyKLines(data string, stockCode string, period string) ([]models.KLine, error) {
	var resp eastMoneyKLineResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("解析东方财富K线JSON失败: %w", err)
	}
	if resp.Data == nil || len(resp.Data.KLines) == 0 {
		return nil, fmt.Errorf("东方财富K线数据为空")
	}

	klines := make([]models.KLine, 0, len(resp.Data.KLines))
	for _, row := range resp.Data.KLines {
		fields := strings.Split(row, ",")
		if len(fields) < 6 {
			continue
		}

		t, err := parseKLineTime(fields[0])
		if err != nil {
			continue
		}

		kline := models.KLine{
			StockCode: normalizeStockCode(stockCode),
			Period:    period,
			Open:      safeDecimal(fields[1]),
			Close:     safeDecimal(fields[2]),
			High:      safeDecimal(fields[3]),
			Low:       safeDecimal(fields[4]),
			Volume:    safeInt64(fields[5]),
			Timestamp: t,
		}
		if len(fields) > 6 {
			kline.Amount = safeDecimal(fields[6])
		}
		klines = append(klines, kline)
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("东方财富K线数据为空")
	}
	return klines, nil
}

func parseKLineTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02 15:04") {
		if t, err := time.ParseInLocation("2006-01-02 15:04", value[:len("2006-01-02 15:04")], time.Local); err == nil {
			return t, nil
		}
	}
	return time.ParseInLocation("2006-01-02", value[:min(len(value), len("2006-01-02"))], time.Local)
}

// isTradingTime 检查是否在A股交易时间
func isTradingTime() bool {
	now := time.Now()

	// 周末不交易
	weekday := now.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}

	hour, minute := now.Hour(), now.Minute()
	timeVal := hour*100 + minute

	// 9:15 - 11:30, 13:00 - 15:00
	if (timeVal >= 915 && timeVal <= 1130) || (timeVal >= 1300 && timeVal <= 1500) {
		return true
	}

	return false
}

// safeDecimal 安全解析decimal
func safeDecimal(s string) decimal.Decimal {
	s = strings.TrimSpace(s)
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

// safeInt64 安全解析int64
func safeInt64(s string) int64 {
	s = strings.TrimSpace(s)
	// 去除可能的引号
	s = strings.Trim(s, "\"")
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func decodeMarketResponse(body []byte) string {
	if utf8.Valid(body) {
		return string(body)
	}

	decoded, err := simplifiedchinese.GB18030.NewDecoder().String(string(body))
	if err != nil {
		return string(body)
	}
	return decoded
}
