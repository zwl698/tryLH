import React, {useEffect, useRef, useState} from 'react';
import {
    Button,
    Card,
    Col,
    DatePicker,
    Divider,
    Form,
    InputNumber,
    message,
    Row,
    Select,
    Spin,
    Statistic,
    Table,
    Tag
} from 'antd';
import {FundOutlined, PlayCircleOutlined} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import {getQuote, getStrategyTemplates, runBacktest} from '../services/api';
import {formatMoney, formatNumber} from '../utils/format';
import dayjs from 'dayjs';

const { RangePicker } = DatePicker;

const DEFAULT_STOCK_OPTIONS = [
  { value: '600519', label: '600519 贵州茅台' },
  { value: '000858', label: '000858 五粮液' },
  { value: '000001', label: '000001 平安银行' },
  { value: '600036', label: '600036 招商银行' },
  { value: '601318', label: '601318 中国平安' },
  { value: '000333', label: '000333 美的集团' },
  { value: '300750', label: '300750 宁德时代' },
  { value: '002475', label: '002475 立讯精密' },
  { value: '600276', label: '600276 恒瑞医药' },
  { value: '601888', label: '601888 中国中免' },
];

function normalizeStockCode(value) {
  return String(value || '')
    .trim()
    .replace(/^(sh|sz)/i, '')
    .replace(/\D/g, '')
    .slice(0, 6);
}

function mergeStockOptions(...groups) {
  const map = new Map();
  groups.flat().forEach(option => {
    if (option?.value) map.set(option.value, option);
  });
  return Array.from(map.values());
}

export default function BacktestPage() {
  const [form] = Form.useForm();
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [selectedType, setSelectedType] = useState('double_ma');
  const [stockOptions, setStockOptions] = useState(DEFAULT_STOCK_OPTIONS);
  const [stockSearching, setStockSearching] = useState(false);
  const searchTimerRef = useRef(null);

  useEffect(() => {
    fetchTemplates();
    return () => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    };
  }, []);

  const fetchTemplates = async () => {
    try {
      const data = await getStrategyTemplates();
      setTemplates(data || []);
    } catch {}
  };

  const handleRunBacktest = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      setResult(null);

      const stocks = [...new Set(values.stocks.map(normalizeStockCode))].filter(code => code.length === 6);
      if (stocks.length === 0) {
        message.error('请输入有效的6位股票代码');
        return;
      }

      const params = {};
      paramFields.forEach(p => {
        const value = values[p.key] ?? p.default;
        if (value !== undefined && value !== null) {
          params[p.key] = value;
        }
      });

      const data = await runBacktest({
        strategy_type: values.strategy_type,
        stocks,
        params,
        start_date: values.dateRange[0].format('YYYY-MM-DD'),
        end_date: values.dateRange[1].format('YYYY-MM-DD'),
        init_capital: values.init_capital || 1000000,
      });

      setResult(data);
      message.success('回测完成');
    } catch (e) {
      if (e.message) message.error('回测失败: ' + e.message);
    } finally {
      setLoading(false);
    }
  };

  const handleStrategyChange = (type) => {
    setSelectedType(type);
    const template = templates.find(t => t.type === type);
    const defaults = {};
    (template?.params || []).forEach(p => {
      defaults[p.key] = p.default;
    });
    form.setFieldsValue(defaults);
  };

  const handleStockSearch = (keyword) => {
    const input = String(keyword || '').trim();
    const localMatches = DEFAULT_STOCK_OPTIONS.filter(option =>
      option.value.includes(input) || option.label.includes(input)
    );
    if (localMatches.length > 0) {
      setStockOptions(prev => mergeStockOptions(localMatches, prev));
    }

    if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    const code = normalizeStockCode(input);
    if (code.length !== 6) return;

    searchTimerRef.current = setTimeout(async () => {
      setStockSearching(true);
      try {
        const quote = await getQuote(code);
        const option = {
          value: quote?.stock_code || code,
          label: `${quote?.stock_code || code} ${quote?.stock_name || ''}`.trim(),
        };
        setStockOptions(prev => mergeStockOptions([option], prev, DEFAULT_STOCK_OPTIONS));
      } catch {
        setStockOptions(prev => mergeStockOptions([{ value: code, label: `${code} 未验证` }], prev, DEFAULT_STOCK_OPTIONS));
      } finally {
        setStockSearching(false);
      }
    }, 300);
  };

  const handleStocksChange = (values) => {
    const normalized = [...new Set(values.map(normalizeStockCode).filter(Boolean))];
    const nextOptions = normalized
      .filter(code => code.length === 6)
      .map(code => ({ value: code, label: code }));
    setStockOptions(prev => mergeStockOptions(prev, nextOptions, DEFAULT_STOCK_OPTIONS));
    form.setFieldsValue({ stocks: normalized });
  };

  // 权益曲线图
  const getEquityOption = () => {
    if (!result?.daily_equity || result.daily_equity.length === 0) return {};

    const dates = result.daily_equity.map(e => dayjs(e.date).format('YYYY-MM-DD'));
    const equities = result.daily_equity.map(e => parseFloat(e.equity));
    const capital = parseFloat(result.initial_capital || 1000000);

    return {
      tooltip: { trigger: 'axis' },
      legend: { data: ['权益曲线', '初始资金'], top: 5 },
      grid: { left: '8%', right: '3%', top: 40, bottom: 30 },
      xAxis: { type: 'category', data: dates },
      yAxis: { type: 'value', scale: true },
      dataZoom: [
        { type: 'inside', start: 0, end: 100 },
        { type: 'slider', start: 0, end: 100 },
      ],
      series: [
        {
          name: '权益曲线',
          type: 'line',
          data: equities,
          smooth: true,
          lineStyle: { width: 2 },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0, y: 0, x2: 0, y2: 1,
              colorStops: [
                { offset: 0, color: 'rgba(22, 119, 255, 0.3)' },
                { offset: 1, color: 'rgba(22, 119, 255, 0.02)' },
              ],
            },
          },
        },
        {
          name: '初始资金',
          type: 'line',
          data: new Array(dates.length).fill(capital),
          lineStyle: { type: 'dashed', color: '#8c8c8c' },
          symbol: 'none',
        },
      ],
    };
  };

  // 回测交易列表
  const tradeColumns = [
    { title: '股票', dataIndex: 'stock_code', key: 'stock_code' },
    {
      title: '方向', dataIndex: 'side', key: 'side', width: 60,
      render: s => <Tag color={s === 'BUY' ? '#f5222d' : '#52c41a'}>{s === 'BUY' ? '买入' : '卖出'}</Tag>,
    },
    { title: '入场价', dataIndex: 'entry_price', key: 'entry_price', align: 'right', render: v => formatNumber(v) },
    { title: '出场价', dataIndex: 'exit_price', key: 'exit_price', align: 'right', render: v => formatNumber(v) },
    { title: '数量', dataIndex: 'volume', key: 'volume', align: 'right', render: v => v?.toLocaleString() },
    {
      title: '盈亏', dataIndex: 'profit', key: 'profit', align: 'right',
      render: v => {
        const val = parseFloat(v || 0);
        return <span style={{ color: val >= 0 ? '#f5222d' : '#52c41a', fontWeight: 500 }}>
          {val >= 0 ? '+' : ''}{formatMoney(val)}
        </span>;
      },
    },
    {
      title: '入场时间', dataIndex: 'entry_time', key: 'entry_time', width: 100,
      render: t => t ? dayjs(t).format('YYYY-MM-DD') : '--',
    },
    {
      title: '出场时间', dataIndex: 'exit_time', key: 'exit_time', width: 100,
      render: t => t ? dayjs(t).format('YYYY-MM-DD') : '--',
    },
  ];

  // 获取当前策略模板的参数
  const currentTemplate = templates.find(t => t.type === selectedType);
  const paramFields = currentTemplate?.params || [];

  return (
    <div>
      <Row gutter={[16, 16]}>
        {/* 回测配置 */}
        <Col xs={24} md={8}>
          <Card title="回测配置" size="small">
            <Form form={form} layout="vertical" initialValues={{
              strategy_type: 'double_ma',
              stocks: ['600519'],
              dateRange: [dayjs().subtract(1, 'year'), dayjs()],
              init_capital: 1000000,
            }}>
              <Form.Item label="策略类型" name="strategy_type" rules={[{ required: true }]}>
                <Select onChange={handleStrategyChange}>
                  {templates.map(t => (
                    <Select.Option key={t.type} value={t.type}>{t.name}</Select.Option>
                  ))}
                </Select>
              </Form.Item>

              <Form.Item
                label="股票代码"
                name="stocks"
                rules={[
                  { required: true, message: '请输入股票代码' },
                  {
                    validator: (_, value = []) => {
                      const invalid = value.map(normalizeStockCode).filter(code => code.length !== 6);
                      return invalid.length === 0 ? Promise.resolve() : Promise.reject(new Error('股票代码必须为6位数字'));
                    },
                  },
                ]}
              >
                <Select
                  mode="tags"
                  showSearch
                  tokenSeparators={[',', '，', ' ', '\n']}
                  placeholder="输入6位股票代码，如 002475"
                  options={stockOptions}
                  loading={stockSearching}
                  onSearch={handleStockSearch}
                  onChange={handleStocksChange}
                  filterOption={(input, option) =>
                    String(option?.label || '').includes(input) || String(option?.value || '').includes(input)
                  }
                />
              </Form.Item>

              <Form.Item label="回测区间" name="dateRange" rules={[{ required: true }]}>
                <RangePicker style={{ width: '100%' }} />
              </Form.Item>

              <Form.Item label="初始资金" name="init_capital">
                <InputNumber style={{ width: '100%' }} min={10000} step={100000} />
              </Form.Item>

              <Divider>策略参数</Divider>

              {paramFields.map(p => (
                <Form.Item
                  key={p.key}
                  label={p.name || p.key}
                  name={p.key}
                  initialValue={p.default}
                  preserve={false}
                >
                  {p.type === 'int' ? (
                    <InputNumber style={{ width: '100%' }} step={1} />
                  ) : p.type === 'float' ? (
                    <InputNumber style={{ width: '100%' }} step={0.01} />
                  ) : (
                    <Select>
                      {p.key === 'ma_type' ? (
                        <>
                          <Select.Option value="SMA">SMA 简单均线</Select.Option>
                          <Select.Option value="EMA">EMA 指数均线</Select.Option>
                        </>
                      ) : (
                        <Select.Option value={p.default}>{String(p.default)}</Select.Option>
                      )}
                    </Select>
                  )}
                </Form.Item>
              ))}

              <Form.Item>
                <Button type="primary" icon={<PlayCircleOutlined />} onClick={handleRunBacktest} loading={loading} block>
                  运行回测
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </Col>

        {/* 回测结果 */}
        <Col xs={24} md={16}>
          {result ? (
            <Spin spinning={loading}>
              {/* 核心指标 */}
              <Row gutter={[16, 16]}>
                <Col xs={12} sm={6}>
                  <Card size="small" className="stat-card">
                    <Statistic
                      title="总收益率"
                      value={parseFloat(result.total_return || 0)}
                      precision={2}
                      suffix="%"
                      valueStyle={{ color: parseFloat(result.total_return || 0) >= 0 ? '#f5222d' : '#52c41a', fontSize: 22 }}
                    />
                  </Card>
                </Col>
                <Col xs={12} sm={6}>
                  <Card size="small" className="stat-card">
                    <Statistic
                      title="年化收益率"
                      value={parseFloat(result.annual_return || 0)}
                      precision={2}
                      suffix="%"
                      valueStyle={{ color: parseFloat(result.annual_return || 0) >= 0 ? '#f5222d' : '#52c41a', fontSize: 22 }}
                    />
                  </Card>
                </Col>
                <Col xs={12} sm={6}>
                  <Card size="small" className="stat-card">
                    <Statistic
                      title="最大回撤"
                      value={parseFloat(result.max_drawdown || 0)}
                      precision={2}
                      suffix="%"
                      valueStyle={{ color: '#f5222d', fontSize: 22 }}
                    />
                  </Card>
                </Col>
                <Col xs={12} sm={6}>
                  <Card size="small" className="stat-card">
                    <Statistic
                      title="夏普比率"
                      value={parseFloat(result.sharpe_ratio || 0)}
                      precision={2}
                      valueStyle={{ fontSize: 22 }}
                    />
                  </Card>
                </Col>
              </Row>

              {/* 第二行指标 */}
              <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
                <Col xs={8}>
                  <Card size="small" className="stat-card">
                    <Statistic title="胜率" value={parseFloat(result.win_rate || 0)} precision={1} suffix="%" />
                  </Card>
                </Col>
                <Col xs={8}>
                  <Card size="small" className="stat-card">
                    <Statistic title="盈利因子" value={parseFloat(result.profit_factor || 0)} precision={2} />
                  </Card>
                </Col>
                <Col xs={8}>
                  <Card size="small" className="stat-card">
                    <Statistic
                      title="总交易"
                      value={result.total_trades || 0}
                      suffix={`(赢${result.win_trades || 0}/亏${result.loss_trades || 0})`}
                    />
                  </Card>
                </Col>
              </Row>

              {/* 权益曲线 */}
              <Card title="权益曲线" size="small" style={{ marginTop: 16 }}>
                <ReactECharts option={getEquityOption()} style={{ height: 350 }} />
              </Card>

              {/* 交易记录 */}
              <Card title="交易记录" size="small" style={{ marginTop: 16 }}>
                <Table
                  dataSource={result.trades || []}
                  columns={tradeColumns}
                  rowKey={(_, i) => i}
                  size="small"
                  pagination={{ pageSize: 10 }}
                  scroll={{ x: 700 }}
                />
              </Card>
            </Spin>
          ) : (
            <Card>
              <div style={{ height: 500, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', color: '#bfbfbf' }}>
                <FundOutlined style={{ fontSize: 64, marginBottom: 16 }} />
                <div style={{ fontSize: 16 }}>配置参数后点击"运行回测"查看结果</div>
              </div>
            </Card>
          )}
        </Col>
      </Row>
    </div>
  );
}
