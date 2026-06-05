import React from 'react';
import {BrowserRouter, Navigate, Route, Routes} from 'react-router-dom';
import {ConfigProvider, theme} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import MainLayout from './components/Layout';
import Dashboard from './pages/Dashboard';
import MarketPage from './pages/Market';
import StrategyPage from './pages/Strategy';
import TradePage from './pages/Trade';
import BacktestPage from './pages/Backtest';
import RiskPage from './pages/Risk';
import SmartTradePage from './pages/SmartTrade';

function App() {
  return (
    <ConfigProvider locale={zhCN} theme={{
      algorithm: theme.darkAlgorithm,
      token: {
        colorPrimary: '#18e7ff',
        colorSuccess: '#24d18d',
        colorWarning: '#f7b955',
        colorError: '#ff4d6d',
        colorInfo: '#18e7ff',
        colorBgBase: '#07111f',
        colorBgContainer: 'rgba(12, 24, 42, 0.86)',
        colorBgElevated: 'rgba(13, 31, 54, 0.96)',
        colorBorder: 'rgba(117, 166, 255, 0.18)',
        colorText: '#e6f4ff',
        colorTextSecondary: '#91a8c5',
        borderRadius: 6,
        fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
      },
      components: {
        Layout: {
          bodyBg: 'transparent',
          headerBg: 'transparent',
          siderBg: 'transparent',
        },
        Card: {
          headerBg: 'transparent',
        },
        Table: {
          headerBg: 'rgba(18, 42, 72, 0.92)',
          rowHoverBg: 'rgba(24, 231, 255, 0.06)',
        },
      },
    }}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<MainLayout />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<Dashboard />} />
            <Route path="market" element={<MarketPage />} />
            <Route path="smart-trade" element={<SmartTradePage />} />
            <Route path="strategy" element={<StrategyPage />} />
            <Route path="trade" element={<TradePage />} />
            <Route path="backtest" element={<BacktestPage />} />
            <Route path="risk" element={<RiskPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  );
}

export default App;
