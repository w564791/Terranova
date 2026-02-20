## v0.2.7

新增 Docker Compose 快速部署方案，修复新建 Workspace 立即触发无意义 Drift 检测的问题，更新项目文档。

### Bug Fixes

- **新建 Workspace 立即触发 Drift 检测** — Drift 调度器对从未成功 Apply 过的 Workspace 跳过检测，避免新建 Workspace 无状态时执行无意义的 drift check (`drift_check_scheduler.go`)

### Features

- **Docker Compose 快速部署** — 新增 `docker-compose.example.yml` 最小化配置（PostgreSQL + Backend + Frontend），无需 K8s 环境，三条命令即可启动平台，适合 POC / 演示 / 评估场景
- **HTTP-only Nginx 配置** — 新增 `docker-compose.nginx.conf`，前端 Nginx 监听 80 端口，反向代理 `/api/` 到后端（支持 WebSocket），无 TLS 依赖
- **Docker Compose 公共变量** — 使用 `x-common-env` / `x-db-env` YAML 锚点统一管理共享环境变量，所有配置均支持 `.env` 文件覆盖

### Docs

- **README 部署章节重构** — 区分 Docker Compose 快速部署（POC）和 Kubernetes 生产部署两种方式，新增启动命令和访问地址说明
- **README 本地开发章节更新** — 技术版本更新为 Go 1.25+ / Node 22+ / PostgreSQL 18+，开发数据库改用 `docker compose up -d postgres`
- **manifests/README 引导提示** — 顶部新增 Docker Compose 快速部署引导，说明本文档面向生产环境 K8s 部署

### Design Docs

- **RunTask 优先级与审批流设计方案** — 新增 `docs/execution-flow/runtask-priority-and-approval-design.md`，覆盖 RunTask 优先级分批执行、IAM Application 集成、审批流 Token 策略等（状态：未实现）

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.2.6...v0.2.7
