import React, {useEffect, useRef, useState} from 'react';
import {Button, Card, Col, Descriptions, Input, message, Row, Space, Spin, Table, Tabs, Tag} from 'antd';
import {ReloadOutlined, SearchOutlined, StarFilled, StarOutlined} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import {getIndexQuote, getKLines, getQuote, getQuotes} from '../services/api';
import {formatNumber, formatVolume} from '../utils/format';

const { Search } = Input;

const POPULAR_STOCKS = [
  { code: '000001', name: '平安银行' },
  { code: '600519', name: '贵州茅台' },
  { code: '000858', name: '五粮液' },
  { code: '600036', name: '招商银行' },
  { code: '601318', name: '中国平安' },
  { code: '000333', name: '美的集团' },
  { code: '600276', name: '恒瑞医药' },
  { code: '002714', name: '牧原股份' },
  { code: '601888', name: '中国中免' },
  { code: '300750', name: '宁德时代' },
];

const INDEX_CODES = {
  '上证指数': '000001',
  '深证成指': '399001',
  '创业板指': '399006',
};

function normalizeMarketCode(value) {
  return String(value || '')
    .trim()
    .replace(/^(sh|sz)/i, '')
    .replace(/\D/g, '')
    .slice(0, 6);
}

function formatKLineTime(timestamp, period) {
  const d = new Date(timestamp);
  if (period === 'minute') {
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function calculateMACD(closes) {
  let ema12 = 0;
  let ema26 = 0;
  let dea = 0;
  const shortAlpha = 2 / (12 + 1);
  const longAlpha = 2 / (26 + 1);
  const signalAlpha = 2 / (9 + 1);

  return closes.map((rawValue, index) => {
    const value = Number.isFinite(rawValue) ? rawValue : 0;
    if (index === 0) {
      ema12 = value;
      ema26 = value;
    } else {
      ema12 = value * shortAlpha + ema12 * (1 - shortAlpha);
      ema26 = value * longAlpha + ema26 * (1 - longAlpha);
    }

    const dif = ema12 - ema26;
    dea = index === 0 ? dif : dif * signalAlpha + dea * (1 - signalAlpha);
    const macd = (dif - dea) * 2;

    return {
      dif: Number(dif.toFixed(4)),
      dea: Number(dea.toFixed(4)),
      macd: Number(macd.toFixed(4)),
    };
  });
}

export default function MarketPage() {
  const [messageApi, contextHolder] = message.useMessage();
  const [searchCode, setSearchCode] = useState('');
  const [quotes, setQuotes] = useState([]);
  const [selectedStock, setSelectedStock] = useState(null);
  const [klineData, setKlineData] = useState(null);
  const [indexQuotes, setIndexQuotes] = useState({});
  const [loading, setLoading] = useState(false);
  const [klineLoading, setKlineLoading] = useState(false);
  const [favorites, setFavorites] = useState(() => {
    try { return JSON.parse(localStorage.getItem('favorites') || '[]'); } catch { return []; }
  });
  const [klinePeriod, setKlinePeriod] = useState('day');
  const timerRef = useRef(null);

  useEffect(() => {
    fetchIndexQuotes();
    fetchPopularQuotes();
    return () => { if (timerRef.current) clearInterval(timerRef.current); };
  }, []);

  useEffect(() => {
    if (favorites.length > 0) {
      fetchFavoriteQuotes();
    }
  }, [favorites]);

  useEffect(() => {
    if (selectedStock?.stock_code) {
      fetchKLine(selectedStock.stock_code);
    }
  }, [klinePeriod]);

  const fetchIndexQuotes = async () => {
    try {
      const results = {};
      for (const [name, code] of Object.entries(INDEX_CODES)) {
        try {
          const data = await getIndexQuote(code);
          results[name] = data;
        } catch {}
      }
      setIndexQuotes(results);
    } catch {}
  };

  const fetchPopularQuotes = async () => {
    setLoading(true);
    try {
      const codes = POPULAR_STOCKS.map(s => s.code);
      const data = await getQuotes(codes);
      setQuotes(data || []);
    } catch {}
    setLoading(false);
  };

  const fetchFavoriteQuotes = async () => {
    try {
      if (favorites.length > 0) {
        const data = await getQuotes(favorites);
        // Merge with popular
        setQuotes(prev => {
          const map = new Map();
          [...prev, ...(data || [])].forEach(q => map.set(q.stock_code, q));
          return Array.from(map.values());
        });
      }
    } catch {}
  };

  const fetchKLine = async (code) => {
    setKlineLoading(true);
    try {
      const data = await getKLines(code, klinePeriod);
      setKlineData(data || []);
      const quoteData = await getQuote(code);
      setSelectedStock(quoteData);
    } catch (e) {
      messageApi.error('获取行情数据失败');
    }
    setKlineLoading(false);
  };

  const handleSearch = (code) => {
    if (!code.trim()) return;
    const cleanCode = normalizeMarketCode(code);
    if (cleanCode.length !== 6) {
      messageApi.warning('请输入6位股票代码');
      return;
    }
    setSearchCode(cleanCode);
    fetchKLine(cleanCode);
  };

  const toggleFavorite = (code) => {
    const newFavs = favorites.includes(code)
      ? favorites.filter(c => c !== code)
      : [...favorites, code];
    setFavorites(newFavs);
    localStorage.setItem('favorites', JSON.stringify(newFavs));
  };

  // K线/分时图配置
  const getKLineOption = () => {
    if (!klineData || klineData.length === 0) return {};

    const isMinute = klinePeriod === 'minute';
    const dates = klineData.map(k => formatKLineTime(k.timestamp, klinePeriod));
    const ohlc = klineData.map(k => [parseFloat(k.open), parseFloat(k.close), parseFloat(k.low), parseFloat(k.high)]);
    const closes = klineData.map(k => parseFloat(k.close || 0));
    const volumes = klineData.map(k => parseFloat(k.volume || 0));
    const macd = calculateMACD(closes);
    const zoomStart = klineData.length > 120 ? 55 : 0;
    const priceSeriesName = isMinute ? '分时' : 'K线';

    return {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
      },
      legend: { data: [priceSeriesName, '成交量', 'DIF', 'DEA', 'MACD'], top: 5 },
      grid: [
        { left: '8%', right: '3%', top: 42, height: '45%' },
        { left: '8%', right: '3%', top: '60%', height: '12%' },
        { left: '8%', right: '3%', top: '76%', height: '12%' },
      ],
      xAxis: [
        { type: 'category', data: dates, gridIndex: 0, boundaryGap: true, axisLabel: { show: false } },
        { type: 'category', data: dates, gridIndex: 1, boundaryGap: true, axisLabel: { show: false } },
        { type: 'category', data: dates, gridIndex: 2, boundaryGap: true },
      ],
      yAxis: [
        { scale: true, gridIndex: 0, splitArea: { show: true } },
        { scale: true, gridIndex: 1, splitNumber: 2 },
        { scale: true, gridIndex: 2, splitNumber: 2 },
      ],
      dataZoom: [
        { type: 'inside', xAxisIndex: [0, 1, 2], start: zoomStart, end: 100 },
        { show: true, xAxisIndex: [0, 1, 2], type: 'slider', bottom: 5, start: zoomStart, end: 100 },
      ],
      series: [
        isMinute ? {
          name: priceSeriesName,
          type: 'line',
          data: closes,
          xAxisIndex: 0,
          yAxisIndex: 0,
          showSymbol: false,
          smooth: true,
          lineStyle: { color: '#1677ff', width: 2 },
          areaStyle: { color: 'rgba(22, 119, 255, 0.08)' },
        } : {
          name: priceSeriesName,
          type: 'candlestick',
          data: ohlc,
          xAxisIndex: 0,
          yAxisIndex: 0,
          itemStyle: {
            color: '#f5222d',
            color0: '#52c41a',
            borderColor: '#f5222d',
            borderColor0: '#52c41a',
          },
        },
        {
          name: '成交量',
          type: 'bar',
          data: volumes,
          xAxisIndex: 1,
          yAxisIndex: 1,
          itemStyle: {
            color: function (params) {
              const idx = params.dataIndex;
              if (isMinute) {
                const prev = idx > 0 ? closes[idx - 1] : closes[idx];
                return closes[idx] >= prev ? '#f5222d' : '#52c41a';
              }
              return ohlc[idx] && ohlc[idx][1] >= ohlc[idx][0] ? '#f5222d' : '#52c41a';
            },
          },
        },
        {
          name: 'MACD',
          type: 'bar',
          data: macd.map(item => item.macd),
          xAxisIndex: 2,
          yAxisIndex: 2,
          itemStyle: {
            color: function (params) {
              return params.value >= 0 ? '#f5222d' : '#52c41a';
            },
          },
        },
        {
          name: 'DIF',
          type: 'line',
          data: macd.map(item => item.dif),
          xAxisIndex: 2,
          yAxisIndex: 2,
          showSymbol: false,
          lineStyle: { color: '#faad14', width: 1.5 },
        },
        {
          name: 'DEA',
          type: 'line',
          data: macd.map(item => item.dea),
          xAxisIndex: 2,
          yAxisIndex: 2,
          showSymbol: false,
          lineStyle: { color: '#722ed1', width: 1.5 },
        },
      ],
    };
  };

  const quoteColumns = [
    {
      title: '股票', key: 'stock', render: (_, r) => (
        <div>
          <div style={{ fontWeight: 500 }}>{r.stock_name || r.stock_code}</div>
          <div style={{ fontSize: 12, color: '#8c8c8c' }}>{r.stock_code}</div>
        </div>
      ),
    },
    {
      title: '最新价', dataIndex: 'close', key: 'close', align: 'right',
      render: v => <span style={{ fontWeight: 600, fontSize: 15 }}>{formatNumber(v)}</span>,
    },
    {
      title: '涨跌幅', key: 'change', align: 'right',
      render: (_, r) => {
        const close = parseFloat(r.close || 0);
        const preClose = parseFloat(r.pre_close || 1);
        const pct = ((close - preClose) / preClose * 100);
        const color = pct > 0 ? '#f5222d' : pct < 0 ? '#52c41a' : '#8c8c8c';
        return (
          <span style={{ color, fontWeight: 500 }}>
            {pct > 0 ? '+' : ''}{pct.toFixed(2)}%
          </span>
        );
      },
    },
    {
      title: '成交量', dataIndex: 'volume', key: 'volume', align: 'right',
      render: v => formatVolume(v),
    },
    {
      title: '操作', key: 'action', width: 120, align: 'center',
      render: (_, r) => (
        <Space>
          <Button size="small" type="link" onClick={() => fetchKLine(r.stock_code)}>K线</Button>
          <Button
            size="small"
            type="link"
            icon={favorites.includes(r.stock_code) ? <StarFilled style={{ color: '#faad14' }} /> : <StarOutlined />}
            onClick={() => toggleFavorite(r.stock_code)}
          />
        </Space>
      ),
    },
  ];

  return (
    <div>
      {contextHolder}
      {/* 指数概览 */}
      <Row gutter={[16, 16]}>
        {Object.entries(indexQuotes).map(([name, data]) => {
          const close = parseFloat(data?.close || 0);
          const preClose = parseFloat(data?.pre_close || 1);
          const change = close - preClose;
          const pct = (change / preClose * 100);
          const isUp = change >= 0;
          return (
            <Col xs={24} sm={8} key={name}>
              <Card size="small" hoverable>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div>
                    <div style={{ fontSize: 13, color: '#8c8c8c' }}>{name}</div>
                    <div style={{ fontSize: 22, fontWeight: 600, color: isUp ? '#f5222d' : '#52c41a' }}>
                      {formatNumber(close)}
                    </div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{ color: isUp ? '#f5222d' : '#52c41a', fontWeight: 500 }}>
                      {isUp ? '+' : ''}{change.toFixed(2)}
                    </div>
                    <Tag color={isUp ? 'red' : 'green'} style={{ margin: 0 }}>
                      {isUp ? '+' : ''}{pct.toFixed(2)}%
                    </Tag>
                  </div>
                </div>
              </Card>
            </Col>
          );
        })}
      </Row>

      {/* 搜索 */}
      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card size="small">
            <Space>
              <Search
                placeholder="输入股票代码，如 600519"
                value={searchCode}
                onChange={e => setSearchCode(e.target.value)}
                onSearch={handleSearch}
                onPressEnter={e => handleSearch(e.currentTarget.value)}
                style={{ width: 300 }}
                enterButton={<><SearchOutlined /> 查询</>}
              />
              <Button icon={<ReloadOutlined />} onClick={() => { fetchPopularQuotes(); fetchIndexQuotes(); }}>
                刷新
              </Button>
            </Space>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        {/* 行情列表 */}
        <Col xs={24} md={10}>
          <Card title="热门行情" size="small" extra={
            <Button size="small" type="link" onClick={fetchPopularQuotes}>刷新</Button>
          }>
            <Spin spinning={loading}>
              <Table
                dataSource={quotes}
                columns={quoteColumns}
                rowKey="stock_code"
                size="small"
                pagination={{ pageSize: 10 }}
                onRow={(r) => ({ onClick: () => fetchKLine(r.stock_code), style: { cursor: 'pointer' } })}
              />
            </Spin>
          </Card>
        </Col>

        {/* K线图 */}
        <Col xs={24} md={14}>
          <Card
            title={selectedStock ? `${selectedStock.stock_name || selectedStock.stock_code} ${klinePeriod === 'minute' ? '分时图' : 'K线图'}` : '行情图'}
            size="small"
            extra={
              <Tabs
                size="small"
                activeKey={klinePeriod}
                onChange={setKlinePeriod}
                items={[
                  { key: 'minute', label: '分时' },
                  { key: 'day', label: '日K' },
                  { key: 'week', label: '周K' },
                  { key: 'month', label: '月K' },
                ]}
              />
            }
          >
            {selectedStock && (
              <Descriptions size="small" column={4} style={{ marginBottom: 12 }}>
                <Descriptions.Item label="最新价">
                  <span style={{ fontWeight: 600, fontSize: 16 }}>{formatNumber(selectedStock.close)}</span>
                </Descriptions.Item>
                <Descriptions.Item label="今开">{formatNumber(selectedStock.open)}</Descriptions.Item>
                <Descriptions.Item label="最高">
                  <span style={{ color: '#f5222d' }}>{formatNumber(selectedStock.high)}</span>
                </Descriptions.Item>
                <Descriptions.Item label="最低">
                  <span style={{ color: '#52c41a' }}>{formatNumber(selectedStock.low)}</span>
                </Descriptions.Item>
              </Descriptions>
            )}
            <Spin spinning={klineLoading}>
              {klineData && klineData.length > 0 ? (
                <ReactECharts option={getKLineOption()} style={{ height: 450 }} />
              ) : (
                <div style={{ height: 450, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#bfbfbf' }}>
                  请点击左侧股票查看K线图
                </div>
              )}
            </Spin>
          </Card>
        </Col>
      </Row>
    </div>
  );
}
