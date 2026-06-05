import React from 'react';
import {BrowserRouter, Navigate, Route, Routes} from 'react-router-dom';
import {ConfigProvider} from 'antd';
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
      token: {
        colorPrimary: '#1677ff',
        borderRadius: 6,
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
