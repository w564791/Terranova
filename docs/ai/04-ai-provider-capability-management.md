# AI Provider 能力场景管理方案

## 1. 概述

本文档描述 AI Provider 配置的能力场景管理功能，允许为不同的 AI Provider 配置指定支持的能力场景，并通过优先级控制配置的选择顺序。

### 1.1 核心概念

- **默认配置**：支持所有场景的兜底配置，全局唯一
- **专用配置**：针对特定场景的配置，可以有多个
- **优先级**：通过拖拽调整专用配置的优先级，优先级高的配置优先使用
- **能力场景**：AI 功能的使用场景，如错误分析、变更分析等

### 1.2 支持的能力场景

| 场景标识 | 场景名称 | 说明 |
|---------|---------|------|
| `error_analysis` | 错误分析 | 分析 Terraform 执行错误并提供解决方案 |
| `change_analysis` | 变更分析 | 分析 Plan 变更内容和影响 |
| `result_analysis` | 结果分析 | 分析 Apply 执行结果 |
| `resource_generation` | 资源生成 | 基于需求生成 Terraform 资源代码 |

## 2. 数据模型设计

### 2.1 数据库表结构

```sql
-- ai_configs 表新增字段
ALTER TABLE ai_configs 
ADD COLUMN capabilities JSONB DEFAULT '[]',
ADD COLUMN priority INTEGER DEFAULT 0;

-- 为 priority 字段创建索引
CREATE INDEX idx_ai_configs_priority ON ai_configs(priority DESC);

-- 为 capabilities 字段创建 GIN 索引（支持 JSONB 查询）
CREATE INDEX idx_ai_configs_capabilities ON ai_configs USING GIN(capabilities);
```

### 2.2 字段说明

| 字段 | 类型 | 说明 |
|-----|------|------|
| `capabilities` | JSONB | 支持的能力场景数组 |
| `priority` | INTEGER | 优先级（数值越大优先级越高） |

### 2.3 配置类型

#### 默认配置
```json
{
  "id": 1,
  "service_type": "bedrock",
  "enabled": true,
  "capabilities": ["*"],
  "priority": 0
}
```
- `capabilities = ["*"]` 表示支持所有场景
- 全局只能有一个默认配置
- `priority` 字段对默认配置无效

#### 专用配置
```json
{
  "id": 2,
  "service_type": "openai",
  "enabled": true,
  "capabilities": ["error_analysis", "change_analysis"],
  "priority": 100
}
```
- `capabilities` 包含具体的场景标识
- 可以有多个专用配置
- `priority` 决定选择顺序

#### 未配置
```json
{
  "id": 3,
  "service_type": "ollama",
  "enabled": true,
  "capabilities": [],
  "priority": 0
}
```
- `capabilities = []` 表示用户还未决定用途
- 不会被任何场景使用

## 3. 配置选择逻辑

### 3.1 选择算法

```go
func GetConfigForCapability(capability string) (*AIConfig, error) {
    // 1. 查找启用的专用配置（按优先级降序，ID 升序）
    var configs []AIConfig
    err := db.Where("enabled = ? AND capabilities @> ?", true, 
        fmt.Sprintf(`["%s"]`, capability)).
        Order("priority DESC, id ASC").
        Find(&configs).Error
    
    if err == nil && len(configs) > 0 {
        return &configs[0], nil
    }
    
    // 2. 查找默认配置
    var defaultConfig AIConfig
    err = db.Where("enabled = ? AND capabilities @> ?", true, `["*"]`).
        First(&defaultConfig).Error
    
    if err == nil {
        return &defaultConfig, nil
    }
    
    return nil, errors.New("未找到可用的 AI 配置")
}
```

### 3.2 选择优先级

1. **专用配置优先**：优先使用支持该场景的专用配置
2. **按优先级排序**：多个专用配置时，使用 `priority` 最大的
3. **ID 作为次要排序**：`priority` 相同时，使用 ID 最小的
4. **默认配置兜底**：没有专用配置时，使用默认配置

### 3.3 使用示例

假设有以下配置：

```
配置1: enabled=true, capabilities=["*"], priority=0
  → 默认配置

配置2: enabled=true, capabilities=["error_analysis"], priority=100
  → 专用配置，高优先级

配置3: enabled=true, capabilities=["error_analysis"], priority=50
  → 专用配置，低优先级

配置4: enabled=true, capabilities=["change_analysis"], priority=100
  → 专用配置
```

场景使用：
- **错误分析**：使用配置2（专用，priority=100）
- **变更分析**：使用配置4（专用，priority=100）
- **结果分析**：使用配置1（默认配置）
- **资源生成**：使用配置1（默认配置）

## 4. 后端实现

### 4.1 模型定义

```go
// AIConfig AI 配置模型
type AIConfig struct {
    ID                  uint      `gorm:"primaryKey" json:"id"`
    ServiceType         string    `gorm:"type:varchar(50);not null;default:'bedrock'" json:"service_type"`
    AWSRegion           string    `gorm:"type:varchar(50)" json:"aws_region,omitempty"`
    ModelID             string    `gorm:"type:varchar(200)" json:"model_id"`
    BaseURL             string    `gorm:"type:varchar(500)" json:"base_url,omitempty"`
    APIKey              string    `gorm:"type:text" json:"api_key,omitempty"`
    CustomPrompt        string    `gorm:"type:text" json:"custom_prompt,omitempty"`
    Enabled             bool      `gorm:"default:true" json:"enabled"`
    RateLimitSeconds    int       `gorm:"default:10" json:"rate_limit_seconds"`
    UseInferenceProfile bool      `gorm:"default:false" json:"use_inference_profile"`
    Capabilities        []string  `gorm:"type:jsonb;default:'[]'" json:"capabilities"` // 新增
    Priority            int       `gorm:"default:0" json:"priority"`                   // 新增
    CreatedAt           time.Time `json:"created_at"`
    UpdatedAt           time.Time `json:"updated_at"`
}
```

### 4.2 服务方法

```go
// GetConfigForCapability 获取指定能力的配置
func (s *AIConfigService) GetConfigForCapability(capability string) (*models.AIConfig, error) {
    // 实现见 3.1 节
}

// UpdatePriority 更新配置优先级
func (s *AIConfigService) UpdatePriority(id uint, priority int) error {
    return s.db.Model(&models.AIConfig{}).
        Where("id = ?", id).
        Update("priority", priority).Error
}

// BatchUpdatePriorities 批量更新优先级
func (s *AIConfigService) BatchUpdatePriorities(updates []PriorityUpdate) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        for _, update := range updates {
            if err := tx.Model(&models.AIConfig{}).
                Where("id = ?", update.ID).
                Update("priority", update.Priority).Error; err != nil {
                return err
            }
        }
        return nil
    })
}

// SetAsDefault 设置为默认配置
func (s *AIConfigService) SetAsDefault(id uint) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 1. 取消其他配置的默认状态
        if err := tx.Model(&models.AIConfig{}).
            Where("id != ? AND capabilities @> ?", id, `["*"]`).
            Update("capabilities", []string{}).Error; err != nil {
            return err
        }
        
        // 2. 设置当前配置为默认
        if err := tx.Model(&models.AIConfig{}).
            Where("id = ?", id).
            Update("capabilities", []string{"*"}).Error; err != nil {
            return err
        }
        
        return nil
    })
}
```

### 4.3 API 接口

```go
// UpdatePriority 更新配置优先级
// @Summary 更新AI配置优先级
// @Tags AI
// @Param id path int true "配置ID"
// @Param priority body int true "优先级"
// @Router /api/v1/admin/ai-configs/{id}/priority [put]
func (c *AIController) UpdatePriority(ctx *gin.Context) {
    // 实现
}

// BatchUpdatePriorities 批量更新优先级
// @Summary 批量更新AI配置优先级
// @Tags AI
// @Param updates body []PriorityUpdate true "优先级更新列表"
// @Router /api/v1/admin/ai-configs/priorities [put]
func (c *AIController) BatchUpdatePriorities(ctx *gin.Context) {
    // 实现
}
```

## 5. 前端实现

### 5.1 配置列表页面

#### 5.1.1 页面布局

```
┌─────────────────────────────────────────────────────────────────┐
│ AI 配置管理                                    [+ 新增配置]      │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│ 默认配置（支持所有场景）                                         │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ 🔵 AWS Bedrock - Claude 3.5 Sonnet                        │   │
│ │ 区域: us-east-1  |  状态: 启用  |  频率限制: 10秒         │   │
│ │ 场景: 全部场景                                             │   │
│ │                                    [编辑] [禁用] [删除]   │   │
│ └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│ 专用配置（按优先级排序）                                         │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ ⋮⋮ 🟢 OpenAI - GPT-4                          优先级: 100 │   │
│ │    状态: 启用  |  频率限制: 10秒                          │   │
│ │    场景: 错误分析, 变更分析                               │   │
│ │                                    [编辑] [禁用] [删除]   │   │
│ └───────────────────────────────────────────────────────────┘   │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ ⋮⋮ 🟢 Azure OpenAI - GPT-4                   优先级: 50  │   │
│ │    状态: 启用  |  频率限制: 10秒                          │   │
│ │    场景: 资源生成                                         │   │
│ │                                    [编辑] [禁用] [删除]   │   │
│ └───────────────────────────────────────────────────────────┘   │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ ⚪ Ollama - Llama2                                        │   │
│ │    状态: 禁用  |  频率限制: 10秒                          │   │
│ │    场景: 未配置                                           │   │
│ │                                    [编辑] [启用] [删除]   │   │
│ └───────────────────────────────────────────────────────────┘   │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

#### 5.1.2 功能说明

1. **分区显示**
   - 默认配置单独显示在顶部
   - 专用配置按优先级降序显示

2. **拖拽排序**
   - 专用配置左侧显示拖拽手柄（⋮⋮）
   - 可以拖拽调整优先级
   - 默认配置不可拖拽

3. **状态标识**
   - 🔵 默认配置（蓝色圆点）
   - 🟢 启用的专用配置（绿色圆点）
   - ⚪ 禁用的配置（灰色圆点）

4. **场景标签**
   - 默认配置显示"全部场景"
   - 专用配置显示具体场景列表
   - 未配置显示"未配置"

### 5.2 配置表单页面

#### 5.2.1 能力场景选择

```
┌─────────────────────────────────────────────────────────────────┐
│ 支持的能力场景                                                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│ ☐ 默认配置（支持所有场景）                                       │
│   全局只能有一个默认配置，设置后会自动取消其他配置的默认状态     │
│                                                                   │
│ 专用场景（可多选）                                               │
│ ☑ 错误分析                                                       │
│   分析 Terraform 执行错误并提供解决方案                          │
│                                                                   │
│ ☑ 变更分析                                                       │
│   分析 Plan 变更内容和影响                                       │
│                                                                   │
│ ☐ 结果分析                                                       │
│   分析 Apply 执行结果                                            │
│                                                                   │
│ ☐ 资源生成                                                       │
│   基于需求生成 Terraform 资源代码                                │
│                                                                   │
│ 提示：不选择任何场景表示"未配置"，该配置不会被使用               │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

#### 5.2.2 交互逻辑

1. **默认配置选择**
   - 勾选"默认配置"时，禁用所有专用场景复选框
   - 取消"默认配置"时，启用专用场景复选框

2. **专用场景选择**
   - 可以多选
   - 至少选择一个场景（否则为"未配置"）

3. **保存验证**
   - 如果选择"默认配置"，检查是否已有其他默认配置
   - 如果有，提示用户确认（会自动取消其他配置的默认状态）

### 5.3 拖拽排序实现

#### 5.3.1 技术方案

使用 `react-beautiful-dnd` 或 `@dnd-kit/core` 实现拖拽功能。

```tsx
import { DndContext, closestCenter } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';

const AIConfigList = () => {
  const [configs, setConfigs] = useState<AIConfig[]>([]);
  
  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    
    if (active.id !== over?.id) {
      const oldIndex = configs.findIndex(c => c.id === active.id);
      const newIndex = configs.findIndex(c => c.id === over?.id);
      
      // 重新排序
      const newConfigs = arrayMove(configs, oldIndex, newIndex);
      setConfigs(newConfigs);
      
      // 计算新的优先级并批量更新
      const updates = newConfigs.map((config, index) => ({
        id: config.id,
        priority: (newConfigs.length - index) * 10
      }));
      
      await batchUpdatePriorities(updates);
    }
  };
  
  return (
    <DndContext onDragEnd={handleDragEnd}>
      <SortableContext items={configs} strategy={verticalListSortingStrategy}>
        {configs.map(config => (
          <SortableConfigItem key={config.id} config={config} />
        ))}
      </SortableContext>
    </DndContext>
  );
};
```

#### 5.3.2 优先级计算

拖拽后自动计算优先级：
- 第1个配置：priority = n * 10
- 第2个配置：priority = (n-1) * 10
- 第3个配置：priority = (n-2) * 10
- ...

其中 n 为配置总数，这样可以保证有足够的间隔插入新配置。

### 5.4 前端服务接口

```typescript
// services/ai.ts

export interface AIConfig {
  id: number;
  service_type: string;
  aws_region?: string;
  model_id: string;
  base_url?: string;
  custom_prompt?: string;
  enabled: boolean;
  rate_limit_seconds: number;
  use_inference_profile: boolean;
  capabilities: string[];  // 新增
  priority: number;        // 新增
  created_at: string;
  updated_at: string;
}

export interface PriorityUpdate {
  id: number;
  priority: number;
}

// 批量更新优先级
export const batchUpdatePriorities = async (updates: PriorityUpdate[]) => {
  const response = await api.put('/api/v1/admin/ai-configs/priorities', updates);
  return response.data;
};

// 设置为默认配置
export const setAsDefault = async (id: number) => {
  const response = await api.put(`/api/v1/admin/ai-configs/${id}/set-default`);
  return response.data;
};
```

## 6. 使用场景示例

### 6.1 场景1：错误分析

```go
// 在错误分析接口中
func (s *AIAnalysisService) AnalyzeError(...) {
    // 获取错误分析配置
    cfg, err := s.configService.GetConfigForCapability("error_analysis")
    if err != nil {
        return nil, 0, fmt.Errorf("无法获取 AI 配置: %w", err)
    }
    
    // 使用配置进行分析
    result, err := s.callAI(cfg, prompt)
    // ...
}
```

### 6.2 场景2：变更分析（未来功能）

```go
// 在变更分析接口中
func (s *ChangeAnalysisService) AnalyzeChanges(...) {
    // 获取变更分析配置
    cfg, err := s.configService.GetConfigForCapability("change_analysis")
    if err != nil {
        return nil, fmt.Errorf("无法获取 AI 配置: %w", err)
    }
    
    // 使用配置进行分析
    result, err := s.callAI(cfg, prompt)
    // ...
}
```

## 7. 迁移方案

### 7.1 数据库迁移

```sql
-- scripts/migrate_ai_capabilities.sql

-- 1. 添加新字段
ALTER TABLE ai_configs 
ADD COLUMN IF NOT EXISTS capabilities JSONB DEFAULT '[]',
ADD COLUMN IF NOT EXISTS priority INTEGER DEFAULT 0;

-- 2. 创建索引
CREATE INDEX IF NOT EXISTS idx_ai_configs_priority 
ON ai_configs(priority DESC);

CREATE INDEX IF NOT EXISTS idx_ai_configs_capabilities 
ON ai_configs USING GIN(capabilities);

-- 3. 迁移现有数据
-- 将当前启用的配置设置为默认配置
UPDATE ai_configs 
SET capabilities = '["*"]'
WHERE enabled = true 
AND id = (SELECT id FROM ai_configs WHERE enabled = true ORDER BY id LIMIT 1);

-- 其他配置设置为未配置
UPDATE ai_configs 
SET capabilities = '[]'
WHERE enabled = true 
AND capabilities != '["*"]';

-- 4. 添加注释
COMMENT ON COLUMN ai_configs.capabilities IS '支持的能力场景，["*"]表示默认配置，[]表示未配置';
COMMENT ON COLUMN ai_configs.priority IS '优先级，数值越大优先级越高';
```

### 7.2 向后兼容

1. **现有配置处理**
   - 第一个启用的配置自动设置为默认配置
   - 其他配置设置为未配置状态

2. **API 兼容**
   - 新增字段有默认值，不影响现有 API
   - 现有的 `GetEnabledConfig()` 方法保持兼容

## 8. 测试计划

### 8.1 单元测试

```go
func TestGetConfigForCapability(t *testing.T) {
    // 测试专用配置优先
    // 测试优先级排序
    // 测试默认配置兜底
    // 测试未配置情况
}

func TestSetAsDefault(t *testing.T) {
    // 测试设置默认配置
    // 测试自动取消其他默认配置
}

func TestBatchUpdatePriorities(t *testing.T) {
    // 测试批量更新优先级
    // 测试事务回滚
}
```

### 8.2 集成测试

1. **配置选择测试**
   - 创建多个配置，验证选择逻辑
   - 测试优先级排序
   - 测试默认配置兜底

2. **拖拽排序测试**
   - 测试拖拽后优先级更新
   - 测试批量更新接口

3. **默认配置切换测试**
   - 测试设置新默认配置
   - 验证旧默认配置被取消

### 8.3 前端测试

1. **列表页面测试**
   - 测试配置分区显示
   - 测试拖拽排序功能
   - 测试状态标识显示

2. **表单页面测试**
   - 测试默认配置选择
   - 测试场景多选
   - 测试保存验证

## 9. 实施计划

### 9.1 第一阶段：后端实现（2天）

- [ ] 数据库迁移脚本
- [ ] 模型字段更新
- [ ] 服务方法实现
- [ ] API 接口实现
- [ ] 单元测试

### 9.2 第二阶段：前端实现（3天）

- [ ] 配置列表页面改造
- [ ] 拖拽排序功能
- [ ] 配置表单页面改造
- [ ] 场景选择组件
- [ ] 前端服务接口

### 9.3 第三阶段：集成测试（1天）

- [ ] 端到端测试
- [ ] 性能测试
- [ ] 兼容性测试

### 9.4 第四阶段：文档和发布（1天）

- [ ] 用户文档
- [ ] API 文档
- [ ] 发布说明

## 10. 注意事项

1. **性能优化**
   - 使用 JSONB 索引优化查询
   - 缓存配置选择结果

2. **安全性**
   - 验证优先级更新权限
   - 防止并发更新冲突

3. **用户体验**
   - 拖拽时提供视觉反馈
   - 保存时显示加载状态
   - 错误时提供明确提示

4. **扩展性**
   - 场景标识使用字符串，便于扩展
   - 优先级使用整数，便于插入新配置

## 11. 附录

### 11.1 场景标识规范

- 使用小写字母和下划线
- 格式：`{功能}_{类型}`
- 示例：`error_analysis`, `change_analysis`

### 11.2 优先级规范

- 默认配置：priority = 0
- 专用配置：priority > 0
- 建议间隔：10（便于插入）
- 最大值：2147483647（INT 最大值）

### 11.3 API 响应示例

```json
{
  "code": 200,
  "message": "Success",
  "data": {
    "id": 1,
    "service_type": "bedrock",
    "model_id": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "enabled": true,
    "capabilities": ["*"],
    "priority": 0,
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-01-01T00:00:00Z"
  }
}
