# IAC Platform 文档目录

本目录包含 IAC Platform 项目的完整技术文档。

## 📁 目录结构

```
docs/
├── README.md                 # 本文件 - 文档索引
├── 01-QUICK_START_FOR_AI.md  # AI助手快速上手指南
├── 02-EXECUTION_GUIDE.md     # 项目执行指南
├── 03-development-guide.md   # 通用开发指南
├── 04-testing-guide.md       # 测试规范和指南
├── 05-project-status.md      # 项目状态
├── 06-project-completion-summary.md  # 项目完成总结
├── 07-FINAL_IMPLEMENTATION_STATUS.md # 最终实施状态
├── 11-id-specification.md    # ID规范
├── 12-database-schema.sql    # 数据库Schema
├── 13-database-schema-fixed.sql  # 修复后的数据库Schema
├── 94-s3-complete-schema.json    # S3完整Schema示例
├── iac-platform.svg          # 平台架构图
│
├── admin/        # 管理功能文档
├── agent/        # Agent系统文档
├── ai/           # AI功能文档
├── backend/      # 后端开发文档
├── fixes/        # 问题修复文档
├── frontend/     # 前端开发文档
├── iam/          # IAM权限系统文档
├── module/       # 模块管理文档
├── schema/       # Schema管理文档
├── security/     # 安全相关文档
├── tasks/        # 任务修复文档
├── terraform/    # Terraform执行文档
├── variables/    # 变量管理文档
└── workspace/    # 工作空间文档
```

## 📚 快速开始

- [快速开始指南 (AI)](01-QUICK_START_FOR_AI.md) - AI 助手快速上手指南
- [执行指南](02-EXECUTION_GUIDE.md) - 项目执行指南
- [开发指南](03-development-guide.md) - 通用开发指南
- [测试指南](04-testing-guide.md) - 测试规范和指南

## 🏗️ 核心架构

- [ID 规范](11-id-specification.md) - 系统 ID 设计规范
- [数据库 Schema](12-database-schema.sql) - 数据库结构定义
- [数据库 Schema (修复版)](13-database-schema-fixed.sql) - 修复后的数据库结构

## 📂 模块文档

### [Agent 系统](agent/README.md)
Agent 架构设计、WebSocket通信、K8s部署等相关文档。

### [工作空间 (Workspace)](workspace/README.md)
工作空间生命周期、执行模式、状态管理、Plan/Apply流程等核心功能文档。

### [IAM 权限系统](iam/README.md)
权限系统设计、角色管理、工作空间权限、API授权等文档。

### [后端开发](backend/README.md)
后端开发指南、Golang规范、API文档、Swagger实施等。

### [前端开发](frontend/README.md)
前端调试指南、表单样式、页面模板、移动端适配等。

### [模块管理](module/README.md)
模块导入、Demo功能、版本对比等文档。

### [Schema 管理](schema/README.md)
动态Schema、JSON编辑器、TF文件解析等文档。

### [AI 功能](ai/README.md)
AI错误分析、Provider管理、OpenAI兼容支持等文档。

### [管理功能](admin/README.md)
管理员管理、Terraform版本管理、功能开关等文档。

### [安全相关](security/README.md)
安全漏洞报告、认证审计、JWT安全、密钥管理等文档。

### [Terraform 执行](terraform/README.md)
Terraform执行优化、Phase 2实施、工作目录清理等文档。

### [变量管理](variables/README.md)
变量快照、版本控制、加密分析等文档。

### [问题修复](fixes/README.md)
各种问题的修复方案、实施记录和完成报告。

### [任务修复](tasks/README.md)
按任务编号组织的问题分析和修复文档。

## 📊 项目状态

- [项目状态](05-project-status.md) - 当前项目状态
- [项目完成总结](06-project-completion-summary.md) - 项目完成情况
- [最终实施状态](07-FINAL_IMPLEMENTATION_STATUS.md) - 最终实施状态

## 📝 文档说明

### 文档组织结构

本文档库按照以下方式组织：

1. **根目录** - 包含快速开始指南、核心架构文档和项目状态
2. **admin/** - 管理功能相关文档
3. **agent/** - Agent系统相关文档
4. **ai/** - AI功能相关文档
5. **backend/** - 后端开发相关文档
6. **fixes/** - 问题修复文档
7. **frontend/** - 前端开发相关文档
8. **iam/** - IAM权限系统文档
9. **module/** - 模块管理相关文档
10. **schema/** - Schema管理相关文档
11. **security/** - 安全相关文档
12. **tasks/** - 任务修复文档（按任务编号组织）
13. **terraform/** - Terraform执行相关文档
14. **variables/** - 变量管理相关文档
15. **workspace/** - 工作空间相关文档

### 文档命名规范

- 使用小写字母和连字符（kebab-case）
- 使用描述性的文件名
- 序号文档使用两位数字前缀（如 01-、02-）
- 总结性文档使用 summary 或 complete 后缀
- 指南性文档使用 guide 后缀
- 修复文档使用 fix 后缀
- 分析文档使用 analysis 后缀

### 文档更新

- 创建新功能时，应同步更新相关文档
- 重大变更应创建新的版本文档
- 废弃的文档应标注状态但保留以供参考

---

**最后更新**: 2025-12-04
**维护者**: IAC Platform Team
