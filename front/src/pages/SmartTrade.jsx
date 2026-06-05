import React, {useEffect, useMemo, useState} from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  InputNumber,
  message,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Steps,
  Table,
  Tag,
  Typography,
} from 'antd';
import {CheckCircleOutlined, PlayCircleOutlined, ThunderboltOutlined} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import dayjs from 'dayjs';
import {
  applySmartTrade,
  getStockSelectionPlans,
  getStockUniverse,
  getSmartTradeBenchmark,
  getStrategyTemplates,
  runSmartTrade,
} from '../services/api';
import {formatMoney, formatPercent} from '../utils/format';

const { RangePicker } = DatePicker;
const { Text } = Typography;

function defaultPlanForStrategy(plans, strategyType) {
  return (plans || []).find(plan => (plan.strategy_types || []).includes(strategyType))?.id
    || (plans || []).find(plan => plan.id === 'balanced_smart')?.id;
}

function equityOption(result) {
  const points = result?.backtest?.daily_equity || [];
  if (!points.length) return {};
  const dates = points.map(p => dayjs(p.date).format('MM-DD'));
  const equities = points.map(p => Number(p.equity || 0));
  const min = Math.min(...equities);
  const max = Math.max(...equities);
  const padding = Math.max((max - min) * 0.15, max * 0.002, 100);
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 56, right: 20, top: 24, bottom: 34 },
    xAxis: { type: 'category', data: dates },
    yAxis: {
      type: 'value',
      scale: true,
      min: Number((min - padding).toFixed(2)),
      max: Number((max + padding).toFixed(2)),
    },
    series: [{
      name: '权益',
      type: 'line',
      smooth: true,
      data: equities,
      symbol: 'none',
      lineStyle: { width: 2, color: '#1677ff' },
      areaStyle: { color: 'rgba(22,119,255,0.12)' },
    }],
  };
}

export default function SmartTradePage() {
  const [form] = Form.useForm();
  const [messageApi, contextHolder] = message.useMessage();
  const [templates, setTemplates] = useState([]);
  const [plans, setPlans] = useState([]);
  const [universe, setUniverse] = useState([]);
  const [benchmark, setBenchmark] = useState([]);
  const [loading, setLoading] = useState(false);
  const [applying, setApplying] = useState(false);
  const [result, setResult] = useState(null);

  useEffect(() => {
    fetchMeta();
  }, []);

  const fetchMeta = async () => {
    try {
      const [templateData, planData, universeData, benchmarkData] = await Promise.all([
        getStrategyTemplates(),
        getStockSelectionPlans(),
        getStockUniverse(),
        getSmartTradeBenchmark(),
      ]);
      setTemplates(templateData || []);
      setPlans(planData || []);
      setUniverse(universeData || []);
      setBenchmark(benchmarkData || []);
      const defaultStrategy = 'momentum';
      form.setFieldsValue({
        strategy_type: defaultStrategy,
        plan_id: defaultPlanForStrategy(planData || [], defaultStrategy),
        top_n: 5,
        lookback_days: 90,
        init_capital: 1000000,
        mode: 'paper',
        auto_start: true,
      });
    } catch (e) {
      messageApi.error('加载智能交易配置失败: ' + (e.message || '未知错误'));
    }
  };

  const selectedStrategy = Form.useWatch('strategy_type', form) || 'momentum';
  const selectedPlanID = Form.useWatch('plan_id', form);
  const selectedPlan = useMemo(
    () => plans.find(plan => plan.id === (selectedPlanID || defaultPlanForStrategy(plans, selectedStrategy))),
    [plans, selectedStrategy, selectedPlanID]
  );

  const handleStrategyChange = (strategyType) => {
    form.setFieldsValue({ plan_id: defaultPlanForStrategy(plans, strategyType) });
  };

  const handleRun = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      setResult(null);
      const dateRange = values.date_range || [];
      const data = await runSmartTrade({
        strategy_type: values.strategy_type,
        plan_id: values.plan_id,
        top_n: values.top_n,
        lookback_days: values.lookback_days,
        init_capital: values.init_capital,
        candidate_codes: values.candidate_codes || [],
        start_date: dateRange[0] ? dateRange[0].format('YYYY-MM-DD') : '',
        end_date: dateRange[1] ? dateRange[1].format('YYYY-MM-DD') : '',
      });
      setResult(data);
      messageApi.success('智能选股与回测完成');
    } catch (e) {
      if (e.message) messageApi.error('智能交易失败: ' + e.message);
    } finally {
      setLoading(false);
    }
  };

  const handleApply = async () => {
    if (!result?.stocks?.length) return;
    try {
      const values = await form.validateFields();
      setApplying(true);
      const data = await applySmartTrade({
        strategy_type: values.strategy_type,
        stocks: result.stocks,
        params: result.recommended_params || {},
        mode: values.mode || 'paper',
        init_capital: values.init_capital,
        auto_start: values.auto_start !== false,
      });
      messageApi.success(`${data.strategy_name || '智能策略'}已应用`);
    } catch (e) {
      if (e.message) messageApi.error('应用失败: ' + e.message);
    } finally {
      setApplying(false);
    }
  };

  const pickColumns = [
    { title: '排名', dataIndex: 'rank', key: 'rank', width: 64 },
    {
      title: '股票',
      key: 'stock',
      render: (_, r) => (
        <div>
          <div style={{ fontWeight: 600 }}>{r.stock_name || r.stock_code}</div>
          <Text type="secondary">{r.stock_code} / {r.sector}</Text>
        </div>
      ),
    },
    {
      title: '智能得分',
      dataIndex: 'score',
      key: 'score',
      align: 'right',
      width: 100,
      render: value => <Tag color={value >= 70 ? 'red' : value >= 55 ? 'blue' : 'default'}>{Number(value || 0).toFixed(2)}</Tag>,
    },
    {
      title: '关键指标',
      key: 'metrics',
      render: (_, r) => (
        <Space size={[4, 4]} wrap>
          <Tag>20日 {formatPercent((r.metrics?.return_20 || 0) / 100)}</Tag>
          <Tag>60日 {formatPercent((r.metrics?.return_60 || 0) / 100)}</Tag>
          <Tag>回撤 {Number(r.metrics?.max_drawdown || 0).toFixed(2)}%</Tag>
          <Tag>波动 {Number(r.metrics?.volatility || 0).toFixed(2)}%</Tag>
        </Space>
      ),
    },
    {
      title: '入选理由',
      dataIndex: 'reasons',
      key: 'reasons',
      render: reasons => (
        <Space direction="vertical" size={2}>
          {(reasons || []).slice(0, 3).map((reason, index) => <Text key={index} type="secondary">{reason}</Text>)}
        </Space>
      ),
    },
  ];

  const candidateBacktestColumns = [
    { title: '股票', dataIndex: 'stock_name', key: 'stock_name', render: (name, r) => `${name || r.stock_code} (${r.stock_code})` },
    { title: '单股收益', dataIndex: 'total_return', key: 'total_return', align: 'right', render: v => `${Number(v || 0).toFixed(2)}%` },
    { title: '最大回撤', dataIndex: 'max_drawdown', key: 'max_drawdown', align: 'right', render: v => `${Number(v || 0).toFixed(2)}%` },
    { title: '夏普', dataIndex: 'sharpe_ratio', key: 'sharpe_ratio', align: 'right', render: v => Number(v || 0).toFixed(2) },
    { title: '交易次数', dataIndex: 'total_trades', key: 'total_trades', align: 'right' },
    { title: '综合排序分', dataIndex: 'rank_score', key: 'rank_score', align: 'right', render: v => <Tag color={v > 0 ? 'blue' : 'default'}>{Number(v || 0).toFixed(2)}</Tag> },
  ];

  const templateOptions = templates.map(t => ({ value: t.type, label: t.name }));
  const planOptions = plans.map(plan => ({ value: plan.id, label: plan.name }));
  const universeOptions = universe.map(stock => ({ value: stock.code, label: `${stock.code} ${stock.name}` }));
  const validation = result?.validation_summary || {};
  const validationWarnings = validation.warnings || [];

  return (
    <Spin spinning={loading}>
      <div>
        {contextHolder}
        <Alert
          showIcon
          type="info"
          message="一键智能交易"
          description="选择策略后，系统会自动匹配选股方案、拉取真实K线、完成回测，并可一键应用到模拟交易策略。"
          style={{ marginBottom: 16 }}
        />

        {benchmark.length > 0 && (
          <Card title="机构对标能力" size="small" style={{ marginBottom: 16 }}>
            <Row gutter={[12, 12]}>
              {benchmark.map(item => (
                <Col xs={24} md={12} lg={6} key={item.module}>
                  <Space direction="vertical" size={4}>
                    <Space>
                      <Tag color={item.implemented ? 'green' : 'orange'}>{item.implemented ? '已接入' : '待接入'}</Tag>
                      <Text strong>{item.module}</Text>
                    </Space>
                    <Text type="secondary">{item.system_field}</Text>
                  </Space>
                </Col>
              ))}
            </Row>
          </Card>
        )}

        <Card size="small" style={{ marginBottom: 16 }}>
          <Form form={form} layout="vertical">
            <Row gutter={16}>
              <Col xs={24} md={6}>
                <Form.Item label="交易策略" name="strategy_type" rules={[{ required: true, message: '请选择策略' }]}>
                  <Select options={templateOptions} onChange={handleStrategyChange} />
                </Form.Item>
              </Col>
              <Col xs={24} md={6}>
                <Form.Item label="选股方案" name="plan_id">
                  <Select options={planOptions} />
                </Form.Item>
              </Col>
              <Col xs={24} md={4}>
                <Form.Item label="选股数量" name="top_n">
                  <InputNumber min={1} max={10} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={4}>
                <Form.Item label="分析周期" name="lookback_days">
                  <InputNumber min={60} max={250} addonAfter="日" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={4}>
                <Form.Item label="交易模式" name="mode">
                  <Select options={[
                    { value: 'paper', label: '模拟交易' },
                    { value: 'live', label: '当前实盘' },
                  ]} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} md={8}>
                <Form.Item label="回测时间" name="date_range">
                  <RangePicker style={{ width: '100%' }} allowClear placeholder={['默认近3个月', '默认今天']} />
                </Form.Item>
              </Col>
              <Col xs={24} md={5}>
                <Form.Item label="初始资金" name="init_capital">
                  <InputNumber min={10000} step={10000} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={7}>
                <Form.Item label="候选股票池" name="candidate_codes">
                  <Select
                    mode="tags"
                    allowClear
                    placeholder="留空则使用核心A股池"
                    options={universeOptions}
                    maxTagCount="responsive"
                  />
                </Form.Item>
              </Col>
              <Col xs={24} md={4}>
                <Form.Item label=" ">
                  <Button type="primary" icon={<ThunderboltOutlined />} block onClick={handleRun} loading={loading}>
                    一键智能运行
                  </Button>
                </Form.Item>
              </Col>
            </Row>
          </Form>
          {selectedPlan && (
            <Alert
              type="success"
              showIcon
              message={`${selectedPlan.name}：${selectedPlan.description}`}
              style={{ marginTop: 4 }}
            />
          )}
        </Card>

        {result && (
          <>
            <Card size="small" style={{ marginBottom: 16 }}>
              <Steps
                size="small"
                items={(result.workflow || []).map(item => ({
                  title: item.text,
                  status: item.status === 'ready' ? 'process' : 'finish',
                  icon: item.status === 'ready' ? <PlayCircleOutlined /> : <CheckCircleOutlined />,
                }))}
              />
            </Card>

            {validationWarnings.length > 0 && (
              <Alert
                showIcon
                type="warning"
                message="策略验证风险提示"
                description={(
                  <Space direction="vertical" size={2}>
                    {validationWarnings.map((warning, index) => <Text key={index}>{warning}</Text>)}
                  </Space>
                )}
                style={{ marginBottom: 16 }}
              />
            )}

            <Card title="机构级验证摘要" size="small" style={{ marginBottom: 16 }}>
              <Row gutter={[16, 16]}>
                <Col xs={12} md={5}>
                  <Statistic title="验证股票" value={validation.validated_count || 0} suffix={`/ ${validation.candidate_count || 0}`} />
                </Col>
                <Col xs={12} md={5}>
                  <Statistic title="正收益数量" value={validation.positive_count || 0} />
                </Col>
                <Col xs={12} md={5}>
                  <Statistic title="正收益率" value={Number(validation.positive_rate || 0)} precision={2} suffix="%" />
                </Col>
                <Col xs={12} md={5}>
                  <Statistic title="最佳单股收益" value={Number(validation.best_return || 0)} precision={2} suffix="%" />
                </Col>
                <Col xs={12} md={4}>
                  <Statistic title="最坏单股回撤" value={Number(validation.worst_drawdown || 0)} precision={2} suffix="%" />
                </Col>
              </Row>
              {validation.method && <Text type="secondary">{validation.method}</Text>}
            </Card>

            <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
              <Col xs={12} md={6}>
                <Card size="small"><Statistic title="总收益" value={Number(result.backtest?.total_return || 0)} precision={2} suffix="%" /></Card>
              </Col>
              <Col xs={12} md={6}>
                <Card size="small"><Statistic title="最大回撤" value={Number(result.backtest?.max_drawdown || 0)} precision={2} suffix="%" /></Card>
              </Col>
              <Col xs={12} md={6}>
                <Card size="small"><Statistic title="交易次数" value={result.backtest?.total_trades || 0} /></Card>
              </Col>
              <Col xs={12} md={6}>
                <Card size="small"><Statistic title="最终资金" value={Number(result.backtest?.final_capital || 0)} precision={2} prefix="¥" formatter={v => formatMoney(v)} /></Card>
              </Col>
            </Row>

            <Row gutter={[16, 16]}>
              <Col xs={24} lg={14}>
                <Card
                  title="智能选股结果"
                  size="small"
                  extra={
                    <Button type="primary" onClick={handleApply} loading={applying}>
                      应用到{form.getFieldValue('mode') === 'live' ? '实盘' : '模拟交易'}
                    </Button>
                  }
                >
                  <Table
                    rowKey="stock_code"
                    size="small"
                    columns={pickColumns}
                    dataSource={result.selection?.picks || []}
                    pagination={false}
                    scroll={{ x: 900 }}
                  />
                </Card>
              </Col>
              <Col xs={24} lg={10}>
                <Card title="回测权益曲线" size="small">
                  <ReactECharts option={equityOption(result)} style={{ height: 360 }} />
                </Card>
              </Col>
            </Row>

            <Card title="推荐策略参数" size="small" style={{ marginTop: 16 }}>
              <Space size={[8, 8]} wrap>
                {Object.entries(result.recommended_params || {}).map(([key, value]) => (
                  <Tag key={key}>{key}: {String(value)}</Tag>
                ))}
              </Space>
            </Card>

            <Card title="策略单股验证回测" size="small" style={{ marginTop: 16 }}>
              <Table
                rowKey="stock_code"
                size="small"
                columns={candidateBacktestColumns}
                dataSource={result.candidate_backtests || []}
                pagination={false}
              />
            </Card>
          </>
        )}
      </div>
    </Spin>
  );
}
