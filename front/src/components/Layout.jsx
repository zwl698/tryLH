import React, {useEffect, useMemo, useState} from 'react';
import {Outlet, useLocation, useNavigate} from 'react-router-dom';
import {Badge, Layout, Menu, Space, Tag, theme} from 'antd';
import {
  ApiOutlined,
  DashboardOutlined,
  FundOutlined,
  LineChartOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  StockOutlined,
  SwapOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import {getSystemStatus} from '../services/api';
import BrokerConnect from './BrokerConnect';

const { Header, Sider, Content } = Layout;
const APP_NAME = 'A股Win量化系统';

const menuItems = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '系统总览' },
  { key: '/market', icon: <LineChartOutlined />, label: '行情中心' },
  { key: '/smart-trade', icon: <ThunderboltOutlined />, label: '智能交易' },
  { key: '/strategy', icon: <RobotOutlined />, label: '策略管理' },
  { key: '/trade', icon: <SwapOutlined />, label: '券商/交易' },
  { key: '/backtest', icon: <FundOutlined />, label: '策略回测' },
  { key: '/risk', icon: <SafetyCertificateOutlined />, label: '风控管理' },
];

export default function MainLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const [systemStatus, setSystemStatus] = useState(null);
  const navigate = useNavigate();
  const location = useLocation();
  const { token } = theme.useToken();
  const activeTitle = useMemo(
    () => menuItems.find(item => item.key === location.pathname)?.label || '系统总览',
    [location.pathname]
  );

  useEffect(() => {
    fetchStatus();
    const interval = setInterval(fetchStatus, 10000);
    return () => clearInterval(interval);
  }, []);

  const fetchStatus = async () => {
    try {
      const status = await getSystemStatus();
      setSystemStatus(status);
    } catch {
      setSystemStatus(null);
    }
  };

  return (
    <Layout className="quant-shell">
      <Sider
        className="quant-sider"
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        width={238}
      >
        <div className="quant-brand">
          <span className="quant-brand-mark">
            <ApiOutlined />
          </span>
          {!collapsed && (
            <span className="quant-brand-text">
              <strong>{APP_NAME}</strong>
              <span>AI A-SHARE QUANT TERMINAL</span>
            </span>
          )}
        </div>
        <Menu
          className="quant-menu"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          style={{ borderRight: 0, background: 'transparent' }}
        />
      </Sider>
      <Layout>
        <Header className="quant-header">
          <div className="quant-header-title">
            <span>{activeTitle}</span>
            <Tag color="cyan" style={{ marginInlineStart: 10 }}>实时研究台</Tag>
          </div>
          <Space size={14} className="quant-header-actions">
            <BrokerConnect compact onChanged={fetchStatus} />
            {systemStatus && (
              <>
                <Badge
                  status="processing"
                  text={`${systemStatus.strategies || 0} 个策略`}
                />
                <span className="quant-clock">
                  <StockOutlined style={{ marginRight: 6, color: token.colorPrimary }} />
                  {systemStatus.timestamp ? new Date(systemStatus.timestamp).toLocaleTimeString('zh-CN') : ''}
                </span>
              </>
            )}
          </Space>
        </Header>
        <Content className="quant-content">
          <div className="quant-content-inner">
            <Outlet />
          </div>
        </Content>
      </Layout>
    </Layout>
  );
}
