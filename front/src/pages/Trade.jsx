import React, {useEffect, useState} from 'react';
import {
    Alert,
    Badge,
    Button,
    Card,
    Checkbox,
    Col,
    Collapse,
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
    getBrokerProviders,
    getBrokerStatus,
    getOrders,
    getPositions,
    submitOrder
} from '../services/api';
import {formatMoney, sideColor, sideLabel} from '../utils/format';

const DEFAULT_BROKER_PROVIDERS = [
  { type: 'simulated', name: '模拟券商', supports_live: false },
  { type: 'csc', name: '中信建投证券', supports_live: true },
  { type: 'xtquant', name: 'QMT / 迅投', supports_live: true },
  { type: 'cj', name: '长江证券', supports_live: true },
];
const CSC_LIVE_API_URL = 'https://quant.csc108.com/api/v1';
const CSC_DEMO_API_URL = 'https://quant-demo.csc108.com/api/v1';

function providerName(providers, type) {
  return (providers.find(p => p.type === type) || {}).name || type || '未选择';
}

function defaultApiUrl(type, isDemo) {
  if (type === 'csc') return isDemo ? CSC_DEMO_API_URL : CSC_LIVE_API_URL;
  return '';
}

export default function TradePage() {
  const [messageApi, contextHolder] = message.useMessage();
  const [account, setAccount] = useState(null);
  const [positions, setPositions] = useState([]);
  const [orders, setOrders] = useState([]);
  const [loading, setLoading] = useState(false);
  const [orderModal, setOrderModal] = useState(false);
  const [loginModal, setLoginModal] = useState(false);
  const [brokerProviders, setBrokerProviders] = useState(DEFAULT_BROKER_PROVIDERS);
  const [brokerStatus, setBrokerStatus] = useState(null);
  const [orderForm] = Form.useForm();
  const [loginForm] = Form.useForm();
  const [loggedIn, setLoggedIn] = useState(false);
  const selectedBrokerType = Form.useWatch('type', loginForm) || 'csc';

  useEffect(() => {
    fetchBrokerProviders();
    fetchData();
  }, []);

  const fetchBrokerProviders = async () => {
    try {
      const data = await getBrokerProviders();
      if (Array.isArray(data) && data.length > 0) {
        setBrokerProviders(data);
      }
    } catch {}
  };

  const fetchData = async () => {
    setLoading(true);
    try {
      const [statusData, accData, posData, ordData] = await Promise.allSettled([
        getBrokerStatus(), getAccount(), getPositions(), getOrders(),
      ]);
      if (statusData.status === 'fulfilled') {
        setBrokerStatus(statusData.value);
        setLoggedIn(!!statusData.value?.logged_in);
      }
      if (accData.status === 'fulfilled') { setAccount(accData.value); setLoggedIn(true); }
      if (posData.status === 'fulfilled') setPositions(posData.value || []);
      if (ordData.status === 'fulfilled') setOrders(ordData.value || []);
    } catch {}
    setLoading(false);
  };

  const openLoginModal = () => {
    const current = brokerStatus?.current || {};
    const type = current.type && current.type !== 'simulated' ? current.type : 'csc';
    const isSameType = current.type === type;
    const isDemo = isSameType ? current.is_demo === true : false;
    loginForm.setFieldsValue({
      type,
      id: isSameType && current.id ? current.id : `${type}_runtime`,
      name: isSameType && current.name ? current.name : providerName(brokerProviders, type),
      api_url: isSameType ? current.api_url || defaultApiUrl(type, isDemo) : defaultApiUrl(type, isDemo),
      account_id: isSameType ? current.account_id || '' : '',
      is_demo: isDemo,
      comm_type: isSameType ? current.comm_type || 'http' : 'http',
      app_key: isSameType ? current.app_key || '' : '',
    });
    setLoginModal(true);
  };

  const handleLogin = async () => {
    try {
      const values = await loginForm.validateFields();
      const provider = brokerProviders.find(p => p.type === values.type);

      const payload = {
        id: values.id || `${values.type}_runtime`,
        name: values.name || provider?.name || values.type,
        type: values.type,
        api_url: values.api_url || '',
        account_id: values.account_id || '',
        password: values.password || '',
        is_demo: values.type === 'simulated' ? true : values.is_demo === true,
        comm_type: values.comm_type || 'http',
        app_key: values.app_key || '',
        app_secret: values.app_secret || '',
      };

      const data = await brokerLogin(payload);
      setBrokerStatus(data?.status || null);
      messageApi.success(`${provider?.name || '券商'}登录成功`);
      setLoginModal(false);
      loginForm.resetFields(['password', 'app_secret']);
      setLoggedIn(true);
      fetchData();
    } catch (e) {
      if (e.message) messageApi.error('登录失败: ' + e.message);
    }
  };

  const handleLogout = async () => {
    try {
      const data = await brokerLogout();
      setBrokerStatus(data?.status || null);
      messageApi.success('券商已登出');
      setLoggedIn(false);
    } catch (e) {
      messageApi.error('登出失败: ' + (e.message || ''));
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
      messageApi.success('下单成功');
      setOrderModal(false);
      orderForm.resetFields();
      fetchData();
    } catch (e) {
      if (e.message) messageApi.error('下单失败: ' + e.message);
    }
  };

  const handleCancelOrder = async (id) => {
    try {
      await cancelOrder(id);
      messageApi.success('撤单成功');
      fetchData();
    } catch (e) {
      messageApi.error('撤单失败: ' + (e.message || ''));
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

  const currentBroker = brokerStatus?.current || {};
  const currentBrokerType = currentBroker.type || 'simulated';
  const currentBrokerName = currentBroker.name || account?.broker_name || providerName(brokerProviders, currentBrokerType);
  const isLiveTrading = !!brokerStatus?.live_trading;

  return (
    <div>
      {contextHolder}
      {/* 券商连接状态 */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Row align="middle" justify="space-between">
          <Col>
            <Space>
              <Badge status={loggedIn ? 'success' : 'error'} />
              <span style={{ fontWeight: 500 }}>{loggedIn ? '券商已连接' : '券商未连接'}</span>
              <Tag color={isLiveTrading ? 'red' : currentBrokerType === 'simulated' ? 'default' : 'blue'}>
                {isLiveTrading ? '实盘' : currentBroker?.is_demo === false ? '实盘未登录' : '仿真/模拟'}
              </Tag>
              <span style={{ color: '#8c8c8c' }}>
                {currentBrokerName} {currentBroker.account_id ? `(${currentBroker.account_id})` : ''}
              </span>
            </Space>
          </Col>
          <Col>
            <Space>
              {!loggedIn ? (
                <Button type="primary" icon={<LoginOutlined />} onClick={openLoginModal}>连接券商</Button>
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

      {/* 券商登录弹窗 */}
      <Modal
        title="连接券商"
        open={loginModal}
        onOk={handleLogin}
        onCancel={() => { setLoginModal(false); loginForm.resetFields(['password', 'app_secret']); }}
        okText="登录"
        cancelText="取消"
        width={620}
      >
        <Alert
          type={selectedBrokerType === 'simulated' ? 'info' : 'warning'}
          showIcon
          style={{ marginBottom: 16 }}
          message={selectedBrokerType === 'csc' ? '中信建投实盘接入' : '券商连接'}
          description={selectedBrokerType === 'csc'
            ? '只需填写资金账号和交易密码。系统已默认中信建投实盘网关、HTTP通信和连接名称；特殊环境可展开高级配置。'
            : selectedBrokerType === 'simulated'
              ? '模拟券商不连接真实资金账户，适合联调策略和手动下单流程。'
              : '请确认已取得对应券商的API/SDK授权和交易权限。'}
        />

        <Form
          form={loginForm}
          layout="vertical"
          initialValues={{
            type: 'csc',
            id: 'csc_runtime',
            name: '中信建投证券',
            api_url: CSC_LIVE_API_URL,
            is_demo: false,
            comm_type: 'http',
          }}
        >
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item label="券商" name="type" rules={[{ required: true, message: '请选择券商' }]}>
                <Select
                  options={brokerProviders.map(provider => ({
                    value: provider.type,
                    label: provider.supports_live ? `${provider.name}（可实盘）` : provider.name,
                  }))}
                  onChange={(type) => {
                    const provider = brokerProviders.find(p => p.type === type);
                    const isDemo = type === 'simulated';
                    loginForm.setFieldsValue({
                      id: `${type}_runtime`,
                      name: provider?.name || type,
                      api_url: defaultApiUrl(type, isDemo),
                      is_demo: isDemo,
                      comm_type: 'http',
                      app_key: '',
                      app_secret: '',
                    });
                  }}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="连接模式">
                <Tag color={selectedBrokerType === 'simulated' ? 'default' : 'red'} style={{ marginTop: 4 }}>
                  {selectedBrokerType === 'simulated' ? '模拟账户' : '实盘默认'}
                </Tag>
              </Form.Item>
            </Col>
          </Row>

          {selectedBrokerType !== 'simulated' && (
            <>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item label="资金账号" name="account_id" rules={[{ required: true, message: '请输入资金账号' }]}>
                    <Input autoComplete="off" />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label="交易密码" name="password" rules={[{ required: true, message: '请输入交易密码' }]}>
                    <Input.Password autoComplete="new-password" />
                  </Form.Item>
                </Col>
              </Row>

              <Collapse
                ghost
                size="small"
                items={[{
                  key: 'advanced',
                  label: '高级配置',
                  children: (
                    <>
                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item label="连接ID" name="id">
                            <Input placeholder="如 csc_runtime" />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item label="显示名称" name="name">
                            <Input placeholder="如 中信建投证券" />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Form.Item label="API网关地址" name="api_url">
                        <Input placeholder="券商提供的量化交易网关地址" />
                      </Form.Item>
                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item label="通信方式" name="comm_type">
                            <Select options={[
                              { value: 'http', label: 'HTTP API' },
                              { value: 'tcp', label: 'TCP 网关' },
                              { value: 'dll', label: '本地 DLL/终端' },
                            ]} />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item label="环境" name="is_demo" valuePropName="checked">
                            <Checkbox
                              onChange={(event) => {
                                if (selectedBrokerType !== 'csc') return;
                                const currentUrl = loginForm.getFieldValue('api_url');
                                if (!currentUrl || currentUrl === CSC_LIVE_API_URL || currentUrl === CSC_DEMO_API_URL) {
                                  loginForm.setFieldsValue({ api_url: defaultApiUrl('csc', event.target.checked) });
                                }
                              }}
                            >
                              使用仿真环境
                            </Checkbox>
                          </Form.Item>
                        </Col>
                      </Row>
                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item label="AppKey（可选）" name="app_key">
                            <Input autoComplete="off" />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item label="AppSecret（可选）" name="app_secret">
                            <Input.Password autoComplete="new-password" />
                          </Form.Item>
                        </Col>
                      </Row>
                    </>
                  ),
                }]}
              />
            </>
          )}
        </Form>
      </Modal>

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
