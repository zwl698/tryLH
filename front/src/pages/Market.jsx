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

export default function MarketPage() {
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
      message.error('获取行情数据失败');
    }
    setKlineLoading(false);
  };

  const handleSearch = (code) => {
    if (!code.trim()) return;
    const cleanCode = code.trim().replace(/[a-zA-Z]/g, '');
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

  // K线图配置
  const getKLineOption = () => {
    if (!klineData || klineData.length === 0) return {};

    const dates = klineData.map(k => {
      const d = new Date(k.timestamp);
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    });
    const ohlc = klineData.map(k => [parseFloat(k.open), parseFloat(k.close), parseFloat(k.low), parseFloat(k.high)]);
    const volumes = klineData.map(k => parseFloat(k.volume || 0));

    return {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
      },
      legend: { data: ['K线', '成交量'], top: 5 },
      grid: [
        { left: '8%', right: '3%', top: 40, height: '55%' },
        { left: '8%', right: '3%', top: '72%', height: '18%' },
      ],
      xAxis: [
        { type: 'category', data: dates, gridIndex: 0, boundaryGap: true, axisLabel: { show: false } },
        { type: 'category', data: dates, gridIndex: 1, boundaryGap: true },
      ],
      yAxis: [
        { scale: true, gridIndex: 0, splitArea: { show: true } },
        { scale: true, gridIndex: 1, splitNumber: 2 },
      ],
      dataZoom: [
        { type: 'inside', xAxisIndex: [0, 1], start: 60, end: 100 },
        { show: true, xAxisIndex: [0, 1], type: 'slider', bottom: 5, start: 60, end: 100 },
      ],
      series: [
        {
          name: 'K线',
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
              return ohlc[idx] && ohlc[idx][1] >= ohlc[idx][0] ? '#f5222d' : '#52c41a';
            },
          },
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
            title={selectedStock ? `${selectedStock.stock_name || selectedStock.stock_code} K线图` : 'K线图'}
            size="small"
            extra={
              <Tabs
                size="small"
                activeKey={klinePeriod}
                onChange={setKlinePeriod}
                items={[
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

