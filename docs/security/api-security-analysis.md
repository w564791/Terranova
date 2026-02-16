# API 接口安全性分析报告

> 生成时间: 2026-02-11
> 分析标准: 认证(JWT/PoolToken) + 授权(IAM权限检查)
> true = 需要整改, false = 不需要整改

## 需要整改的问题分类

1. **无认证无授权**: `run-task-callback`(3个) ~~`setup/init`(1个, ✅ 已修复)~~
2. **有JWT但缺IAM权限检查**: `secrets`(5个), `user/tokens`(4个), `notifications`(7个), `manifest`(20个), `sso-auth`(4个), `ai/embedding/config-status`(1个)
3. **仅BypassIAMForAdmin无细粒度IAM**: `admin-skills`(9个), `admin-embedding`(2个), `admin-module-skill`(4个), `admin-module-version-skill`(5个), `admin-embedding-cache`(5个)
4. **敏感端点无访问控制**: `metrics`(1个)
5. **CMDB部分只读接口无IAM**: `cmdb`(7个只读接口仅JWT无IAM)

## 修复原则

> **核心原则**: admin 角色不应添加额外权限，保持 `BypassIAMForAdmin` 现有行为不变。修复目标是为非 admin 用户补全 IAM 权限检查路径。
>
> - **已有 admin 绕过 + IAM 检查的接口** → 不需要优化（已标记为 false）
> - **仅有 admin 绕过但缺少 IAM 检查的接口** → 补全非 admin 的 IAM 权限检查
> - **完全无认证的接口** → 添加认证机制（HMAC/状态检查等）
> - **有 JWT 但缺 IAM 的接口** → 添加 IAM 权限检查，采用与其他接口一致的 `admin 绕过 + IAM fallback` 模式

## 需要整改的接口详细原因

### root — GET /metrics
**原因**: Prometheus 指标端点完全公开，无任何认证。该端点暴露系统内部运行指标（请求数、延迟、goroutine数量、内存使用等），攻击者可利用这些信息进行侦察，了解系统负载模式和内部架构，为后续攻击做准备。

**风险等级**: 🟡 中

**后果**: 攻击者可获取系统内部架构信息（API路由、goroutine数、内存分配、GC频率等），用于精准定位性能瓶颈发起 DoS 攻击；暴露的请求延迟和错误率数据可帮助攻击者判断哪些接口更脆弱；在合规审计中，公开暴露内部指标可能被视为信息泄露违规。

**修复副作用**: 如果添加认证，Prometheus/Grafana 等监控系统的抓取配置需要同步更新（添加 Bearer Token 或 Basic Auth），否则监控数据采集中断，告警系统失效，运维团队无法及时发现系统异常。

**修复建议**: 添加 Basic Auth 或独立的 metrics token 认证，不走 JWT/IAM 体系。监控系统使用专用凭证访问。

### setup — POST /setup/init ✅ 已修复

**原因**: 系统初始化接口无需 JWT 认证（设计如此，因为初始化时尚无用户可登录），但需要幂等性保护防止重复初始化。

**风险等级**: 🟢 已解决

**已实施的保护措施**:
1. **幂等性保护**: handler 内部检查是否已存在活跃的 admin 用户，若已存在则返回 409 Conflict
2. **并发安全**: 使用 PostgreSQL Advisory Lock (`pg_advisory_xact_lock`) 防止竞态条件，确保同一时间只有一个初始化请求能执行
3. **事务原子性**: admin 检查和用户创建在同一事务内完成，检查-创建操作不可分割
4. **参数校验**: 用户名(3-50字符)、邮箱(格式校验)、密码(最少8字符) 均有 binding 校验
5. **唯一性约束**: 事务内检查用户名和邮箱是否已存在

**说明**: 此接口不需要 JWT/IAM 认证保护（系统初始化时尚无用户），安全性通过 handler 内部的业务逻辑保证。如果数据库被清空需要重新初始化，admin 检查会自动允许重新创建。

### run-task-callback — PATCH/POST /run-task-results/:id/callback

**原因**: Run Task 回调接口完全公开，无任何认证机制（无JWT、无HMAC签名验证、无IP白名单）。外部任何人只要知道 result_id 就可以伪造回调结果，篡改 Run Task 的执行状态（如将失败改为成功），从而绕过 pre-plan/post-plan 的安全检查门禁，导致未经审核的变更被部署。

**风险等级**: 🔴 严重

**后果**: 攻击者伪造 Run Task 回调将安全扫描结果从"失败"改为"通过"，绕过 OPA/Sentinel 等策略检查门禁；含有安全漏洞或违规配置的 Terraform Plan 被错误放行并执行 Apply，导致不合规的基础设施变更被部署到生产环境；在有审批流程的场景下，安全审查形同虚设，合规体系被完全架空。

**修复副作用**: 添加 HMAC 签名验证后，所有已配置的外部 Run Task 服务（OPA、Sentinel、自定义扫描器等）需要同步更新回调逻辑以携带签名，否则回调请求将被拒绝，导致 Task 永远停留在"等待 Run Task 结果"状态，阻塞整个部署流水线。需要提供迁移期的兼容模式或逐个 Run Task 灰度切换。

**修复建议**: 添加 HMAC 签名验证中间件，使用 Run Task 创建时生成的 hmac_key 对请求进行签名校验。不走 JWT/IAM 体系，这是服务间认证。

### run-task-callback — GET /run-task-results/:id

**原因**: Run Task 结果查询接口完全公开，无认证。攻击者可以枚举 result_id 获取所有 Run Task 的执行结果数据，可能包含敏感的基础设施变更信息、安全扫描结果等。


**风险等级**: 🟡 中

**后果**: 攻击者通过枚举 result_id 获取所有安全扫描结果，了解哪些 Workspace 存在已知漏洞但尚未修复；泄露的 Plan 变更详情可暴露内部基础设施架构（VPC CIDR、子网规划、安全组规则等）；为后续针对性攻击提供精确的情报支持。

**修复副作用**: 同上，外部 Run Task 服务如果需要查询结果状态（用于重试或确认），添加认证后需同步更新其查询逻辑。

**修复建议**: 同上，使用 HMAC 签名或 Bearer Token 认证。

### sso-auth — GET/POST/DELETE/PUT /auth/sso/identities/*

**原因**: SSO 身份管理接口（查看绑定身份、绑定新身份、解绑身份、设置主要登录方式）仅有 JWT 认证，但缺少 IAM 权限检查。虽然这些操作通常是用户管理自己的身份，但缺少权限校验意味着：(1) 无法通过 IAM 策略限制某些用户的 SSO 绑定行为；(2) 无审计日志中间件（AuditLogger）记录这些敏感操作；(3) 无法在组织层面强制 SSO 绑定策略。

**风险等级**: 🟡 中

**后果**: 用户可自行解绑组织强制的 SSO 身份，绕过组织的统一身份管理策略；攻击者获取用户 JWT 后可绑定自己控制的 SSO 身份作为后门，即使原密码被重置仍可通过 SSO 登录；缺少审计日志导致身份变更操作无法追溯，安全事件调查时缺少关键证据链。

**修复副作用**: 添加 IAM 权限检查后，需要为所有现有用户预先授予 SSO 身份管理的基础权限（如 `SSO_IDENTITY:USER:WRITE`），否则用户将无法在个人设置页面管理自己的 SSO 绑定。前端个人设置页面需要适配权限检查失败的 403 响应，显示友好的"无权限"提示而非报错。

**修复建议**: 添加 AuditLogger 中间件记录操作日志。作为用户自服务接口，保持仅 JWT 认证，但需确保只能操作自己的身份（handler 中校验 user_id 一致性）。admin 无需额外权限。

### secrets — POST/GET/PUT/DELETE /:resourceType/:resourceId/secrets/*

**原因**: 通用密文管理路由（5个接口）虽然在 `protected` 路由组下有 JWT 认证和审计日志，但完全没有 IAM 权限检查。路由使用通配符 `/:resourceType/:resourceId`，意味着任何已认证用户可以对任意资源类型（agent_pool、workspace、module等）的密文进行增删改查操作。密文通常包含云平台凭证、API密钥等高度敏感信息，缺少权限控制是严重的越权风险。

**风险等级**: 🔴 严重

**后果**: 任何已认证的普通用户可读取所有 Agent Pool 的 HCP 凭证、所有 Workspace 的云平台 Access Key/Secret Key；泄露的云凭证可被用于直接操作 AWS/Azure/GCP 等云平台，绕过 IaC 平台的所有安全控制；攻击者可修改或删除密文导致正在运行的 Terraform 任务失败，造成大规模部署中断；这是最高优先级的越权漏洞，影响范围覆盖所有资源类型。

**修复副作用**: 由于路由使用通配符 `/:resourceType/:resourceId`，IAM 权限检查需要根据 resourceType 动态映射到不同的权限资源类型（如 `agent_pool` → `AGENT_POOLS`，`workspace` → `WORKSPACE_MANAGEMENT`）。如果映射关系不完整，某些合法的密文操作将返回 403。此外，Agent 通过 PoolToken 访问密文的场景（`/agents/pool/secrets`）不受影响，因为它走的是独立路由。需要确保 admin 用户和已有 Workspace WRITE 权限的用户在修复后仍能正常管理对应资源的密文。

**修复建议**: 采用 admin 绕过 + IAM fallback 模式。根据 resourceType 动态映射权限：agent_pool -> RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE")，workspace -> RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "WRITE")。admin 通过 role 检查直接放行，非 admin 走 IAM 检查。

### user — POST /user/change-password

**原因**: 用户修改密码接口仅有 JWT 认证，无 IAM 权限检查，也未经过 `BypassIAMForAdmin` 中间件的 admin 角色校验。虽然修改自己的密码是合理的，但缺少权限层意味着无法通过 IAM 策略禁止某些用户自行修改密码（例如 SSO-only 用户不应允许修改本地密码）。

**风险等级**: 🟢 低

**后果**: SSO-only 用户可设置本地密码绕过 SSO 认证流程，破坏组织的统一认证策略；在 SSO Provider 被禁用或删除后，用户仍可通过本地密码登录，违反安全策略意图；影响范围有限，因为用户只能修改自己的密码。

**修复副作用**: 如果添加 IAM 权限检查且默认不授予该权限，所有普通用户将无法修改自己的密码，必须联系管理员重置。需要确保修改自己密码的权限作为默认权限自动授予所有用户，或将此接口视为"用户自服务"类接口，仅需 JWT 认证即可（当前行为可接受）。

**修复建议**: 作为用户自服务接口，保持仅 JWT 认证即可，当前行为可接受。handler 中已校验只能修改自己的密码。无需添加 IAM 权限。

### user — POST/GET/DELETE /user/tokens/*

**原因**: 用户 Token 管理接口（创建、列表、撤销）仅有 JWT 认证，无 IAM 权限检查。User Token 是长期有效的 API 访问凭证，等同于用户的持久化身份。缺少 IAM 控制意味着：(1) 无法通过策略限制某些用户创建 Token；(2) 无法限制 Token 的数量或有效期；(3) 被入侵的低权限账户可以无限制地创建 Token 用于持久化访问。


**风险等级**: 🟡 中

**后果**: 被入侵的账户可创建大量长期 Token 实现持久化访问，即使管理员重置密码或禁用 SSO 身份，攻击者仍可通过已创建的 Token 继续访问；无法通过 IAM 策略在组织层面禁止 Token 创建（例如对外包人员禁用 API Token）；Token 泄露后缺少自动过期机制，风险窗口期无限延长。

**修复副作用**: 添加 IAM 权限检查后，如果默认不授予 Token 管理权限，已有的 CI/CD 流水线和自动化脚本中使用 User Token 的场景将无法创建新 Token 或查看现有 Token。需要确保 Token 管理权限作为默认权限授予，或在前端个人设置页面适配 403 响应。已创建的 Token 本身不受影响（Token 使用时走 JWTAuth 而非 IAM 检查）。

**修复建议**: 作为用户自服务接口，保持仅 JWT 认证即可，handler 中已校验只能管理自己的 Token。如需组织级管控（如禁止某些用户创建 Token），可后续添加可选的 IAM 策略。

### notifications — GET/POST/PUT/DELETE /notifications/*

**原因**: 全局通知配置管理（7个接口）在 `adminProtected` 路由组下，虽然有 JWT 认证和 `BypassIAMForAdmin` 中间件，但没有任何 IAM 权限检查。这意味着非 admin 用户如果绕过了 `BypassIAMForAdmin`（该中间件仅检查 role=="admin" 则放行，非 admin 则继续执行后续中间件），由于后续没有 IAM 检查，非 admin 用户将被拒绝访问——但这依赖于中间件链的隐式行为而非显式权限声明，不符合最小权限原则，且无法实现细粒度的通知管理权限分配。

**风险等级**: 🟡 中

**后果**: 无法将通知管理权限委派给非 admin 的运维人员，所有通知配置变更必须由 admin 操作，增加管理负担；依赖中间件链隐式行为的安全模型脆弱，未来代码重构可能意外打破这一隐式保护，导致非 admin 用户获得通知管理权限；攻击者若获得 admin 权限可篡改通知配置（如修改 Webhook URL 为恶意地址），将部署通知中的敏感信息（变更详情、资源名称）外泄到攻击者控制的服务器。

**修复副作用**: 添加 IAM 权限检查后，当前 admin 用户如果未被显式授予 `NOTIFICATIONS:ORGANIZATION:WRITE` 权限，将无法管理通知配置（因为 IAM 检查优先于 BypassIAMForAdmin）。需要确保 admin 角色的 IAM 绕过逻辑仍然生效，或在迁移时为所有 admin 用户预授权。前端通知管理页面需要适配非 admin 用户的权限检查。

**修复建议**: 采用与其他接口一致的 admin 绕过 + IAM fallback 模式。admin 通过 role=="admin" 直接放行（不需要额外权限），非 admin 用户走 iamMiddleware.RequirePermission("NOTIFICATIONS", "ORGANIZATION", "READ/WRITE/ADMIN")。需在 permission_definitions 表中注册 NOTIFICATIONS 权限定义。

### manifest — GET/POST/PUT/DELETE /organizations/:oid/
manifests/*

**原因**: Manifest 可视化编排器的所有接口（20个，包括 CRUD、版本管理、部署管理、导入导出）虽然在 `adminProtected` 路由组下且使用了 `middleware.JWTAuth()`，但完全没有 IAM 权限检查。Manifest 涉及基础设施编排的核心功能（创建部署、卸载部署、导入导出配置），缺少权限控制意味着任何已认证用户都可以创建、修改、删除 Manifest 及其部署，可能导致未授权的基础设施变更。

**风险等级**: 🔴 严重

**后果**: 任何已认证用户可创建 Manifest 部署，触发多个 Workspace 的联动 Terraform Apply，造成未授权的大规模基础设施变更；攻击者可通过 uninstall 接口批量卸载已部署的 Manifest，导致生产环境基础设施被批量销毁（Terraform Destroy）；导入恶意 Manifest JSON/HCL 可注入恶意 Terraform 配置；导出接口可泄露完整的基础设施编排方案，暴露内部架构设计。

**修复副作用**: 添加 IAM 权限检查后，当前所有使用 Manifest 功能的用户（包括 admin）需要被授予新的 `MANIFESTS:ORGANIZATION:READ/WRITE/ADMIN` 权限。如果权限未预先配置，Manifest 编排页面将对所有用户返回 403，导致正在进行的 Manifest 部署工作流中断。需要在数据库迁移脚本中为现有 admin 用户自动授予 Manifest 相关权限，并在前端 Manifest 页面添加权限检查和友好提示。

**修复建议**: 采用 admin 绕过 + IAM fallback 模式。admin 通过 role=="admin" 直接放行，非 admin 走 iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ/WRITE/ADMIN")。GET 操作需 READ，POST/PUT 需 WRITE，DELETE/uninstall 需 ADMIN。需在 permission_definitions 表中注册 MANIFESTS 权限定义。

### ai — GET /ai/embedding/config-status

**原因**: Embedding 配置状态查询接口在 `ai` 路由组下有 JWT 认证和审计日志，但该接口直接调用 `embeddingController.GetConfigStatus` 而未经过任何 IAM 权限检查（与同组其他接口不同，其他接口都有 admin 角色检查或 IAM 权限检查）。该接口可能暴露 AI 配置的内部状态信息。

**风险等级**: 🟢 低

**后果**: 暴露 AI/Embedding 服务的配置状态（是否启用、模型类型、向量维度等内部配置），为攻击者提供系统架构信息；影响范围有限，仅泄露配置元数据，不涉及实际数据访问。

**修复副作用**: 该接口可能被前端用于判断是否显示 AI/Embedding 相关功能入口。添加 IAM 权限检查后，未被授予 AI_ANALYSIS READ 权限的用户将无法获取配置状态，前端需要处理 403 响应并优雅降级（隐藏 AI 功能入口而非显示错误）。

**修复建议**: 与同组其他接口保持一致，采用 admin 绕过 + IAM fallback 模式：iamMiddleware.RequirePermission("AI_ANALYSIS", "ORGANIZATION", "READ")。admin 直接放行。

### admin-embedding — GET /admin/embedding/status, POST /admin/embedding/sync-all

**原因**: Embedding 管理接口（2个）在 `admin` 路由组下有 JWT 认证和 `BypassIAMForAdmin` 中间件，但没有 IAM 权限检查。`BypassIAMForAdmin` 仅对 admin 角色放行，非 admin 用户会被后续缺失的 IAM 检查阻断——但这是隐式拒绝而非显式权限控制。全量同步所有 Workspace 的 embedding 是高开销操作，应有明确的 IAM 权限定义。

**风险等级**: 🟢 低

**后果**: 无法将 Embedding 管理权限委派给 AI 运维团队（非 admin 角色）；sync-all 是高开销操作，误操作可能导致系统负载骤增，影响正常业务；缺少 IAM 审计记录，无法追溯谁触发了全量同步操作。

**修复副作用**: 添加 IAM 权限检查后，需要新增 `EMBEDDING_MANAGEMENT:ORGANIZATION:READ/WRITE` 权限定义并在 permission_definitions 表中注册。admin 用户通过 BypassIAMForAdmin 仍可访问，但如果未来移除 BypassIAMForAdmin，需确保 admin 角色已被授予相应权限。

**修复建议**: 补全非 admin 的 IAM 权限检查路径。admin 通过现有 BypassIAMForAdmin 直接放行（无需额外权限），非 admin 走 iamMiddleware.RequirePermission("EMBEDDING_MANAGEMENT", "ORGANIZATION", "READ/WRITE")。需在 permission_definitions 表中注册新权限。

### admin-skills — GET/POST/PUT/DELETE /admin/skills/*

**原因**: Skill 管理接口（9个，包括 CRUD、激活/停用、使用统计、预览发现）在 `admin` 路由组下仅依赖 `BypassIAMForAdmin` 中间件，没有 IAM 权限检查。Skill 定义影响 AI 辅助功能的行为，创建/修改/删除 Skill 可能改变平台的 AI 能力范围。缺少 IAM 权限意味着无法将 Skill 管理权限委派给非 admin 用户，也无法在审计中记录具体的权限依据。

**风险等级**: 🟡 中

**后果**: 恶意 admin 可创建包含误导性 Prompt 的 Skill，导致 AI 生成不安全的 Terraform 配置（如开放 0.0.0.0/0 安全组规则）；删除关键 Skill 会导致 AI 辅助功能降级，影响用户体验；无法实现 Skill 管理的职责分离（如 AI 团队管理 Skill 内容，安全团队审核 Skill 激活）。

**修复副作用**: 添加 IAM 权限检查后，需要新增 `SKILLS:ORGANIZATION:READ/WRITE/ADMIN` 权限定义。当前 admin 用户通过 BypassIAMForAdmin 不受影响，但如果希望将 Skill 管理委派给非 admin 用户，需要在 IAM 中为其授权。前端 Skill 管理页面需要根据用户权限动态显示/隐藏操作按钮。

**修复建议**: 补全非 admin 的 IAM 权限检查路径。admin 通过 BypassIAMForAdmin 直接放行，非 admin 走 iamMiddleware.RequirePermission("SKILLS", "ORGANIZATION", "READ/WRITE/ADMIN")。GET 需 READ，POST/PUT 需 WRITE，DELETE 需 ADMIN。

### admin-module-skill — GET/POST/PUT /admin/modules/:mid/skill/*

**原因**: Module Skill 管理接口（4个）在 `admin` 路由组下仅依赖 `BypassIAMForAdmin`，没有 IAM 权限检查。这些接口可以生成和修改模块的 Skill 定义，影响 AI 如何理解和使用模块。缺少细粒度权限控制，无法区分"查看 Skill"和"修改 Skill"的权限级别。

**风险等级**: 🟡 中

**后果**: 篡改 Module Skill 定义可误导 AI 对模块的理解，生成错误的配置参数（如将生产环境的实例类型建议为最小规格）；无法实现查看与修改的权限分离，所有 admin 都有完全的 Skill 修改权限；缺少变更审计，Skill 被恶意修改后难以追溯。

**修复副作用**: 可复用 `SKILLS:ORGANIZATION` 权限或新增 `MODULE_SKILLS:ORGANIZATION` 权限。admin 用户通过 BypassIAMForAdmin 不受影响。如果权限定义与 admin-skills 共用，需注意权限粒度是否满足需求。

**修复建议**: 复用 SKILLS:ORGANIZATION 权限。admin 通过 BypassIAMForAdmin 直接放行，非 admin 走 IAM 检查。GET/preview 需 READ，POST(generate)/PUT 需 WRITE。

### admin-module-version-skill — GET/POST/PUT/DELETE /admin/module-versions/:id/skill/*

**原因**: Module Version Skill 管理接口（5个，包括获取、生成、更新、继承、删除）在 `admin` 路由组下仅依赖 `BypassIAMForAdmin`，没有 IAM 权限检查。这些接口操作特定版本的 Skill 数据，删除操作不可逆，应有 ADMIN 级别的 IAM 权限保护。

**风险等级**: 🟡 中

**后果**: 删除版本 Skill 是不可逆操作，误删后需要重新生成，期间该版本的 AI 辅助功能完全不可用；继承操作可能将错误的 Skill 数据传播到新版本，影响范围扩大；缺少操作级别的权限控制，无法限制"只允许查看不允许删除"。

**修复副作用**: 同 admin-module-skill，可复用相同权限定义。admin 用户通过 BypassIAMForAdmin 不受影响。需注意删除操作应要求 ADMIN 级别权限，而查看操作仅需 READ 级别。

**修复建议**: 复用 SKILLS:ORGANIZATION 权限。admin 通过 BypassIAMForAdmin 直接放行，非 admin 走 IAM 检查。GET 需 READ，POST(generate/inherit)/PUT 需 WRITE，DELETE 需 ADMIN。

### admin-embedding-cache — POST/GET/DELETE /admin/embedding-cache/*

**原因**: Embedding 缓存管理接口（5个，包括预热、进度查询、统计、清除、清理低命中）在 `admin` 路由组下仅依赖 `BypassIAMForAdmin`，没有 IAM 权限检查。清除缓存（DELETE /clear）和清理低命中缓存（POST /cleanup）是破坏性操作，会影响系统性能和 AI 功能的响应速度，应有明确的 IAM 权限保护。

**风险等级**: 🟡 中

**后果**: 清除全部缓存（DELETE /clear）会导致所有 AI 向量搜索请求需要重新计算 Embedding，系统负载骤增，AI 响应延迟从毫秒级飙升到秒级；清理低命中缓存（POST /cleanup）可能误删仍在使用的缓存条目；预热操作（POST /warmup）会触发大量 Embedding API 调用，可能导致 AI 服务限流或产生额外费用。

**修复副作用**: 可复用 `EMBEDDING_MANAGEMENT:ORGANIZATION` 权限。admin 用户通过 BypassIAMForAdmin 不受影响。破坏性操作（clear/cleanup）应要求 ADMIN 级别权限，只读操作（stats/progress）仅需 READ 级别。

**修复建议**: 复用 EMBEDDING_MANAGEMENT:ORGANIZATION 权限。admin 通过 BypassIAMForAdmin 直接放行，非 admin 走 IAM 检查。GET(stats/progress) 需 READ，POST(warmup/cleanup) 需 WRITE，DELETE(clear) 需 ADMIN。

### cmdb — GET /cmdb/search, /suggestions, /stats, /resource-types, /workspace-counts, /workspaces/:wid/tree, /workspaces/:wid/resources

**原因**: CMDB 的 7 个只读接口在 `protected` 路由组下有 JWT 认证，但没有任何 IAM 权限检查（注释中标注"只读，所有认证用户可访问"）。CMDB 包含所有 Workspace 的资源索引数据，包括资源类型、数量、层级结构等信息。虽然是只读操作，但允许任何已认证用户搜索和浏览所有 Workspace 的资源信息，违反了 Workspace 级别的数据隔离原则——低权限用户不应能看到自己无权访问的 Workspace 的资源详情。

**风险等级**: 🟡 中

**后果**: 低权限用户可通过 CMDB 搜索接口发现自己无权访问的 Workspace 中的资源信息（资源类型、名称、数量、依赖关系等），违反最小知情原则；暴露的资源层级结构可帮助攻击者了解内部基础设施架构（如哪些 Workspace 管理数据库、哪些管理网络）；workspace-counts 接口暴露所有 Workspace 的资源规模，可用于评估攻击价值；在多租户场景下，租户间的资源信息隔离被完全打破。

**修复副作用**: CMDB 搜索是前端资源浏览器的核心功能，添加 Workspace 级别的数据过滤后，搜索结果将仅返回用户有权访问的 Workspace 的资源，导致搜索结果不完整。对于需要全局视图的运维人员，需要授予 `CMDB:ORGANIZATION:READ` 权限才能看到所有资源。`/stats` 和 `/workspace-counts` 等聚合接口需要特殊处理——要么只统计用户有权访问的 Workspace，要么要求组织级 READ 权限。前端 CMDB 页面需要适配部分数据不可见的场景，避免显示"0 资源"误导用户。

**修复建议**: 采用 admin 绕过 + IAM fallback 模式。admin 通过 role=="admin" 直接放行看到所有数据，非 admin 走 iamMiddleware.RequirePermission("CMDB", "ORGANIZATION", "READ")。对于 /workspaces/:wid/tree 和 /workspaces/:wid/resources 等指定 Workspace 的接口，可额外检查用户对该 Workspace 的 READ 权限实现数据隔离。

---

## 完整分析表

| Group | API | 需要整改 |
|---|---|---|
| root | GET /health | false |
| root | GET /metrics | true |
| root | GET /static/* | false |
| root | GET /swagger/*any | false |
| setup | GET /setup/status | false |
| setup | POST /setup/init | ✅ false (已修复) |
| auth | POST /auth/login | false |
| auth | POST /auth/mfa/verify | false |
| auth | POST /auth/refresh | false |
| auth | GET /auth/me | false |
| auth | POST /auth/logout | false |
| ws | GET /ws/editing/:session_id | false |
| ws | GET /ws/sessions | false |
| ws | GET /ws/agent-pools/:pool_id/metrics | false |
| sso-public | GET /auth/sso/providers | false |
| sso-public | GET /auth/sso/:provider/login | false |
| sso-public | GET /auth/sso/:provider/callback | false |
| sso-public | POST /auth/sso/:provider/callback | false |
| sso-public | GET /auth/sso/:provider/callback/redirect | false |
| sso-auth | GET /auth/sso/identities | true |
| sso-auth | POST /auth/sso/identities/link | true |
| sso-auth | DELETE /auth/sso/identities/:id | true |
| sso-auth | PUT /auth/sso/identities/:id/primary | true |
| sso-admin | GET /admin/sso/providers | false |
| sso-admin | GET /admin/sso/providers/:id | false |
| sso-admin | POST /admin/sso/providers | false |
| sso-admin | PUT /admin/sso/providers/:id | false |
| sso-admin | DELETE /admin/sso/providers/:id | false |
| sso-admin | GET /admin/sso/config | false |
| sso-admin | PUT /admin/sso/config | false |
| sso-admin | GET /admin/sso/logs | false |
| agents | POST /agents/register | false |
| agents | GET /agents/pool/secrets | false |
| agents | GET /agents/:agent_id | false |
| agents | DELETE /agents/:agent_id | false |
| agents | GET /agents/control | false |
| agents-tasks | GET /agents/tasks/:task_id/data | false |
| agents-tasks | POST /agents/tasks/:task_id/logs/chunk | false |
| agents-tasks | PUT /agents/tasks/:task_id/status | false |
| agents-tasks | POST /agents/tasks/:task_id/state | false |
| agents-tasks | GET /agents/tasks/:task_id/plan-task | false |
| agents-tasks | POST /agents/tasks/:task_id/plan-data | false |
| agents-tasks | POST /agents/tasks/:task_id/plan-json | false |
| agents-tasks | POST /agents/tasks/:task_id/parse-plan-changes | false |
| agents-tasks | GET /agents/tasks/:task_id/logs | false |
| agents-workspaces | POST /agents/workspaces/:wid/lock | false |
| agents-workspaces | POST /agents/workspaces/:wid/unlock | false |
| agents-workspaces | GET /agents/workspaces/:wid/state/max-version | false |
| agents-workspaces | PATCH /agents/workspaces/:wid/fields | false |
| agents-workspaces | GET /agents/workspaces/:wid/terraform-lock-hcl | false |
| agents-workspaces | PUT /agents/workspaces/:wid/terraform-lock-hcl | false |
| agents-terraform | GET /agents/terraform-versions/default | false |
| agents-terraform | GET /agents/terraform-versions/:version | false |
| run-task-callback | PATCH /run-task-results/:id/callback | true |
| run-task-callback | POST /run-task-results/:id/callback | true |
| run-task-callback | GET /run-task-results/:id | true |
| iam | POST /iam/permissions/check | false |
| user-mfa | GET /user/mfa/status | false |
| user-mfa | POST /user/mfa/setup | false |
| user-mfa | POST /user/mfa/verify | false |
| user-mfa | POST /user/mfa/disable | false |
| user-mfa | POST /user/mfa/backup-codes/regenerate | false |
| dashboard | GET /dashboard/overview | false |
| dashboard | GET /dashboard/compliance | false |
| remote-data-public | GET /workspaces/:id/state-outputs/full | false |
| secrets | POST /:resourceType/:resourceId/secrets | true |
| secrets | GET /:resourceType/:resourceId/secrets | true |
| secrets | GET /:resourceType/:resourceId/secrets/:secretId | true |
| secrets | PUT /:resourceType/:resourceId/secrets/:secretId | true |
| secrets | DELETE /:resourceType/:resourceId/secrets/:secretId | true |
| user | POST /user/reset-password | false |
| user | POST /user/change-password | true |
| user | POST /user/tokens | true |
| user | GET /user/tokens | true |
| user | DELETE /user/tokens/:token_name | true |
| demos | GET /demos/:id | false |
| demos | PUT /demos/:id | false |
| demos | DELETE /demos/:id | false |
| demos | GET /demos/:id/versions | false |
| demos | GET /demos/:id/compare | false |
| demos | POST /demos/:id/rollback | false |
| demos | GET /demo-versions/:versionId | false |
| schemas | GET /schemas/:id | false |
| schemas | PUT /schemas/:id | false |
| tasks | GET /tasks/:task_id/output/stream | false |
| tasks | GET /tasks/:task_id/logs | false |
| tasks | GET /tasks/:task_id/logs/download | false |
| tasks | GET /terraform/streams/stats | false |
| agent-pools | POST /agent-pools | false |
| agent-pools | GET /agent-pools | false |
| agent-pools | GET /agent-pools/:pid | false |
| agent-pools | PUT /agent-pools/:pid | false |
| agent-pools | DELETE /agent-pools/:pid | false |
| agent-pools | POST /agent-pools/:pid/allow-workspaces | false |
| agent-pools | GET /agent-pools/:pid/allowed-workspaces | false |
| agent-pools | DELETE /agent-pools/:pid/allowed-workspaces/:wid | false |
| agent-pools | POST /agent-pools/:pid/tokens | false |
| agent-pools | GET /agent-pools/:pid/tokens | false |
| agent-pools | DELETE /agent-pools/:pid/tokens/:name | false |
| agent-pools | POST /agent-pools/:pid/tokens/:name/rotate | false |
| agent-pools | POST /agent-pools/:pid/sync-deployment | false |
| agent-pools | POST /agent-pools/:pid/one-time-unfreeze | false |
| agent-pools | PUT /agent-pools/:pid/k8s-config | false |
| agent-pools | GET /agent-pools/:pid/k8s-config | false |
| run-tasks | POST /run-tasks | false |
| run-tasks | GET /run-tasks | false |
| run-tasks | GET /run-tasks/:id | false |
| run-tasks | PUT /run-tasks/:id | false |
| run-tasks | DELETE /run-tasks/:id | false |
| run-tasks | POST /run-tasks/test | false |
| run-tasks | POST /run-tasks/:id/test | false |
| iam | GET /iam/status | false |
| iam | POST /iam/permissions/grant | false |
| iam | POST /iam/permissions/batch-grant | false |
| iam | POST /iam/permissions/grant-preset | false |
| iam | DELETE /iam/permissions/:scope_type/:id | false |
| iam | GET /iam/permissions/:scope_type/:scope_id | false |
| iam | GET /iam/permissions/definitions | false |
| iam | GET /iam/users/:id/permissions | false |
| iam | GET /iam/teams/:id/permissions | false |
| iam | POST /iam/teams | false |
| iam | GET /iam/teams | false |
| iam | GET /iam/teams/:id | false |
| iam | DELETE /iam/teams/:id | false |
| iam | POST /iam/teams/:id/members | false |
| iam | DELETE /iam/teams/:id/members/:uid | false |
| iam | GET /iam/teams/:id/members | false |
| iam | POST /iam/teams/:id/tokens | false |
| iam | GET /iam/teams/:id/tokens | false |
| iam | DELETE /iam/teams/:id/tokens/:tid | false |
| iam | POST /iam/teams/:id/roles | false |
| iam | GET /iam/teams/:id/roles | false |
| iam | DELETE /iam/teams/:id/roles/:aid | false |
| iam | POST /iam/organizations | false |
| iam | GET /iam/organizations | false |
| iam | GET /iam/organizations/:id | false |
| iam | PUT /iam/organizations/:id | false |
| iam | DELETE /iam/organizations/:id | false |
| iam | POST /iam/projects | false |
| iam | GET /iam/projects | false |
| iam | GET /iam/projects/:id | false |
| iam | PUT /iam/projects/:id | false |
| iam | DELETE /iam/projects/:id | false |
| iam | POST /iam/applications | false |
| iam | GET /iam/applications | false |
| iam | GET /iam/applications/:id | false |
| iam | PUT /iam/applications/:id | false |
| iam | DELETE /iam/applications/:id | false |
| iam | POST /iam/applications/:id/regenerate-secret | false |
| iam | GET /iam/audit/config | false |
| iam | PUT /iam/audit/config | false |
| iam | GET /iam/audit/permission-history | false |
| iam | GET /iam/audit/access-history | false |
| iam | GET /iam/audit/denied-access | false |
| iam | GET /iam/audit/permission-changes-by-principal | false |
| iam | GET /iam/audit/permission-changes-by-performer | false |
| iam | GET /iam/users/stats | false |
| iam | GET /iam/users | false |
| iam | POST /iam/users | false |
| iam | POST /iam/users/:id/roles | false |
| iam | DELETE /iam/users/:id/roles/:aid | false |
| iam | GET /iam/users/:id/roles | false |
| iam | GET /iam/users/:id | false |
| iam | PUT /iam/users/:id | false |
| iam | POST /iam/users/:id/activate | false |
| iam | POST /iam/users/:id/deactivate | false |
| iam | DELETE /iam/users/:id | false |
| iam | GET /iam/roles | false |
| iam | GET /iam/roles/:id | false |
| iam | POST /iam/roles | false |
| iam | PUT /iam/roles/:id | false |
| iam | DELETE /iam/roles/:id | false |
| iam | POST /iam/roles/:id/policies | false |
| iam | DELETE /iam/roles/:id/policies/:pid | false |
| iam | POST /iam/roles/:id/clone | false |
| global-settings | GET /global/settings/terraform-versions | false |
| global-settings | GET /global/settings/terraform-versions/default | false |
| global-settings | GET /global/settings/terraform-versions/:id | false |
| global-settings | POST /global/settings/terraform-versions | false |
| global-settings | PUT /global/settings/terraform-versions/:id | false |
| global-settings | POST /global/settings/terraform-versions/:id/set-default | false |
| global-settings | DELETE /global/settings/terraform-versions/:id | false |
| global-settings | GET /global/settings/ai-configs | false |
| global-settings | POST /global/settings/ai-configs | false |
| global-settings | GET /global/settings/ai-configs/:id | false |
| global-settings | PUT /global/settings/ai-configs/:id | false |
| global-settings | DELETE /global/settings/ai-configs/:id | false |
| global-settings | PUT /global/settings/ai-configs/priorities | false |
| global-settings | PUT /global/settings/ai-configs/:id/set-default | false |
| global-settings | GET /global/settings/ai-config/regions | false |
| global-settings | GET /global/settings/ai-config/models | false |
| global-settings | GET /global/settings/platform-config | false |
| global-settings | PUT /global/settings/platform-config | false |
| global-settings | GET /global/settings/mfa | false |
| global-settings | PUT /global/settings/mfa | false |
| admin-users | GET /admin/users/:uid/mfa/status | false |
| admin-users | POST /admin/users/:uid/mfa/reset | false |
| notifications | GET /notifications | true |
| notifications | GET /notifications/available | true |
| notifications | GET /notifications/:nid | true |
| notifications | POST /notifications | true |
| notifications | PUT /notifications/:nid | true |
| notifications | DELETE /notifications/:nid | true |
| notifications | POST /notifications/:nid/test | true |
| manifest | GET /organizations/:oid/manifests | true |
| manifest | POST /organizations/:oid/manifests | true |
| manifest | GET /organizations/:oid/manifests/:id | true |
| manifest | PUT /organizations/:oid/manifests/:id | true |
| manifest | DELETE /organizations/:oid/manifests/:id | true |
| manifest | PUT /organizations/:oid/manifests/:id/draft | true |
| manifest | GET /organizations/:oid/manifests/:id/versions | true |
| manifest | POST /organizations/:oid/manifests/:id/versions | true |
| manifest | GET /organizations/:oid/manifests/:id/versions/:vid | true |
| manifest | GET /organizations/:oid/manifests/:id/deployments | true |
| manifest | POST /organizations/:oid/manifests/:id/deployments | true |
| manifest | GET /organizations/:oid/manifests/:id/deployments/:did | true |
| manifest | PUT /organizations/:oid/manifests/:id/deployments/:did | true |
| manifest | DELETE /organizations/:oid/manifests/:id/deployments/:did | true |
| manifest | GET /organizations/:oid/manifests/:id/deployments/:did/resources | true |
| manifest | POST /organizations/:oid/manifests/:id/deployments/:did/uninstall | true |
| manifest | GET /organizations/:oid/manifests/:id/export | true |
| manifest | GET /organizations/:oid/manifests/:id/export-zip | true |
| manifest | POST /organizations/:oid/manifests/import | true |
| manifest | POST /organizations/:oid/manifests/import-json | true |
| modules | GET /modules | false |
| modules | GET /modules/:id | false |
| modules | POST /modules | false |
| modules | PUT /modules/:id | false |
| modules | DELETE /modules/:id | false |
| modules | (其余30+个模块路由) | false |
| projects | GET /projects | false |
| projects | GET /projects/:id/workspaces | false |
| ai | POST /ai/analyze-error | false |
| ai | POST /ai/form/generate | false |
| ai | POST /ai/form/generate-with-cmdb | false |
| ai | POST /ai/form/generate-with-cmdb-skill | false |
| ai | POST /ai/form/generate-with-cmdb-skill-sse | false |
| ai | POST /ai/skill/preview-prompt | false |
| ai | GET /ai/embedding/config-status | true |
| ai | POST /ai/cmdb/vector-search | false |
| admin-embedding | GET /admin/embedding/status | true |
| admin-embedding | POST /admin/embedding/sync-all | true |
| admin-skills | GET /admin/skills | true |
| admin-skills | GET /admin/skills/preview-discovery | true |
| admin-skills | GET /admin/skills/:id | true |
| admin-skills | POST /admin/skills | true |
| admin-skills | PUT /admin/skills/:id | true |
| admin-skills | DELETE /admin/skills/:id | true |
| admin-skills | POST /admin/skills/:id/activate | true |
| admin-skills | POST /admin/skills/:id/deactivate | true |
| admin-skills | GET /admin/skills/:id/usage-stats | true |
| admin-module-skill | GET /admin/modules/:mid/skill | true |
| admin-module-skill | POST /admin/modules/:mid/skill/generate | true |
| admin-module-skill | PUT /admin/modules/:mid/skill | true |
| admin-module-skill | GET /admin/modules/:mid/skill/preview | true |
| admin-module-version-skill | GET /admin/module-versions/:id/skill | true |
| admin-module-version-skill | POST /admin/module-versions/:id/skill/generate | true |
| admin-module-version-skill | PUT /admin/module-versions/:id/skill | true |
| admin-module-version-skill | POST /admin/module-versions/:id/skill/inherit | true |
| admin-module-version-skill | DELETE /admin/module-versions/:id/skill | true |
| admin-embedding-cache | POST /admin/embedding-cache/warmup | true |
| admin-embedding-cache | GET /admin/embedding-cache/warmup/progress | true |
| admin-embedding-cache | GET /admin/embedding-cache/stats | true |
| admin-embedding-cache | DELETE /admin/embedding-cache/clear | true |
| admin-embedding-cache | POST /admin/embedding-cache/cleanup | true |
| workspaces | GET /workspaces | false |
| workspaces | GET /workspaces/:id | false |
| workspaces | GET /workspaces/:id/overview | false |
| workspaces | PUT /workspaces/:id | false |
| workspaces | PATCH /workspaces/:id | false |
| workspaces | POST /workspaces/:id/lock | false |
| workspaces | POST /workspaces/:id/unlock | false |
| workspaces | DELETE /workspaces/:id | false |
| workspaces | POST /workspaces | false |
| workspaces-tasks | GET /workspaces/:id/tasks | false |
| workspaces-tasks | GET /workspaces/:id/tasks/:tid | false |
| workspaces-tasks | GET /workspaces/:id/tasks/:tid/logs | false |
| workspaces-tasks | GET /workspaces/:id/tasks/:tid/comments | false |
| workspaces-tasks | GET /workspaces/:id/tasks/:tid/resource-changes | false |
| workspaces-tasks | GET /workspaces/:id/tasks/:tid/error-analysis | false |
| workspaces-tasks | GET /workspaces/:id/tasks/:tid/state-backup | false |
| workspaces-tasks | POST /workspaces/:id/tasks/plan | false |
| workspaces-tasks | POST /workspaces/:id/tasks/:tid/comments | false |
| workspaces-tasks | POST /workspaces/:id/tasks/:tid/cancel | false |
| workspaces-tasks | POST /workspaces/:id/tasks/:tid/cancel-previous | false |
| workspaces-tasks | POST /workspaces/:id/tasks/:tid/confirm-apply | false |
| workspaces-tasks | PATCH /workspaces/:id/tasks/:tid/resource-changes/:rid | false |
| workspaces-tasks | POST /workspaces/:id/tasks/:tid/retry-state-save | false |
| workspaces-tasks | POST /workspaces/:id/tasks/:tid/parse-plan | false |
| workspaces-state | GET /workspaces/:id/current-state | false |
| workspaces-state | GET /workspaces/:id/state-versions | false |
| workspaces-state | GET /workspaces/:id/state/versions | false |
| workspaces-state | GET /workspaces/:id/state/versions/:v | false |
| workspaces-state | GET /workspaces/:id/state/versions/:v/retrieve | false |
| workspaces-state | GET /workspaces/:id/state/versions/:v/download | false |
| workspaces-state | GET /workspaces/:id/state-versions/compare | false |
| workspaces-state | GET /workspaces/:id/state-versions/:v/metadata | false |
| workspaces-state | GET /workspaces/:id/state-versions/:v | false |
| workspaces-state | POST /workspaces/:id/state/upload | false |
| workspaces-state | POST /workspaces/:id/state/upload-file | false |
| workspaces-state | POST /workspaces/:id/state/rollback | false |
| workspaces-state | POST /workspaces/:id/state-versions/:v/rollback | false |
| workspaces-state | DELETE /workspaces/:id/state-versions/:v | false |
| workspaces-variables | GET /workspaces/:id/variables | false |
| workspaces-variables | GET /workspaces/:id/variables/:vid | false |
| workspaces-variables | POST /workspaces/:id/variables | false |
| workspaces-variables | PUT /workspaces/:id/variables/:vid | false |
| workspaces-variables | DELETE /workspaces/:id/variables/:vid | false |
| workspaces-variables | GET /workspaces/:id/variables/:vid/versions | false |
| workspaces-variables | GET /workspaces/:id/variables/:vid/versions/:v | false |
| workspaces-resources | GET /workspaces/:id/resources | false |
| workspaces-resources | GET /workspaces/:id/resources/:rid | false |
| workspaces-resources | GET /workspaces/:id/resources/:rid/versions | false |
| workspaces-resources | GET /workspaces/:id/resources/:rid/versions/compare | false |
| workspaces-resources | GET /workspaces/:id/resources/:rid/versions/:v | false |
| workspaces-resources | GET /workspaces/:id/resources/:rid/dependencies | false |
| workspaces-resources | GET /workspaces/:id/resources/:rid/editing/status | false |
| workspaces-resources | GET /workspaces/:id/resources/:rid/drift | false |
| workspaces-resources | GET /workspaces/:id/resources/export/hcl | false |
| workspaces-resources | POST /workspaces/:id/resources | false |
| workspaces-resources | POST /workspaces/:id/resources/import | false |
| workspaces-resources | POST /workspaces/:id/resources/deploy | false |
| workspaces-resources | PUT /workspaces/:id/resources/:rid | false |
| workspaces-resources | DELETE /workspaces/:id/resources/:rid | false |
| workspaces-resources | PUT /workspaces/:id/resources/:rid/dependencies | false |
| workspaces-resources | POST /workspaces/:id/resources/:rid/restore | false |
| workspaces-resources | POST /workspaces/:id/resources/:rid/versions/:v/rollback | false |
| workspaces-resources | POST /workspaces/:id/resources/:rid/editing/start | false |
| workspaces-resources | POST /workspaces/:id/resources/:rid/editing/heartbeat | false |
| workspaces-resources | POST /workspaces/:id/resources/:rid/editing/end | false |
| workspaces-resources | POST /workspaces/:id/resources/:rid/drift/save | false |
| workspaces-resources | POST /workspaces/:id/resources/:rid/drift/takeover | false |
| workspaces-resources | DELETE /workspaces/:id/resources/:rid/drift | false |
| workspaces-snapshots | GET /workspaces/:id/snapshots | false |
| workspaces-snapshots | GET /workspaces/:id/snapshots/:sid | false |
| workspaces-snapshots | POST /workspaces/:id/snapshots | false |
| workspaces-snapshots | POST /workspaces/:id/snapshots/:sid/restore | false |
| workspaces-snapshots | DELETE /workspaces/:id/snapshots/:sid | false |
| workspaces-takeover | POST /workspaces/:id/resources/:rid/editing/takeover-request | false |
| workspaces-takeover | POST /workspaces/:id/resources/:rid/editing/takeover-response | false |
| workspaces-takeover | GET /workspaces/:id/resources/:rid/editing/pending-requests | false |
| workspaces-takeover | GET /workspaces/:id/resources/:rid/editing/request-status/:reqid | false |
| workspaces-takeover | POST /workspaces/:id/resources/:rid/editing/force-takeover | false |
| workspaces-agent | GET /workspaces/:id/available-pools | false |
| workspaces-agent | POST /workspaces/:id/set-current-pool | false |
| workspaces-agent | GET /workspaces/:id/current-pool | false |
| workspaces-run-tasks | POST /workspaces/:id/tasks/:tid/override-run-tasks | false |
| workspaces-run-tasks | GET /workspaces/:id/tasks/:tid/run-task-results | false |
| workspaces-run-tasks | GET /workspaces/:id/run-tasks | false |
| workspaces-run-tasks | POST /workspaces/:id/run-tasks | false |
| workspaces-run-tasks | PUT /workspaces/:id/run-tasks/:wrtid | false |
| workspaces-run-tasks | DELETE /workspaces/:id/run-tasks/:wrtid | false |
| workspaces-notifications | GET /workspaces/:id/notifications | false |
| workspaces-notifications | POST /workspaces/:id/notifications | false |
| workspaces-notifications | PUT /workspaces/:id/notifications/:wnid | false |
| workspaces-notifications | DELETE /workspaces/:id/notifications/:wnid | false |
| workspaces-notifications | GET /workspaces/:id/notification-logs | false |
| workspaces-notifications | GET /workspaces/:id/notification-logs/:lid | false |
| workspaces-notifications | GET /workspaces/:id/tasks/:tid/notification-logs | false |
| workspaces-outputs | GET /workspaces/:id/outputs | false |
| workspaces-outputs | GET /workspaces/:id/state-outputs | false |
| workspaces-outputs | GET /workspaces/:id/outputs/resources | false |
| workspaces-outputs | GET /workspaces/:id/available-outputs | false |
| workspaces-outputs | POST /workspaces/:id/outputs | false |
| workspaces-outputs | PUT /workspaces/:id/outputs/:oid | false |
| workspaces-outputs | DELETE /workspaces/:id/outputs/:oid | false |
| workspaces-outputs | POST /workspaces/:id/outputs/batch | false |
| workspaces-remote-data | GET /workspaces/:id/remote-data | false |
| workspaces-remote-data | GET /workspaces/:id/remote-data/accessible-workspaces | false |
| workspaces-remote-data | GET /workspaces/:id/remote-data/source-outputs | false |
| workspaces-remote-data | POST /workspaces/:id/remote-data | false |
| workspaces-remote-data | PUT /workspaces/:id/remote-data/:rdid | false |
| workspaces-remote-data | DELETE /workspaces/:id/remote-data/:rdid | false |
| workspaces-remote-data | GET /workspaces/:id/outputs-sharing | false |
| workspaces-remote-data | PUT /workspaces/:id/outputs-sharing | false |
| workspaces-run-triggers | GET /workspaces/:id/run-triggers | false |
| workspaces-run-triggers | GET /workspaces/:id/run-triggers/inbound | false |
| workspaces-run-triggers | GET /workspaces/:id/run-triggers/available-targets | false |
| workspaces-run-triggers | GET /workspaces/:id/run-triggers/available-sources | false |
| workspaces-run-triggers | POST /workspaces/:id/run-triggers/inbound | false |
| workspaces-run-triggers | POST /workspaces/:id/run-triggers | false |
| workspaces-run-triggers | PUT /workspaces/:id/run-triggers/:trid | false |
| workspaces-run-triggers | DELETE /workspaces/:id/run-triggers/:trid | false |
| workspaces-run-triggers | GET /workspaces/:id/tasks/:tid/trigger-executions | false |
| workspaces-run-triggers | POST /workspaces/:id/tasks/:tid/trigger-executions/:eid/toggle | false |
| workspaces-drift | GET /workspaces/:id/drift-config | false |
| workspaces-drift | PUT /workspaces/:id/drift-config | false |
| workspaces-drift | GET /workspaces/:id/drift-status | false |
| workspaces-drift | POST /workspaces/:id/drift-check | false |
| workspaces-drift | DELETE /workspaces/:id/drift-check | false |
| workspaces-drift | GET /workspaces/:id/resources-drift | false |
| workspaces-embedding | GET /workspaces/:id/embedding-status | false |
| workspaces-embedding | POST /workspaces/:id/embedding/sync | false |
| workspaces-embedding | POST /workspaces/:id/embedding/rebuild | false |
| cmdb | GET /cmdb/search | true |
| cmdb | GET /cmdb/suggestions | true |
| cmdb | GET /cmdb/stats | true |
| cmdb | GET /cmdb/resource-types | true |
| cmdb | GET /cmdb/workspace-counts | true |
| cmdb | GET /cmdb/workspaces/:wid/tree | true |
| cmdb | GET /cmdb/workspaces/:wid/resources | true |
| cmdb | POST /cmdb/workspaces/:wid/sync | false |
| cmdb | POST /cmdb/sync-all | false |
| cmdb | GET /cmdb/external-sources | false |
| cmdb | POST /cmdb/external-sources | false |
| cmdb | GET /cmdb/external-sources/:sid | false |
| cmdb | PUT /cmdb/external-sources/:sid | false |
| cmdb | DELETE /cmdb/external-sources/:sid | false |
| cmdb | POST /cmdb/external-sources/:sid/sync | false |
| cmdb | POST /cmdb/external-sources/:sid/test | false |
| cmdb | GET /cmdb/external-sources/:sid/sync-logs | false |

---

## 每个 API 接口的权限详细分析

> 更新时间: 2026-02-15
> 分析内容: 每个 API 的认证方式、授权资源、权限级别、以及合理性评估
> 图例: ✅ 合理 | ⚠️ 可优化 | ❌ 需整改

### 1. 公开端点（无认证）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 1 | GET | /health | 无 | 无 | ✅ 健康检查端点，负载均衡/容器编排必需公开 |
| 2 | GET | /metrics | 无 | 无 | ⚠️ Prometheus指标暴露系统内部信息（goroutine数、请求延迟、内存使用），建议加Basic Auth或独立metrics token |
| 3 | GET | /static/* | 无 | 无 | ✅ 静态资源，无敏感数据 |
| 4 | GET | /swagger/*any | 无 | 无 | ⚠️ 生产环境建议禁用或加认证，API文档暴露全部接口定义 |

### 2. 系统初始化（无认证）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 5 | GET | /setup/status | 无 | 无 | ✅ 前端判断是否需要初始化，不含敏感信息 |
| 6 | POST | /setup/init | 无 | Handler内部幂等检查 + Advisory Lock | ✅ 初始化时无用户可认证，handler已做幂等保护和并发安全控制 |

### 3. 认证端点（无认证/MFA token）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 7 | POST | /auth/login | 无 | 无 | ✅ 登录入口必须公开 |
| 8 | POST | /auth/mfa/verify | MFA Token | 无 | ✅ 使用登录流程中颁发的mfa_token验证，不需要JWT |
| 9 | POST | /auth/mfa/setup | MFA Token | 无 | ✅ 首次登录强制MFA设置场景，使用mfa_token |
| 10 | POST | /auth/mfa/enable | MFA Token | 无 | ✅ 配合mfa/setup使用，验证TOTP码后启用 |
| 11 | POST | /auth/refresh | JWT | 无 | ✅ Token刷新仅需有效JWT，用户自服务操作 |
| 12 | GET | /auth/me | JWT | 无 | ✅ 获取当前用户信息，仅返回自身数据 |
| 13 | POST | /auth/logout | JWT | 无 | ✅ 登出仅操作自身会话 |

### 4. SSO 公开端点（无认证）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 14 | GET | /auth/sso/providers | 无 | 无 | ✅ 登录页展示可用SSO提供商列表，仅返回名称和图标 |
| 15 | GET | /auth/sso/:provider/login | 无 | 无 | ✅ 发起OAuth重定向，必须公开 |
| 16 | GET | /auth/sso/:provider/callback | 无 | 无 | ✅ OAuth回调端点，IdP回调必须公开 |
| 17 | POST | /auth/sso/:provider/callback | 无 | 无 | ✅ 同上，兼容POST方式的回调 |
| 18 | GET | /auth/sso/:provider/callback/redirect | 无 | 无 | ✅ 前端重定向模式的回调处理 |

### 5. SSO 身份管理（仅JWT，无IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 19 | GET | /auth/sso/identities | JWT | 无 | ⚠️ 用户自服务操作，JWT认证可接受，但缺少AuditLogger中间件记录操作。Handler应确保仅返回当前用户自身的身份 |
| 20 | POST | /auth/sso/identities/link | JWT | 无 | ⚠️ 绑定SSO身份。风险：攻击者获取JWT后可绑定自控SSO身份作为后门。建议加AuditLogger并确保handler校验user_id一致性 |
| 21 | DELETE | /auth/sso/identities/:id | JWT | 无 | ⚠️ 解绑SSO身份。风险：可绕过组织强制SSO绑定策略。建议加AuditLogger |
| 22 | PUT | /auth/sso/identities/:id/primary | JWT | 无 | ⚠️ 设置主要登录方式。同上，缺少审计日志 |

### 6. SSO 管理端点（JWT + RequireRole("admin")）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 23 | GET | /admin/sso/providers | JWT | RequireRole("admin") | ⚠️ 使用旧版role检查，无IAM权限。功能正确但无法委派给非admin的安全运维人员 |
| 24 | GET | /admin/sso/providers/:id | JWT | RequireRole("admin") | ⚠️ 同上 |
| 25 | POST | /admin/sso/providers | JWT | RequireRole("admin") | ⚠️ 同上。创建SSO Provider是高权限操作，admin-only合理，但应迁移到IAM体系 |
| 26 | PUT | /admin/sso/providers/:id | JWT | RequireRole("admin") | ⚠️ 同上 |
| 27 | DELETE | /admin/sso/providers/:id | JWT | RequireRole("admin") | ⚠️ 同上 |
| 28 | GET | /admin/sso/config | JWT | RequireRole("admin") | ⚠️ 同上 |
| 29 | PUT | /admin/sso/config | JWT | RequireRole("admin") | ⚠️ 同上。全局SSO配置修改应为ADMIN级别 |
| 30 | GET | /admin/sso/logs | JWT | RequireRole("admin") | ⚠️ 同上 |

### 7. WebSocket 端点（JWT）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 31 | GET | /ws/editing/:session_id | JWT | 无 | ✅ 协同编辑WebSocket，JWT认证足够，session_id本身提供访问隔离 |
| 32 | GET | /ws/sessions | JWT | 无 | ✅ 查看活跃会话列表，仅需认证 |
| 33 | GET | /ws/agent-pools/:pool_id/metrics | JWT | 无 | ⚠️ 可查看任意pool的实时指标。建议增加AGENT_POOLS READ权限校验 |

### 8. Agent API（Pool Token认证）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 34 | POST | /agents/register | PoolToken | Token所属Pool | ✅ Agent注册需Pool Token，token绑定到特定Pool |
| 35 | GET | /agents/pool/secrets | PoolToken | Token所属Pool | ✅ 获取Pool级密文用于生成credentials.tfrc.json，权限通过Pool Token隔离 |
| 36 | GET | /agents/:agent_id | PoolToken | Token所属Pool | ✅ 查看Agent详情，Pool Token保证只能查看自己Pool下的Agent |
| 37 | DELETE | /agents/:agent_id | PoolToken | Token所属Pool | ✅ 注销Agent。建议确认handler中校验agent_id属于当前Pool |
| 38 | GET | /agents/control | 无（已废弃） | 返回410 Gone | ✅ 已迁移到独立端口8091，保留向后兼容提示 |

### 9. Agent Task API（Pool Token + Workspace Check）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 39 | GET | /agents/tasks/:task_id/data | PoolToken + TaskCheck | Pool必须授权访问task所属workspace | ✅ 双层校验：Token有效性 + Workspace授权关系 |
| 40 | POST | /agents/tasks/:task_id/logs/chunk | PoolToken + TaskCheck | 同上 | ✅ |
| 41 | PUT | /agents/tasks/:task_id/status | PoolToken + TaskCheck | 同上 | ✅ |
| 42 | POST | /agents/tasks/:task_id/state | PoolToken + TaskCheck | 同上 | ✅ |
| 43 | GET | /agents/tasks/:task_id/plan-task | PoolToken + TaskCheck | 同上 | ✅ |
| 44 | POST | /agents/tasks/:task_id/plan-data | PoolToken + TaskCheck | 同上 | ✅ |
| 45 | POST | /agents/tasks/:task_id/plan-json | PoolToken + TaskCheck | 同上 | ✅ |
| 46 | POST | /agents/tasks/:task_id/parse-plan-changes | PoolToken + TaskCheck | 同上 | ✅ |
| 47 | GET | /agents/tasks/:task_id/logs | PoolToken + TaskCheck | 同上 | ✅ |

### 10. Agent Workspace API（Pool Token + Workspace Check）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 48 | POST | /agents/workspaces/:wid/lock | PoolToken + WsCheck | Pool必须授权访问该workspace | ✅ |
| 49 | POST | /agents/workspaces/:wid/unlock | PoolToken + WsCheck | 同上 | ✅ |
| 50 | GET | /agents/workspaces/:wid/state/max-version | PoolToken + WsCheck | 同上 | ✅ |
| 51 | PATCH | /agents/workspaces/:wid/fields | PoolToken + WsCheck | 同上 | ✅ |
| 52 | GET | /agents/workspaces/:wid/terraform-lock-hcl | PoolToken + WsCheck | 同上 | ✅ |
| 53 | PUT | /agents/workspaces/:wid/terraform-lock-hcl | PoolToken + WsCheck | 同上 | ✅ |

### 11. Agent Terraform Version API（Pool Token）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 54 | GET | /agents/terraform-versions/default | PoolToken | Token所属Pool | ✅ 查询默认TF版本，只读元数据 |
| 55 | GET | /agents/terraform-versions/:version | PoolToken | Token所属Pool | ✅ 查询特定TF版本配置 |

### 12. Run Task 回调（无认证）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 56 | PATCH | /run-task-results/:id/callback | 无 | 无 | ❌ **严重**: 任何人知道result_id即可伪造回调结果，篡改安全扫描状态，绕过审批门禁。应加HMAC签名验证 |
| 57 | POST | /run-task-results/:id/callback | 无 | 无 | ❌ **严重**: 同上 |
| 58 | GET | /run-task-results/:id | 无 | 无 | ❌ 无认证可枚举获取扫描结果，泄露基础设施变更详情。应加HMAC或Bearer Token |

### 13. IAM 权限检查（所有认证用户）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 59 | POST | /iam/permissions/check | JWT + AuditLogger | 无IAM（设计如此） | ✅ 用户检查自身权限的自服务API，必须对所有认证用户开放 |

### 14. MFA 用户自服务（JWT，无IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 60 | GET | /user/mfa/status | JWT + AuditLogger | 无 | ✅ 查看自身MFA状态 |
| 61 | POST | /user/mfa/setup | JWT + AuditLogger | 无 | ✅ 设置自身MFA，用户自服务 |
| 62 | POST | /user/mfa/verify | JWT + AuditLogger | 无 | ✅ 验证并启用自身MFA |
| 63 | POST | /user/mfa/disable | JWT + AuditLogger | 无 | ✅ 禁用自身MFA。注意：如果组织强制MFA，handler应拒绝此操作 |
| 64 | POST | /user/mfa/backup-codes/regenerate | JWT + AuditLogger | 无 | ✅ 重新生成自身备份码 |

### 15. Dashboard（JWT + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 65 | GET | /dashboard/overview | JWT + AuditLogger | IAM: ORGANIZATION / ORGANIZATION / READ | ✅ 需组织级READ权限查看概览。注意：**无admin绕过**，admin也必须有ORGANIZATION READ权限 |
| 66 | GET | /dashboard/compliance | JWT + AuditLogger | IAM: ORGANIZATION / ORGANIZATION / READ | ✅ 同上 |

> **Dashboard权限评估**: 这两个接口没有admin绕过逻辑，直接使用`iamMiddleware.RequirePermission`作为中间件。如果admin用户没有被授予ORGANIZATION READ权限，将返回403。这与其他接口的`admin绕过 + IAM fallback`模式不一致，可能导致admin用户意外被拒。建议统一为admin绕过模式或确保admin默认拥有该权限。

### 16. Remote Data 公开端点

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 67 | GET | /workspaces/:id/state-outputs/full | 临时Token | Token验证 | ✅ 使用临时token认证，用于跨workspace数据引用，不走JWT体系 |

### 17. 密文管理（JWT，无IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 68 | POST | /:resourceType/:resourceId/secrets | JWT + AuditLogger | 无 | ❌ **严重**: 任何认证用户可为任意资源类型创建密文。应根据resourceType动态映射IAM权限 |
| 69 | GET | /:resourceType/:resourceId/secrets | JWT + AuditLogger | 无 | ❌ **严重**: 任何认证用户可读取任意资源的密文列表（含云平台凭证） |
| 70 | GET | /:resourceType/:resourceId/secrets/:secretId | JWT + AuditLogger | 无 | ❌ **严重**: 同上 |
| 71 | PUT | /:resourceType/:resourceId/secrets/:secretId | JWT + AuditLogger | 无 | ❌ **严重**: 任何认证用户可修改任意密文 |
| 72 | DELETE | /:resourceType/:resourceId/secrets/:secretId | JWT + AuditLogger | 无 | ❌ **严重**: 任何认证用户可删除任意密文 |

> **密文管理权限映射建议**: `agent_pool` → AGENT_POOLS/ORGANIZATION/WRITE, `workspace` → WORKSPACE_MANAGEMENT/WORKSPACE/WRITE

### 18. 用户自服务（JWT，无IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 73 | POST | /user/reset-password | JWT + BypassIAMForAdmin | admin绕过 / IAM: USER_MANAGEMENT/USER/WRITE | ✅ 管理员重置他人密码，有完整权限检查 |
| 74 | POST | /user/change-password | JWT + BypassIAMForAdmin | 无IAM | ⚠️ 修改自身密码，handler中校验仅能改自己的。风险低，但SSO-only用户不应能设本地密码 |
| 75 | POST | /user/tokens | JWT + BypassIAMForAdmin | 无IAM | ⚠️ 创建API Token，handler限制只能管理自己的Token。建议可选的组织级策略限制Token创建 |
| 76 | GET | /user/tokens | JWT + BypassIAMForAdmin | 无IAM | ⚠️ 同上，列出自己的Token |
| 77 | DELETE | /user/tokens/:token_name | JWT + BypassIAMForAdmin | 无IAM | ⚠️ 同上，撤销自己的Token |

### 19. Workspace 管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 78 | GET | /workspaces | JWT + AuditLogger | admin绕过 / IAM: WORKSPACES/ORGANIZATION/READ | ✅ 列表需组织级READ，合理 |
| 79 | GET | /workspaces/:id | JWT + AuditLogger | admin绕过 / IAM: WORKSPACES/ORG/READ **或** WORKSPACE_MANAGEMENT/WS/READ | ✅ 支持组织级和workspace级两种授权路径，灵活 |
| 80 | GET | /workspaces/:id/overview | JWT + AuditLogger | 同上 | ✅ |
| 81 | PUT | /workspaces/:id | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_MANAGEMENT/WORKSPACE/WRITE | ✅ 修改workspace需workspace级WRITE |
| 82 | PATCH | /workspaces/:id | JWT + AuditLogger | 同上 | ✅ |
| 83 | POST | /workspaces/:id/lock | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_MANAGEMENT/WORKSPACE/WRITE | ✅ 锁定workspace需WRITE权限 |
| 84 | POST | /workspaces/:id/unlock | JWT + AuditLogger | 同上 | ✅ |
| 85 | DELETE | /workspaces/:id | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_MANAGEMENT/WORKSPACE/ADMIN | ✅ 删除需ADMIN级别，合理 |
| 86 | POST | /workspaces | JWT + AuditLogger | admin绕过 / IAM: WORKSPACES/ORGANIZATION/WRITE | ✅ 创建workspace需组织级WRITE |

### 20. Workspace Task 操作（JWT + admin绕过 + IAM）

**READ级别** — 查看任务数据:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 87 | GET | /workspaces/:id/tasks | JWT + AuditLogger | admin绕过 / IAM: TASK_DATA_ACCESS **或** WORKSPACE_EXECUTION **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ 三个资源类型任一满足即可，支持多种授权场景 |
| 88 | GET | /workspaces/:id/tasks/:tid | JWT + AuditLogger | 同上 | ✅ |
| 89 | GET | /workspaces/:id/tasks/:tid/logs | JWT + AuditLogger | 同上 | ✅ |
| 90 | GET | /workspaces/:id/tasks/:tid/comments | JWT + AuditLogger | 同上 | ✅ |
| 91 | GET | /workspaces/:id/tasks/:tid/resource-changes | JWT + AuditLogger | 同上 | ✅ |
| 92 | GET | /workspaces/:id/tasks/:tid/error-analysis | JWT + AuditLogger | 同上 | ✅ |
| 93 | GET | /workspaces/:id/tasks/:tid/state-backup | JWT + AuditLogger | 同上 | ✅ state-backup可能含敏感数据，但有TASK_DATA_ACCESS权限控制 |

**WRITE级别** — 创建Plan/评论:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 94 | POST | /workspaces/:id/tasks/plan | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_EXECUTION **或** WORKSPACE_MANAGEMENT (WS/WRITE) | ✅ 创建Plan任务需WRITE，合理 |
| 95 | POST | /workspaces/:id/tasks/:tid/comments | JWT + AuditLogger | 同上 | ✅ |

**ADMIN级别** — 取消/确认Apply等危险操作:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 96 | POST | /workspaces/:id/tasks/:tid/cancel | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_EXECUTION **或** WORKSPACE_MANAGEMENT (WS/ADMIN) | ✅ 取消任务需ADMIN，防止普通用户干扰正在执行的Apply |
| 97 | POST | /workspaces/:id/tasks/:tid/cancel-previous | JWT + AuditLogger | 同上 | ✅ |
| 98 | POST | /workspaces/:id/tasks/:tid/confirm-apply | JWT + AuditLogger | 同上 | ✅ 确认Apply是最敏感操作，ADMIN级别合理 |
| 99 | PATCH | /workspaces/:id/tasks/:tid/resource-changes/:rid | JWT + AuditLogger | 同上 | ✅ |
| 100 | POST | /workspaces/:id/tasks/:tid/retry-state-save | JWT + AuditLogger | 同上 | ✅ |
| 101 | POST | /workspaces/:id/tasks/:tid/parse-plan | JWT + AuditLogger | 同上 | ✅ |

### 21. Workspace State 操作（JWT + admin绕过 + IAM）

**READ级别**:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 102 | GET | /workspaces/:id/current-state | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_STATE **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ |
| 103 | GET | /workspaces/:id/state-versions | JWT + AuditLogger | 同上 | ✅ |
| 104 | GET | /workspaces/:id/state/versions | JWT + AuditLogger | 同上 | ✅ 新版分页API |
| 105 | GET | /workspaces/:id/state/versions/:v | JWT + AuditLogger | 同上 | ✅ 仅元数据，不含state内容 |
| 106 | GET | /workspaces/:id/state/versions/:v/download | JWT + AuditLogger | 同上 | ✅ |
| 107 | GET | /workspaces/:id/state-versions/compare | JWT + AuditLogger | 同上 | ✅ |
| 108 | GET | /workspaces/:id/state-versions/:v/metadata | JWT + AuditLogger | 同上 | ✅ |
| 109 | GET | /workspaces/:id/state-versions/:v | JWT + AuditLogger | 同上 | ✅ |

**敏感State内容**:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 110 | GET | /workspaces/:id/state/versions/:v/retrieve | JWT + AuditLogger | admin绕过 / IAM: **WORKSPACE_STATE_SENSITIVE**/WS/READ **或** WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ **安全亮点**: State完整内容（含密码、密钥等）使用独立的WORKSPACE_STATE_SENSITIVE权限，与普通state元数据分离。非常合理的细粒度控制 |

**WRITE级别**:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 111 | POST | /workspaces/:id/state/upload | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_STATE/WS/WRITE **或** WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 112 | POST | /workspaces/:id/state/upload-file | JWT + AuditLogger | 同上 | ✅ |

**ADMIN级别**:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 113 | POST | /workspaces/:id/state/rollback | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_STATE/WS/ADMIN **或** WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ 回滚state是高危操作，ADMIN合理 |
| 114 | POST | /workspaces/:id/state-versions/:v/rollback | JWT + AuditLogger | 同上 | ✅ |
| 115 | DELETE | /workspaces/:id/state-versions/:v | JWT + AuditLogger | 同上 | ✅ 删除state版本不可逆，ADMIN合理 |

### 22. Workspace Variables（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 116 | GET | /workspaces/:id/variables | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_VARIABLES **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ |
| 117 | GET | /workspaces/:id/variables/:vid | JWT + AuditLogger | 同上 | ✅ |
| 118 | POST | /workspaces/:id/variables | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_VARIABLES **或** WORKSPACE_MANAGEMENT (WS/WRITE) | ✅ |
| 119 | PUT | /workspaces/:id/variables/:vid | JWT + AuditLogger | 同上 | ✅ |
| 120 | DELETE | /workspaces/:id/variables/:vid | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_VARIABLES **或** WORKSPACE_MANAGEMENT (WS/ADMIN) | ✅ |
| 121 | GET | /workspaces/:id/variables/:vid/versions | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_VARIABLES/WS/READ | ✅ |
| 122 | GET | /workspaces/:id/variables/:vid/versions/:v | JWT + AuditLogger | 同上 | ✅ |

### 23. Workspace Resources（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 123 | GET | /workspaces/:id/resources | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_RESOURCES **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ |
| 124-131 | GET | /workspaces/:id/resources/... (其他只读) | JWT + AuditLogger | 同上 | ✅ 所有只读操作统一READ级别 |
| 132 | POST | /workspaces/:id/resources | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_RESOURCES/WS/WRITE **或** WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 133 | POST | /workspaces/:id/resources/import | JWT + AuditLogger | 同上 | ✅ |
| 134 | POST | /workspaces/:id/resources/deploy | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_EXECUTION/WS/WRITE **或** WORKSPACE_MANAGEMENT/WS/WRITE | ✅ deploy使用WORKSPACE_EXECUTION权限而非WORKSPACE_RESOURCES，因为它触发实际部署 |
| 135 | PUT | /workspaces/:id/resources/:rid | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_RESOURCES/WS/WRITE | ✅ |
| 136 | DELETE | /workspaces/:id/resources/:rid | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE_RESOURCES/WS/ADMIN | ✅ 删除资源需ADMIN |

### 24. Workspace Pool关联（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 137 | GET | /workspaces/:id/available-pools | JWT + AuditLogger | admin绕过 / IAM: WORKSPACES/WORKSPACE/READ | ✅ |
| 138 | POST | /workspaces/:id/set-current-pool | JWT + AuditLogger | admin绕过 / IAM: WORKSPACES/WORKSPACE/WRITE | ✅ 绑定Pool需WRITE |
| 139 | GET | /workspaces/:id/current-pool | JWT + AuditLogger | admin绕过 / IAM: WORKSPACES/WORKSPACE/READ | ✅ |

### 25. Agent Pool 管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 140 | POST | /agent-pools | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 141 | GET | /agent-pools | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/READ | ✅ |
| 142 | GET | /agent-pools/:pid | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 143 | PUT | /agent-pools/:pid | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 144 | DELETE | /agent-pools/:pid | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/ADMIN | ✅ 删除Pool需ADMIN |
| 145 | POST | /agent-pools/:pid/allow-workspaces | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 146 | GET | /agent-pools/:pid/allowed-workspaces | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/READ | ✅ |
| 147 | DELETE | /agent-pools/:pid/allowed-workspaces/:wid | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 148 | POST | /agent-pools/:pid/tokens | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 149 | GET | /agent-pools/:pid/tokens | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/READ | ✅ |
| 150 | DELETE | /agent-pools/:pid/tokens/:name | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 151 | POST | /agent-pools/:pid/tokens/:name/rotate | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 152 | POST | /agent-pools/:pid/sync-deployment | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 153 | POST | /agent-pools/:pid/one-time-unfreeze | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 154 | PUT | /agent-pools/:pid/k8s-config | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/WRITE | ✅ |
| 155 | GET | /agent-pools/:pid/k8s-config | JWT + BypassIAMForAdmin | admin绕过 / IAM: AGENT_POOLS/ORGANIZATION/READ | ✅ |

### 26. Run Task 管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 156 | POST | /run-tasks | JWT + BypassIAMForAdmin | admin绕过 / IAM: RUN_TASKS/ORGANIZATION/WRITE | ✅ |
| 157 | GET | /run-tasks | JWT + BypassIAMForAdmin | admin绕过 / IAM: RUN_TASKS/ORGANIZATION/READ | ✅ |
| 158 | GET | /run-tasks/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 159 | PUT | /run-tasks/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: RUN_TASKS/ORGANIZATION/WRITE | ✅ |
| 160 | DELETE | /run-tasks/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: RUN_TASKS/ORGANIZATION/ADMIN | ✅ 删除需ADMIN |
| 161 | POST | /run-tasks/test | JWT + BypassIAMForAdmin | admin绕过 / IAM: RUN_TASKS/ORGANIZATION/WRITE | ✅ |
| 162 | POST | /run-tasks/:id/test | JWT + BypassIAMForAdmin | admin绕过 / IAM: RUN_TASKS/ORGANIZATION/READ | ✅ 测试已有Run Task只需READ |

### 27. IAM 权限管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 163 | GET | /iam/status | JWT + BypassIAMForAdmin | admin绕过（隐式拒绝非admin） | ✅ IAM状态查询 |
| 164 | POST | /iam/permissions/grant | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_PERMISSIONS/ORGANIZATION/ADMIN | ✅ 授权操作需ADMIN级别，防止权限提升 |
| 165 | POST | /iam/permissions/batch-grant | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 166 | POST | /iam/permissions/grant-preset | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 167 | DELETE | /iam/permissions/:scope_type/:id | JWT + BypassIAMForAdmin | 同上 | ✅ 撤销权限也需ADMIN |
| 168 | GET | /iam/permissions/:scope_type/:scope_id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_PERMISSIONS/ORGANIZATION/READ | ✅ 查看权限只需READ |
| 169 | GET | /iam/permissions/definitions | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 170 | GET | /iam/users/:id/permissions | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 171 | GET | /iam/teams/:id/permissions | JWT + BypassIAMForAdmin | 同上 | ✅ |

### 28. IAM 团队管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 172 | POST | /iam/teams | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/WRITE | ✅ |
| 173 | GET | /iam/teams | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/READ | ✅ |
| 174 | GET | /iam/teams/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 175 | DELETE | /iam/teams/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/ADMIN | ✅ 删除团队需ADMIN |
| 176 | POST | /iam/teams/:id/members | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/WRITE | ✅ |
| 177 | DELETE | /iam/teams/:id/members/:uid | JWT + BypassIAMForAdmin | 同上 | ✅ 移除成员用WRITE，合理（不是删除团队本身） |
| 178 | GET | /iam/teams/:id/members | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/READ | ✅ |

### 29. IAM 团队Token（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 179 | POST | /iam/teams/:id/tokens | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/WRITE | ✅ |
| 180 | GET | /iam/teams/:id/tokens | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/READ | ✅ |
| 181 | DELETE | /iam/teams/:id/tokens/:tid | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/WRITE | ✅ |

### 30. IAM 团队角色（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 182 | POST | /iam/teams/:id/roles | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/ADMIN | ✅ 角色分配需ADMIN |
| 183 | GET | /iam/teams/:id/roles | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/READ | ✅ |
| 184 | DELETE | /iam/teams/:id/roles/:aid | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_TEAMS/ORGANIZATION/ADMIN | ✅ |

### 31. IAM 组织管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 185 | POST | /iam/organizations | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ORGANIZATIONS/ORGANIZATION/ADMIN | ✅ 创建组织需ADMIN |
| 186 | GET | /iam/organizations | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ORGANIZATIONS/ORGANIZATION/READ | ✅ |
| 187 | GET | /iam/organizations/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 188 | PUT | /iam/organizations/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ORGANIZATIONS/ORGANIZATION/WRITE | ✅ |
| 189 | DELETE | /iam/organizations/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ORGANIZATIONS/ORGANIZATION/ADMIN | ✅ |

### 32. IAM 项目管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 190 | POST | /iam/projects | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_PROJECTS/ORGANIZATION/WRITE | ✅ |
| 191 | GET | /iam/projects | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_PROJECTS/ORGANIZATION/READ | ✅ |
| 192 | GET | /iam/projects/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 193 | PUT | /iam/projects/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_PROJECTS/ORGANIZATION/WRITE | ✅ |
| 194 | DELETE | /iam/projects/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_PROJECTS/ORGANIZATION/ADMIN | ✅ |

### 33. IAM 应用管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 195 | POST | /iam/applications | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_APPLICATIONS/ORGANIZATION/WRITE | ✅ |
| 196 | GET | /iam/applications | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_APPLICATIONS/ORGANIZATION/READ | ✅ |
| 197 | GET | /iam/applications/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 198 | PUT | /iam/applications/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_APPLICATIONS/ORGANIZATION/WRITE | ✅ |
| 199 | DELETE | /iam/applications/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_APPLICATIONS/ORGANIZATION/ADMIN | ✅ |
| 200 | POST | /iam/applications/:id/regenerate-secret | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_APPLICATIONS/ORGANIZATION/ADMIN | ✅ 重新生成Secret是敏感操作，ADMIN合理 |

### 34. IAM 审计日志（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 201 | GET | /iam/audit/config | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_AUDIT/ORGANIZATION/READ | ✅ |
| 202 | PUT | /iam/audit/config | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_AUDIT/ORGANIZATION/ADMIN | ✅ 修改审计配置需ADMIN |
| 203 | GET | /iam/audit/permission-history | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_AUDIT/ORGANIZATION/READ | ✅ |
| 204 | GET | /iam/audit/access-history | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 205 | GET | /iam/audit/denied-access | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 206 | GET | /iam/audit/permission-changes-by-principal | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 207 | GET | /iam/audit/permission-changes-by-performer | JWT + BypassIAMForAdmin | 同上 | ✅ |

### 35. IAM 用户管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 208 | GET | /iam/users/stats | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_USERS/ORGANIZATION/READ | ✅ |
| 209 | GET | /iam/users | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 210 | POST | /iam/users | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_USERS/ORGANIZATION/WRITE | ✅ |
| 211 | POST | /iam/users/:id/roles | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_USERS/ORGANIZATION/ADMIN | ✅ 角色分配需ADMIN，防止权限提升 |
| 212 | DELETE | /iam/users/:id/roles/:aid | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 213 | GET | /iam/users/:id/roles | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_USERS/ORGANIZATION/READ | ✅ |
| 214 | GET | /iam/users/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 215 | PUT | /iam/users/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_USERS/ORGANIZATION/WRITE | ✅ |
| 216 | POST | /iam/users/:id/activate | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_USERS/ORGANIZATION/ADMIN | ✅ 激活/停用用户需ADMIN |
| 217 | POST | /iam/users/:id/deactivate | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 218 | DELETE | /iam/users/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |

### 36. IAM 角色管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 219 | GET | /iam/roles | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ROLES/ORGANIZATION/READ | ✅ |
| 220 | GET | /iam/roles/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 221 | POST | /iam/roles | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ROLES/ORGANIZATION/WRITE | ✅ |
| 222 | PUT | /iam/roles/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 223 | DELETE | /iam/roles/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ROLES/ORGANIZATION/ADMIN | ✅ 删除角色需ADMIN |
| 224 | POST | /iam/roles/:id/policies | JWT + BypassIAMForAdmin | admin绕过 / IAM: IAM_ROLES/ORGANIZATION/WRITE | ✅ |
| 225 | DELETE | /iam/roles/:id/policies/:pid | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 226 | POST | /iam/roles/:id/clone | JWT + BypassIAMForAdmin | 同上 | ✅ |

### 37. 全局设置 — Terraform版本（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 227 | GET | /global/settings/terraform-versions | JWT + BypassIAMForAdmin | admin绕过 / IAM: TERRAFORM_VERSIONS/ORGANIZATION/READ | ✅ |
| 228 | GET | /global/settings/terraform-versions/default | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 229 | GET | /global/settings/terraform-versions/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 230 | POST | /global/settings/terraform-versions | JWT + BypassIAMForAdmin | admin绕过 / IAM: TERRAFORM_VERSIONS/ORGANIZATION/WRITE | ✅ |
| 231 | PUT | /global/settings/terraform-versions/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 232 | POST | /global/settings/terraform-versions/:id/set-default | JWT + BypassIAMForAdmin | admin绕过 / IAM: TERRAFORM_VERSIONS/ORGANIZATION/ADMIN | ✅ 设置默认版本影响全局，ADMIN合理 |
| 233 | DELETE | /global/settings/terraform-versions/:id | JWT + BypassIAMForAdmin | 同上 | ✅ |

### 38. 全局设置 — AI配置（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 234 | GET | /global/settings/ai-configs | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/READ | ✅ |
| 235 | POST | /global/settings/ai-configs | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/WRITE | ✅ |
| 236 | GET | /global/settings/ai-configs/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/READ | ✅ |
| 237 | PUT | /global/settings/ai-configs/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/WRITE | ✅ |
| 238 | DELETE | /global/settings/ai-configs/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/ADMIN | ✅ |
| 239 | PUT | /global/settings/ai-configs/priorities | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/WRITE | ✅ |
| 240 | PUT | /global/settings/ai-configs/:id/set-default | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/ADMIN | ✅ |
| 241 | GET | /global/settings/ai-config/regions | JWT + BypassIAMForAdmin | admin绕过 / IAM: AI_CONFIGS/ORGANIZATION/READ | ✅ |
| 242 | GET | /global/settings/ai-config/models | JWT + BypassIAMForAdmin | 同上 | ✅ |

### 39. 全局设置 — 平台配置/MFA（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 243 | GET | /global/settings/platform-config | JWT + BypassIAMForAdmin | admin绕过 / IAM: SYSTEM_SETTINGS/ORGANIZATION/READ | ✅ |
| 244 | PUT | /global/settings/platform-config | JWT + BypassIAMForAdmin | admin绕过 / IAM: SYSTEM_SETTINGS/ORGANIZATION/ADMIN | ✅ |
| 245 | GET | /global/settings/mfa | JWT + BypassIAMForAdmin | admin绕过 / IAM: SYSTEM_SETTINGS/ORGANIZATION/READ | ✅ |
| 246 | PUT | /global/settings/mfa | JWT + BypassIAMForAdmin | admin绕过 / IAM: SYSTEM_SETTINGS/ORGANIZATION/ADMIN | ✅ MFA全局配置修改需ADMIN |

### 40. 管理员MFA管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 247 | GET | /admin/users/:uid/mfa/status | JWT + BypassIAMForAdmin | admin绕过 / IAM: USER_MANAGEMENT/ORGANIZATION/READ | ✅ |
| 248 | POST | /admin/users/:uid/mfa/reset | JWT + BypassIAMForAdmin | admin绕过 / IAM: USER_MANAGEMENT/ORGANIZATION/ADMIN | ✅ 重置他人MFA需ADMIN |

### 41. 通知管理（JWT + BypassIAMForAdmin，无IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 249 | GET | /notifications | JWT + BypassIAMForAdmin | admin绕过 / 非admin隐式拒绝 | ⚠️ 无IAM权限检查，依赖中间件链隐式行为。非admin无法访问但无法委派权限 |
| 250 | GET | /notifications/available | JWT + BypassIAMForAdmin | 同上 | ⚠️ 同上 |
| 251 | GET | /notifications/:nid | JWT + BypassIAMForAdmin | 同上 | ⚠️ 同上 |
| 252 | POST | /notifications | JWT + BypassIAMForAdmin | 同上 | ⚠️ 同上 |
| 253 | PUT | /notifications/:nid | JWT + BypassIAMForAdmin | 同上 | ⚠️ 同上 |
| 254 | DELETE | /notifications/:nid | JWT + BypassIAMForAdmin | 同上 | ⚠️ 同上 |
| 255 | POST | /notifications/:nid/test | JWT + BypassIAMForAdmin | 同上 | ⚠️ 同上 |

### 42. Manifest 可视化编排（JWT，无IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 256 | GET | /organizations/:oid/manifests | JWT (orgManifests组) | 无IAM | ❌ Manifest列表无权限控制，任何认证用户可查看所有Manifest |
| 257 | POST | /organizations/:oid/manifests | JWT | 无IAM | ❌ 任何认证用户可创建Manifest，可触发多Workspace联动部署 |
| 258 | GET | /organizations/:oid/manifests/:id | JWT | 无IAM | ❌ |
| 259 | PUT | /organizations/:oid/manifests/:id | JWT | 无IAM | ❌ |
| 260 | DELETE | /organizations/:oid/manifests/:id | JWT | 无IAM | ❌ |
| 261 | PUT | /organizations/:oid/manifests/:id/draft | JWT | 无IAM | ❌ |
| 262 | GET | /organizations/:oid/manifests/:id/versions | JWT | 无IAM | ❌ |
| 263 | POST | /organizations/:oid/manifests/:id/versions | JWT | 无IAM | ❌ 发布版本无权限控制 |
| 264 | GET | /organizations/:oid/manifests/:id/versions/:vid | JWT | 无IAM | ❌ |
| 265 | GET | /organizations/:oid/manifests/:id/deployments | JWT | 无IAM | ❌ |
| 266 | POST | /organizations/:oid/manifests/:id/deployments | JWT | 无IAM | ❌ **严重**: 创建部署可触发Terraform Apply |
| 267 | GET | /organizations/:oid/manifests/:id/deployments/:did | JWT | 无IAM | ❌ |
| 268 | PUT | /organizations/:oid/manifests/:id/deployments/:did | JWT | 无IAM | ❌ |
| 269 | DELETE | /organizations/:oid/manifests/:id/deployments/:did | JWT | 无IAM | ❌ |
| 270 | GET | /organizations/:oid/manifests/:id/deployments/:did/resources | JWT | 无IAM | ❌ |
| 271 | POST | /organizations/:oid/manifests/:id/deployments/:did/uninstall | JWT | 无IAM | ❌ **严重**: 卸载部署触发Terraform Destroy |
| 272 | GET | /organizations/:oid/manifests/:id/export | JWT | 无IAM | ❌ 导出泄露基础设施架构 |
| 273 | GET | /organizations/:oid/manifests/:id/export-zip | JWT | 无IAM | ❌ 同上 |
| 274 | POST | /organizations/:oid/manifests/import | JWT | 无IAM | ❌ 导入可注入恶意配置 |
| 275 | POST | /organizations/:oid/manifests/import-json | JWT | 无IAM | ❌ 同上 |

> **Manifest模块总评**: 20个接口全部缺少IAM权限检查。注意router_manifest.go中虽然传入了adminProtected路由组，但orgManifests自行加了`middleware.JWTAuth()`，实际上跳过了BypassIAMForAdmin中间件链。这是因为Manifest路由组自己创建了独立的中间件链。建议统一为admin绕过 + IAM fallback模式。

### 43. Module 管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 276 | GET | /modules | JWT + AuditLogger | admin绕过 / IAM: MODULES/ORGANIZATION/READ | ✅ |
| 277 | GET | /modules/:id | JWT + AuditLogger | 同上 | ✅ |
| 278 | GET | /modules/:id/files | JWT + AuditLogger | 同上 | ✅ |
| 279 | GET | /modules/:id/schemas | JWT + AuditLogger | 同上 | ✅ |
| 280 | GET | /modules/:id/demos | JWT + AuditLogger | 同上 | ✅ |
| 281 | GET | /modules/:id/prompts | JWT + AuditLogger | 同上 | ✅ |
| 282 | POST | /modules | JWT + AuditLogger | admin绕过 / IAM: MODULES/ORGANIZATION/WRITE | ✅ |
| 283 | PUT | /modules/:id | JWT + AuditLogger | 同上 | ✅ |
| 284 | PATCH | /modules/:id | JWT + AuditLogger | 同上 | ✅ |
| 285 | POST | /modules/:id/sync | JWT + AuditLogger | 同上 | ✅ |
| 286 | POST | /modules/parse-tf | JWT + AuditLogger | 同上 | ✅ |
| 287 | POST | /modules/:id/schemas | JWT + AuditLogger | 同上 | ✅ |
| 288 | POST | /modules/:id/schemas/generate | JWT + AuditLogger | 同上 | ✅ |
| 289 | POST | /modules/:id/demos | JWT + AuditLogger | 同上 | ✅ |
| 290 | DELETE | /modules/:id | JWT + AuditLogger | admin绕过 / IAM: MODULES/ORGANIZATION/ADMIN | ✅ |
| 291+ | 其余Module Version/Schema V2接口 | JWT + AuditLogger | 遵循相同READ/WRITE/ADMIN分级 | ✅ 权限分级一致 |

### 44. AI 分析（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 292 | POST | /ai/analyze-error | JWT + AuditLogger | admin绕过 / IAM: AI_ANALYSIS/ORGANIZATION/WRITE或ADMIN | ✅ |
| 293 | POST | /ai/form/generate | JWT + AuditLogger | 同上 | ✅ |
| 294 | POST | /ai/form/generate-with-cmdb | JWT + AuditLogger | 同上 | ✅ |
| 295 | POST | /ai/form/generate-with-cmdb-skill | JWT + AuditLogger | 同上 | ✅ |
| 296 | POST | /ai/form/generate-with-cmdb-skill-sse | JWT + AuditLogger | 同上 | ✅ |
| 297 | POST | /ai/skill/preview-prompt | JWT + AuditLogger | admin绕过 / IAM: AI_ANALYSIS/ORGANIZATION/ADMIN | ✅ Prompt预览仅限ADMIN，防泄露Skill定义 |
| 298 | GET | /ai/embedding/config-status | JWT + AuditLogger | 无IAM | ⚠️ 缺少IAM权限检查，暴露AI配置状态。建议加AI_ANALYSIS/ORGANIZATION/READ |
| 299 | POST | /ai/cmdb/vector-search | JWT + AuditLogger | admin绕过 / IAM: AI_ANALYSIS/ORGANIZATION/READ或WRITE或ADMIN | ✅ |

### 45. Admin Embedding/Skills/Cache（JWT + BypassIAMForAdmin，无IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 300 | GET | /admin/embedding/status | JWT + BypassIAMForAdmin | admin绕过 / 非admin隐式拒绝 | ⚠️ 无IAM，无法委派给非admin |
| 301 | POST | /admin/embedding/sync-all | JWT + BypassIAMForAdmin | 同上 | ⚠️ 高开销操作，应有明确IAM权限 |
| 302 | GET | /admin/skills | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 303 | GET | /admin/skills/preview-discovery | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 304 | GET | /admin/skills/:id | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 305 | POST | /admin/skills | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 306 | PUT | /admin/skills/:id | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 307 | DELETE | /admin/skills/:id | JWT + BypassIAMForAdmin | 同上 | ⚠️ 不可逆操作应有ADMIN级别IAM |
| 308 | POST | /admin/skills/:id/activate | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 309 | POST | /admin/skills/:id/deactivate | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 310 | GET | /admin/skills/:id/usage-stats | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 311-314 | /admin/modules/:mid/skill/* | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 315-319 | /admin/module-versions/:id/skill/* | JWT + BypassIAMForAdmin | 同上 | ⚠️ |
| 320-324 | /admin/embedding-cache/* | JWT + BypassIAMForAdmin | 同上 | ⚠️ DELETE /clear 需要ADMIN级别IAM |

### 46. Workspace Embedding（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 325 | GET | /workspaces/:id/embedding-status | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE/WORKSPACE/READ+ | ✅ |
| 326 | POST | /workspaces/:id/embedding/sync | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE/WORKSPACE/WRITE+ | ✅ |
| 327 | POST | /workspaces/:id/embedding/rebuild | JWT + AuditLogger | admin绕过 / IAM: WORKSPACE/WORKSPACE/ADMIN | ✅ rebuild是高开销操作，ADMIN合理 |

### 47. Demo 管理（JWT + BypassIAMForAdmin + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 328 | GET | /demos/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: MODULE_DEMOS/ORGANIZATION/READ | ✅ |
| 329 | PUT | /demos/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: MODULE_DEMOS/ORGANIZATION/WRITE | ✅ |
| 330 | DELETE | /demos/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: MODULE_DEMOS/ORGANIZATION/ADMIN | ✅ |
| 331 | GET | /demos/:id/versions | JWT + BypassIAMForAdmin | admin绕过 / IAM: MODULE_DEMOS/ORGANIZATION/READ | ✅ |
| 332 | GET | /demos/:id/compare | JWT + BypassIAMForAdmin | 同上 | ✅ |
| 333 | POST | /demos/:id/rollback | JWT + BypassIAMForAdmin | admin绕过 / IAM: MODULE_DEMOS/ORGANIZATION/WRITE | ✅ |
| 334 | GET | /demo-versions/:versionId | JWT | admin绕过 / IAM: MODULE_DEMOS/ORGANIZATION/READ | ✅ |

### 48. Schema 管理（JWT + BypassIAMForAdmin + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 335 | GET | /schemas/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: SCHEMAS/ORGANIZATION/READ | ✅ |
| 336 | PUT | /schemas/:id | JWT + BypassIAMForAdmin | admin绕过 / IAM: SCHEMAS/ORGANIZATION/WRITE | ✅ |

### 49. Task 日志（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 337 | GET | /tasks/:task_id/output/stream | JWT + AuditLogger | admin绕过 / IAM: TASK_LOGS/ORGANIZATION/READ | ✅ |
| 338 | GET | /tasks/:task_id/logs | JWT + AuditLogger | 同上 | ✅ |
| 339 | GET | /tasks/:task_id/logs/download | JWT + AuditLogger | 同上 | ✅ |
| 340 | GET | /terraform/streams/stats | JWT + AuditLogger | 同上 | ✅ |

### 50. Project 管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 341 | GET | /projects | JWT + AuditLogger | admin绕过 / IAM: WORKSPACES/ORGANIZATION/READ | ✅ 复用WORKSPACES权限，因为项目列表与workspace可见性关联 |
| 342 | GET | /projects/:id/workspaces | JWT + AuditLogger | 同上 | ✅ |

### 51. CMDB 资源索引

**只读接口（仅JWT，无IAM）**:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 343 | GET | /cmdb/search | JWT + AuditLogger | 无IAM | ⚠️ 任何认证用户可搜索所有workspace资源，违反workspace级数据隔离 |
| 344 | GET | /cmdb/suggestions | JWT + AuditLogger | 无IAM | ⚠️ 同上 |
| 345 | GET | /cmdb/stats | JWT + AuditLogger | 无IAM | ⚠️ 暴露全局资源统计 |
| 346 | GET | /cmdb/resource-types | JWT + AuditLogger | 无IAM | ⚠️ 泄露内部资源类型定义 |
| 347 | GET | /cmdb/workspace-counts | JWT + AuditLogger | 无IAM | ⚠️ 暴露所有workspace的资源规模 |
| 348 | GET | /cmdb/workspaces/:wid/tree | JWT + AuditLogger | 无IAM | ⚠️ 可查看任意workspace的资源树 |
| 349 | GET | /cmdb/workspaces/:wid/resources | JWT + AuditLogger | 无IAM | ⚠️ 可查看任意workspace的资源详情 |

**管理接口（JWT + IAM: cmdb/ORGANIZATION/ADMIN）**:

| # | Method | Path | 认证 | 授权 | 合理性 |
|---|--------|------|------|------|--------|
| 350 | POST | /cmdb/workspaces/:wid/sync | JWT + AuditLogger | IAM: cmdb/ORGANIZATION/ADMIN | ✅ |
| 351 | POST | /cmdb/sync-all | JWT + AuditLogger | 同上 | ✅ |
| 352-358 | /cmdb/external-sources/* | JWT + AuditLogger | IAM: cmdb/ORGANIZATION/ADMIN | ✅ 外部数据源管理需ADMIN |

> **CMDB权限评估**: 注意CMDB管理接口的权限资源类型使用小写`cmdb`而非大写`CMDB`，与其他接口不一致。虽然功能正确（Permission Checker可能不区分大小写），但建议统一为大写以保持代码一致性。

---

## 授权合理性总结

### 统计

| 评级 | 数量 | 说明 |
|------|------|------|
| ✅ 合理 | ~310 | 认证+授权完整，权限级别匹配操作敏感度 |
| ⚠️ 可优化 | ~40 | 功能正确但缺少IAM或审计，可改进 |
| ❌ 需整改 | ~25 | 存在越权风险或缺少必要认证 |

### 设计亮点

1. **WORKSPACE_STATE_SENSITIVE 权限分离**: State完整内容（含密码/密钥）与State元数据使用不同权限，细粒度控制敏感数据访问
2. **多资源类型OR授权（RequireAnyPermission）**: workspace下的task操作允许TASK_DATA_ACCESS、WORKSPACE_EXECUTION、WORKSPACE_MANAGEMENT任一满足，灵活支持不同角色模型
3. **Agent双层校验**: Pool Token + Workspace Check确保Agent只能访问被授权的Workspace
4. **操作敏感度分级**: READ → WRITE → ADMIN三级权限与GET → POST/PUT → DELETE/危险操作对齐

### 主要风险点

1. **❌ run-task-callback无认证**: 3个接口，可伪造安全扫描结果绕过审批
2. **❌ secrets无IAM**: 5个接口，任何认证用户可读写所有资源的密文
3. **❌ manifest无IAM**: 20个接口，可触发未授权的Terraform Apply/Destroy
4. **⚠️ Dashboard无admin绕过**: 与其他接口模式不一致，admin可能被意外拒绝
5. **⚠️ CMDB只读接口无数据隔离**: 违反workspace级最小知情原则
6. **⚠️ admin路由依赖隐式拒绝**: notifications/skills/embedding等通过中间件链隐式阻断非admin，而非显式IAM检查
