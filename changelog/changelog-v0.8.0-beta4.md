## v0.8.0-beta4

Bedrock Prompt Caching 从硬编码改为可配置:AI Config 新增 `cache_enabled` 字段,管理员可按配置灵活开关 prompt caching;覆盖全部 4 条 Bedrock 调用路径;前端 AI 配置表单仅在 Bedrock 类型下显示开关。

### 新增功能

#### AI Config 缓存开关

- **新增** `cache_enabled` 字段:`AIConfig` 模型增加 `CacheEnabled bool` (默认 `true`),控制 Bedrock prompt caching 是否启用
- **新增** SQL migration:`add_cache_enabled.sql` — `ALTER TABLE ai_configs ADD COLUMN cache_enabled boolean NOT NULL DEFAULT true`
- **新增** 前端开关:`AIConfigForm.tsx` 仅在 `service_type === 'bedrock'` 时显示 "Prompt Caching" checkbox,带提示文案说明 90% 折扣效果
- **新增** `UpdateConfig` 持久化:`ai_config_service.go` 手动字段拷贝块补充 `existing.CacheEnabled = cfg.CacheEnabled`

#### 全路径条件缓存

- **优化** `BedrockCaller.buildBedrockRequest`:system prompt 的 `cache_control` 由 `c.cacheEnabled` 控制,true 时附加 `{type: "ephemeral"}`,false 时不附加
- **优化** `AIFormService.callBedrockForForm`:签名增加 `cacheEnabled bool`,条件化构建 system block
- **优化** `ModuleSkillAIService.callBedrock`:prompt 移至 system message + 条件缓存(原先 prompt 在 user message)
- **优化** `NewAICallerFromConfig`:bedrock 和 default 两个分支均传入 `cfg.CacheEnabled`

### 优化改进

#### 死代码标注

- **标注** `AIAnalysisService.callBedrock` 和 `callOpenAICompatible` 为 `Deprecated`,已被 `NewAICallerFromConfig + AIAgentLoop` 替代,当前无调用者

### 修改文件

- `backend/internal/models/ai_config.go` — `CacheEnabled` 字段
- `backend/migrations/add_cache_enabled.sql` — 新增列 migration
- `backend/services/ai_caller.go` — `BedrockCaller.cacheEnabled` + 条件 cache_control
- `backend/services/ai_caller_test.go` — 修复过期测试 + 新增 5 个缓存测试
- `backend/services/ai_config_service.go` — `UpdateConfig` 补充 `CacheEnabled` 拷贝
- `backend/services/ai_form_service.go` — `callBedrockForForm` 条件缓存
- `backend/services/ai_analysis_service.go` — 死方法标记 `Deprecated`
- `backend/services/module_skill_ai_service.go` — `callBedrock` 条件缓存 + prompt 移至 system
- `frontend/src/services/ai.ts` — `AIConfig` interface 增加 `cache_enabled`
- `frontend/src/pages/AIConfigForm.tsx` — 表单状态 + 编辑加载 + Bedrock 专用开关
- `manifests/db/init_seed_data.sql` — CREATE TABLE + 19 条 INSERT 增加 `cache_enabled`

### 技术细节

#### Review 发现的 Critical bug

- **问题**:`UpdateConfig` 逐字段手动拷贝 `cfg → existing`,`CacheEnabled` 被遗漏,导致 UI 切换缓存开关后保存无效
- **方案**:在 `existing.ThinkingBudgetTokens` 之后补充 `existing.CacheEnabled = cfg.CacheEnabled`
- **效果**:UI 切换 Prompt Caching 后点保存,数据库值正确持久化
