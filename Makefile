# IaC平台开发工具

.PHONY: help dev-up dev-down db-init db-reset logs build-server build-agent docker-server docker-agent

# 默认变量
DB_PORT ?= 5432
DB_USER ?= postgres
DB_PASSWORD ?= postgres123
DB_NAME ?= iac_platform
SERVER_PORT ?= 8080
AGENT_WS_PORT ?= 8091
HOST_IP = 10.101.0.75
IAC_AGENT_TOKEN = apt_pool-abcdefghijklmnop_50a26ac346864671cef8c53add6048fae38e6b0d388e065a093ee1bbb198916e
IAC_AGENT_NAME = test-container
# 获取主机IP并设置为DB_HOST

DB_HOST ?= $(HOST_IP)


help: ## 显示帮助信息
	@echo "IaC平台开发命令:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev-up: ## 启动开发环境
	@echo "启动PostgreSQL数据库..."
	docker-compose up -d postgres
	@echo "等待数据库启动..."
	@sleep 5
	@echo "数据库已启动，连接信息:"
	@echo "  Host: localhost"
	@echo "  Port: 5432"
	@echo "  Database: iac_platform"
	@echo "  Username: postgres"
	@echo "  Password: postgres123"

dev-down: ## 停止开发环境
	@echo "停止开发环境..."
	docker-compose down

db-init: ## 初始化数据库
	@echo "初始化数据库..."
	docker-compose exec postgres psql -U postgres -d iac_platform -c "SELECT 'Database initialized successfully';"

db-reset: ## 重置数据库
	@echo "重置数据库..."
	docker-compose down -v
	docker-compose up -d postgres
	@sleep 5
	@echo "数据库已重置"

logs: ## 查看数据库日志
	docker-compose logs -f postgres

test-db: ## 测试数据库连接
	@echo "测试数据库连接..."
	docker-compose exec postgres psql -U postgres -d iac_platform -c "\dt"

# 构建命令
build-server: ## 构建服务器二进制文件
	@echo "构建服务器..."
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o iac-platform main.go
	@echo "服务器构建完成: backend/iac-platform"

build-agent: ## 构建Agent二进制文件
	@echo "构建Agent..."
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o iac-agent cmd/agent/main.go
	@echo "Agent构建完成: backend/iac-agent"

build-all: build-server build-agent ## 构建所有二进制文件

# Docker运行命令(编译后运行)
run-server: build-server ## 在Docker容器中运行服务器
	@echo "主机IP: $(HOST_IP)"
	@echo "在Docker容器中启动服务器..."
	docker run --rm --platform linux/arm64 -it \
		-p $(SERVER_PORT):$(SERVER_PORT) \
		-p $(AGENT_WS_PORT):$(AGENT_WS_PORT) \
		-e DB_HOST=$(DB_HOST) \
		-e DB_PORT=$(DB_PORT) \
		-e DB_USER=$(DB_USER) \
		-e DB_PASSWORD=$(DB_PASSWORD) \
		-e DB_NAME=$(DB_NAME) \
		-e DB_SSLMODE=disable \
		-e SERVER_PORT=$(SERVER_PORT) \
		-e AGENT_WS_PORT=$(AGENT_WS_PORT) \
		-e SERVER_HOST=0.0.0.0 \
		-v $(PWD)/backend:/app \
		-w /app \
		golang:1.25 \
		./iac-platform

run-agent: build-agent ## 在Docker容器中运行Agent
	@echo "主机IP: $(HOST_IP)"
	@echo "在Docker容器中启动Agent..."
	@echo "API端点: http://$(HOST_IP):$(SERVER_PORT)"
	@echo "WebSocket端点: ws://$(HOST_IP):$(AGENT_WS_PORT)"
	docker run  --rm -it \
		-e IAC_API_ENDPOINT=$(HOST_IP) \
		-e SERVER_PORT=8080 \
		-e CC_SERVER_PORT=8090 \
		-e IAC_AGENT_TOKEN=$(IAC_AGENT_TOKEN) \
		-e IAC_AGENT_NAME=$(IAC_AGENT_NAME) \
		-v $(PWD)/backend:/app \
		-w /app \
		amazonlinux:unzip \
		./iac-agent

# 本地运行命令
local-server: ## 本地运行服务器
	@echo "启动服务器..."
	cd backend && go run main.go

local-agent: ## 本地运行Agent
	@echo "启动Agent..."
	@echo "请确保已设置环境变量: IAC_API_ENDPOINT, IAC_CC_ENDPOINT, IAC_AGENT_TOKEN, IAC_AGENT_NAME"
	cd backend/cmd/agent && go run main.go

# 清理命令
clean: ## 清理构建文件
	@echo "清理构建文件..."
	rm -f backend/iac-platform backend/iac-agent
	@echo "清理完成"
