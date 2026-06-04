// 格式化金额
export function formatMoney(value, decimals = 2) {
  if (!value && value !== 0) return '--';
  const num = typeof value === 'string' ? parseFloat(value) : value;
  if (isNaN(num)) return '--';
  return num.toLocaleString('zh-CN', {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  });
}

// 格式化百分比
export function formatPercent(value, decimals = 2) {
  if (!value && value !== 0) return '--';
  const num = typeof value === 'string' ? parseFloat(value) : value;
  if (isNaN(num)) return '--';
  return (num * 100).toFixed(decimals) + '%';
}

// 格式化数字
export function formatNumber(value, decimals = 2) {
  if (!value && value !== 0) return '--';
  const num = typeof value === 'string' ? parseFloat(value) : value;
  if (isNaN(num)) return '--';
  return num.toFixed(decimals);
}

// 格式化成交量
export function formatVolume(value) {
  if (!value && value !== 0) return '--';
  const num = typeof value === 'string' ? parseFloat(value) : value;
  if (isNaN(num)) return '--';
  if (num >= 100000000) return (num / 100000000).toFixed(2) + '亿';
  if (num >= 10000) return (num / 10000).toFixed(2) + '万';
  return num.toString();
}

// 格式化时间
export function formatTime(timeStr) {
  if (!timeStr) return '--';
  const d = new Date(timeStr);
  return d.toLocaleString('zh-CN');
}

// 股票代码转新浪格式
export function toSinaCode(code) {
  if (!code) return '';
  if (code.startsWith('6') || code.startsWith('9')) return 'sh' + code;
  if (code.startsWith('0') || code.startsWith('3') || code.startsWith('2')) return 'sz' + code;
  return code;
}

// 策略状态标签颜色
export function strategyStatusColor(status) {
  const colors = {
    ACTIVE: 'green',
    PAUSED: 'orange',
    STOPPED: 'default',
    ERROR: 'red',
  };
  return colors[status] || 'default';
}

// 策略状态中文名
export function strategyStatusLabel(status) {
  const labels = {
    ACTIVE: '运行中',
    PAUSED: '已暂停',
    STOPPED: '已停止',
    ERROR: '异常',
  };
  return labels[status] || status;
}

// 订单方向颜色
export function sideColor(side) {
  return side === 'BUY' ? '#f5222d' : '#52c41a';
}

// 订单方向中文名
export function sideLabel(side) {
  return side === 'BUY' ? '买入' : '卖出';
}

