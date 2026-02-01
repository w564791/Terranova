#!/bin/bash

# 后端服务启动脚本
set -e

BACKEND_DIR="backend"
LOG_DIR="$BACKEND_DIR/logs"
PID_FILE="$LOG_DIR/server.pid"
LOG_FILE="$LOG_DIR/server.log"

# 创建日志目录
mkdir -p "$LOG_DIR"

# 停止现有服务
stop_service() {
    echo "🛑 Stopping existing backend service..."
    pkill -f "go run main.go" 2>/dev/null || true
    lsof -ti:8080 | xargs kill -9 2>/dev/null || true
    rm -f "$PID_FILE"
    sleep 1
}

# 启动服务
start_service() {
    echo "🚀 Starting backend service..."
    cd "$BACKEND_DIR"
    nohup go run main.go > "logs/server.log" 2>&1 &
    echo $! > "logs/server.pid"
    echo "📝 Backend service started with PID: $(cat logs/server.pid)"
    cd ..
}

# 验证服务
verify_service() {
    echo "⏳ Waiting for service to start..."
    sleep 3
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        echo " Backend service is running successfully!"
        echo "🌐 Health check: http://localhost:8080/health"
    else
        echo "❌ Failed to start backend service"
        echo "📋 Check logs: tail -f $LOG_FILE"
        return 1
    fi
}

# 主函数
main() {
    case "${1:-start}" in
        "start")
            stop_service
            start_service
            verify_service
            ;;
        "stop")
            stop_service
            echo " Backend service stopped"
            ;;
        "logs")
            tail -f "$LOG_FILE"
            ;;
        *)
            echo "Usage: $0 {start|stop|logs}"
            ;;
    esac
}

main "$@"