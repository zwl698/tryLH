import React, {useEffect, useState} from 'react';
import {
    Alert,
    Button,
    Card,
    Col,
    Divider,
    Form,
    InputNumber,
    message,
    Progress,
    Row,
    Spin,
    Switch,
    Table,
    Tag
} from 'antd';
import {ReloadOutlined, SafetyCertificateOutlined, SaveOutlined} from '@ant-design/icons';
import {getRiskConfig, getRiskEvents, updateRiskConfig} from '../services/api';
import {formatPercent, formatTime} from '../utils/format';

const RISK_LEVEL_MAP = {
  LOW: { color: 'green', label: '低' },
  MEDIUM: { color: 'orange', label: '中' },
  HIGH: { color: 'red', label: '高' },
  CRITICAL: { color: '#cf1322', label: '严重' },
};

const RISK_TYPE_MAP = {
  single_position_exceed: '单股仓位超限',
  total_position_exceed: '总仓位超限',
  daily_loss_exceed: '日亏损超限',
  max_drawdown_exceed: '最大回撤超限',
  daily_trades_exceed: '日交易次数超限',
  stop_loss: '触发止损',
  take_profit: '触发止盈',
  blacklist: '黑名单股票',
};

export default function RiskPage() {
  const [riskConfig, setRiskConfig] = useState(null);
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    setLoading(true);
    try {
      const [configData, eventsData] = await Promise.allSettled([
        getRiskConfig(), getRiskEvents(),
      ]);
      if (configData.status === 'fulfilled') {
        setRiskConfig(configData.value);
        form.setFieldsValue({
          max_single_position_pct: parseFloat(configData.value.max_single_position_pct || 0) * 100,
          max_total_position_pct: parseFloat(configData.value.max_total_position_pct || 0) * 100,
          max_daily_loss_pct: parseFloat(configData.value.max_daily_loss_pct || 0) * 100,
          max_drawdown_pct: parseFloat(configData.value.max_drawdown_pct || 0) * 100,
          max_daily_trades: configData.value.max_daily_trades,
          stop_loss_pct: parseFloat(configData.value.stop_loss_pct || 0) * 100,
          take_profit_pct: parseFloat(configData.value.take_profit_pct || 0) * 100,
          allow_margin_trade: configData.value.allow_margin_trade || false,
          allow_t0_trade: configData.value.allow_t0_trade || false,
        });
      }
      if (eventsData.status === 'fulfilled') setEvents(eventsData.value || []);
    } catch {}
    setLoading(false);
  };

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await updateRiskConfig({
        max_single_position_pct: (values.max_single_position_pct || 0) / 100,
        max_total_position_pct: (values.max_total_position_pct || 0) / 100,
        max_daily_loss_pct: (values.max_daily_loss_pct || 0) / 100,
        max_drawdown_pct: (values.max_drawdown_pct || 0) / 100,
        max_daily_trades: values.max_daily_trades,
        stop_loss_pct: (values.stop_loss_pct || 0) / 100,
        take_profit_pct: (values.take_profit_pct || 0) / 100,
        allow_margin_trade: values.allow_margin_trade,
        allow_t0_trade: values.allow_t0_trade,
        blacklist_stocks: riskConfig?.blacklist_stocks || [],
      });
      message.success('风控配置更新成功');
      fetchData();
    } catch (e) {
      if (e.message) message.error('保存失败: ' + e.message);
    } finally {
      setSaving(false);
    }
  };

  const eventColumns = [
    {
      title: '等级', dataIndex: 'level', key: 'level', width: 80,
      render: l => {
        const info = RISK_LEVEL_MAP[l] || {};
        return <Tag color={info.color}>{info.label || l}</Tag>;
      },
    },
    {
      title: '类型', dataIndex: 'type', key: 'type', width: 140,
      render: t => RISK_TYPE_MAP[t] || t,
    },
    { title: '消息', dataIndex: 'message', key: 'message', ellipsis: true },
    { title: '股票', dataIndex: 'stock_code', key: 'stock_code', width: 100 },
    { title: '策略', dataIndex: 'strategy_id', key: 'strategy_id', width: 120, ellipsis: true },
    {
      title: '触发值', dataIndex: 'value', key: 'value', width: 80, align: 'right',
      render: v => v ? formatPercent(v) : '--',
    },
    {
      title: '阈值', dataIndex: 'threshold', key: 'threshold', width: 80, align: 'right',
      render: v => v ? formatPercent(v) : '--',
    },
    {
      title: '时间', dataIndex: 'timestamp', key: 'timestamp', width: 160,
      render: t => formatTime(t),
    },
  ];

  return (
    <Spin spinning={loading}>
      <div>
        {/* 风控概览 */}
        {riskConfig && (
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            <Col xs={12} sm={6}>
              <Card size="small" className="stat-card">
                <Progress
                  type="dashboard"
                  percent={parseFloat(riskConfig.max_single_position_pct || 0) * 100}
                  format={() => formatPercent(riskConfig.max_single_position_pct)}
                  size={80}
                />
                <div style={{ color: '#8c8c8c', marginTop: 8 }}>单股最大仓位</div>
              </Card>
            </Col>
            <Col xs={12} sm={6}>
              <Card size="small" className="stat-card">
                <Progress
                  type="dashboard"
                  percent={parseFloat(riskConfig.max_total_position_pct || 0) * 100}
                  format={() => formatPercent(riskConfig.max_total_position_pct)}
                  size={80}
                />
                <div style={{ color: '#8c8c8c', marginTop: 8 }}>总仓位上限</div>
              </Card>
            </Col>
            <Col xs={12} sm={6}>
              <Card size="small" className="stat-card">
                <Progress
                  type="dashboard"
                  percent={parseFloat(riskConfig.max_daily_loss_pct || 0) * 100}
                  format={() => formatPercent(riskConfig.max_daily_loss_pct)}
                  size={80}
                  strokeColor="#f5222d"
                />
                <div style={{ color: '#8c8c8c', marginTop: 8 }}>日最大亏损</div>
              </Card>
            </Col>
            <Col xs={12} sm={6}>
              <Card size="small" className="stat-card">
                <Progress
                  type="dashboard"
                  percent={parseFloat(riskConfig.max_drawdown_pct || 0) * 100}
                  format={() => formatPercent(riskConfig.max_drawdown_pct)}
                  size={80}
                  strokeColor="#f5222d"
                />
                <div style={{ color: '#8c8c8c', marginTop: 8 }}>最大回撤</div>
              </Card>
            </Col>
          </Row>
        )}

        <Row gutter={[16, 16]}>
          {/* 风控配置 */}
          <Col xs={24} md={10}>
            <Card
              title="风控配置"
              size="small"
              extra={
                <Button type="primary" icon={<SaveOutlined />} onClick={handleSave} loading={saving} size="small">
                  保存配置
                </Button>
              }
            >
              <Form form={form} layout="vertical" size="small">
                <Divider orientation="left" style={{ fontSize: 12 }}>仓位限制</Divider>
                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item label="单股最大仓位(%)" name="max_single_position_pct">
                      <InputNumber style={{ width: '100%' }} min={0} max={100} step={5} />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label="总仓位上限(%)" name="max_total_position_pct">
                      <InputNumber style={{ width: '100%' }} min={0} max={100} step={5} />
                    </Form.Item>
                  </Col>
                </Row>

                <Divider orientation="left" style={{ fontSize: 12 }}>亏损限制</Divider>
                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item label="日最大亏损(%)" name="max_daily_loss_pct">
                      <InputNumber style={{ width: '100%' }} min={0} max={50} step={1} />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label="最大回撤(%)" name="max_drawdown_pct">
                      <InputNumber style={{ width: '100%' }} min={0} max={50} step={1} />
                    </Form.Item>
                  </Col>
                </Row>

                <Divider orientation="left" style={{ fontSize: 12 }}>交易限制</Divider>
                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item label="日最大交易次数" name="max_daily_trades">
                      <InputNumber style={{ width: '100%' }} min={1} max={200} step={5} />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label="默认止损(%)" name="stop_loss_pct">
                      <InputNumber style={{ width: '100%' }} min={0} max={50} step={1} />
                    </Form.Item>
                  </Col>
                </Row>
                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item label="默认止盈(%)" name="take_profit_pct">
                      <InputNumber style={{ width: '100%' }} min={0} max={100} step={1} />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label=" " name="init_capital">
                      <div />
                    </Form.Item>
                  </Col>
                </Row>

                <Divider orientation="left" style={{ fontSize: 12 }}>高级设置</Divider>
                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item label="允许融资融券" name="allow_margin_trade" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item label="允许T+0(回测)" name="allow_t0_trade" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                  </Col>
                </Row>
              </Form>
            </Card>
          </Col>

          {/* 风控事件 */}
          <Col xs={24} md={14}>
            <Card
              title={`风控事件 (${events.length})`}
              size="small"
              extra={<Button size="small" icon={<ReloadOutlined />} onClick={fetchData}>刷新</Button>}
            >
              {events.length === 0 ? (
                <Alert
                  type="success"
                  message="暂无风控事件"
                  description="系统运行正常，未触发任何风控规则"
                  showIcon
                  icon={<SafetyCertificateOutlined />}
                />
              ) : (
                <Table
                  dataSource={events}
                  columns={eventColumns}
                  rowKey="id"
                  size="small"
                  pagination={{ pageSize: 10 }}
                  scroll={{ x: 800 }}
                />
              )}
            </Card>
          </Col>
        </Row>
      </div>
    </Spin>
  );
}

