# Domain Skill 自动发现优化方案

## 文档信息

- **版本**: 1.1
- **日期**: 2026-01-29
- **状态**: 🔄 待 Review（标签匹配优化）

---

## 一、问题背景

### 1.1 当前问题

当前 AI 配置中的 `SkillComposition.domain_skills` 是手动配置的固定列表：

```json
{
  "foundation_skills": ["platform_introduction", "output_format_standard"],
  "domain_skills": ["schema_validation_rules", "cmdb_resource_matching"],  // ← 写死的！
  "task_skill": "resource_generation_workflow",
  "auto_load_module_skill": true
}
```

**存在的问题**：

1. **配置混乱**：如果 Domain 层有 100 个基线规范 Skill，管理员需要手动选择哪些要加载
2. **维护困难**：随着 Domain Skill 数量增加，配置变得难以维护
3. **缺乏灵活性**：不同任务可能需要不同的 Domain Skill 组合，但当前是"一刀切"
4. **违背设计理念**：根据 claude-skill.md 的设计，Task 层应该自动发现需要的 Domain 层规则

### 1.2 目标

- Task Skill 能够声明自己需要哪些 Domain 知识
- 运行时自动发现并加载相关的 Domain Skills
- 保留手动固定选择的能力，两种模式并行
- 小规模场景下固定选择更可靠，大规模场景下自动发现更灵活

---

## 二、核心设计

### 2.1 Domain Skill 加载模式

| 模式 | 值 | 说明 | 适用场景 |
|------|---|------|---------|
| 固定选择 | `fixed` | 只使用 `domain_skills` 中手动选择的 | 小规模、需要精确控制 |
| 自动发现 | `auto` | 只使用 Task Skill 内容中声明的依赖 | 大规模、Domain Skills 多 |
| 混合模式 | `hybrid` | 固定选择 + 自动发现补充 | 两者结合 |

**默认值**：`fixed`（保持向后兼容）

### 2.2 Task Skill 依赖声明语法

在 Task Skill 的 Markdown 内容中使用 HTML 注释声明依赖：

```markdown
# resource_generation_workflow

## Dependencies
<!-- @require-domain: schema_validation_rules -->
<!-- @require-domain: security_compliance_rules -->
<!-- @require-domain: tagging_standards -->
<!-- @require-domain-if: use_cmdb == true -> cmdb_resource_matching -->
<!-- @require-domain-tag: security -->

## 工作流程
1. 分析用户需求
2. 根据 Schema 约束确定必填字段
...
```

**声明类型**：

| 语法 | 说明 | 示例 |
|------|------|------|
| `@require-domain: skill_name` | 直接加载指定的 Domain Skill | `@require-domain: schema_validation_rules` |
| `@require-domain-if: condition -> skill_name` | 条件满足时加载 | `@require-domain-if: use_cmdb == true -> cmdb_resource_matching` |
| `@require-domain-tag: tag_name` | 加载所有带该标签的 Domain Skills | `@require-domain-tag: security` |

**设计优势**：
- 使用 HTML 注释，不影响 Skill 内容的渲染
- 声明式依赖，Task Skill 自己声明需要什么
- 灵活性高，支持直接指定、条件加载、按标签加载

---

## 三、数据模型变更

### 3.1 SkillComposition 扩展

**文件**: `backend/internal/models/skill.go`

**新增字段**:

```go
type SkillComposition struct {
    FoundationSkills    []string               `json:"foundation_skills"`
    DomainSkills        []string               `json:"domain_skills"`        // 固定选择的 Domain Skills
    TaskSkill           string                 `json:"task_skill"`
    AutoLoadModuleSkill bool                   `json:"auto_load_module_skill"`
    
    // 新增：Domain Skill 加载模式
    DomainSkillMode     string                 `json:"domain_skill_mode"`    // "fixed" | "auto" | "hybrid"
    
    ConditionalRules    []SkillConditionalRule `json:"conditional_rules"`
}
```

**字段说明**：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `DomainSkillMode` | string | `"fixed"` | Domain Skill 加载模式 |

### 3.2 数据库变更

**无需变更**：`DomainSkillMode` 存储在 `ai_configs.skill_composition` JSONB 字段中，不需要新增数据库字段。

---

## 四、后端改动

### 4.1 SkillAssembler 改动

**文件**: `backend/services/skill_assembler.go`

#### 4.1.1 AssemblePrompt 方法改动

在加载 Domain Skills 的步骤中，根据 `DomainSkillMode` 选择不同的加载逻辑：

```
// 伪代码
switch composition.DomainSkillMode {
case "fixed", "":  // 默认为 fixed
    domainSkills = loadSkillsByNames(composition.DomainSkills)
    
case "auto":
    taskSkill = LoadSkill(composition.TaskSkill)
    domainSkills = discoverDomainSkillsFromContent(taskSkill.Content, dynamicContext)
    
case "hybrid":
    // 先加载固定选择的
    domainSkills = loadSkillsByNames(composition.DomainSkills)
    // 再从 Task Skill 中发现补充的
    taskSkill = LoadSkill(composition.TaskSkill)
    discoveredSkills = discoverDomainSkillsFromContent(taskSkill.Content, dynamicContext)
    // 合并去重
    domainSkills = mergeAndDeduplicate(domainSkills, discoveredSkills)
}
```

#### 4.1.2 新增方法：discoverDomainSkillsFromContent

**功能**：解析 Task Skill 内容中的 `@require-domain` 声明，返回发现的 Domain Skills 列表

**解析逻辑**：

1. **解析 `@require-domain: skill_name`**
   - 正则：`@require-domain:\s*(\w+)`
   - 直接加载指定的 Skill

2. **解析 `@require-domain-if: condition -> skill_name`**
   - 正则：`@require-domain-if:\s*(.+?)\s*->\s*(\w+)`
   - 评估条件，满足时加载

3. **解析 `@require-domain-tag: tag_name`**
   - 正则：`@require-domain-tag:\s*(\w+)`
   - 调用 `loadDomainSkillsByTag()` 加载

#### 4.1.3 新增方法：loadDomainSkillsByTag

**功能**：根据标签查询 Domain Skills

**查询逻辑**：
- 从 `skills` 表查询
- 条件：`layer = 'domain' AND is_active = true AND metadata->>'tags' LIKE '%tag_name%'`

### 4.2 加载流程图

```
AssemblePrompt()
  │
  ├─1. 加载 Foundation Skills
  │     └── loadSkillsByNames(foundation_skills)
  │
  ├─2. 根据 DomainSkillMode 加载 Domain Skills
  │     ├── fixed:  loadSkillsByNames(domain_skills)
  │     ├── auto:   discoverDomainSkillsFromContent(task_skill.content)
  │     └── hybrid: 两者合并去重
  │
  ├─3. 评估条件规则，加载额外 Skills
  │     └── evaluateConditionalRules(conditional_rules)
  │
  ├─4. 如果启用，加载 Module Skill
  │     └── GetOrGenerateModuleSkill(module_id)
  │
  ├─5. 加载 Task Skill
  │     └── LoadSkill(task_skill)
  │
  └─6. 排序、组装、填充上下文
        └── sortSkills() → join() → fillDynamicContext()
```

---

## 五、前端改动

### 5.1 AI 配置表单

**文件**: `frontend/src/pages/AIConfigForm.tsx`

#### 5.1.1 新增 UI 元素

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Domain Skills 配置                                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ 加载模式：                                                              │
│ ● 固定选择 - 手动选择具体的 Domain Skills（推荐小规模场景）             │
│ ○ 自动发现 - 从 Task Skill 中自动发现依赖（推荐大规模场景）             │
│ ○ 混合模式 - 固定选择 + 自动发现补充                                    │
│                                                                         │
│ ─────────────────────────────────────────────────────────────────────── │
│                                                                         │
│ 固定选择的 Domain Skills：                                              │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ ☑ schema_validation_rules (Schema 验证规则)                         │ │
│ │ ☑ security_compliance_rules (安全合规规则)                          │ │
│ │ ☐ cmdb_resource_matching (CMDB 资源匹配)                            │ │
│ │ ☐ tagging_standards (标签规范)                                      │ │
│ │ ☐ region_mapping (区域映射)                                         │ │
│ │ ...                                                                 │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
│                                                                         │
│ 💡 提示：自动发现模式会解析 Task Skill 中的 @require-domain 声明        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### 5.1.2 交互逻辑

| 模式 | 固定选择列表 | 说明 |
|------|-------------|------|
| `fixed` | 可编辑 | 只使用选中的 Domain Skills |
| `auto` | 隐藏或只读 | 完全由 Task Skill 声明决定 |
| `hybrid` | 可编辑 | 选中的 + 自动发现的 |

---

## 六、Task Skills 更新

### 6.1 需要更新的 Task Skills

| Task Skill | 需要声明的依赖 |
|------------|---------------|
| `resource_generation_workflow` | schema_validation_rules, security_compliance_rules, tagging_standards, cmdb_resource_matching(条件) |
| `intent_assertion_workflow` | 无（不需要 Domain Skills） |
| `cmdb_query_plan_workflow` | cmdb_resource_types, region_mapping |

### 6.2 示例：resource_generation_workflow

```markdown
# 资源配置生成工作流

## Dependencies
<!-- @require-domain: schema_validation_rules -->
<!-- @require-domain: security_compliance_rules -->
<!-- @require-domain: tagging_standards -->
<!-- @require-domain-if: use_cmdb == true -> cmdb_resource_matching -->

## 输入
- 用户描述：{user_description}
- Workspace 上下文：{workspace_context}
- CMDB 数据：{cmdb_data}
- Schema 约束：{schema_constraints}

## 处理步骤
1. 分析用户需求，识别要创建的资源类型
2. 根据 Schema 约束确定必填字段
3. 从 CMDB 数据中选择合适的关联资源
4. 生成符合约束的配置值

## 输出格式
{
  "field_name": "value",
  ...
}

## 注意事项
- 如果 Schema 中有默认值且用户未明确要求修改，不要输出该字段
- 对于无法确定的值，使用占位符格式
- 优先使用 CMDB 中匹配的资源 ID
```

### 6.3 示例：cmdb_query_plan_workflow

```markdown
# CMDB 查询计划生成工作流

## Dependencies
<!-- @require-domain: cmdb_resource_types -->
<!-- @require-domain: region_mapping -->

## 任务
分析用户描述，生成 CMDB 资源查询计划。

## 输入
- 用户描述：{user_description}
- 可查询的资源类型：{available_resource_types}

## 处理步骤
1. 分析用户描述中提到的资源需求
2. 识别需要查询的资源类型（VPC、子网、安全组、密钥对等）
3. 提取查询条件（区域、环境、标签等）
4. 生成查询计划

## 输出格式
{
  "queries": [...],
  "analysis": "..."
}
```

---

## 七、向后兼容性

| 场景 | 处理方式 |
|------|---------|
| 现有 AI 配置没有 `domain_skill_mode` | 默认为 `fixed`，行为完全不变 |
| 现有 Task Skills 没有 `@require-domain` | 自动发现模式返回空列表，不影响固定选择 |
| 混合模式下两者有重复 | 自动去重，不会重复加载 |
| `domain_skill_mode` 为空字符串 | 等同于 `fixed` |

---

## 八、实施步骤

### 8.1 后端改动

1. **Model 改动**：`SkillComposition` 新增 `DomainSkillMode` 字段
2. **Service 改动**：
   - `SkillAssembler.AssemblePrompt()` 根据模式选择加载逻辑
   - 新增 `discoverDomainSkillsFromContent()` 方法
   - 新增 `loadDomainSkillsByTag()` 方法

### 8.2 前端改动

1. **AI 配置表单**：新增"加载模式"单选框
2. **交互逻辑**：根据模式显示/隐藏固定选择列表

### 8.3 数据更新

1. **更新 Task Skills**：在现有 Task Skills 中添加 `@require-domain` 声明

### 8.4 测试验证

1. 验证 `fixed` 模式行为不变
2. 验证 `auto` 模式能正确解析依赖
3. 验证 `hybrid` 模式能正确合并去重
4. 验证条件加载 `@require-domain-if` 正常工作
5. 验证标签加载 `@require-domain-tag` 正常工作

---

## 九、风险评估

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 自动发现解析错误 | 低 | 中 | 添加详细日志，解析失败时降级到空列表 |
| 循环依赖 | 极低 | 高 | Domain Skills 不能声明依赖，只有 Task 可以 |
| 性能影响 | 低 | 低 | 解析是简单的正则匹配，耗时可忽略 |
| 向后兼容问题 | 极低 | 高 | 默认值为 `fixed`，不影响现有配置 |

---

## 十、后续扩展

### 10.1 可能的扩展方向

1. **优先级控制**：`@require-domain: skill_name [priority=10]`
2. **排除规则**：`@exclude-domain: deprecated_skill`
3. **版本约束**：`@require-domain: skill_name [version>=1.0.0]`

### 10.2 暂不实现的原因

当前方案已经能满足需求，保持简单。后续根据实际使用情况再决定是否扩展。

---

## 十一、优化方案：双向标签匹配（v1.1）

### 11.1 问题分析

当前 `@require-domain` 声明是**精确匹配**，需要在 Task Skill 中手动写明依赖名称。存在以下问题：

1. **维护成本高**：每次新增 Domain Skill 都需要更新 Task Skill
2. **容易遗漏**：管理员可能忘记在 Task Skill 中声明新的依赖
3. **复合名称问题**：如 `ec2_network_policy` 包含 "ec2" 但实际是网络相关的规则

**示例问题**：
- Domain Skill: `ec2_network_policy`
- 如果用关键词 "ec2" 匹配，会错误地加载这个偏向网络的规则
- 但它实际上应该被 "network" 或 "vpc" 相关的任务使用

### 11.2 双向标签匹配方案

#### 核心思想

- **Domain Skill 定义标签（tags）**：描述自己属于哪些领域
- **Task Skill 定义需要的标签（domain_tags）**：声明需要哪些领域的知识
- **运行时匹配**：查找 tags 与 domain_tags 有交集的 Domain Skills

#### 数据结构

**Domain Skill 的 metadata**：
```json
{
  "name": "ec2_network_policy",
  "metadata": {
    "tags": ["network", "vpc", "subnet", "security_group"],
    "description": "EC2 网络相关的策略规则"
  }
}
```

**Task Skill 的 metadata**：
```json
{
  "name": "resource_generation_workflow",
  "metadata": {
    "domain_tags": ["schema", "validation", "security"],
    "description": "资源配置生成工作流"
  }
}
```

#### 匹配逻辑

```sql
-- 查找 Domain Skills，其 tags 与 Task Skill 的 domain_tags 有交集
SELECT * FROM skills 
WHERE layer = 'domain' 
  AND is_active = true
  AND metadata->'tags' ?| ARRAY['schema', 'validation', 'security']
```

**PostgreSQL `?|` 操作符**：检查 JSONB 数组是否包含右侧数组中的任意一个元素。

### 11.3 标签设计示例

#### Domain Skills 标签

| Domain Skill | tags | 说明 |
|--------------|------|------|
| `schema_validation_rules` | `["schema", "validation", "openapi", "constraint"]` | Schema 验证规则 |
| `ec2_network_policy` | `["network", "vpc", "subnet", "security_group"]` | EC2 网络策略 |
| `ec2_instance_rules` | `["ec2", "instance", "compute", "ami"]` | EC2 实例规则 |
| `rds_security_rules` | `["rds", "database", "security", "encryption"]` | RDS 安全规则 |
| `cmdb_resource_matching` | `["cmdb", "matching", "resource", "lookup"]` | CMDB 资源匹配 |
| `tagging_standards` | `["tagging", "naming", "convention", "compliance"]` | 标签规范 |
| `security_compliance_rules` | `["security", "compliance", "audit", "policy"]` | 安全合规规则 |

#### Task Skills 需要的标签

| Task Skill | domain_tags | 会匹配到的 Domain Skills |
|------------|-------------|-------------------------|
| `resource_generation_workflow` | `["schema", "validation", "security"]` | schema_validation_rules, security_compliance_rules |
| `network_config_workflow` | `["network", "vpc", "security_group"]` | ec2_network_policy |
| `database_setup_workflow` | `["rds", "database", "security"]` | rds_security_rules, security_compliance_rules |
| `cmdb_query_plan_workflow` | `["cmdb", "matching"]` | cmdb_resource_matching |

### 11.4 优势分析

| 维度 | 精确匹配（@require-domain） | 标签匹配（domain_tags） |
|------|---------------------------|------------------------|
| **维护成本** | 高（每次新增都要更新 Task Skill） | 低（只需给 Domain Skill 打标签） |
| **灵活性** | 低（一对一绑定） | 高（多对多关系） |
| **误匹配风险** | 无 | 低（标签设计合理即可） |
| **可扩展性** | 差 | 好（新增 Domain Skill 自动被发现） |
| **复合名称处理** | 需要精确指定 | 通过标签精确控制 |

### 11.5 实现方案

#### 后端改动

**1. SkillAssembler 新增方法**：

```go
// discoverDomainSkillsByTags 根据 Task Skill 的 domain_tags 发现 Domain Skills
func (a *SkillAssembler) discoverDomainSkillsByTags(taskSkill *Skill) ([]*Skill, error) {
    // 1. 从 Task Skill 的 metadata 中提取 domain_tags
    domainTags := extractDomainTags(taskSkill.Metadata)
    if len(domainTags) == 0 {
        return nil, nil
    }
    
    // 2. 查询 tags 与 domain_tags 有交集的 Domain Skills
    var skills []*Skill
    err := a.db.Where("layer = ? AND is_active = ?", "domain", true).
        Where("metadata->'tags' ?| ?", pq.Array(domainTags)).
        Order("priority ASC").
        Find(&skills).Error
    
    return skills, err
}
```

**2. AssemblePrompt 方法改动**：

```go
case "auto":
    taskSkill := LoadSkill(composition.TaskSkill)
    // 优先使用标签匹配
    domainSkills = discoverDomainSkillsByTags(taskSkill)
    // 如果没有 domain_tags，降级到内容解析
    if len(domainSkills) == 0 {
        domainSkills = discoverDomainSkillsFromContent(taskSkill.Content, dynamicContext)
    }
```

#### 前端改动

**1. Skill 编辑器新增标签输入**：

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 编辑 Domain Skill: ec2_network_policy                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ 标签 (tags)：                                                           │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ [network] [vpc] [subnet] [security_group] [+ 添加]                  │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
│                                                                         │
│ 💡 提示：标签用于自动发现，Task Skill 会根据 domain_tags 匹配          │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**2. Task Skill 编辑器新增 domain_tags 输入**：

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 编辑 Task Skill: resource_generation_workflow                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ 需要的领域标签 (domain_tags)：                                          │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ [schema] [validation] [security] [+ 添加]                           │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
│                                                                         │
│ 预览匹配的 Domain Skills：                                              │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ ✓ schema_validation_rules (匹配: schema, validation)                │ │
│ │ ✓ security_compliance_rules (匹配: security)                        │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 11.6 迁移计划

1. **Phase 1**：为现有 Domain Skills 添加 tags
2. **Phase 2**：为现有 Task Skills 添加 domain_tags
3. **Phase 3**：修改 SkillAssembler 支持标签匹配
4. **Phase 4**：更新前端 Skill 编辑器

### 11.7 向后兼容

| 场景 | 处理方式 |
|------|---------|
| Domain Skill 没有 tags | 不会被标签匹配发现，但可以被精确声明加载 |
| Task Skill 没有 domain_tags | 降级到内容解析（@require-domain） |
| 两者都没有 | 使用固定选择模式 |

---

## 附录：配置示例

### A.1 固定选择模式（小规模场景）

```json
{
  "foundation_skills": ["platform_introduction", "output_format_standard"],
  "domain_skills": ["schema_validation_rules", "security_compliance_rules"],
  "task_skill": "resource_generation_workflow",
  "auto_load_module_skill": true,
  "domain_skill_mode": "fixed"
}
```

### A.2 自动发现模式（大规模场景）

```json
{
  "foundation_skills": ["platform_introduction", "output_format_standard"],
  "domain_skills": [],
  "task_skill": "resource_generation_workflow",
  "auto_load_module_skill": true,
  "domain_skill_mode": "auto"
}
```

### A.3 混合模式

```json
{
  "foundation_skills": ["platform_introduction", "output_format_standard"],
  "domain_skills": ["custom_company_rules"],
  "task_skill": "resource_generation_workflow",
  "auto_load_module_skill": true,
  "domain_skill_mode": "hybrid"
}