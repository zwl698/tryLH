import React, {useEffect, useState} from 'react';
import {Alert, Badge, Button, Checkbox, Col, Collapse, Form, Input, message, Modal, Row, Select, Space, Tag, Tooltip} from 'antd';
import {LoginOutlined, LogoutOutlined, SafetyCertificateOutlined} from '@ant-design/icons';
import {brokerLogin, brokerLogout, getBrokerProviders, getBrokerStatus} from '../services/api';

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

function brokerTag(status) {
  const current = status?.current || {};
  if (status?.live_trading) return { color: 'red', text: '实盘已连接' };
  if (current.type && current.type !== 'simulated' && current.is_demo === false) {
    return { color: 'orange', text: '实盘未登录' };
  }
  if (current.type && current.type !== 'simulated') return { color: 'blue', text: '仿真已连接' };
  return { color: 'default', text: '模拟账户' };
}

export default function BrokerConnect({compact = false, onChanged}) {
  const [messageApi, contextHolder] = message.useMessage();
  const [providers, setProviders] = useState(DEFAULT_BROKER_PROVIDERS);
  const [status, setStatus] = useState(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm();
  const selectedType = Form.useWatch('type', form) || 'csc';

  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, []);

  const refresh = async () => {
    try {
      const [providerData, statusData] = await Promise.allSettled([
        getBrokerProviders(),
        getBrokerStatus(),
      ]);
      if (providerData.status === 'fulfilled' && Array.isArray(providerData.value) && providerData.value.length > 0) {
        setProviders(providerData.value);
      }
      if (statusData.status === 'fulfilled') {
        setStatus(statusData.value);
      }
    } catch {}
  };

  const openModal = async () => {
    await refresh();
    const current = status?.current || {};
    const type = current.type && current.type !== 'simulated' ? current.type : 'csc';
    const sameType = current.type === type;
    const isDemo = sameType ? current.is_demo === true : false;
    form.setFieldsValue({
      type,
      id: sameType && current.id ? current.id : `${type}_runtime`,
      name: sameType && current.name ? current.name : providerName(providers, type),
      api_url: sameType ? current.api_url || defaultApiUrl(type, isDemo) : defaultApiUrl(type, isDemo),
      account_id: sameType ? current.account_id || '' : '',
      is_demo: isDemo,
      comm_type: sameType ? current.comm_type || 'http' : 'http',
      app_key: sameType ? current.app_key || '' : '',
    });
    setModalOpen(true);
  };

  const handleLogin = async () => {
    try {
      const values = await form.validateFields();
      const provider = providers.find(p => p.type === values.type);

      setLoading(true);
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
      const nextStatus = data?.status || null;
      setStatus(nextStatus);
      setModalOpen(false);
      form.resetFields(['password', 'app_secret']);
      messageApi.success(`${provider?.name || '券商'}登录成功`);
      onChanged?.(nextStatus);
    } catch (e) {
      if (e.message) messageApi.error('登录失败: ' + e.message);
    } finally {
      setLoading(false);
    }
  };

  const handleLogout = async () => {
    try {
      setLoading(true);
      const data = await brokerLogout();
      const nextStatus = data?.status || null;
      setStatus(nextStatus);
      messageApi.success('券商已断开');
      onChanged?.(nextStatus);
    } catch (e) {
      messageApi.error('断开失败: ' + (e.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  const current = status?.current || {};
  const connected = !!status?.logged_in;
  const currentName = current.name || providerName(providers, current.type);
  const tag = brokerTag(status);
  const statusText = connected ? `${currentName}${current.account_id ? ` / ${current.account_id}` : ''}` : '未连接券商';

  return (
    <>
      {contextHolder}
      <Space size={compact ? 8 : 12} wrap={!compact}>
        <Tooltip title={statusText}>
          <Space size={6}>
            <Badge status={connected ? 'success' : 'error'} />
            <SafetyCertificateOutlined style={{ color: status?.live_trading ? '#cf1322' : '#1677ff' }} />
            {!compact && <span style={{ fontWeight: 500 }}>{currentName}</span>}
            <Tag color={tag.color} style={{ margin: 0 }}>{tag.text}</Tag>
          </Space>
        </Tooltip>
        <Button
          size={compact ? 'small' : 'middle'}
          type={status?.live_trading ? 'primary' : 'default'}
          danger={status?.live_trading}
          icon={<LoginOutlined />}
          onClick={openModal}
        >
          切换/登录券商
        </Button>
        {connected && (
          <Button
            size={compact ? 'small' : 'middle'}
            icon={<LogoutOutlined />}
            loading={loading}
            onClick={handleLogout}
          >
            断开
          </Button>
        )}
      </Space>

      <Modal
        title="运行时券商切换与登录"
        open={modalOpen}
        onOk={handleLogin}
        onCancel={() => { setModalOpen(false); form.resetFields(['password', 'app_secret']); }}
        okText="登录并切换"
        cancelText="取消"
        width={660}
        confirmLoading={loading}
      >
        <Alert
          type={selectedType === 'simulated' ? 'info' : 'warning'}
          showIcon
          style={{ marginBottom: 16 }}
          message={selectedType === 'csc' ? '中信建投实盘接入' : '运行时券商连接'}
          description={selectedType === 'csc'
            ? '只需填写资金账号和交易密码。系统已默认中信建投实盘网关、HTTP通信和连接名称；特殊环境可展开高级配置。'
            : selectedType === 'simulated'
              ? '模拟券商只用于开发、演示和策略联调，不连接真实资金账户。'
              : '请确认已取得对应券商的API/SDK授权和交易权限。'}
        />

        <Form
          form={form}
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
                  options={providers.map(provider => ({
                    value: provider.type,
                    label: provider.supports_live ? `${provider.name}（可实盘）` : provider.name,
                  }))}
                  onChange={(type) => {
                    const provider = providers.find(p => p.type === type);
                    const isDemo = type === 'simulated';
                    form.setFieldsValue({
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
                <Tag color={selectedType === 'simulated' ? 'default' : 'red'} style={{ marginTop: 4 }}>
                  {selectedType === 'simulated' ? '模拟账户' : '实盘默认'}
                </Tag>
              </Form.Item>
            </Col>
          </Row>

          {selectedType !== 'simulated' && (
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
                                if (selectedType !== 'csc') return;
                                const currentUrl = form.getFieldValue('api_url');
                                if (!currentUrl || currentUrl === CSC_LIVE_API_URL || currentUrl === CSC_DEMO_API_URL) {
                                  form.setFieldsValue({ api_url: defaultApiUrl('csc', event.target.checked) });
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
    </>
  );
}
