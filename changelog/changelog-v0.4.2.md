## v0.4.2

UI 统一化、MFA 修复、外部 CMDB 增强、资源摘要质量优化。

### New Features

- **外部 CMDB 资源详情** — 展开外部 CMDB 资源可查看完整信息：AI Summary、Description、ARN、Address，以及可折叠的 Attributes JSON (`CMDB.tsx`)
- **外部 CMDB Rebuild 按钮** — 外部数据源树视图新增 Rebuild 按钮，支持强制重建 embedding (`CMDB.tsx`)
- **CMDB 测试服务器增强** — 测试数据新增安全组规则（含公网暴露场景）、EC2 实例属性、S3 加密配置、RDS 备份配置等 (`cmdb-test-server/main.go`)

### Bug Fixes

- **MFA OTP 自动提交** — 首次 MFA 设置输入 6 位后自动提交，与验证页面行为一致 (`MFASetup.tsx`)
- **MFA OTP 退格删除** — 退格键从当前位置开始删除，不再需要手动将光标移到第一位 (`MFASetup.tsx`, `MFAVerify.tsx`)
- **外部 CMDB 密码覆盖** — 编辑外部数据源时不修改密码不再覆盖现有 secret (`ExternalSourcesTab.tsx`)
- **外部 CMDB 字段映射默认值** — 新建外部数据源时自动填充默认字段映射（$.type、$.name 等），不再提交空映射 (`ExternalSourcesTab.tsx`)
- **外部 CMDB 空数据源闪现** — 无外部数据源时 tab 和树视图不再闪现后消失 (`CMDB.tsx`)
- **CMDB 页面 null sources** — 后端返回 sources: null 时不再报 filter 错误 (`CMDB.tsx`)
- **StatePreview 布局** — 移除冗余的 globalHeader，操作按钮移入 stateInfoCard 标题行 (`StatePreview.tsx`)
- **资源摘要空属性跳过** — attributes 为空的资源不再调 AI 生成无效摘要 (`resource_summary_service.go`)
- **资源摘要 prompt 优化** — 纯文本输出（禁止 markdown），使用实际名称（禁止占位符），不推测不建议，200 字限制 (`resource_summary_service.go`)

### Improvements

- **全局 UI 统一** — 所有页面 sidebar 宽度统一 260px、背景渐变一致、TopBar header 高度统一 68px (`Layout.module.css`, `WorkspaceSidebar.module.css`, `WorkspaceDetail.module.css`, `TaskDetail.module.css`, `StatePreview.module.css`, `AddResources.module.css`, `TopBar.module.css`)
- **Header 精简** — 移除页面标题，保留 MotivationalQuote + 用户头像 (`TopBar.tsx`, `Layout.tsx`)
- **激励语精简** — 从 30+ 字缩短到 10 字以内 (`MotivationalQuote.tsx`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.1...v0.4.2
