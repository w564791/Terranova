# Run Task 功能开发 - 继续指南

> **AI 助手必读**: 开始任务前，请先阅读 `docs/run-task/README.md` 设计文档的 **第 11 章 "实现进度跟踪"** 了解当前进度和下一步任务。

## 快速开始

1. **阅读设计文档**: `docs/run-task/README.md`
2. **查看进度**: 文档第 11 章 "实现进度跟踪"
3. **找到下一个任务**: 在 "11.2 任务清单" 中找到状态为 ⬜ 的任务
4. **执行任务**
5. **更新进度**: 完成后更新文档中的任务状态

## 当前状态

- **总体进度**: 1/20 子任务完成 (5%)
- **当前阶段**: Phase 1 - 基础设施
- **下一个任务**: 1.2 执行数据库迁移

## 下一步操作

### 任务 1.2: 执行数据库迁移

```bash
# 在数据库中执行迁移脚本
psql -U postgres -d iac_platform -f scripts/create_run_tasks_tables.sql
```

### 任务 1.3: 创建 Go 模型定义

创建文件 `backend/internal/models/run_task.go`，参考设计文档第 6.1 节的代码。

## 文件清单

### 已创建
- `docs/run-task/README.md` - 设计文档
- `scripts/create_run_tasks_tables.sql` - 数据库迁移脚本

### 待创建
- `backend/internal/models/run_task.go` - Go 模型定义
- `backend/internal/handlers/run_task_handler.go` - Run Task API Handler
- `backend/internal/handlers/workspace_run_task_handler.go` - Workspace Run Task API Handler
- `backend/internal/handlers/run_task_callback_handler.go` - 回调 Handler
- `backend/services/run_task_executor.go` - 执行服务
- `backend/services/run_task_token_service.go` - Token 服务
- `backend/services/run_task_timeout_checker.go` - 超时检测
- `frontend/src/pages/admin/RunTaskManagement.tsx` - 管理页面
- `frontend/src/pages/admin/RunTaskManagement.module.css` - 管理页面样式
- `frontend/src/components/WorkspaceRunTaskConfig.tsx` - Workspace 配置组件
- `frontend/src/components/ConfigureRunTaskDialog.tsx` - 配置对话框
- `frontend/src/components/RunTaskResults.tsx` - 结果展示组件

## 重要提示

1. 每完成一个任务，必须更新 `docs/run-task/README.md` 中的进度
2. 如果任务被中断，在文档的 "中断点" 部分记录当前位置
3. 参考设计文档中的代码示例进行实现
