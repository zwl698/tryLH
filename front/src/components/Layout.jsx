import React, {useEffect, useState} from 'react';
import {Outlet, useLocation, useNavigate} from 'react-router-dom';
import {Badge, Layout, Menu, theme} from 'antd';
import {
  ApiOutlined,
  DashboardOutlined,
  FundOutlined,
  LineChartOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  SwapOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import {getSystemStatus} from '../services/api';
import BrokerConnect from './BrokerConnect';

const { Header, Sider, Content } = Layout;

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
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        style={{
          background: token.colorBgContainer,
          borderRight: '1px solid #f0f0f0',
        }}
      >
        <div style={{
          height: 64,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          borderBottom: '1px solid #f0f0f0',
        }}>
          <ApiOutlined style={{ fontSize: 24, color: '#1677ff' }} />
          {!collapsed && (
            <span style={{ marginLeft: 8, fontSize: 16, fontWeight: 600, whiteSpace: 'nowrap' }}>
              A股量化系统
            </span>
          )}
        </div>
        <Menu
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          style={{ borderRight: 0 }}
        />
      </Sider>
      <Layout>
        <Header style={{
          padding: '0 24px',
          background: token.colorBgContainer,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          borderBottom: '1px solid #f0f0f0',
          height: 64,
        }}>
          <span style={{ fontSize: 16, fontWeight: 500 }}>
            A股量化交易系统
          </span>
          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <BrokerConnect compact onChanged={fetchStatus} />
            {systemStatus && (
              <>
                <Badge
                  status="processing"
                  text={`${systemStatus.strategies || 0} 个策略`}
                />
                <span style={{ color: '#8c8c8c', fontSize: 12 }}>
                  {systemStatus.timestamp ? new Date(systemStatus.timestamp).toLocaleTimeString('zh-CN') : ''}
                </span>
              </>
            )}
          </div>
        </Header>
        <Content style={{
          margin: 16,
          padding: 0,
          minHeight: 280,
          overflow: 'auto',
        }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
