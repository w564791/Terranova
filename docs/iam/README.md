# IAM 权限系统文档说明

本目录包含 IaC Platform 权限系统的设计文档。

## 📁 文档列表

### 1. permission-system-design-FirstDraft.md
- **来源**：参考 Terraform Enterprise 的权限设计
- **内容**：基于 Terraform Enterprise 的权限设计参考方案
- **用途**：作为设计 IaC Platform 权限系统的理论基础和参考模型

### 2. iac-platform-permission-system-design-v2.md
- **状态**： **最终设计方案（推荐使用）**
- **内容**：IaC Platform 权限系统的完整优化版设计方案
- **特点**：
  -  **明确权限继承规则**：拒绝优先级 > workspace > project > org
  -  **权限预设功能**：快速授予 READ/WRITE/ADMIN 权限集
  -  **统一数据类型**：PostgreSQL SERIAL
  -  **完善缓存失效策略**：精准失效机制
  -  **临时权限系统**：基于 Webhook 的任务级临时授权
  -  **批量操作优化**：批量权限检查
  -  **完整实施路线图**：7周分阶段计划
  
### 3. iac-platform-permission-system-design.md
- **状态**：📦 **v1版本（历史参考）**
- **内容**：IaC Platform 权限系统的初始完整设计方案
- **说明**：v2版本是在此基础上的优化，建议使用v2版本

### 4. admin-ui-prototype.md
- **状态**： **UI设计**
- **内容**：IaC Platform 权限系统的UI设计方案

## 📊 文档关系

```
permission-system-design-FirstDraft.md (参考文档)
    ↓
    提供理论基础
    ↓
iac-platform-permission-system-design.md (v1版本)
    ↓
    优化改进
    ↓
iac-platform-permission-system-design-v2.md (v2最终版本)  推荐使用
    ↓
    提供UI设计基础
    ↓
admin-ui-prototype.md
```

## 🎯 v2版本的核心优化

| 优化项 | v1版本 | v2版本（优化后） |
|--------|--------|-----------------|
| **权限继承规则** | 未明确说明 |  明确：拒绝优先级 > workspace > project > org |
| **权限预设** | 未实现 |  完整实现 permission_presets 表 |
| **数据类型** | 混合使用 |  统一使用 PostgreSQL SERIAL |
| **缓存失效** | 基础设计 |  完善的精准失效策略 |
| **临时权限整合** | 独立系统 |  明确与常规权限的整合逻辑 |
| **工作空间类型** | 未定义 |  定义7种类型（GENERAL, TASK_POOL等） |
| **批量操作** | 提到但未实现 |  完整的批量检查实现 |

## 🚀 使用建议

### 如果您要实施权限系统：
1. **主要参考**：`iac-platform-permission-system-design-v2.md` ⭐ **推荐**
2. **理论学习**：`permission-system-design-FirstDraft.md`（了解 Terraform Enterprise 模型）
3. **UI设计**：`admin-ui-prototype.md`
4. **历史参考**：`iac-platform-permission-system-design.md`（v1版本）

### 关键设计决策：
-  采用三层模型：Organization → Project → Workspace
-  **权限继承规则**：拒绝优先级 > workspace > project > org
-  Organization 作为租户边界
-  通过关联表扩展，保持向后兼容
-  Agent 作为 Application 独立实体
-  Team 为主要授权方式
-  支持基于 Webhook 的临时权限
-  权限预设功能（READ/WRITE/ADMIN）

## 📝 实施步骤

基于 `iac-platform-permission-system-design-v2.md` 的实施计划：

1. **第一阶段**（2周）：基础架构
   - 创建 Organization、Project、Team 表
   - 创建权限预设表
   - 实现基础权限检查（含继承规则）

2. **第二阶段**（1周）：团队管理
   - 团队 CRUD 和成员管理
   - 权限继承逻辑
   - 缓存失效策略

3. **第三阶段**（1周）：应用授权
   - Application 表和 API Key 认证
   - Agent 迁移

4. **第四阶段**（2周）：临时权限
   - Webhook 集成
   - 任务级临时授权
   - 临时权限与常规权限整合

5. **第五阶段**（1周）：优化完善
   - 批量权限检查
   - 性能优化
   - 完整管理界面

## 🔗 相关资源

- **数据库迁移脚本**：参考 v2 文档中的 SQL
- **API 文档**：参考 v2 文档第7章
- **实施路线图**：参考 v2 文档第8章

## 📞 联系方式

如有疑问，请参考：
- **主要文档**：`iac-platform-permission-system-design-v2.md` ⭐
- **理论基础**：`permission-system-design-FirstDraft.md`
- **UI设计**：`admin-ui-prototype.md`

---

*最后更新：2025-10-21*
