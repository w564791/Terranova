# IAM 权限系统文档说明

本目录包含 IaC Platform 权限系统的设计文档。

## 当前权威文档（SoT）

### `32-iam-remediation-report.md` ⭐ **现行整改与裁决规范（2026-07-16）**

产品决策与实现整改以本文为准。与历史文档冲突时 **以 32 号文为准**。

锁定决策摘要：

| 决策 | 内容 |
|------|------|
| NONE | 不授权 = 拒绝访问 |
| Workspace 写 | 授到指定 WS 仅改该 WS；授到组织级可全改 |
| 项目负责人 | 可管成员，但必须显式 Role 授权 |
| Team Token | 自动化鉴权（约 24h），主体可为用户或应用（对齐 TFE） |
| 主模型 | **Role 为主**；一用户多 Role；取消日常 Direct Grant 双轨 |

## 📁 文档列表

### 1. permission-system-design-FirstDraft.md
- **来源**：参考 Terraform Enterprise 的权限设计
- **内容**：基于 Terraform Enterprise 的权限设计参考方案
- **用途**：理论基础与参考模型（历史）

### 2. iac-platform-permission-system-design-v2.md
- **状态**：📦 **历史方案（部分废止）**
- **说明**：三层模型与资源切分仍有参考价值；裁决算法、双轨 Grant、显式拒绝等条款以 `32-iam-remediation-report.md` 为准

### 3. iac-platform-permission-system-design.md
- **状态**：📦 **v1版本（历史参考）**
- **内容**：IaC Platform 权限系统的初始完整设计方案

### 4. admin-ui-prototype.md
- **状态**：UI 设计（历史）
- **内容**：IaC Platform 权限系统的 UI 设计方案

### 5. 32-iam-remediation-report.md
- **状态**：✅ **现行 SoT**
- **内容**：整改报告、裁决语义、Role 主路径、身份与分期实施

### 6. 34-iam-fix-progress-report.md
- **状态**：⚠️ **历史进度快照（勿作完成证明）**
- **内容**：修复进度宣称、覆盖率数据；与代码不符处以 `35` 为准
- **SQL 补丁（不完整）**：仅列 `patch_system_admin_iam_roles.sql`（另见 `patch_admin_role_iam_policies.sql`）

### 7. 35-iam-security-review-report.md
- **状态**：✅ **Wave A 前/中期代码核实**（历史基线）
- **内容**：P0/P1 完整清单、已确认有效修复、测试/门禁、双 SQL 上线清单、Wave 修复计划
- **冲突处理**：与 `34` 冲突时不以 `34` 为准；**当前残留与修复计划以 `36` 为准**

### 8. 36-iam-remaining-issues-and-fix-plan.md
- **状态**：✅ **二次审查残留核实（Wave C 代码侧关闭基线）**
- **内容**：相对 35 已关闭项、残留 P0/P1/P2、测试/SQL 说明
- **冲突处理**：实现状态与 `35` 开篇清单冲突时，以本文件「本轮核实」为准；**下一步做什么以 `37` 为准**

### 9. 37-iam-fix-recommendations.md ⭐ **下一波修复建议 / Backlog（2026-07-17）**
- **状态**：✅ **修复排期 Backlog**（§0/R2-1 部分摘要可能滞后）
- **内容**：R0 上线阻断 → R1 安全扫尾 → R2 对齐 32 终态 → R3 测试文档；验收标准与「不要做」清单
- **冲突处理**：与 35/36 的「未修项」表述冲突时，以代码核实为准；**整体进度与覆盖以 `40` 为准**

### 10. 38-application-principal-integration.md ⭐ **Application 选项 A 集成指南**
- **状态**：✅ **集成 / 冒烟 SoT**
- **内容**：App 密钥、Application Role、`workspace_tag_filter`、curl 示例、冒烟表、相关 SQL
- **冲突处理**：与「Application 未启用」旧表述冲突时以本文 + 代码 `/api/v1/app/*` 为准

### 11. 39-direct-grant-retirement.md ⭐ **Direct Grant 下线**
- **状态**：✅ **D5 落地说明**
- **内容**：USER/TEAM/**APPLICATION** HTTP 写 410、应急开关、Role 主路径（含 App Role）、前端入口，以及唯一支持的 `iac-migrate` 发布路径

### 12. 40-iam-remediation-status-report.md ⭐ **进度 + 覆盖报告（2026-07-17）**
- **状态**：✅ **现行进度 / 测试覆盖 SoT**
- **内容**：分波进度与代码证据、cover 实测、风险登记、P0–P3 待办与建议门禁
- **冲突处理**：与 34/36/37 进度宣称冲突时 **以本文 + 代码为准**

## 📊 文档关系

```
permission-system-design-FirstDraft.md (参考)
    ↓
iac-platform-permission-system-design.md (v1)
    ↓
iac-platform-permission-system-design-v2.md (v2，部分废止)
    ↓
32-iam-remediation-report.md  ★ 现行裁决与整改规范（产品 SoT）
    ↓
33 / 34 中间 CR 与进度宣称
    ↓
35-iam-security-review-report.md  （Wave A 核实基线）
    ↓
36-iam-remaining-issues-and-fix-plan.md  （残留核实 / 已关项）
    ↓
37-iam-fix-recommendations.md  ★ 下一波修复建议（执行 Backlog）
    ↓
38-application-principal-integration.md  ★ Application 选项 A 集成
39-direct-grant-retirement.md            ★ D5 Direct Grant 下线
    ↓
40-iam-remediation-status-report.md      ★ 进度 + 测试覆盖 SoT（2026-07-17）
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
