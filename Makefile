# IaC平台开发工具

.PHONY: help dev-up dev-down db-init db-reset logs build-server build-agent build-all \
	docker-build docker-build-frontend docker-build-agent docker-build-db-init docker-build-all \
	docker-push docker-push-frontend docker-push-agent docker-push-db-init docker-push-all \
	run-server run-agent local-server local-agent \
	generate-secret deploy-local export-seed-data clean

# 默认变量（可通过 .env 文件或环境变量覆盖）
DB_PORT ?= 5432
DB_USER ?= postgres
DB_PASSWORD ?= postgres123
DB_NAME ?= iac_platform
SERVER_PORT ?= 8080
CC_SERVER_PORT ?= 8090
DB_HOST ?= localhost

# Docker 镜像配置
DOCKER_REPO ?= w564791
IMAGE_SERVER ?= $(DOCKER_REPO)/iac-platform
IMAGE_FRONTEND ?= $(DOCKER_REPO)/iac-frontend
IMAGE_AGENT ?= $(DOCKER_REPO)/iac-agent
IMAGE_DB_INIT ?= $(DOCKER_REPO)/iac-db-init
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
PLATFORMS ?= linux/arm64,linux/amd64

help: ## 显示帮助信息
	@echo "IaC平台开发命令:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# =============================================================================
# 开发环境
# =============================================================================

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

# =============================================================================
# 本地构建（Go 二进制）
# =============================================================================

build-server: ## 构建服务器二进制文件（当前平台）
	@echo "构建服务器..."
	cd backend && CGO_ENABLED=0 go build -ldflags="-s -w" -o iac-platform main.go
	@echo "服务器构建完成: backend/iac-platform"

build-agent: ## 构建Agent二进制文件（当前平台）
	@echo "构建Agent..."
	cd backend && CGO_ENABLED=0 go build -ldflags="-s -w" -o iac-agent cmd/agent/main.go
	@echo "Agent构建完成: backend/iac-agent"

build-all: build-server build-agent ## 构建所有二进制文件

# =============================================================================
# Docker 镜像构建与推送
# =============================================================================

docker-build: ## 构建后端 Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_SERVER):$(VERSION)"
	docker build \
		-t $(IMAGE_SERVER):$(VERSION) \
		-t $(IMAGE_SERVER):latest \
		backend/
	@echo "镜像构建完成: $(IMAGE_SERVER):$(VERSION)"

docker-build-frontend: ## 构建前端 Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_FRONTEND):$(VERSION)"
	docker build \
		-t $(IMAGE_FRONTEND):$(VERSION) \
		-t $(IMAGE_FRONTEND):latest \
		frontend/
	@echo "镜像构建完成: $(IMAGE_FRONTEND):$(VERSION)"

docker-build-agent: ## 构建 Agent Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_AGENT):$(VERSION)"
	docker build \
		-t $(IMAGE_AGENT):$(VERSION) \
		-t $(IMAGE_AGENT):latest \
		-f backend/cmd/agent/Dockerfile backend/
	@echo "镜像构建完成: $(IMAGE_AGENT):$(VERSION)"

docker-build-db-init: ## 构建 DB 初始化 Docker 镜像（本地，当前架构）
	@echo "构建镜像: $(IMAGE_DB_INIT):$(VERSION)"
	docker build \
		-t $(IMAGE_DB_INIT):$(VERSION) \
		-t $(IMAGE_DB_INIT):latest \
		manifests/db/
	@echo "镜像构建完成: $(IMAGE_DB_INIT):$(VERSION)"

docker-build-all: docker-build docker-build-frontend docker-build-agent docker-build-db-init ## 构建所有 Docker 镜像

docker-push: ## 构建多架构后端镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_SERVER):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_SERVER):$(VERSION) \
		-t $(IMAGE_SERVER):latest \
		--push backend/
	@echo "推送完成"

docker-push-frontend: ## 构建多架构前端镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_FRONTEND):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_FRONTEND):$(VERSION) \
		-t $(IMAGE_FRONTEND):latest \
		--push frontend/
	@echo "推送完成"

docker-push-agent: ## 构建多架构 Agent 镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_AGENT):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_AGENT):$(VERSION) \
		-t $(IMAGE_AGENT):latest \
		-f backend/cmd/agent/Dockerfile --push backend/
	@echo "推送完成"

docker-push-db-init: ## 构建多架构 DB 初始化镜像并推送 (arm64+amd64)
	@echo "构建并推送: $(IMAGE_DB_INIT):$(VERSION) [$(PLATFORMS)]"
	docker buildx build --platform $(PLATFORMS) \
		-t $(IMAGE_DB_INIT):$(VERSION) \
		-t $(IMAGE_DB_INIT):latest \
		--push manifests/db/
	@echo "推送完成"

docker-push-all: docker-push docker-push-frontend docker-push-agent docker-push-db-init ## 构建并推送所有多架构镜像

# =============================================================================
# Docker 容器运行（编译后运行）
# =============================================================================

run-server: build-server ## 在Docker容器中运行服务器
	@echo "在Docker容器中启动服务器..."
	docker run --rm --platform linux/arm64 -it \
		-p $(SERVER_PORT):$(SERVER_PORT) \
		-p $(CC_SERVER_PORT):$(CC_SERVER_PORT) \
		-e DB_HOST=$(DB_HOST) \
		-e DB_PORT=$(DB_PORT) \
		-e DB_USER=$(DB_USER) \
		-e DB_PASSWORD=$(DB_PASSWORD) \
		-e DB_NAME=$(DB_NAME) \
		-e DB_SSLMODE=disable \
		-e SERVER_PORT=$(SERVER_PORT) \
		-e CC_SERVER_PORT=$(CC_SERVER_PORT) \
		-e SERVER_HOST=0.0.0.0 \
		-v $(PWD)/backend:/app \
		-w /app \
		golang:1.25 \
		./iac-platform

run-agent: build-agent ## 在Docker容器中运行Agent（需要设置环境变量 IAC_AGENT_TOKEN 和 IAC_AGENT_NAME）
	@echo "在Docker容器中启动Agent..."
	@echo "API端点: http://$(DB_HOST):$(SERVER_PORT)"
	@echo "CC端点: ws://$(DB_HOST):$(CC_SERVER_PORT)"
	@if [ -z "$(IAC_AGENT_TOKEN)" ]; then echo "[ERROR] 请设置 IAC_AGENT_TOKEN 环境变量（从平台 Agent Pool 页面获取）"; exit 1; fi
	@if [ -z "$(IAC_AGENT_NAME)" ]; then echo "[ERROR] 请设置 IAC_AGENT_NAME 环境变量"; exit 1; fi
	docker run --rm -it \
		-e IAC_API_ENDPOINT=$(DB_HOST) \
		-e SERVER_PORT=$(SERVER_PORT) \
		-e CC_SERVER_PORT=$(CC_SERVER_PORT) \
		-e IAC_AGENT_TOKEN=$(IAC_AGENT_TOKEN) \
		-e IAC_AGENT_NAME=$(IAC_AGENT_NAME) \
		-v $(PWD)/backend:/app \
		-w /app \
		amazonlinux:unzip \
		./iac-agent

# =============================================================================
# 本地运行
# =============================================================================

local-server: ## 本地运行服务器
	@echo "启动服务器..."
	cd backend && go run main.go

local-agent: ## 本地运行Agent
	@echo "启动Agent..."
	@echo "请确保已设置环境变量: IAC_API_ENDPOINT, IAC_CC_ENDPOINT, IAC_AGENT_TOKEN, IAC_AGENT_NAME"
	cd backend/cmd/agent && go run main.go

# =============================================================================
# 密钥和环境配置
# =============================================================================

generate-secret: ## 生成平台私钥和 .env 配置文件
	@if [ -f .env ] && grep -q "^JWT_SECRET=" .env 2>/dev/null; then \
		echo "[OK] .env 文件已存在，跳过生成"; \
	else \
		JWT_KEY=$$(openssl rand -base64 48 | tr -d '\n/+=' | head -c 64); \
		echo "# IaC Platform 环境变量配置" > .env; \
		echo "# 自动生成，请勿提交到版本控制" >> .env; \
		echo "" >> .env; \
		echo "# 平台私钥（用于 JWT 签名和变量加密）" >> .env; \
		echo "JWT_SECRET=$$JWT_KEY" >> .env; \
		echo "" >> .env; \
		echo "# 数据库配置" >> .env; \
		echo "DB_HOST=localhost" >> .env; \
		echo "DB_PORT=15433" >> .env; \
		echo "DB_NAME=iac_platform" >> .env; \
		echo "DB_USER=postgres" >> .env; \
		echo "DB_PASSWORD=postgres123" >> .env; \
		echo "" >> .env; \
		echo "# 服务端口" >> .env; \
		echo "SERVER_PORT=8080" >> .env; \
		echo "CC_SERVER_PORT=8090" >> .env; \
		echo "FRONTEND_PORT=5173" >> .env; \
		echo ""; \
		echo "[OK] .env 配置文件已生成"; \
		echo "  JWT_SECRET: 64 字符随机密钥"; \
		echo "  DB_PORT: 15433"; \
		echo "  SERVER_PORT: 8080"; \
		echo "  [WARN] 请妥善保管 JWT_SECRET，更换将导致："; \
		echo "     - 所有已登录用户的 Token 失效"; \
		echo "     - 所有已加密的变量无法解密"; \
	fi

# =============================================================================
# 部署
# =============================================================================

deploy-local: generate-secret ## 本地部署（初始化数据库 + 启动服务，首次访问引导创建管理员）
	@echo "=========================================="
	@echo "IaC Platform 本地部署"
	@echo "=========================================="
	@echo ""
	@echo "1. 加载环境变量..."
	@if [ -f .env ]; then echo "  从 .env 文件加载配置"; fi
	@echo ""
	@echo "2. 启动数据库（首次启动自动初始化表结构和种子数据）..."
	docker-compose up -d
	@echo "等待数据库启动..."
	@sleep 5
	@echo ""
	@echo "4. 启动后端服务..."
	@set -a && [ -f .env ] && . ./.env; set +a && cd backend && go run main.go &
	@sleep 3
	@echo ""
	@echo "5. 启动前端..."
	cd frontend && npm run dev &
	@echo ""
	@echo "=========================================="
	@echo "部署完成！"
	@echo "前端: http://localhost:5173"
	@echo "后端: http://localhost:8080"
	@echo ""
	@echo "首次使用请访问前端页面完成管理员初始化"
	@echo "=========================================="

export-seed-data: ## 从当前数据库导出种子数据
	@echo "导出种子数据..."
	bash scripts/export_seed_data.sh

# =============================================================================
# 清理
# =============================================================================

clean: ## 清理构建文件
	@echo "清理构建文件..."
	rm -f backend/iac-platform backend/iac-agent
	@echo "清理完成"
