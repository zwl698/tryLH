import axios from 'axios';

const api = axios.create({
  baseURL: '',
  timeout: 15000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 响应拦截器
api.interceptors.response.use(
  (response) => {
    const data = response.data;
    if (data.code === 0) {
      return data.data;
    }
    return Promise.reject(new Error(data.message || '请求失败'));
  },
  (error) => {
    const message = error.response?.data?.message || error.message || '请求失败';
    return Promise.reject(new Error(message));
  }
);

// ====== 券商接口 ======
export const brokerLogin = () => api.post('/api/v1/broker/login');
export const brokerLogout = () => api.post('/api/v1/broker/logout');
export const getAccount = () => api.get('/api/v1/broker/account');
export const getPositions = () => api.get('/api/v1/broker/positions');
export const getOrders = () => api.get('/api/v1/broker/orders');

// ====== 交易接口 ======
export const submitOrder = (data) => api.post('/api/v1/trade/order', data);
export const cancelOrder = (id) => api.delete(`/api/v1/trade/order/${id}`);

// ====== 行情接口 ======
export const getQuote = (code) => api.get(`/api/v1/market/quote/${code}`);
export const getQuotes = async (codes) => {
  const data = await api.post('/api/v1/market/quotes', { codes });
  return Array.isArray(data) ? data : Object.values(data || {});
};
export const getKLines = (code, period = 'day') => api.get(`/api/v1/market/kline/${code}?period=${period}`);
export const getIndexQuote = (code) => api.get(`/api/v1/market/index/${code}`);
export const subscribe = (codes) => api.post('/api/v1/market/subscribe', { codes });

// ====== 策略接口 ======
export const listStrategies = () => api.get('/api/v1/strategies');
export const getStrategy = (id) => api.get(`/api/v1/strategy/${id}`);
export const startStrategy = (id) => api.post(`/api/v1/strategy/${id}/start`);
export const stopStrategy = (id) => api.post(`/api/v1/strategy/${id}/stop`);
export const pauseStrategy = (id) => api.post(`/api/v1/strategy/${id}/pause`);
export const updateStrategyParams = (id, params) => api.put(`/api/v1/strategy/${id}/params`, params);
export const getStrategyParamDefs = (id) => api.get(`/api/v1/strategy/${id}/params-defs`);
export const getStrategyTemplates = () => api.get('/api/v1/strategy-templates');

// ====== 回测接口 ======
export const runBacktest = (data) => api.post('/api/v1/backtest', data);

// ====== 风控接口 ======
export const getRiskConfig = () => api.get('/api/v1/risk/config');
export const updateRiskConfig = (data) => api.put('/api/v1/risk/config', data);
export const getRiskEvents = () => api.get('/api/v1/risk/events');

// ====== 系统状态 ======
export const getSystemStatus = () => api.get('/api/v1/system/status');
export const healthCheck = () => api.get('/health');

export default api;
