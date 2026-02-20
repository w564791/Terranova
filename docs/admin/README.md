# Admin管理模块文档

> **模块版本**: v1.0  
> **最后更新**: 2025-10-09  
> **状态**: 后端开发完成，前端待开发

## 📘 概述

Admin管理模块提供平台级别的配置和管理功能，是IaC平台的基础设施模块。主要负责Terraform版本管理、系统配置等核心功能。

---

## 🎯 核心功能

### 1. Terraform版本管理 

管理平台支持的Terraform版本，包括：
- 版本配置（版本号、下载链接、校验和）
- 启用/禁用版本
- 标记版本为Deprecated
- 版本使用情况检查
- **设置默认版本** ⭐ (全局唯一，新增功能)

### 2. 系统配置（规划中）⏳

- 全局设置
- 通知配置
- 日志配置
- 安全设置

---

## 📊 开发进度

### 后端开发  100%

- [x] 数据库Schema设计
- [x] Model定义
- [x] Service实现
- [x] Controller实现
- [x] Router配置
- [x] API文档

### 前端开发 ⏳ 0%

- [ ] Admin页面框架
- [ ] Terraform版本列表
- [ ] 添加版本对话框
- [ ] 编辑版本对话框
- [ ] 删除确认对话框
- [ ] 状态徽章组件

---

## 📁 文档结构

```
docs/admin/
├── README.md                    # 本文档
├── 01-admin-management.md       # 主要文档（完整设计）
├── 02-api-specification.md      # API规范
├── 03-terraform-version-management.md  # Terraform版本管理
└── 04-development-progress.md   # 开发进度
```

---

## 🔗 文档导航

### 1. [01-admin-management.md](./01-admin-management.md) ⭐ 主要文档
**Admin管理功能完整设计**
- 功能范围和概述
- UI设计（含线框图）
- 数据库设计
- API设计
- 业务逻辑
- 前后端实现指南（含完整代码示例）
- 安全考虑
- 测试计划
- 实现计划
- 未来扩展

### 2. [02-api-specification.md](./02-api-specification.md)
**API接口规范文档**
- 所有API端点详细说明
- 请求/响应格式
- 错误处理
- 认证方式
- 使用示例
- 前端集成示例
- 测试脚本

### 3. [03-terraform-version-management.md](./03-terraform-version-management.md)
**Terraform版本管理**
- 版本管理功能详细说明

### 4. [04-development-progress.md](./04-development-progress.md)
**开发进度跟踪**
- 后端开发进度
- 前端开发进度
- 测试进度
- 问题跟踪

---

## 🚀 快速开始

### 后端开发者

1. **查看需求**: 阅读 [01-admin-management.md](./01-admin-management.md)
2. **了解API**: 阅读 [02-api-specification.md](./02-api-specification.md)
3. **查看代码**:
   - Model: `backend/internal/models/terraform_version.go`
   - Service: `backend/services/terraform_version_service.go`
   - Controller: `backend/controllers/terraform_version_controller.go`
   - Router: `backend/internal/router/router.go`

### 前端开发者

1. **查看需求**: 阅读 [01-admin-management.md](./01-admin-management.md) 的UI设计部分
2. **查看API**: 阅读 [02-api-specification.md](./02-api-specification.md) 的前端集成示例
3. **开始开发**:
   - 创建 `frontend/src/pages/Admin.tsx`
   - 创建 `frontend/src/services/admin.ts`
   - 参考API文档实现功能

---

## 🗄️ 数据库

### terraform_versions表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | SERIAL | 主键 |
| version | VARCHAR(50) | 版本号（唯一） |
| download_url | TEXT | 下载链接 |
| checksum | VARCHAR(64) | SHA256校验和 |
| enabled | BOOLEAN | 是否启用 |
| deprecated | BOOLEAN | 是否弃用 |
| created_at | TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | 更新时间 |

**SQL脚本**: `scripts/create_terraform_versions.sql`

---

## 🔌 API端点

### Terraform版本管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/terraform-versions` | 获取所有版本 |
| GET | `/api/v1/admin/terraform-versions/:id` | 获取单个版本 |
| POST | `/api/v1/admin/terraform-versions` | 创建版本 |
| PUT | `/api/v1/admin/terraform-versions/:id` | 更新版本 |
| DELETE | `/api/v1/admin/terraform-versions/:id` | 删除版本 |

**详细说明**: 查看 [02-api-specification.md](./02-api-specification.md)

---

## 💻 代码示例

### 后端Service示例

```go
// 获取所有启用的版本
versions, err := service.List(&enabled, nil)

// 创建新版本
version, err := service.Create(&models.CreateTerraformVersionRequest{
    Version:     "1.6.0",
    DownloadURL: "https://...",
    Checksum:    "abc123...",
    Enabled:     true,
})

// 更新版本
version, err := service.Update(id, &models.UpdateTerraformVersionRequest{
    Deprecated: &deprecated,
})

// 删除版本
err := service.Delete(id)
```

### 前端Service示例

```typescript
// 获取所有版本
const versions = await adminService.getTerraformVersions();

// 创建版本
const newVersion = await adminService.createTerraformVersion({
  version: '1.6.0',
  download_url: 'https://...',
  checksum: 'abc123...',
  enabled: true,
});

// 更新版本
await adminService.updateTerraformVersion(1, {
  deprecated: true,
});

// 删除版本
await adminService.deleteTerraformVersion(3);
```

---

## 🧪 测试

### 后端测试

```bash
# 运行单元测试
cd backend
go test ./services/terraform_version_service_test.go

# 运行集成测试
go test ./controllers/terraform_version_controller_test.go
```

### API测试

```bash
# 使用测试脚本
./scripts/test_admin_api.sh

# 或手动测试
curl -X GET "http://localhost:8080/api/v1/admin/terraform-versions" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

## 📈 开发计划

### Phase 1: 后端开发  完成

- [x] 数据库Schema
- [x] Model定义
- [x] Service实现
- [x] Controller实现
- [x] Router配置
- [x] API文档

**完成时间**: 2025-10-09

### Phase 2: 前端开发 ⏳ 进行中

- [ ] Admin页面框架
- [ ] 版本列表组件
- [ ] 添加/编辑对话框
- [ ] 删除确认
- [ ] API集成

**预计时间**: 1.5小时

### Phase 3: 测试和优化 ⏳ 待开始

- [ ] 单元测试
- [ ] 集成测试
- [ ] E2E测试
- [ ] 性能优化

**预计时间**: 1小时

---

## 🔐 安全考虑

1. **认证**: 所有API需要JWT认证
2. **权限**: 需要管理员权限（未来RBAC）
3. **验证**: 
   - 版本号格式验证
   - URL格式验证
   - Checksum格式验证
4. **防护**:
   - SQL注入防护（GORM）
   - XSS防护
   - CSRF防护

---

## 📝 注意事项

1. **版本号唯一性**: 不能创建重复的版本号
2. **Checksum格式**: 必须是64位SHA256哈希值
3. **删除限制**: 如果有workspace使用该版本，无法删除
4. **URL验证**: download_url必须是有效的URL
5. **状态管理**: 
   - enabled: 控制是否在创建workspace时显示
   - deprecated: 显示警告但仍可使用

---

## 🤝 贡献指南

### 代码规范

- 遵循项目统一的代码风格
- 添加必要的注释
- 编写单元测试
- 更新相关文档

### 提交规范

```
feat: 添加新功能
fix: 修复bug
docs: 更新文档
test: 添加测试
refactor: 重构代码
```

---

## 📞 联系方式

如有疑问，请通过以下方式联系：

- 项目Issue: [GitHub Issues]
- 技术讨论: [Slack Channel]
- 邮件: dev@iac-platform.com

---

## 🔗 相关文档

- [Workspace模块文档](../workspace/README.md)
- [项目总览](../QUICK_START_FOR_AI.md)
- [开发指南](../development-guide.md)

---

**下一步**: 开始前端开发，实现Admin管理页面
