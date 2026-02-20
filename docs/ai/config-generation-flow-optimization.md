# 配置生成流程优化方案

## 文档信息

| 项目 | 内容 |
|------|------|
| 文档版本 | 1.0.0 |
| 创建日期 | 2026-01-31 |
| 状态 | 待审核 |
| 作者 | AI Assistant |

---

## 1. 背景与目标

### 1.1 当前流程

当前的配置生成流程是**串行执行**的：

```
用户输入需求
    ↓
安全断言 (performIntentAssertion)
    ↓
判断是否需要 CMDB (shouldUseCMDB)
  ├── 关键词快速检测 (shouldUseCMDBByKeywords)
  └── AI 语义分析 (shouldUseCMDBByAI) ← 第 1 次 AI 调用
    ↓
[如果需要 CMDB] 执行 CMDB 查询 (performCMDBQuery)
  └── 生成查询计划 (parseQueryPlan) ← 第 2 次 AI 调用
    ↓
[如果有多个匹配结果] 返回 need_selection 等待用户选择
    ↓
装载 Skills (标签匹配)
  ├── Foundation Skills (固定配置)
  ├── Domain Skills (标签匹配: domain_tags ↔ tags)
  ├── Module Skill (自动加载)
  └── Task Skill (固定配置)
    ↓
组装 Prompt
    ↓
调用 AI 生成最终配置 ← 第 3 次 AI 调用
```

### 1.2 存在的问题

1. **AI 调用次数多**：CMDB 判断和查询计划生成分两次 AI 调用
2. **串行执行延迟高**：CMDB 查询和 Skill 选择串行执行
3. **Domain Skill 选择不精准**：标签匹配可能加载过多不相关的 Skills
   - 当前 `resource_generation_workflow` 的 `domain_tags` 是 `["cmdb", "matching", "region", "resource-management", "tagging"]`
   - 这会匹配到所有带这些标签的 Domain Skills（20+ 个）
   - 过多的 Skills 导致 Prompt 过长，影响 AI 理解和生成质量

### 1.3 优化目标

1. **减少 AI 调用次数**：合并 CMDB 判断和查询计划生成
2. **降低延迟**：CMDB 查询和 Skill 选择并行执行
3. **提高 Skill 选择精准度**：AI 根据用户需求智能选择 Domain Skills

---

## 2. 优化后的流程

### 2.1 流程图

```
用户输入需求
    ↓
安全断言 (performIntentAssertion)
    ↓
┌─────────────────────────────────────────────────────────────┐
│                      并行执行                                │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────┐  ┌─────────────────────────┐  │
│  │ CMDB 判断 + 查询        │  │ AI 选择 Domain Skills   │  │
│  │ (assessAndQueryCMDB)    │  │ (selectDomainSkillsByAI)│  │
│  │                         │  │                         │  │
│  │ 1. AI 判断是否需要 CMDB │  │ 1. 获取所有 Domain Skill│  │
│  │ 2. 同时输出查询计划     │  │    的 name + description│  │
│  │ 3. 立即执行 CMDB 查询   │  │ 2. AI 选择需要的 Skills │  │
│  │                         │  │ 3. 返回选中的 Skill 列表│  │
│  │ ← 第 1 次 AI 调用       │  │ ← 第 2 次 AI 调用       │  │
│  └─────────────────────────┘  └─────────────────────────┘  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
    ↓
合并结果
    ↓
[如果有多个 CMDB 匹配结果] 返回 need_selection 等待用户选择
    ↓
装载 Skills
  ├── Foundation Skills (固定配置)
  ├── Domain Skills (AI 选择的)  ← 优化点
  ├── Module Skill (自动加载)
  └── Task Skill (固定配置)
    ↓
组装 Prompt
    ↓
调用 AI 生成最终配置 ← 第 3 次 AI 调用
```

### 2.2 优化效果对比

| 指标 | 优化前 | 优化后 | 改进 |
|------|--------|--------|------|
| AI 调用次数 | 3 次（串行） | 3 次（2 次并行 + 1 次） | 延迟降低 |
| CMDB 相关 AI 调用 | 2 次（判断 + 查询计划） | 1 次（合并） | 减少 1 次 |
| Domain Skill 选择 | 标签匹配（可能 20+ 个） | AI 智能选择（3-5 个） | 更精准 |
| 总延迟 | T1 + T2 + T3 | max(T1, T2) + T3 | 降低约 30-40% |

---

## 3. 详细设计

### 3.1 数据模型变更

#### 3.1.1 Skill 模型添加 `description` 字段

**当前状态**：`description` 存储在 `metadata.description`（JSONB 字段内）

**问题**：
- 查询需要解析 JSONB，性能较低
- 无法在数据库层面限制长度
- 前端展示和编辑不便

**优化方案**：将 `description` 提升为顶级字段

```go
// backend/internal/models/skill.go

type Skill struct {
    ID          string          `gorm:"primaryKey;type:varchar(36)" json:"id"`
    Name        string          `gorm:"type:varchar(255);not null;uniqueIndex" json:"name"`
    DisplayName string          `gorm:"type:varchar(255);not null" json:"display_name"`
    Description string          `gorm:"type:varchar(500)" json:"description"`  // 新增
    Layer       SkillLayer      `gorm:"type:varchar(20);not null" json:"layer"`
    Content     string          `gorm:"type:text;not null" json:"content"`
    // ... 其他字段保持不变
}
```

**数据库迁移脚本**：

```sql
-- scripts/add_description_to_skills.sql

-- 1. 添加 description 字段
ALTER TABLE skills ADD COLUMN IF NOT EXISTS description VARCHAR(500);

-- 2. 从 metadata 迁移现有数据
UPDATE skills 
SET description = metadata->>'description' 
WHERE metadata->>'description' IS NOT NULL 
  AND description IS NULL;

-- 3. 添加注释
COMMENT ON COLUMN skills.description IS 'Skill 的简短描述，用于 AI 智能选择，限制 500 字符';
```

#### 3.1.2 Description 规范

| 要求 | 说明 |
|------|------|
| 长度 | 50-200 字符，最多 500 字符 |
| 格式 | 一句话描述 Skill 的用途和适用场景 |
| 语言 | 中文或英文，保持一致 |
| 关键词 | 包含核心功能关键词，便于 AI 理解 |

**示例**：

| Skill Name | Description |
|------------|-------------|
| `cmdb_resource_matching` | 从 CMDB 查询结果中匹配用户需要的资源，支持精确匹配和模糊匹配 |
| `aws_s3_policy_patterns` | S3 桶策略模式，用于生成允许/拒绝特定 Principal 访问的策略 |
| `aws_iam_policy_patterns` | IAM 策略模式，用于生成 IAM 角色信任策略和权限策略 |
| `aws_resource_tagging` | AWS 资源标签规范，定义必填标签、可选标签和命名规则 |
| `region_mapping` | AWS 区域映射规则，处理区域代码和名称的转换 |

### 3.2 优化 1: CMDB 判断与查询合并

#### 3.2.1 修改 `cmdb_need_assessment_workflow` Skill

**当前输出格式**：
```json
{
  "need_cmdb": true,
  "reason": "用户提到使用现有的 EC2 role",
  "resource_types": ["aws_iam_role"]
}
```

**优化后输出格式**：
```json
{
  "need_cmdb": true,
  "reason": "用户提到使用现有的 EC2 role",
  "resource_types": ["aws_iam_role"],
  "query_plan": [
    {
      "resource_type": "aws_iam_role",
      "filters": {
        "name_contains": "ec2",
        "tags": {"Environment": "production"}
      },
      "limit": 10
    }
  ]
}
```

#### 3.2.2 新增服务方法

```go
// backend/services/ai_cmdb_skill_service.go

// CMDBAssessmentResult CMDB 评估结果（合并判断和查询计划）
type CMDBAssessmentResult struct {
    NeedCMDB      bool                   `json:"need_cmdb"`
    Reason        string                 `json:"reason"`
    ResourceTypes []string               `json:"resource_types"`
    QueryPlan     []CMDBQueryPlanItem    `json:"query_plan"`
}

type CMDBQueryPlanItem struct {
    ResourceType string                 `json:"resource_type"`
    Filters      map[string]interface{} `json:"filters"`
    Limit        int                    `json:"limit"`
}

// assessAndQueryCMDB 合并 CMDB 判断和查询
// 返回: CMDB 查询结果, 是否需要用户选择, 选择列表, 错误
func (s *AICMDBSkillService) assessAndQueryCMDB(
    userID string,
    userDescription string,
) (*CMDBQueryResults, bool, []CMDBLookupResult, error) {
    // 1. 关键词快速检测
    if !s.shouldUseCMDBByKeywords(userDescription) {
        // 不需要 CMDB，直接返回
        return nil, false, nil, nil
    }
    
    // 2. AI 判断 + 生成查询计划（合并为一次调用）
    assessment, err := s.assessCMDBWithQueryPlan(userDescription)
    if err != nil {
        log.Printf("[AICMDBSkillService] CMDB 评估失败: %v，继续执行（不使用 CMDB）", err)
        return nil, false, nil, nil
    }
    
    if !assessment.NeedCMDB {
        return nil, false, nil, nil
    }
    
    // 3. 执行 CMDB 查询
    results, err := s.executeCMDBQueriesFromPlan(userID, assessment.QueryPlan)
    if err != nil {
        log.Printf("[AICMDBSkillService] CMDB 查询失败: %v，继续执行（不使用 CMDB）", err)
        return nil, false, nil, nil
    }
    
    // 4. 检查是否需要用户选择
    needSelection, lookups := s.checkNeedSelection(results)
    
    return results, needSelection, lookups, nil
}
```

### 3.3 优化 2: AI 智能选择 Domain Skills

#### 3.3.1 新增 `domain_skill_selection` AI 配置

```sql
-- scripts/add_domain_skill_selection_ai_config.sql

INSERT INTO ai_configs (
    id, name, description, service_type, model_id, aws_region,
    capabilities, is_active, priority, rate_limit_seconds, mode
) VALUES (
    'ai-config-domain-skill-selection',
    'Domain Skill 智能选择',
    '根据用户需求智能选择需要的 Domain Skills',
    'bedrock',
    'anthropic.claude-3-haiku-20240307-v1:0',  -- 使用轻量模型
    'us-west-2',
    '["domain_skill_selection"]',
    true,
    100,  -- 高优先级
    5,    -- 5 秒限流
    'prompt'
) ON CONFLICT (id) DO NOTHING;
```

#### 3.3.2 新增服务方法

```go
// backend/services/ai_cmdb_skill_service.go

// DomainSkillInfo Domain Skill 简要信息
type DomainSkillInfo struct {
    Name        string `json:"name"`
    Description string `json:"description"`
}

// DomainSkillSelectionResult AI 选择结果
type DomainSkillSelectionResult struct {
    SelectedSkills []string `json:"selected_skills"`
    Reason         string   `json:"reason"`
}

// getAllDomainSkillDescriptions 获取所有 Domain Skill 的描述（带缓存）
func (s *AICMDBSkillService) getAllDomainSkillDescriptions() ([]DomainSkillInfo, error) {
    // 检查缓存
    cacheKey := "domain_skill_descriptions"
    if cached, ok := s.cache.Get(cacheKey); ok {
        return cached.([]DomainSkillInfo), nil
    }
    
    // 查询数据库
    var skills []models.Skill
    err := s.db.Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true).
        Select("name", "description").
        Order("priority ASC").
        Find(&skills).Error
    if err != nil {
        return nil, err
    }
    
    // 转换为简要信息
    result := make([]DomainSkillInfo, len(skills))
    for i, skill := range skills {
        result[i] = DomainSkillInfo{
            Name:        skill.Name,
            Description: skill.Description,
        }
    }
    
    // 缓存 5 分钟
    s.cache.Set(cacheKey, result, 5*time.Minute)
    
    return result, nil
}

// selectDomainSkillsByAI AI 智能选择 Domain Skills
func (s *AICMDBSkillService) selectDomainSkillsByAI(
    userDescription string,
) ([]string, error) {
    // 1. 获取所有 Domain Skill 描述
    skillInfos, err := s.getAllDomainSkillDescriptions()
    if err != nil {
        return nil, err
    }
    
    // 2. 构建 Prompt
    prompt := s.buildDomainSkillSelectionPrompt(userDescription, skillInfos)
    
    // 3. 获取 AI 配置
    aiConfig, err := s.configService.GetConfigForCapability("domain_skill_selection")
    if err != nil || aiConfig == nil {
        log.Printf("[AICMDBSkillService] domain_skill_selection AI 配置不可用，降级到标签匹配")
        return nil, fmt.Errorf("AI 配置不可用")
    }
    
    // 4. 调用 AI
    result, err := s.aiFormService.callAI(aiConfig, prompt)
    if err != nil {
        return nil, err
    }
    
    // 5. 解析结果
    selection, err := s.parseDomainSkillSelection(result)
    if err != nil {
        return nil, err
    }
    
    return selection.SelectedSkills, nil
}

// buildDomainSkillSelectionPrompt 构建 Domain Skill 选择 Prompt
func (s *AICMDBSkillService) buildDomainSkillSelectionPrompt(
    userDescription string,
    skillInfos []DomainSkillInfo,
) string {
    var sb strings.Builder
    
    sb.WriteString("你是一个 IaC 平台的 Skill 选择助手。请根据用户需求，从可用的 Domain Skills 中选择需要的 Skills。\n\n")
    
    sb.WriteString("【用户需求】\n")
    sb.WriteString(userDescription)
    sb.WriteString("\n\n")
    
    sb.WriteString("【可用的 Domain Skills】\n")
    for i, info := range skillInfos {
        sb.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, info.Name, info.Description))
    }
    sb.WriteString("\n")
    
    sb.WriteString("【选择规则】\n")
    sb.WriteString("1. 只选择与用户需求直接相关的 Skills\n")
    sb.WriteString("2. 通常选择 2-5 个 Skills 即可，不要贪多\n")
    sb.WriteString("3. 如果用户需求涉及 CMDB 资源引用，选择 cmdb_resource_matching\n")
    sb.WriteString("4. 如果用户需求涉及 AWS 策略（IAM/S3/KMS 等），选择对应的策略 Skill\n")
    sb.WriteString("5. 如果用户需求涉及资源标签，选择 aws_resource_tagging\n")
    sb.WriteString("\n")
    
    sb.WriteString("【输出格式】\n")
    sb.WriteString("请返回 JSON 格式，不要有任何额外文字：\n")
    sb.WriteString("```json\n")
    sb.WriteString("{\n")
    sb.WriteString("  \"selected_skills\": [\"skill_name_1\", \"skill_name_2\"],\n")
    sb.WriteString("  \"reason\": \"简短说明选择理由\"\n")
    sb.WriteString("}\n")
    sb.WriteString("```\n")
    
    return sb.String()
}
```

### 3.4 并行执行实现

#### 3.4.1 超时控制

并行执行时需要设置超时，避免单个协程卡住影响整体响应：

```go
// backend/services/ai_cmdb_skill_service.go

const (
    ParallelExecutionTimeout = 15 * time.Second  // 并行执行总超时
    CMDBQueryTimeout         = 10 * time.Second  // CMDB 查询超时
    SkillSelectionTimeout    = 8 * time.Second   // Skill 选择超时
)
```

#### 3.4.2 主流程实现

```go
// backend/services/ai_cmdb_skill_service.go

// GenerateConfigWithCMDBSkill 使用 Skill 模式生成配置（优化版）
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkill(
    userID string,
    moduleID uint,
    userDescription string,
    workspaceID string,
    organizationID string,
    userSelections map[string]interface{},
    currentConfig map[string]interface{},
    mode string,
) (*GenerateConfigWithCMDBResponse, error) {
    totalTimer := NewTimer()
    requestID := generateRequestID() // 用于日志追踪
    log.Printf("[AICMDBSkillService] [%s] ========== 开始优化版 Skill 模式配置生成 ==========", requestID)
    
    // 1. 安全断言
    assertionResult, err := s.performIntentAssertion(userID, userDescription)
    if err != nil {
        log.Printf("[AICMDBSkillService] [%s] 意图断言服务不可用: %v，继续执行", requestID, err)
    } else if assertionResult != nil && !assertionResult.IsSafe {
        return &GenerateConfigWithCMDBResponse{
            Status:  "blocked",
            Message: assertionResult.Suggestion,
        }, nil
    }
    
    // 2. 转换用户选择
    convertedSelections := s.convertUserSelections(userSelections)
    
    // 3. 如果用户已选择资源，跳过 CMDB 查询
    if len(convertedSelections) > 0 {
        return s.generateWithUserSelections(
            userID, moduleID, userDescription, workspaceID, organizationID,
            convertedSelections, currentConfig, mode,
        )
    }
    
    // 4. 并行执行 CMDB 查询和 Skill 选择（带超时控制）
    ctx, cancel := context.WithTimeout(context.Background(), ParallelExecutionTimeout)
    defer cancel()
    
    parallelTimer := NewTimer()
    
    var wg sync.WaitGroup
    var cmdbResults *CMDBQueryResults
    var needSelection bool
    var cmdbLookups []CMDBLookupResult
    var selectedSkills []string
    var cmdbErr, skillErr error
    
    wg.Add(2)
    
    // 协程 1: CMDB 判断 + 查询（带超时）
    go func() {
        defer wg.Done()
        cmdbCtx, cmdbCancel := context.WithTimeout(ctx, CMDBQueryTimeout)
        defer cmdbCancel()
        
        done := make(chan struct{})
        go func() {
            cmdbResults, needSelection, cmdbLookups, cmdbErr = s.assessAndQueryCMDB(userID, userDescription)
            close(done)
        }()
        
        select {
        case <-done:
            // 正常完成
        case <-cmdbCtx.Done():
            cmdbErr = fmt.Errorf("CMDB 查询超时: %w", cmdbCtx.Err())
        }
    }()
    
    // 协程 2: AI 选择 Domain Skills（带超时）
    go func() {
        defer wg.Done()
        skillCtx, skillCancel := context.WithTimeout(ctx, SkillSelectionTimeout)
        defer skillCancel()
        
        done := make(chan struct{})
        go func() {
            selectedSkills, skillErr = s.selectDomainSkillsByAI(userDescription)
            close(done)
        }()
        
        select {
        case <-done:
            // 正常完成
        case <-skillCtx.Done():
            skillErr = fmt.Errorf("Skill 选择超时: %w", skillCtx.Err())
        }
    }()
    
    wg.Wait()
    
    log.Printf("[AICMDBSkillService] [%s] [耗时] 并行执行: %.0fms", requestID, parallelTimer.ElapsedMs())
    
    // 5. 处理 CMDB 错误（降级：继续执行，不使用 CMDB）
    if cmdbErr != nil {
        log.Printf("[AICMDBSkillService] [%s] CMDB 查询失败: %v，继续执行（不使用 CMDB）", requestID, cmdbErr)
        cmdbResults = nil
        // 记录指标
        aiCallCounter.WithLabelValues("cmdb_assessment", "error").Inc()
    }
    
    // 6. 处理 Skill 选择错误（降级：使用标签匹配）
    if skillErr != nil {
        log.Printf("[AICMDBSkillService] [%s] Skill 选择失败: %v，降级到标签匹配", requestID, skillErr)
        selectedSkills = nil // 后续会使用标签匹配
        // 记录指标
        aiCallCounter.WithLabelValues("domain_skill_selection", "error").Inc()
    } else {
        // 验证 AI 选择的 Skills
        selectedSkills = s.validateSelectedSkills(selectedSkills)
        // 记录指标
        selectedSkillCount.WithLabelValues("ai").Observe(float64(len(selectedSkills)))
    }
    
    // 7. 如果需要用户选择 CMDB 资源
    if needSelection {
        return &GenerateConfigWithCMDBResponse{
            Status:      "need_selection",
            CMDBLookups: cmdbLookups,
            Message:     "找到多个匹配的资源，请选择",
        }, nil
    }
    
    // 8. 组装 Prompt 并生成配置
    return s.assembleAndGenerate(
        userID, moduleID, userDescription, workspaceID, organizationID,
        cmdbResults, selectedSkills, currentConfig, mode,
    )
}
```

### 3.5 结果验证

#### 3.5.1 验证 AI 选择的 Skills

AI 返回的 `selected_skills` 可能包含不存在的 Skill 名称，需要验证：

```go
// validateSelectedSkills 验证 AI 选择的 Skills 是否存在
func (s *AICMDBSkillService) validateSelectedSkills(selected []string) []string {
    if len(selected) == 0 {
        return selected
    }
    
    // 获取所有有效的 Domain Skill 名称
    validNames := s.getAllDomainSkillNames()
    validNameSet := make(map[string]bool)
    for _, name := range validNames {
        validNameSet[name] = true
    }
    
    // 过滤无效的 Skill 名称
    validSkills := make([]string, 0, len(selected))
    for _, name := range selected {
        if validNameSet[name] {
            validSkills = append(validSkills, name)
        } else {
            log.Printf("[AICMDBSkillService] AI 选择了不存在的 Skill: %s，已忽略", name)
        }
    }
    
    return validSkills
}

// getAllDomainSkillNames 获取所有 Domain Skill 名称（带缓存）
func (s *AICMDBSkillService) getAllDomainSkillNames() []string {
    cacheKey := "domain_skill_names"
    if cached, ok := s.cache.Get(cacheKey); ok {
        return cached.([]string)
    }
    
    var names []string
    s.db.Model(&models.Skill{}).
        Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true).
        Pluck("name", &names)
    
    s.cache.Set(cacheKey, names, 5*time.Minute)
    return names
}
```

#### 3.5.2 验证 CMDB 查询计划

AI 生成的 `query_plan` 可能包含无效的 `resource_type`，需要验证：

```go
// validateQueryPlan 验证查询计划中的资源类型
func (s *AICMDBSkillService) validateQueryPlan(plan []CMDBQueryPlanItem) []CMDBQueryPlanItem {
    if len(plan) == 0 {
        return plan
    }
    
    // 获取 CMDB 支持的资源类型
    supportedTypes := s.cmdbService.GetSupportedResourceTypes()
    supportedSet := make(map[string]bool)
    for _, t := range supportedTypes {
        supportedSet[t] = true
    }
    
    // 过滤无效的资源类型
    validPlan := make([]CMDBQueryPlanItem, 0, len(plan))
    for _, item := range plan {
        if supportedSet[item.ResourceType] {
            validPlan = append(validPlan, item)
        } else {
            log.Printf("[AICMDBSkillService] 不支持的资源类型: %s，已忽略", item.ResourceType)
        }
    }
    
    return validPlan
}
```

### 3.6 监控指标

使用 Prometheus 监控优化效果：

```go
// backend/services/metrics.go

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // AI 调用计数器
    aiCallCounter = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ai_call_total",
            Help: "Total number of AI calls",
        },
        []string{"capability", "status"},
    )
    
    // AI 调用延迟
    aiCallDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "ai_call_duration_seconds",
            Help:    "AI call duration in seconds",
            Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
        },
        []string{"capability"},
    )
    
    // Skill 选择数量
    selectedSkillCount = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "selected_skill_count",
            Help:    "Number of skills selected",
            Buckets: []float64{1, 2, 3, 5, 10, 20},
        },
        []string{"mode"}, // "ai" or "tag_match"
    )
    
    // 并行执行延迟
    parallelExecutionDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "parallel_execution_duration_seconds",
            Help:    "Parallel execution duration in seconds",
            Buckets: []float64{0.5, 1, 2, 5, 10, 15},
        },
    )
)
```

### 3.7 灰度发布策略

建议使用灰度发布逐步验证优化效果：

```go
// backend/services/ai_cmdb_skill_service.go

// shouldUseOptimizedFlow 判断是否使用优化后的流程
func (s *AICMDBSkillService) shouldUseOptimizedFlow(userID string) bool {
    // 1. 检查功能开关
    if !s.configService.IsFeatureEnabled("optimized_config_generation") {
        return false
    }
    
    // 2. 检查灰度名单
    if s.isInGrayList(userID) {
        return true
    }
    
    // 3. 按百分比灰度
    grayPercentage := s.configService.GetGrayPercentage("optimized_config_generation")
    if grayPercentage > 0 {
        // 使用用户 ID 的哈希值确保同一用户始终得到相同结果
        hash := fnv.New32a()
        hash.Write([]byte(userID))
        userHash := hash.Sum32()
        return float64(userHash%100) < grayPercentage
    }
    
    return false
}

// isInGrayList 检查用户是否在灰度名单中
func (s *AICMDBSkillService) isInGrayList(userID string) bool {
    grayList := s.configService.GetGrayList("optimized_config_generation")
    for _, id := range grayList {
        if id == userID {
            return true
        }
    }
    return false
}
```

### 3.5 SkillAssembler 适配

```go
// backend/services/skill_assembler.go

// AssemblePromptWithSelectedSkills 使用 AI 选择的 Skills 组装 Prompt
func (a *SkillAssembler) AssemblePromptWithSelectedSkills(
    composition *models.SkillComposition,
    moduleID uint,
    dynamicContext *DynamicContext,
    selectedDomainSkills []string,  // AI 选择的 Domain Skills
) (*AssemblePromptResult, error) {
    // ... 省略部分代码
    
    // 2. 加载 Domain 层 Skills
    if len(selectedDomainSkills) > 0 {
        // 使用 AI 选择的 Skills
        log.Printf("[SkillAssembler] 使用 AI 选择的 Domain Skills: %v", selectedDomainSkills)
        domainSkills, err := a.loadSkillsByNames(selectedDomainSkills)
        if err != nil {
            log.Printf("[SkillAssembler] 加载 AI 选择的 Domain Skills 失败: %v", err)
        }
        allSkills = append(allSkills, domainSkills...)
    } else {
        // 降级到标签匹配
        log.Printf("[SkillAssembler] 降级到标签匹配模式")
        // ... 原有的标签匹配逻辑
    }
    
    // ... 后续逻辑保持不变
}
```

---

## 4. 实现计划

### 4.1 阶段划分

| 阶段 | 任务 | 预计时间 | 依赖 |
|------|------|----------|------|
| 1 | 数据模型变更 | 1 小时 | 无 |
| 2 | 更新现有 Skills 的 description | 1 小时 | 阶段 1 |
| 3 | 实现 CMDB 判断与查询合并 | 2 小时 | 阶段 1 |
| 4 | 实现 AI 智能选择 Domain Skills | 2 小时 | 阶段 1, 2 |
| 5 | 实现并行执行 | 1 小时 | 阶段 3, 4 |
| 6 | 前端适配 | 1 小时 | 阶段 1 |
| 7 | 测试和调试 | 2 小时 | 阶段 5 |

**总计：约 10 小时**

### 4.2 详细任务清单

#### 阶段 1: 数据模型变更 ✅

- [x] 修改 `backend/internal/models/skill.go`，添加 `Description` 字段
- [x] 创建数据库迁移脚本 `scripts/add_description_to_skills.sql`
- [x] 执行迁移脚本
- [x] 更新 Skill 相关的 API（创建、更新、查询）

#### 阶段 2: 更新现有 Skills 的 description ✅

- [x] 为所有 Domain Skills 编写精简的 description（15 个）
- [x] 为所有 Foundation Skills 编写 description（5 个）
- [x] 为所有 Task Skills 编写 description（5 个）
- [x] 执行更新脚本

#### 阶段 3: 实现 CMDB 判断与查询合并 ✅

- [x] 修改 `skill/cmdb/task/cmdb_need_assessment_workflow.md`，扩展输出格式（添加 query_plan）
- [x] 新增 `CMDBAssessmentWithQueryPlan` 结构体
- [x] 实现 `assessAndQueryCMDB()` 方法
- [x] 实现 `assessCMDBWithQueryPlan()` 方法
- [x] 实现 `executeCMDBQueriesFromPlan()` 方法

#### 阶段 4: 实现 AI 智能选择 Domain Skills ✅

- [x] 创建 `domain_skill_selection` AI 配置（id=15）
- [x] 实现 `getAllDomainSkillDescriptions()` 方法
- [x] 实现 `selectDomainSkillsByAI()` 方法
- [x] 实现 `buildDomainSkillSelectionPrompt()` 方法
- [x] 实现 `parseDomainSkillSelection()` 方法
- [x] 实现 `validateSelectedSkills()` 方法

#### 阶段 5: 实现并行执行 ✅

- [x] 新增 `GenerateConfigWithCMDBSkillOptimized()` 方法（优化版）
- [x] 实现 `executeParallel()` 方法（并行执行 CMDB 查询和 Skill 选择）
- [x] 实现 `generateWithCMDBDataAndSkills()` 方法
- [x] 实现错误处理和降级逻辑
- [x] 添加超时控制常量

#### 阶段 6: 前端适配（可选）

- [ ] 修改 Skill 编辑器，添加 `description` 字段
- [ ] 添加字符计数和长度限制提示

#### 阶段 7: 测试和调试

- [ ] 单元测试：CMDB 判断与查询合并
- [ ] 单元测试：AI 智能选择 Domain Skills
- [ ] 集成测试：完整流程测试
- [ ] 性能测试：对比优化前后的延迟

---

## 5. 风险与缓解措施

### 5.1 风险识别

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|----------|
| AI 选择 Skills 不准确 | 生成质量下降 | 中 | 降级到标签匹配 |
| 并行执行导致竞态条件 | 数据不一致 | 低 | 使用 sync.WaitGroup 同步 |
| CMDB 查询超时 | 用户体验差 | 中 | 设置超时，降级继续执行 |
| 缓存失效导致性能下降 | 延迟增加 | 低 | 合理设置缓存过期时间 |

### 5.2 回滚方案

如果优化后出现问题，可以通过以下方式回滚：

1. **AI 配置开关**：在 `ai_configs` 表中禁用 `domain_skill_selection` 配置
2. **代码回滚**：恢复 `GenerateConfigWithCMDBSkill()` 方法到优化前版本
3. **数据库回滚**：`description` 字段是新增的，不影响现有功能

---

## 6. 附录

### 6.1 完整 Skills 列表（从数据库查询）

#### 6.1.1 Domain Skills（15 个）

| 序号 | Name | Display Name | 当前 Description | 建议的 Description | 需要更新 |
|------|------|--------------|------------------|---------------------|----------|
| 1 | `terraform_module_best_practices` | terraform_module_best_practices | ❌ null | Terraform Module 最佳实践，包括命名规范、变量设计、输出定义等 | ✅ |
| 2 | `aws_resource_tagging` | aws_resource_tagging | ❌ null | AWS 资源标签规范，定义必填标签、可选标签和命名规则 | ✅ |
| 3 | `skill_doc_writing_standards` | skill_doc_writing_standards | ❌ null | Skill 文档编写标准，用于生成 Module Skill 文档 | ✅ |
| 4 | `schema_validation_rules` | Schema 验证规则 | ✅ Schema 验证规则 | Schema 验证规则，用于验证用户输入是否符合 OpenAPI Schema 约束 | ⚠️ 可优化 |
| 5 | `openapi_schema_interpretation` | openapi_schema_interpretation | ❌ null | OpenAPI Schema 解释规则，用于理解 Module 的参数定义 | ✅ |
| 6 | `cmdb_resource_matching` | CMDB 资源匹配 | ❌ null | 从 CMDB 查询结果中匹配用户需要的资源，支持精确匹配和模糊匹配 | ✅ |
| 7 | `cmdb_resource_types` | CMDB 资源类型映射 | ✅ CMDB 资源类型映射表 | CMDB 资源类型与 Terraform 资源类型的映射关系 | ⚠️ 可优化 |
| 8 | `region_mapping` | 区域映射 | ❌ null | AWS 区域映射规则，处理区域代码和名称的转换 | ✅ |
| 9 | `security_detection_rules` | 安全检测规则 | ✅ 定义各类安全威胁的检测规则 | 安全威胁检测规则，用于识别恶意输入和越狱攻击 | ⚠️ 可优化 |
| 10 | `aws_s3_policy_patterns` | AWS S3 桶策略模式 | ❌ null | S3 桶策略模式，用于生成允许/拒绝特定 Principal 访问的策略 | ✅ |
| 11 | `aws_kms_policy_patterns` | AWS KMS 密钥策略模式 | ✅ 完整 | KMS 密钥策略模式，包括密钥管理员、密钥用户、服务集成等场景 | ✅ 已有 |
| 12 | `aws_secrets_manager_policy_patterns` | AWS Secrets Manager 策略模式 | ✅ 完整 | Secrets Manager 策略模式，包括基本访问、跨账户、Lambda 轮换等场景 | ✅ 已有 |
| 13 | `aws_policy_core_principles` | AWS 策略核心原则 | ✅ 完整 | AWS 策略核心原则，包括最小权限、显式拒绝、策略结构等 | ✅ 已有 |
| 14 | `aws_iam_policy_patterns` | AWS IAM 策略模式 | ✅ 完整 | IAM 身份策略模式，包括 EC2、S3、Lambda、RDS 等服务的策略示例 | ✅ 已有 |
| 15 | `aws_condition_keys_reference` | AWS 条件键参考 | ✅ 完整 | AWS 条件键参考，包括全局条件键、服务特定条件键和使用示例 | ✅ 已有 |

**统计**：
- 需要新增 description：7 个
- 可优化 description：3 个
- 已有完整 description：5 个

#### 6.1.2 Foundation Skills（5 个）

| 序号 | Name | Display Name | 当前 Description |
|------|------|--------------|------------------|
| 1 | `markdown_output_format` | markdown_output_format | ❌ null |
| 2 | `json_output_format` | json_output_format | ❌ null |
| 3 | `placeholder_standard` | placeholder_standard | ❌ null |
| 4 | `json_schema_parser` | json_schema_parser | ❌ null |
| 5 | `output_format_standard` | 输出格式规范 | ❌ null |

> **注意**：Foundation Skills 不需要 AI 选择，但为了一致性，建议也补充 description。

#### 6.1.3 Task Skills（5 个）

| 序号 | Name | Display Name | 当前 Description |
|------|------|--------------|------------------|
| 1 | `cmdb_query_plan_workflow` | CMDB 查询计划生成工作流 | ✅ CMDB 查询计划生成的任务工作流 |
| 2 | `intent_assertion_workflow` | 意图断言工作流 | ✅ 检测用户输入是否安全，防止越狱攻击、提示注入等安全威胁 |
| 3 | `cmdb_need_assessment_workflow` | cmdb_need_assessment_workflow | ❌ null |
| 4 | `resource_generation_workflow` | 资源生成工作流 | ❌ null |
| 5 | `module_skill_generation_workflow` | Module Skill 生成工作流 | ❌ null |

> **注意**：Task Skills 不需要 AI 选择，但为了一致性，建议也补充 description。

### 6.2 需要更新 Description 的 SQL 脚本

```sql
-- scripts/update_skill_descriptions.sql

-- Domain Skills
UPDATE skills SET description = 'Terraform Module 最佳实践，包括命名规范、变量设计、输出定义等' WHERE name = 'terraform_module_best_practices';
UPDATE skills SET description = 'AWS 资源标签规范，定义必填标签、可选标签和命名规则' WHERE name = 'aws_resource_tagging';
UPDATE skills SET description = 'Skill 文档编写标准，用于生成 Module Skill 文档' WHERE name = 'skill_doc_writing_standards';
UPDATE skills SET description = 'Schema 验证规则，用于验证用户输入是否符合 OpenAPI Schema 约束' WHERE name = 'schema_validation_rules';
UPDATE skills SET description = 'OpenAPI Schema 解释规则，用于理解 Module 的参数定义' WHERE name = 'openapi_schema_interpretation';
UPDATE skills SET description = '从 CMDB 查询结果中匹配用户需要的资源，支持精确匹配和模糊匹配' WHERE name = 'cmdb_resource_matching';
UPDATE skills SET description = 'CMDB 资源类型与 Terraform 资源类型的映射关系' WHERE name = 'cmdb_resource_types';
UPDATE skills SET description = 'AWS 区域映射规则，处理区域代码和名称的转换' WHERE name = 'region_mapping';
UPDATE skills SET description = '安全威胁检测规则，用于识别恶意输入和越狱攻击' WHERE name = 'security_detection_rules';
UPDATE skills SET description = 'S3 桶策略模式，用于生成允许/拒绝特定 Principal 访问的策略' WHERE name = 'aws_s3_policy_patterns';

-- Foundation Skills (可选)
UPDATE skills SET description = 'Markdown 输出格式规范' WHERE name = 'markdown_output_format';
UPDATE skills SET description = 'JSON 输出格式规范' WHERE name = 'json_output_format';
UPDATE skills SET description = '占位符标准规范' WHERE name = 'placeholder_standard';
UPDATE skills SET description = 'JSON Schema 解析器' WHERE name = 'json_schema_parser';
UPDATE skills SET description = '通用输出格式规范' WHERE name = 'output_format_standard';

-- Task Skills (可选)
UPDATE skills SET description = 'CMDB 需求评估工作流，判断用户需求是否需要查询 CMDB' WHERE name = 'cmdb_need_assessment_workflow';
UPDATE skills SET description = '资源配置生成工作流，根据用户描述生成 Terraform 配置' WHERE name = 'resource_generation_workflow';
UPDATE skills SET description = 'Module Skill 生成工作流，从 Module Schema 自动生成 Skill 文档' WHERE name = 'module_skill_generation_workflow';
```

### 6.2 AI 调用成本估算

假设使用 Claude 3 Haiku 模型进行 Domain Skill 选择：

| 项目 | 估算值 |
|------|--------|
| 输入 Token（20 个 Skill 描述） | ~500 tokens |
| 输出 Token（选择结果） | ~100 tokens |
| 单次调用成本 | ~$0.0003 |
| 每日调用量（假设 1000 次） | ~$0.30/天 |

### 6.3 性能基准测试计划

优化前后需要对比以下指标：

1. **端到端延迟**
   - 测试场景：创建 S3 桶，允许来自 CMDB 中 EC2 的 role 访问
   - 测量点：从用户提交到返回配置的总时间

2. **AI 调用延迟**
   - CMDB 判断 + 查询计划：预期 500-1000ms
   - Domain Skill 选择：预期 300-500ms
   - 最终配置生成：预期 1000-2000ms

3. **Prompt 长度**
   - 优化前：可能包含 20+ 个 Domain Skills 的完整内容
   - 优化后：只包含 3-5 个选中的 Domain Skills

### 6.4 相关文件清单

| 文件路径 | 说明 |
|----------|------|
| `backend/internal/models/skill.go` | Skill 数据模型 |
| `backend/services/ai_cmdb_skill_service.go` | AI + CMDB + Skill 集成服务 |
| `backend/services/skill_assembler.go` | Skill 组装器 |
| `skill/cmdb/task/cmdb_need_assessment_workflow.md` | CMDB 需求评估 Skill |
| `skill/resource_generation/task/resource_generation_workflow.md` | 资源生成工作流 Skill |

---

## 7. 审核记录

| 日期 | 审核人 | 状态 | 备注 |
|------|--------|------|------|
| 2026-01-31 | - | 待审核 | 初稿完成 |
| 2026-01-31 | - | 已实现 | 核心功能实现完成 |

---

## 8. 变更历史

| 版本 | 日期 | 作者 | 变更内容 |
|------|------|------|----------|
| 1.0.0 | 2026-01-31 | AI Assistant | 初始版本 |
| 1.1.0 | 2026-01-31 | AI Assistant | 实现核心功能：数据模型变更、AI 智能选择 Domain Skills、CMDB 判断与查询合并、并行执行 |
| 1.2.0 | 2026-01-31 | AI Assistant | 添加 Domain Skill 智能选择自定义 Prompt 支持 |

---

## 9. 自定义 Prompt 支持

### 9.1 Domain Skill 智能选择

`domain_skill_selection` 能力支持自定义 Prompt，可以通过以下方式配置：

1. **capability_prompts 字段**（推荐）：
   ```json
   {
     "capability_prompts": {
       "domain_skill_selection": "你的自定义 Prompt..."
     }
   }
   ```

2. **custom_prompt 字段**：
   ```json
   {
     "custom_prompt": "你的自定义 Prompt..."
   }
   ```

### 9.2 可用占位符

在自定义 Prompt 中可以使用以下占位符：

| 占位符 | 说明 |
|--------|------|
| `{user_description}` | 用户的需求描述 |
| `{skill_list}` | 所有可用 Domain Skills 的列表（格式：`序号. skill_name - description`） |

### 9.3 示例

```
你是一个 IaC 平台的 Skill 选择助手。

【用户需求】
{user_description}

【可用的 Domain Skills】
{skill_list}

【输出格式】
请返回 JSON 格式：
{
  "selected_skills": ["skill_name_1", "skill_name_2"],
  "reason": "选择理由"
}
```

### 9.4 优先级

Prompt 选择优先级（从高到低）：
1. `capability_prompts["domain_skill_selection"]`
2. `custom_prompt`
3. 硬编码默认 Prompt
