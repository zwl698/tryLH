import React, {useEffect, useState} from 'react';
import {
    Alert,
    Button,
    Card,
    Col,
    Descriptions,
    Form,
    Input,
    InputNumber,
    message,
    Modal,
    Row,
    Space,
    Spin,
    Table,
    Tag,
    Tooltip
} from 'antd';
import {
    InfoCircleOutlined,
    PauseCircleOutlined,
    PlayCircleOutlined,
    SettingOutlined,
    StopOutlined
} from '@ant-design/icons';
import {
    getBrokerStatus,
    getStrategy,
    getStrategyParamDefs,
    getStrategyTemplates,
    listStrategies,
    pauseStrategy,
    startStrategy,
    stopStrategy,
    updateStrategyParams
} from '../services/api';
import {strategyStatusColor, strategyStatusLabel} from '../utils/format';
import BrokerConnect from '../components/BrokerConnect';

const STRATEGY_TYPE_INFO = {
  double_ma: { name: '双均线交叉', color: '#1677ff', icon: '📈' },
  turtle: { name: '海龟交易', color: '#722ed1', icon: '🐢' },
  momentum: { name: '动量策略', color: '#eb2f96', icon: '🚀' },
  mean_reversion: { name: '均值回归', color: '#13c2c2', icon: '📊' },
  grid: { name: '网格交易', color: '#fa8c16', icon: '🔲' },
};

export default function StrategyPage() {
  const [strategies, setStrategies] = useState([]);
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(true);
  const [detailModal, setDetailModal] = useState(false);
  const [paramsModal, setParamsModal] = useState(false);
  const [selectedStrategy, setSelectedStrategy] = useState(null);
  const [paramDefs, setParamDefs] = useState([]);
  const [brokerStatus, setBrokerStatus] = useState(null);
  const [paramsForm] = Form.useForm();

  useEffect(() => {
    fetchStrategies();
    fetchTemplates();
    fetchBrokerStatus();
  }, []);

  const fetchStrategies = async () => {
    setLoading(true);
    try {
      const data = await listStrategies();
      setStrategies(data || []);
    } catch {}
    setLoading(false);
  };

  const fetchTemplates = async () => {
    try {
      const data = await getStrategyTemplates();
      setTemplates(data || []);
    } catch {}
  };

  const fetchBrokerStatus = async () => {
    try {
      const data = await getBrokerStatus();
      setBrokerStatus(data);
    } catch {
      setBrokerStatus(null);
    }
  };

  const handleStart = async (id) => {
    try {
      await startStrategy(id);
      message.success('策略已启动');
      fetchStrategies();
    } catch (e) {
      message.error('启动失败: ' + (e.message || '未知错误'));
    }
  };

  const handlePause = async (id) => {
    try {
      await pauseStrategy(id);
      message.success('策略已暂停');
      fetchStrategies();
    } catch (e) {
      message.error('暂停失败: ' + (e.message || '未知错误'));
    }
  };

  const handleStop = async (id) => {
    try {
      await stopStrategy(id);
      message.success('策略已停止');
      fetchStrategies();
    } catch (e) {
      message.error('停止失败: ' + (e.message || '未知错误'));
    }
  };

  const handleViewDetail = async (id) => {
    try {
      const data = await getStrategy(id);
      setSelectedStrategy(data);
      setDetailModal(true);
    } catch {}
  };

  const handleEditParams = async (id) => {
    try {
      const [strategy, defs] = await Promise.all([
        getStrategy(id),
        getStrategyParamDefs(id),
      ]);
      setSelectedStrategy(strategy);
      setParamDefs(defs || []);
      const formValues = {};
      (defs || []).forEach(d => {
        formValues[d.key] = strategy.params?.[d.key] !== undefined ? strategy.params[d.key] : d.default;
      });
      paramsForm.setFieldsValue(formValues);
      setParamsModal(true);
    } catch {}
  };

  const handleSaveParams = async () => {
    try {
      const values = await paramsForm.validateFields();
      await updateStrategyParams(selectedStrategy.id, values);
      message.success('参数更新成功');
      setParamsModal(false);
      fetchStrategies();
    } catch (e) {
      if (e.message) message.error('更新失败: ' + e.message);
    }
  };

  const columns = [
    {
      title: '策略', key: 'name', render: (_, r) => {
        const info = STRATEGY_TYPE_INFO[r.type] || {};
        return (
          <Space>
            <span style={{ fontSize: 18 }}>{info.icon || '⚙️'}</span>
            <div>
              <div style={{ fontWeight: 500 }}>{r.name}</div>
              <Tag color={info.color} style={{ margin: 0 }}>{info.name || r.type}</Tag>
            </div>
          </Space>
        );
      },
    },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 100,
      render: s => <Tag color={strategyStatusColor(s)}>{strategyStatusLabel(s)}</Tag>,
    },
    {
      title: '描述', dataIndex: 'description', key: 'description', ellipsis: { showTitle: false },
      render: t => <Tooltip title={t}>{t}</Tooltip>,
    },
    {
      title: '参数', key: 'params', width: 200, ellipsis: true,
      render: (_, r) => {
        if (!r.params) return '--';
        return Object.entries(r.params).map(([k, v]) => (
          <Tag key={k} style={{ margin: 2 }}>{k}: {typeof v === 'object' ? JSON.stringify(v) : String(v)}</Tag>
        ));
      },
    },
    {
      title: '操作', key: 'action', width: 220, fixed: 'right',
      render: (_, r) => (
        <Space size="small">
          {r.status !== 'ACTIVE' && (
            <Button size="small" type="primary" icon={<PlayCircleOutlined />} onClick={() => handleStart(r.id)}>启动</Button>
          )}
          {r.status === 'ACTIVE' && (
            <Button size="small" icon={<PauseCircleOutlined />} onClick={() => handlePause(r.id)}>暂停</Button>
          )}
          {r.status !== 'STOPPED' && r.status !== 'PAUSED' && (
            <Button size="small" danger icon={<StopOutlined />} onClick={() => handleStop(r.id)}>停止</Button>
          )}
          <Button size="small" icon={<SettingOutlined />} onClick={() => handleEditParams(r.id)}>参数</Button>
          <Button size="small" icon={<InfoCircleOutlined />} onClick={() => handleViewDetail(r.id)}>详情</Button>
        </Space>
      ),
    },
  ];

  const currentBroker = brokerStatus?.current || {};
  const currentBrokerType = currentBroker.type || 'simulated';
  const currentBrokerName = currentBroker.name || '模拟券商';
  const brokerAlertType = brokerStatus?.live_trading ? 'error' : currentBrokerType === 'simulated' ? 'warning' : 'info';
  const brokerMessage = brokerStatus?.live_trading
    ? `策略实时交易目标：${currentBrokerName} 实盘账户`
    : currentBrokerType === 'simulated'
      ? '当前策略实时交易目标：模拟券商'
      : `当前策略实时交易目标：${currentBrokerName} 仿真/未实盘`;
  const brokerDescription = brokerStatus?.live_trading
    ? '启动策略后，策略信号会经过风控检查并提交到当前真实券商连接。'
    : '启动策略前请先在顶部或此处切换/登录真实券商，否则策略信号只会走当前模拟或仿真连接。';

  return (
    <Spin spinning={loading}>
      <div>
        <Alert
          showIcon
          type={brokerAlertType}
          message={brokerMessage}
          description={brokerDescription}
          action={<BrokerConnect compact onChanged={setBrokerStatus} />}
          style={{ marginBottom: 16 }}
        />

        {/* 策略模板卡片 */}
        <Row gutter={[16, 16]}>
          {templates.map((t, i) => {
            const info = STRATEGY_TYPE_INFO[t.type] || {};
            return (
              <Col xs={24} sm={12} md={8} lg={4} key={i}>
                <Card
                  size="small"
                  hoverable
                  style={{ borderTop: `3px solid ${info.color || '#1677ff'}` }}
                >
                  <div style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: 32, marginBottom: 8 }}>{info.icon || '⚙️'}</div>
                    <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 4 }}>{t.name}</div>
                    <Tag color={info.color} style={{ margin: 0 }}>{t.type}</Tag>
                    <div style={{ fontSize: 12, color: '#8c8c8c', marginTop: 8, lineHeight: 1.5, minHeight: 40 }}>
                      {t.description?.substring(0, 50)}...
                    </div>
                  </div>
                </Card>
              </Col>
            );
          })}
        </Row>

        {/* 策略列表 */}
        <Card title="策略列表" style={{ marginTop: 16 }} size="small" extra={
          <Button size="small" onClick={fetchStrategies}>刷新</Button>
        }>
          <Table
            dataSource={strategies}
            columns={columns}
            rowKey="id"
            size="small"
            scroll={{ x: 800 }}
            pagination={false}
          />
        </Card>

        {/* 详情弹窗 */}
        <Modal
          title="策略详情"
          open={detailModal}
          onCancel={() => setDetailModal(false)}
          footer={null}
          width={600}
        >
          {selectedStrategy && (
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="策略ID">{selectedStrategy.id}</Descriptions.Item>
              <Descriptions.Item label="策略名称">{selectedStrategy.name}</Descriptions.Item>
              <Descriptions.Item label="策略类型">
                <Tag color={(STRATEGY_TYPE_INFO[selectedStrategy.type] || {}).color}>{selectedStrategy.type}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={strategyStatusColor(selectedStrategy.status)}>{strategyStatusLabel(selectedStrategy.status)}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="描述" span={2}>{selectedStrategy.description}</Descriptions.Item>
              <Descriptions.Item label="参数" span={2}>
                <pre style={{ margin: 0, fontSize: 12 }}>{JSON.stringify(selectedStrategy.params, null, 2)}</pre>
              </Descriptions.Item>
              {selectedStrategy.config && (
                <>
                  <Descriptions.Item label="最大持仓">{selectedStrategy.config.max_position}</Descriptions.Item>
                  <Descriptions.Item label="止损比例">{selectedStrategy.config.stop_loss}</Descriptions.Item>
                  <Descriptions.Item label="止盈比例">{selectedStrategy.config.take_profit}</Descriptions.Item>
                  <Descriptions.Item label="关注股票">
                    {selectedStrategy.config.stocks?.join(', ') || '未配置'}
                  </Descriptions.Item>
                </>
              )}
            </Descriptions>
          )}
        </Modal>

        {/* 参数编辑弹窗 */}
        <Modal
          title="编辑策略参数"
          open={paramsModal}
          onOk={handleSaveParams}
          onCancel={() => setParamsModal(false)}
          okText="保存"
          cancelText="取消"
        >
          <Form form={paramsForm} layout="vertical">
            {paramDefs.map(def => (
              <Form.Item
                key={def.key}
                label={def.name || def.key}
                name={def.key}
                tooltip={def.description}
              >
                {def.type === 'int' || def.type === 'float' ? (
                  <InputNumber style={{ width: '100%' }} step={def.type === 'float' ? 0.01 : 1} />
                ) : (
                  <Input />
                )}
              </Form.Item>
            ))}
          </Form>
        </Modal>
      </div>
    </Spin>
  );
}
