#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
FRONTEND_DIR="$ROOT_DIR/front"
BACKEND_PORT=8080
FRONTEND_PORT=3000
BACKEND_PID=""
FRONTEND_PID=""
OPEN_BROWSER="${OPEN_BROWSER:-1}"

kill_port() {
  local port="$1"
  local pids
  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"

  if [[ -n "$pids" ]]; then
    echo "检测到端口 $port 已被占用，正在清理旧进程..."
    echo "$pids" | xargs kill -9 2>/dev/null || true
    sleep 1
  fi
}

cleanup() {
  echo
  echo "正在停止前后端服务..."

  if [[ -n "$BACKEND_PID" ]] && kill -0 "$BACKEND_PID" 2>/dev/null; then
    kill "$BACKEND_PID" 2>/dev/null || true
  fi

  if [[ -n "$FRONTEND_PID" ]] && kill -0 "$FRONTEND_PID" 2>/dev/null; then
    kill "$FRONTEND_PID" 2>/dev/null || true
  fi

  wait 2>/dev/null || true
}

trap cleanup EXIT INT TERM

open_frontend() {
  local url="http://localhost:$FRONTEND_PORT"

  for _ in {1..60}; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      if command -v open >/dev/null 2>&1; then
        open "$url" >/dev/null 2>&1 || true
      elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "$url" >/dev/null 2>&1 || true
      fi
      return
    fi
    sleep 1
  done
}

if [[ ! -f "$BACKEND_DIR/go.mod" ]]; then
  echo "未找到后端项目: $BACKEND_DIR"
  exit 1
fi

if [[ ! -f "$FRONTEND_DIR/package.json" ]]; then
  echo "未找到前端项目: $FRONTEND_DIR"
  exit 1
fi

if [[ ! -d "$FRONTEND_DIR/node_modules" ]]; then
  echo "前端依赖未安装，正在执行 npm install..."
  (cd "$FRONTEND_DIR" && npm install)
fi

kill_port "$BACKEND_PORT"
kill_port "$FRONTEND_PORT"

echo "启动后端服务..."
(
  cd "$BACKEND_DIR"
  go run main.go
) &
BACKEND_PID=$!

echo "启动前端服务..."
(
  cd "$FRONTEND_DIR"
  npm run dev -- --host 0.0.0.0
) &
FRONTEND_PID=$!

echo
printf '后端地址: http://localhost:%s\n' "$BACKEND_PORT"
printf '前端地址: http://localhost:%s\n' "$FRONTEND_PORT"
echo "按 Ctrl+C 可同时停止前后端。"
echo

if [[ "$OPEN_BROWSER" != "0" ]]; then
  open_frontend &
fi

EXIT_CODE=0
while true; do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    wait "$BACKEND_PID" 2>/dev/null || EXIT_CODE=$?
    break
  fi

  if ! kill -0 "$FRONTEND_PID" 2>/dev/null; then
    wait "$FRONTEND_PID" 2>/dev/null || EXIT_CODE=$?
    break
  fi

  sleep 1
done

echo "检测到有服务退出，正在关闭另一项服务..."
exit "$EXIT_CODE"
