import React, {useEffect, useState} from 'react';
import {Alert, Card, Col, Progress, Row, Spin, Statistic, Table, Tag} from 'antd';
import {
    ArrowDownOutlined,
    ArrowUpOutlined,
    DollarOutlined,
    RobotOutlined,
    SafetyCertificateOutlined,
    StockOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import {getAccount, getPositions, getRiskConfig, getRiskEvents, getSystemStatus, listStrategies} from '../services/api';
import {formatMoney, formatPercent, strategyStatusColor, strategyStatusLabel} from '../utils/format';

export default function Dashboard() {
  const [loading, setLoading] = useState(true);
  const [account, setAccount] = useState(null);
  const [positions, setPositions] = useState([]);
  const [strategies, setStrategies] = useState([]);
  const [status, setStatus] = useState(null);
  const [riskConfig, setRiskConfig] = useState(null);
  const [riskEvents, setRiskEvents] = useState([]);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 15000);
    return () => clearInterval(interval);
  }, []);

  const fetchData = async () => {
    try {
      const [accountData, positionsData, strategiesData, statusData, riskData, eventsData] = await Promise.allSettled([
        getAccount(),
        getPositions(),
        listStrategies(),
        getSystemStatus(),
        getRiskConfig(),
        getRiskEvents(),
      ]);

      if (accountData.status === 'fulfilled') setAccount(accountData.value);
      if (positionsData.status === 'fulfilled') setPositions(positionsData.value || []);
      if (strategiesData.status === 'fulfilled') setStrategies(strategiesData.value || []);
      if (statusData.status === 'fulfilled') setStatus(statusData.value);
      if (riskData.status === 'fulfilled') setRiskConfig(riskData.value);
      if (eventsData.status === 'fulfilled') setRiskEvents(eventsData.value || []);
      setError(null);
    } catch (e) {
      setError('数据加载失败，请检查后端服务是否运行');
    } finally {
      setLoading(false);
    }
  };

  // 资产分布饼图
  const getAssetPieOption = () => {
    if (!positions || positions.length === 0) {
      return {
        title: { text: '持仓分布', left: 'center', top: 10, textStyle: { fontSize: 14 } },
        series: [{
          type: 'pie', radius: ['40%', '70%'], center: ['50%', '55%'],
          data: [{ value: 0, name: '暂无持仓' }],
          label: { show: false },
        }],
      };
    }
    const pieData = positions.map(p => ({
      value: parseFloat(p.market_value || 0),
      name: p.stock_name || p.stock_code,
    }));
    if (account) {
      const cash = parseFloat(account.cash || 0);
      if (cash > 0) pieData.push({ value: cash, name: '可用现金' });
    }
    return {
      title: { text: '资产分布', left: 'center', top: 10, textStyle: { fontSize: 14 } },
      tooltip: { trigger: 'item', formatter: '{b}: ¥{c} ({d}%)' },
      series: [{
        type: 'pie', radius: ['40%', '70%'], center: ['50%', '55%'],
        data: pieData,
        emphasis: { itemStyle: { shadowBlur: 10, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.5)' } },
        label: { formatter: '{b}\n{d}%' },
      }],
    };
  };

  // 策略状态分布
  const getStrategyPieOption = () => {
    const statusMap = {};
    strategies.forEach(s => {
      const st = s.status || 'UNKNOWN';
      statusMap[st] = (statusMap[st] || 0) + 1;
    });
    const colorMap = { ACTIVE: '#52c41a', PAUSED: '#faad14', STOPPED: '#d9d9d9', ERROR: '#f5222d' };
    const nameMap = { ACTIVE: '运行中', PAUSED: '已暂停', STOPPED: '已停止', ERROR: '异常' };
    const data = Object.entries(statusMap).map(([k, v]) => ({
      value: v, name: nameMap[k] || k, itemStyle: { color: colorMap[k] || '#999' },
    }));
    return {
      title: { text: '策略状态', left: 'center', top: 10, textStyle: { fontSize: 14 } },
      tooltip: { trigger: 'item', formatter: '{b}: {c}个 ({d}%)' },
      series: [{
        type: 'pie', radius: ['40%', '70%'], center: ['50%', '55%'],
        data: data.length > 0 ? data : [{ value: 0, name: '暂无策略' }],
        label: { formatter: '{b}\n{c}个' },
      }],
    };
  };

  const positionColumns = [
    { title: '股票', dataIndex: 'stock_name', key: 'stock_name', render: (t, r) => t || r.stock_code },
    { title: '代码', dataIndex: 'stock_code', key: 'stock_code' },
    { title: '持仓量', dataIndex: 'volume', key: 'volume', align: 'right', render: v => v?.toLocaleString() },
    {
      title: '市值', dataIndex: 'market_value', key: 'market_value', align: 'right',
      render: v => '¥' + formatMoney(v),
    },
    {
      title: '盈亏', dataIndex: 'profit_loss', key: 'profit_loss', align: 'right',
      render: (v, r) => {
        const val = parseFloat(v || 0);
        const pct = parseFloat(r.profit_pct || 0) * 100;
        const color = val > 0 ? '#f5222d' : val < 0 ? '#52c41a' : '#8c8c8c';
        return <span style={{ color }}>{val > 0 ? '+' : ''}{formatMoney(val)} ({pct.toFixed(2)}%)</span>;
      },
    },
  ];

  if (error) {
    return <Alert type="error" message={error} style={{ margin: 16 }} showIcon />;
  }

  return (
    <Spin spinning={loading}>
      <div style={{ padding: 0 }}>
        {/* 核心指标卡片 */}
        <Row gutter={[16, 16]}>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="总资产"
                value={account ? parseFloat(account.total_assets || 0) : 0}
                precision={2}
                prefix={<DollarOutlined />}
                suffix="元"
                valueStyle={{ color: '#1677ff', fontSize: 26 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="可用资金"
                value={account ? parseFloat(account.cash || 0) : 0}
                precision={2}
                prefix="¥"
                valueStyle={{ fontSize: 26 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="今日盈亏"
                value={account ? parseFloat(account.today_pl || 0) : 0}
                precision={2}
                prefix={account && parseFloat(account.today_pl || 0) >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
                suffix="元"
                valueStyle={{
                  color: account && parseFloat(account.today_pl || 0) >= 0 ? '#f5222d' : '#52c41a',
                  fontSize: 26,
                }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="运行策略"
                value={strategies.filter(s => s.status === 'ACTIVE').length}
                suffix={`/ ${strategies.length}`}
                prefix={<RobotOutlined />}
                valueStyle={{ color: '#52c41a', fontSize: 26 }}
              />
            </Card>
          </Col>
        </Row>

        {/* 第二行统计 */}
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="持仓市值"
                value={account ? parseFloat(account.market_value || 0) : 0}
                precision={2}
                prefix="¥"
                valueStyle={{ fontSize: 22 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="累计盈亏"
                value={account ? parseFloat(account.total_pl || 0) : 0}
                precision={2}
                prefix="¥"
                valueStyle={{
                  color: account && parseFloat(account.total_pl || 0) >= 0 ? '#f5222d' : '#52c41a',
                  fontSize: 22,
                }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="持仓数"
                value={positions?.length || 0}
                suffix="只"
                prefix={<StockOutlined />}
                valueStyle={{ fontSize: 22 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card className="stat-card" hoverable>
              <Statistic
                title="风控事件"
                value={riskEvents?.length || 0}
                suffix="条"
                prefix={<SafetyCertificateOutlined />}
                valueStyle={{ color: riskEvents?.length > 0 ? '#faad14' : '#52c41a', fontSize: 22 }}
              />
            </Card>
          </Col>
        </Row>

        {/* 图表区域 */}
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} md={12}>
            <Card>
              <ReactECharts option={getAssetPieOption()} style={{ height: 320 }} />
            </Card>
          </Col>
          <Col xs={24} md={12}>
            <Card>
              <ReactECharts option={getStrategyPieOption()} style={{ height: 320 }} />
            </Card>
          </Col>
        </Row>

        {/* 风控概览 */}
        {riskConfig && (
          <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
            <Col span={24}>
              <Card title="风控参数概览" size="small">
                <Row gutter={16}>
                  <Col span={6}>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ color: '#8c8c8c', fontSize: 12 }}>单股最大仓位</div>
                      <Progress type="circle" percent={parseFloat(riskConfig.max_single_position_pct || 0) * 100} size={60} format={() => formatPercent(riskConfig.max_single_position_pct)} />
                    </div>
                  </Col>
                  <Col span={6}>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ color: '#8c8c8c', fontSize: 12 }}>总仓位上限</div>
                      <Progress type="circle" percent={parseFloat(riskConfig.max_total_position_pct || 0) * 100} size={60} format={() => formatPercent(riskConfig.max_total_position_pct)} />
                    </div>
                  </Col>
                  <Col span={6}>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ color: '#8c8c8c', fontSize: 12 }}>日最大亏损</div>
                      <Progress type="circle" percent={parseFloat(riskConfig.max_daily_loss_pct || 0) * 100} size={60} format={() => formatPercent(riskConfig.max_daily_loss_pct)} strokeColor="#f5222d" />
                    </div>
                  </Col>
                  <Col span={6}>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ color: '#8c8c8c', fontSize: 12 }}>最大回撤</div>
                      <Progress type="circle" percent={parseFloat(riskConfig.max_drawdown_pct || 0) * 100} size={60} format={() => formatPercent(riskConfig.max_drawdown_pct)} strokeColor="#f5222d" />
                    </div>
                  </Col>
                </Row>
              </Card>
            </Col>
          </Row>
        )}

        {/* 持仓表格 */}
        <Row style={{ marginTop: 16 }}>
          <Col span={24}>
            <Card title="当前持仓" size="small">
              <Table
                dataSource={positions || []}
                columns={positionColumns}
                rowKey="stock_code"
                size="small"
                pagination={false}
                locale={{ emptyText: '暂无持仓' }}
              />
            </Card>
          </Col>
        </Row>

        {/* 策略状态列表 */}
        <Row style={{ marginTop: 16 }}>
          <Col span={24}>
            <Card title="策略运行状态" size="small">
              <Table
                dataSource={strategies}
                columns={[
                  { title: '策略名称', dataIndex: 'name', key: 'name' },
                  { title: '类型', dataIndex: 'type', key: 'type' },
                  {
                    title: '状态', dataIndex: 'status', key: 'status',
                    render: s => <Tag color={strategyStatusColor(s)}>{strategyStatusLabel(s)}</Tag>,
                  },
                  { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
                ]}
                rowKey="id"
                size="small"
                pagination={false}
              />
            </Card>
          </Col>
        </Row>
      </div>
    </Spin>
  );
}

