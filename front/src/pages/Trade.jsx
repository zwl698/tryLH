import React, {useEffect, useState} from 'react';
import {
    Badge,
    Button,
    Card,
    Col,
    Form,
    Input,
    InputNumber,
    message,
    Modal,
    Row,
    Select,
    Space,
    Statistic,
    Table,
    Tag
} from 'antd';
import {DeleteOutlined, LoginOutlined, LogoutOutlined, PlusOutlined, ReloadOutlined} from '@ant-design/icons';
import {
    brokerLogin,
    brokerLogout,
    cancelOrder,
    getAccount,
    getOrders,
    getPositions,
    submitOrder
} from '../services/api';
import {formatMoney, sideColor, sideLabel} from '../utils/format';

export default function TradePage() {
  const [account, setAccount] = useState(null);
  const [positions, setPositions] = useState([]);
  const [orders, setOrders] = useState([]);
  const [loading, setLoading] = useState(false);
  const [orderModal, setOrderModal] = useState(false);
  const [orderForm] = Form.useForm();
  const [loggedIn, setLoggedIn] = useState(false);

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    setLoading(true);
    try {
      const [accData, posData, ordData] = await Promise.allSettled([
        getAccount(), getPositions(), getOrders(),
      ]);
      if (accData.status === 'fulfilled') { setAccount(accData.value); setLoggedIn(true); }
      if (posData.status === 'fulfilled') setPositions(posData.value || []);
      if (ordData.status === 'fulfilled') setOrders(ordData.value || []);
    } catch {}
    setLoading(false);
  };

  const handleLogin = async () => {
    try {
      await brokerLogin();
      message.success('券商登录成功');
      setLoggedIn(true);
      fetchData();
    } catch (e) {
      message.error('登录失败: ' + (e.message || ''));
    }
  };

  const handleLogout = async () => {
    try {
      await brokerLogout();
      message.success('券商已登出');
      setLoggedIn(false);
    } catch (e) {
      message.error('登出失败: ' + (e.message || ''));
    }
  };

  const handleSubmitOrder = async () => {
    try {
      const values = await orderForm.validateFields();
      await submitOrder({
        stock_code: values.stock_code,
        side: values.side,
        type: values.type || 'LIMIT',
        price: String(values.price),
        volume: values.volume,
        strategy_id: values.strategy_id || '',
      });
      message.success('下单成功');
      setOrderModal(false);
      orderForm.resetFields();
      fetchData();
    } catch (e) {
      if (e.message) message.error('下单失败: ' + e.message);
    }
  };

  const handleCancelOrder = async (id) => {
    try {
      await cancelOrder(id);
      message.success('撤单成功');
      fetchData();
    } catch (e) {
      message.error('撤单失败: ' + (e.message || ''));
    }
  };

  const positionColumns = [
    {
      title: '股票', key: 'stock', render: (_, r) => (
        <div>
          <div style={{ fontWeight: 500 }}>{r.stock_name || r.stock_code}</div>
          <div style={{ fontSize: 12, color: '#8c8c8c' }}>{r.stock_code}</div>
        </div>
      ),
    },
    { title: '市场', dataIndex: 'market', key: 'market', width: 60 },
    { title: '持仓量', dataIndex: 'volume', key: 'volume', align: 'right', render: v => v?.toLocaleString() },
    { title: '可用量', dataIndex: 'available_vol', key: 'available_vol', align: 'right', render: v => v?.toLocaleString() },
    { title: '成本价', dataIndex: 'avg_cost', key: 'avg_cost', align: 'right', render: v => formatMoney(v) },
    { title: '现价', dataIndex: 'current_price', key: 'current_price', align: 'right', render: v => formatMoney(v) },
    { title: '市值', dataIndex: 'market_value', key: 'market_value', align: 'right', render: v => '¥' + formatMoney(v) },
    {
      title: '盈亏', key: 'pl', align: 'right',
      render: (_, r) => {
        const val = parseFloat(r.profit_loss || 0);
        const pct = parseFloat(r.profit_pct || 0) * 100;
        const color = val > 0 ? '#f5222d' : val < 0 ? '#52c41a' : '#8c8c8c';
        return (
          <span style={{ color, fontWeight: 500 }}>
            {val > 0 ? '+' : ''}{formatMoney(val)} ({pct.toFixed(2)}%)
          </span>
        );
      },
    },
  ];

  const orderColumns = [
    { title: '订单号', dataIndex: 'id', key: 'id', width: 120, ellipsis: true },
    {
      title: '股票', key: 'stock', render: (_, r) => (
        <div>
          <div>{r.stock_name || r.stock_code}</div>
          <div style={{ fontSize: 12, color: '#8c8c8c' }}>{r.stock_code}</div>
        </div>
      ),
    },
    {
      title: '方向', dataIndex: 'side', key: 'side', width: 60,
      render: s => <Tag color={sideColor(s)}>{sideLabel(s)}</Tag>,
    },
    { title: '类型', dataIndex: 'type', key: 'type', width: 60 },
    { title: '价格', dataIndex: 'price', key: 'price', align: 'right', render: v => formatMoney(v) },
    { title: '数量', dataIndex: 'volume', key: 'volume', align: 'right', render: v => v?.toLocaleString() },
    { title: '已成交', dataIndex: 'filled_vol', key: 'filled_vol', align: 'right', render: v => v?.toLocaleString() || '0' },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 80,
      render: s => {
        const colors = { PENDING: 'default', SUBMITTED: 'processing', PARTIAL: 'warning', FILLED: 'success', CANCELLED: 'default', REJECTED: 'error' };
        const labels = { PENDING: '待提交', SUBMITTED: '已提交', PARTIAL: '部分成交', FILLED: '已成交', CANCELLED: '已撤销', REJECTED: '已拒绝' };
        return <Tag color={colors[s]}>{labels[s] || s}</Tag>;
      },
    },
    {
      title: '操作', key: 'action', width: 60,
      render: (_, r) => (r.status === 'PENDING' || r.status === 'SUBMITTED') ? (
        <Button size="small" danger icon={<DeleteOutlined />} onClick={() => handleCancelOrder(r.id)}>撤单</Button>
      ) : null,
    },
  ];

  return (
    <div>
      {/* 券商连接状态 */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Row align="middle" justify="space-between">
          <Col>
            <Space>
              <Badge status={loggedIn ? 'success' : 'error'} />
              <span style={{ fontWeight: 500 }}>{loggedIn ? '券商已连接' : '券商未连接'}</span>
              {account && (
                <span style={{ color: '#8c8c8c' }}>
                  ({account.broker_name || account.broker_id})
                </span>
              )}
            </Space>
          </Col>
          <Col>
            <Space>
              {!loggedIn ? (
                <Button type="primary" icon={<LoginOutlined />} onClick={handleLogin}>连接券商</Button>
              ) : (
                <Button icon={<LogoutOutlined />} onClick={handleLogout}>断开连接</Button>
              )}
              <Button icon={<ReloadOutlined />} onClick={fetchData}>刷新数据</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setOrderModal(true)} disabled={!loggedIn}>
                手动下单
              </Button>
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 账户概览 */}
      {account && (
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={12} sm={6}>
            <Card size="small" className="stat-card">
              <Statistic title="总资产" value={parseFloat(account.total_assets || 0)} precision={2} prefix="¥" />
            </Card>
          </Col>
          <Col xs={12} sm={6}>
            <Card size="small" className="stat-card">
              <Statistic title="可用资金" value={parseFloat(account.cash || 0)} precision={2} prefix="¥" />
            </Card>
          </Col>
          <Col xs={12} sm={6}>
            <Card size="small" className="stat-card">
              <Statistic
                title="今日盈亏"
                value={parseFloat(account.today_pl || 0)}
                precision={2}
                prefix="¥"
                valueStyle={{ color: parseFloat(account.today_pl || 0) >= 0 ? '#f5222d' : '#52c41a' }}
              />
            </Card>
          </Col>
          <Col xs={12} sm={6}>
            <Card size="small" className="stat-card">
              <Statistic
                title="累计盈亏"
                value={parseFloat(account.total_pl || 0)}
                precision={2}
                prefix="¥"
                valueStyle={{ color: parseFloat(account.total_pl || 0) >= 0 ? '#f5222d' : '#52c41a' }}
              />
            </Card>
          </Col>
        </Row>
      )}

      {/* 持仓 */}
      <Card title="当前持仓" size="small" style={{ marginBottom: 16 }}>
        <Table
          dataSource={positions}
          columns={positionColumns}
          rowKey="stock_code"
          size="small"
          pagination={false}
          loading={loading}
          locale={{ emptyText: '暂无持仓' }}
          scroll={{ x: 800 }}
        />
      </Card>

      {/* 委托单 */}
      <Card title="委托订单" size="small">
        <Table
          dataSource={orders}
          columns={orderColumns}
          rowKey="id"
          size="small"
          pagination={{ pageSize: 10 }}
          loading={loading}
          locale={{ emptyText: '暂无委托' }}
          scroll={{ x: 800 }}
        />
      </Card>

      {/* 下单弹窗 */}
      <Modal
        title="手动下单"
        open={orderModal}
        onOk={handleSubmitOrder}
        onCancel={() => { setOrderModal(false); orderForm.resetFields(); }}
        okText="确认下单"
        cancelText="取消"
      >
        <Form form={orderForm} layout="vertical" initialValues={{ side: 'BUY', type: 'LIMIT' }}>
          <Form.Item label="股票代码" name="stock_code" rules={[{ required: true, message: '请输入股票代码' }]}>
            <Input placeholder="如 600519" />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item label="方向" name="side" rules={[{ required: true }]}>
                <Select options={[
                  { value: 'BUY', label: '买入' },
                  { value: 'SELL', label: '卖出' },
                ]} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="订单类型" name="type">
                <Select options={[
                  { value: 'LIMIT', label: '限价单' },
                  { value: 'MARKET', label: '市价单' },
                ]} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item label="价格" name="price" rules={[{ required: true, message: '请输入价格' }]}>
                <InputNumber style={{ width: '100%' }} min={0} step={0.01} placeholder="0.00" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="数量" name="volume" rules={[{ required: true, message: '请输入数量' }]}>
                <InputNumber style={{ width: '100%' }} min={100} step={100} placeholder="100的整数倍" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item label="策略ID(可选)" name="strategy_id">
            <Input placeholder="手动下单可留空" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}

