# 后端开发指南

## 🚀 服务启动和管理

### 开发环境启动
```bash
# 创建日志目录
mkdir -p backend/logs

# 启动后端服务（后台运行，日志重定向）
cd backend
nohup go run main.go > logs/server.log 2>&1 &
echo $! > logs/server.pid

# 查看服务状态
ps aux | grep "go run main.go"

# 查看实时日志
tail -f logs/server.log

# 查看最近100行日志
tail -n 100 logs/server.log

# 搜索错误日志
grep -i error logs/server.log
grep -i "failed\|panic\|fatal" logs/server.log
```

### 服务管理命令
```bash
# 停止服务
kill $(cat logs/server.pid) 2>/dev/null || pkill -f "go run main.go"

# 重启服务
kill $(cat logs/server.pid) 2>/dev/null
nohup go run main.go > logs/server.log 2>&1 &
echo $! > logs/server.pid

# 检查服务是否运行
curl -s http://localhost:8080/health || echo "Service not running"

# 清理日志文件
> logs/server.log  # 清空日志但保留文件
# 或
rm logs/server.log && touch logs/server.log  # 删除并重新创建
```

### Makefile集成
```makefile
# 添加到 Makefile
.PHONY: start-backend stop-backend restart-backend logs

# 启动后端服务
start-backend:
	@mkdir -p backend/logs
	@cd backend && nohup go run main.go > logs/server.log 2>&1 & echo $$! > logs/server.pid
	@echo "Backend started, PID: $$(cat backend/logs/server.pid)"
	@echo "View logs: tail -f backend/logs/server.log"

# 停止后端服务
stop-backend:
	@if [ -f backend/logs/server.pid ]; then \
		kill $$(cat backend/logs/server.pid) 2>/dev/null && \
		rm backend/logs/server.pid && \
		echo "Backend stopped"; \
	else \
		pkill -f "go run main.go" && echo "Backend stopped (fallback)"; \
	fi

# 重启后端服务
restart-backend: stop-backend start-backend

# 查看日志
logs:
	@tail -f backend/logs/server.log

# 完整开发环境启动
dev-start: dev-up start-backend
	@echo "Development environment started"
	@echo "Backend: http://localhost:8080"
	@echo "Frontend: cd frontend && npm run dev"
	@echo "Logs: make logs"

# 完整开发环境停止
dev-stop: stop-backend dev-down
	@echo "Development environment stopped"
```

## 🔍 日志管理和调试

### 日志级别配置
```go
// 在main.go中配置日志级别
import (
    "log"
    "os"
)

func setupLogging() {
    // 设置日志输出格式
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    
    // 根据环境变量设置日志级别
    if os.Getenv("LOG_LEVEL") == "debug" {
        gin.SetMode(gin.DebugMode)
    } else {
        gin.SetMode(gin.ReleaseMode)
    }
}
```

### 结构化日志
```go
// 推荐使用logrus或zap进行结构化日志
import "github.com/sirupsen/logrus"

func init() {
    // 设置JSON格式日志
    logrus.SetFormatter(&logrus.JSONFormatter{})
    
    // 设置日志级别
    logrus.SetLevel(logrus.InfoLevel)
    
    // 输出到文件
    file, err := os.OpenFile("logs/app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
    if err == nil {
        logrus.SetOutput(file)
    }
}

// 使用示例
logrus.WithFields(logrus.Fields{
    "user_id": userID,
    "action":  "create_module",
}).Info("Module created successfully")
```

### 常用调试命令
```bash
# 实时监控API请求
tail -f logs/server.log | grep -E "(GET|POST|PUT|DELETE)"

# 监控错误日志
tail -f logs/server.log | grep -i error

# 监控数据库操作
tail -f logs/server.log | grep -i "gorm\|sql"

# 按时间过滤日志
grep "2024-01-01 15:" logs/server.log

# 统计API调用次数
grep -c "GET /api/v1/modules" logs/server.log

# 查找慢请求（假设记录了响应时间）
grep -E "took [0-9]{3,}ms" logs/server.log
```

## 🧪 API测试和验证

### 快速API测试脚本
```bash
#!/bin/bash
# 保存为 scripts/test-api.sh

BASE_URL="http://localhost:8080"
TOKEN=""

# 获取认证token
get_token() {
    response=$(curl -s -X POST $BASE_URL/api/v1/auth/login \
        -H "Content-Type: application/json" \
        -d '{"username":"admin","password":"admin123"}')
    TOKEN=$(echo $response | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    echo "Token: $TOKEN"
}

# 测试模块API
test_modules() {
    echo "Testing modules API..."
    curl -H "Authorization: Bearer $TOKEN" $BASE_URL/api/v1/modules
    echo ""
}

# 测试工作空间API
test_workspaces() {
    echo "Testing workspaces API..."
    curl -H "Authorization: Bearer $TOKEN" $BASE_URL/api/v1/workspaces
    echo ""
}

# 健康检查
health_check() {
    echo "Health check..."
    curl -s $BASE_URL/health
    echo ""
}

# 执行测试
main() {
    health_check
    get_token
    test_modules
    test_workspaces
}

main
```

### 使用脚本
```bash
# 给脚本执行权限
chmod +x scripts/test-api.sh

# 运行测试
./scripts/test-api.sh

# 或者直接测试单个接口
curl -s http://localhost:8080/health | jq .
```

## 🔧 开发工具和配置

### VS Code配置
```json
// .vscode/launch.json - Go调试配置
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch Backend",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/backend/main.go",
            "env": {
                "GIN_MODE": "debug",
                "LOG_LEVEL": "debug"
            },
            "args": [],
            "showLog": true
        }
    ]
}
```

### 环境变量配置
```bash
# .env 文件示例
SERVER_PORT=8080
GIN_MODE=debug
LOG_LEVEL=debug

# 数据库配置
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=iac_platform
DB_SSLMODE=disable

# JWT配置
JWT_SECRET=your-secret-key
JWT_EXPIRES_IN=24h
```

### 热重载开发
```bash
# 安装air进行热重载
go install github.com/cosmtrek/air@latest

# 创建air配置文件 .air.toml
cat > backend/.air.toml << 'EOF'
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "logs"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  kill_delay = "0s"
  log = "build-errors.log"
  send_interrupt = false
  stop_on_root = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  time = false

[misc]
  clean_on_exit = false
EOF

# 使用air启动（自动热重载）
cd backend && air
```

## 📊 性能监控

### 添加性能监控中间件
```go
// middleware/metrics.go
func MetricsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        c.Next()
        
        duration := time.Since(start)
        logrus.WithFields(logrus.Fields{
            "method":     c.Request.Method,
            "path":       c.Request.URL.Path,
            "status":     c.Writer.Status(),
            "duration":   duration.Milliseconds(),
            "ip":         c.ClientIP(),
            "user_agent": c.Request.UserAgent(),
        }).Info("Request completed")
    }
}
```

### 监控脚本
```bash
#!/bin/bash
# scripts/monitor.sh - 服务监控脚本

check_service() {
    if curl -s http://localhost:8080/health > /dev/null; then
        echo " Backend service is running"
    else
        echo "❌ Backend service is down"
        return 1
    fi
}

check_logs() {
    error_count=$(grep -c -i error backend/logs/server.log 2>/dev/null || echo 0)
    echo "📊 Error count in logs: $error_count"
    
    if [ $error_count -gt 10 ]; then
        echo "  High error count detected!"
    fi
}

main() {
    echo "🔍 Service Health Check - $(date)"
    check_service
    check_logs
    echo "---"
}

# 持续监控
while true; do
    main
    sleep 30
done
```

## 🎯 最佳实践

### 1. 日志管理
- 使用结构化日志格式（JSON）
- 按日期轮转日志文件
- 区分不同级别的日志
- 记录关键业务操作

### 2. 错误处理
- 统一的错误响应格式
- 详细的错误日志记录
- 适当的HTTP状态码
- 用户友好的错误信息

### 3. 性能优化
- 监控API响应时间
- 数据库查询优化
- 适当的缓存策略
- 连接池配置

### 4. 安全考虑
- 敏感信息不记录到日志
- JWT token安全管理
- 输入参数验证
- SQL注入防护

记住：**良好的日志管理是快速定位和解决问题的关键！**