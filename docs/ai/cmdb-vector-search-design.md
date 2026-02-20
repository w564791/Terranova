# CMDB 向量化搜索方案设计文档

## 一、概述

### 1.1 背景

当前 CMDB 支持结构化关键字搜索，但在 AI 场景下存在以下问题：

- 用户输入自然语言千奇百怪，传统关键字搜索无法理解语义
- 英文/中文混合或同义词问题无法覆盖
- 用户可能只提供部分信息（如只说"我要一台 EC2"）

### 1.2 目标

1. 支持自然语言查询，提升 AI 检索能力
2. 保留结构化字段操作，保证操作精确性
3. 复用现有 AI Config 优先级机制，新增 `embedding` 能力场景
4. 使用 PGVector 内嵌 PostgreSQL，无需外部向量数据库
5. 优先精度，使用 3072 维高精度向量

### 1.3 技术选型

| 组件 | 选择 | 理由 |
|------|------|------|
| 向量数据库 | PGVector (PostgreSQL 扩展) | 已有 `postgres-pgvector` 容器（端口 15432），无需额外运维 |
| Embedding 模型 | OpenAI `text-embedding-3-large` | 3072 维，精度最高 |
| 向量维度 | 3072 | 用户要求高精度 |
| 索引类型 | 精确搜索 / IVFFlat | **注意：HNSW 索引不支持超过 2000 维**，对于小数据集使用精确搜索，大数据集使用 IVFFlat |

---

## 二、系统架构

### 2.1 整体流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    CMDB 向量化搜索完整流程                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  【数据入库阶段】（同步，快速）                                               │
│                                                                             │
│  State 文件 ──→ 解析资源 ──→ 写入 resource_index 表                         │
│                              （embedding 字段为 NULL）                       │
│                                    │                                        │
│                                    ▼                                        │
│                           写入 embedding_tasks 队列表                        │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  【Embedding 生成阶段】（异步，后台处理）                                     │
│                                                                             │
│  后台 Worker ──→ 从队列取任务 ──→ 批量调用 API ──→ 更新 embedding            │
│       │              │                │                                     │
│       │              │                ▼                                     │
│       │              │         OpenAI Batch API                             │
│       │              │         (一次最多 100 条)                             │
│       │              │                                                      │
│       │              ▼                                                      │
│       │         控制并发和速率                                               │
│       │         - 每批 100 条                                               │
│       │         - 批次间隔 2 秒                                             │
│       │         - 失败重试 3 次                                             │
│       │                                                                     │
│       ▼                                                                     │
│  定时任务（每分钟检查一次）                                                   │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  【用户查询阶段】                                                            │
│                                                                             │
│  用户输入: "我要在 exchange vpc 的东京1a区域创建 ec2"                        │
│       │                                                                     │
│       ▼                                                                     │
│  步骤 1: 意图断言                                                           │
│  GetConfigForCapability("intent_assertion") → 检查安全性                    │
│       │                                                                     │
│       ▼                                                                     │
│  步骤 2: CMDB 查询计划生成                                                  │
│  GetConfigForCapability("cmdb_query_plan") → 解析资源需求                   │
│  输出: [{"type": "aws_vpc", "keyword": "exchange vpc"}, ...]               │
│       │                                                                     │
│       ▼                                                                     │
│  步骤 3: 向量搜索                                                           │
│  GetConfigForCapability("embedding") → 生成查询向量                         │
│       │                                                                     │
│       ▼                                                                     │
│  步骤 4: PGVector 混合检索                                                  │
│  ├─ 先尝试精确匹配（cloud_resource_id / cloud_resource_name）              │
│  └─ 无精确匹配时，使用向量相似度搜索 Top-N                                  │
│       │                                                                     │
│       ▼                                                                     │
│  步骤 5: 用户选择候选                                                       │
│       │                                                                     │
│       ▼                                                                     │
│  步骤 6: 配置生成                                                           │
│  GetConfigForCapability("form_generation") → 生成最终配置                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 AI Config 能力场景

| 能力场景 | 用途 | 推荐模型 |
|---------|------|---------|
| `intent_assertion` | 意图断言（安全检查） | Claude Haiku |
| `cmdb_query_plan` | CMDB 查询计划生成 | Claude Sonnet |
| `form_generation` | 表单配置生成 | Claude Opus |
| **`embedding`** | **向量生成（新增）** | **OpenAI text-embedding-3-large** |

### 2.3 AI Config 优先级机制

所有 AI 调用必须遵循现有的 `GetConfigForCapability` 优先级机制：

```go
// GetConfigForCapability 的选择逻辑：
// 1. 查找专用配置（enabled=false，capabilities 包含指定能力）
// 2. 按 priority 降序、id 升序排序
// 3. 如果没找到专用配置，使用默认配置（enabled=true）
```

**优先级规则**：
1. `enabled = false` 的专用配置优先
2. 在专用配置中，`priority` 值越大优先级越高
3. 相同 `priority` 时，`id` 越小优先级越高
4. 如果没有专用配置，使用 `enabled = true` 的默认配置

---

## 三、数据库设计

### 3.1 resource_index 表修改

```sql
-- scripts/add_embedding_columns.sql

-- 1. 添加 embedding 相关列
ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding VECTOR(3072);  -- 3072 维向量（OpenAI large）

ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding_text TEXT;  -- 用于生成 embedding 的原始文本

ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding_model VARCHAR(100);  -- 使用的 embedding 模型

ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding_updated_at TIMESTAMP;  -- embedding 更新时间

-- 2. 创建 HNSW 向量索引
CREATE INDEX IF NOT EXISTS idx_resource_embedding ON resource_index 
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- 3. 创建部分索引（只索引有 embedding 的记录）
CREATE INDEX IF NOT EXISTS idx_resource_has_embedding ON resource_index (id)
WHERE embedding IS NOT NULL;
```

### 3.2 embedding_tasks 队列表

```sql
-- scripts/create_embedding_tasks_table.sql

-- 创建 embedding 任务队列表
CREATE TABLE IF NOT EXISTS embedding_tasks (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL REFERENCES resource_index(id) ON DELETE CASCADE,
    workspace_id VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, processing, completed, failed
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    
    CONSTRAINT uk_embedding_task_resource UNIQUE (resource_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_embedding_tasks_status ON embedding_tasks(status);
CREATE INDEX IF NOT EXISTS idx_embedding_tasks_workspace ON embedding_tasks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_embedding_tasks_created ON embedding_tasks(created_at);
```

### 3.3 AI Config 新增 embedding 配置

```sql
-- scripts/add_embedding_ai_config.sql

-- OpenAI text-embedding-3-large (3072 维，高精度)
INSERT INTO ai_configs (
    name,
    service_type, 
    model_id,
    api_key,
    api_base_url,
    enabled, 
    capabilities, 
    priority,
    created_at,
    updated_at
) VALUES (
    'OpenAI Embedding Large (3072维)',
    'openai',
    'text-embedding-3-large',
    '',  -- 用户需要在 AI 配置管理界面填写 API Key
    'https://api.openai.com/v1',
    false,  -- 专用配置
    '["embedding"]',
    20,
    NOW(),
    NOW()
) ON CONFLICT DO NOTHING;

-- 备选：OpenAI text-embedding-3-small (1536 维，成本更低)
INSERT INTO ai_configs (
    name,
    service_type, 
    model_id,
    api_key,
    api_base_url,
    enabled, 
    capabilities, 
    priority,
    created_at,
    updated_at
) VALUES (
    'OpenAI Embedding Small (1536维)',
    'openai',
    'text-embedding-3-small',
    '',
    'https://api.openai.com/v1',
    false,
    '["embedding"]',
    10,  -- 优先级较低，作为备选
    NOW(),
    NOW()
) ON CONFLICT DO NOTHING;
```

---

## 四、核心代码设计

### 4.1 EmbeddingService

```go
// backend/services/embedding_service.go

package services

import (
    "context"
    "fmt"
    "log"
    "strings"
    
    "github.com/sashabaranov/go-openai"
    "gorm.io/gorm"
    "iac-platform/internal/models"
)

// EmbeddingService embedding 服务
type EmbeddingService struct {
    db            *gorm.DB
    configService *AIConfigService
}

// NewEmbeddingService 创建 embedding 服务实例
func NewEmbeddingService(db *gorm.DB) *EmbeddingService {
    return &EmbeddingService{
        db:            db,
        configService: NewAIConfigService(db),
    }
}

// GenerateEmbedding 生成文本的 embedding 向量
// 遵循 AI Config 优先级机制
func (s *EmbeddingService) GenerateEmbedding(text string) ([]float32, error) {
    // 1. 获取 embedding 配置（遵循优先级）
    aiConfig, err := s.configService.GetConfigForCapability("embedding")
    if err != nil {
        return nil, fmt.Errorf("未找到 embedding 的 AI 配置: %w", err)
    }
    
    log.Printf("[EmbeddingService] 使用配置: ID=%d, Model=%s, Priority=%d",
        aiConfig.ID, aiConfig.ModelID, aiConfig.Priority)
    
    // 2. 根据 service_type 调用对应的 API
    switch aiConfig.ServiceType {
    case "openai":
        return s.callOpenAIEmbedding(aiConfig, text)
    case "bedrock":
        return s.callBedrockEmbedding(aiConfig, text)
    default:
        return nil, fmt.Errorf("不支持的服务类型: %s", aiConfig.ServiceType)
    }
}

// GenerateEmbeddingsBatch 批量生成 embedding
func (s *EmbeddingService) GenerateEmbeddingsBatch(texts []string) ([][]float32, error) {
    aiConfig, err := s.configService.GetConfigForCapability("embedding")
    if err != nil {
        return nil, err
    }
    
    switch aiConfig.ServiceType {
    case "openai":
        return s.callOpenAIEmbeddingBatch(aiConfig, texts)
    default:
        // 不支持批量的服务，逐个调用
        results := make([][]float32, len(texts))
        for i, text := range texts {
            embedding, err := s.GenerateEmbedding(text)
            if err != nil {
                return nil, err
            }
            results[i] = embedding
        }
        return results, nil
    }
}

// callOpenAIEmbedding 调用 OpenAI embedding API
func (s *EmbeddingService) callOpenAIEmbedding(config *models.AIConfig, text string) ([]float32, error) {
    client := s.createOpenAIClient(config)
    
    resp, err := client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
        Model: openai.EmbeddingModel(config.ModelID),
        Input: []string{text},
    })
    if err != nil {
        return nil, fmt.Errorf("OpenAI embedding 调用失败: %w", err)
    }
    
    return resp.Data[0].Embedding, nil
}

// callOpenAIEmbeddingBatch 批量调用 OpenAI embedding API
func (s *EmbeddingService) callOpenAIEmbeddingBatch(config *models.AIConfig, texts []string) ([][]float32, error) {
    client := s.createOpenAIClient(config)
    
    // OpenAI 支持一次请求多个文本
    resp, err := client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
        Model: openai.EmbeddingModel(config.ModelID),
        Input: texts,
    })
    if err != nil {
        return nil, fmt.Errorf("OpenAI embedding 批量调用失败: %w", err)
    }
    
    results := make([][]float32, len(texts))
    for i, data := range resp.Data {
        results[i] = data.Embedding
    }
    
    return results, nil
}

// createOpenAIClient 创建 OpenAI 客户端
func (s *EmbeddingService) createOpenAIClient(config *models.AIConfig) *openai.Client {
    clientConfig := openai.DefaultConfig(config.APIKey)
    
    // 如果有自定义 base URL
    if config.APIBaseURL != "" {
        clientConfig.BaseURL = config.APIBaseURL
    }
    
    return openai.NewClientWithConfig(clientConfig)
}

// BuildEmbeddingText 构建用于生成 embedding 的文本
func (s *EmbeddingService) BuildEmbeddingText(r *models.ResourceIndex) string {
    parts := []string{}
    
    // 资源名称
    if r.CloudResourceName != "" {
        parts = append(parts, r.CloudResourceName)
    }
    
    // 描述
    if r.Description != "" {
        parts = append(parts, r.Description)
    }
    
    // 重要的 Tags
    if r.Tags != nil {
        if name, ok := r.Tags["Name"].(string); ok && name != "" {
            parts = append(parts, name)
        }
        if env, ok := r.Tags["Environment"].(string); ok && env != "" {
            parts = append(parts, env)
        }
        if team, ok := r.Tags["Team"].(string); ok && team != "" {
            parts = append(parts, team)
        }
    }
    
    // 资源类型的可读名称
    parts = append(parts, getResourceTypeDisplayName(r.ResourceType))
    
    // 区域信息
    if r.CloudRegion != "" {
        parts = append(parts, r.CloudRegion)
    }
    
    return strings.Join(parts, " ")
}

// getResourceTypeDisplayName 获取资源类型的可读名称
func getResourceTypeDisplayName(resourceType string) string {
    displayNames := map[string]string{
        "aws_vpc":            "VPC 虚拟私有云",
        "aws_subnet":         "子网 Subnet",
        "aws_security_group": "安全组 Security Group",
        "aws_instance":       "EC2 实例",
        "aws_s3_bucket":      "S3 存储桶",
        "aws_iam_role":       "IAM 角色",
        "aws_iam_policy":     "IAM 策略",
        "aws_db_instance":    "RDS 数据库实例",
        "aws_eks_cluster":    "EKS 集群",
        "aws_lambda_function": "Lambda 函数",
    }
    
    if name, ok := displayNames[resourceType]; ok {
        return name
    }
    return resourceType
}
```

### 4.2 EmbeddingWorker 后台处理（数据库队列 + 守护协程）

#### 4.2.1 设计原则

1. **数据库队列**：使用 `embedding_tasks` 表作为任务队列
2. **守护协程**：应用启动时启动一个 goroutine 守护队列
3. **过期清理**：超过 3 天的队列数据直接忽略/清理
4. **启动检查**：应用启动时检查队列，恢复未完成的任务
5. **支持全量同步**：支持 Sync All Workspace 功能

```go
// backend/services/embedding_worker.go

package services

import (
    "context"
    "log"
    "sync"
    "time"
    
    "gorm.io/gorm"
    "iac-platform/internal/models"
)

const (
    // TaskExpireDays 任务过期天数，超过此天数的任务将被忽略
    TaskExpireDays = 3
)

// EmbeddingWorker embedding 后台处理 worker（守护协程）
type EmbeddingWorker struct {
    db               *gorm.DB
    embeddingService *EmbeddingService
    batchSize        int           // 每批处理数量
    batchInterval    time.Duration // 批次间隔
    maxRetries       int           // 最大重试次数
    running          bool
    mu               sync.Mutex
    ctx              context.Context
    cancel           context.CancelFunc
}

// NewEmbeddingWorker 创建 embedding worker 实例
func NewEmbeddingWorker(db *gorm.DB) *EmbeddingWorker {
    return &EmbeddingWorker{
        db:               db,
        embeddingService: NewEmbeddingService(db),
        batchSize:        100,              // 每批 100 条
        batchInterval:    time.Second * 2,  // 批次间隔 2 秒
        maxRetries:       3,
    }
}

// Start 启动 Worker（守护协程）
// 应用启动时调用此方法，启动一个 goroutine 守护队列
func (w *EmbeddingWorker) Start(ctx context.Context) {
    w.mu.Lock()
    if w.running {
        w.mu.Unlock()
        log.Println("[EmbeddingWorker] 已经在运行中")
        return
    }
    w.running = true
    w.ctx, w.cancel = context.WithCancel(ctx)
    w.mu.Unlock()
    
    log.Println("[EmbeddingWorker] ========== 启动守护协程 ==========")
    
    // 1. 启动时清理过期任务
    w.cleanupExpiredTasks()
    
    // 2. 启动时恢复 processing 状态的任务（可能是上次异常退出）
    w.recoverProcessingTasks()
    
    // 3. 启动时立即处理一次待处理任务
    w.processPendingTasks()
    
    // 4. 启动定时器，每分钟检查一次
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    
    // 5. 启动每日清理定时器
    cleanupTicker := time.NewTicker(24 * time.Hour)
    defer cleanupTicker.Stop()
    
    for {
        select {
        case <-w.ctx.Done():
            log.Println("[EmbeddingWorker] ========== 停止守护协程 ==========")
            w.mu.Lock()
            w.running = false
            w.mu.Unlock()
            return
        case <-ticker.C:
            w.processPendingTasks()
        case <-cleanupTicker.C:
            w.cleanupExpiredTasks()
        }
    }
}

// Stop 停止 Worker
func (w *EmbeddingWorker) Stop() {
    w.mu.Lock()
    defer w.mu.Unlock()
    
    if w.cancel != nil {
        w.cancel()
    }
}

// cleanupExpiredTasks 清理过期任务（超过 3 天）
func (w *EmbeddingWorker) cleanupExpiredTasks() {
    expireTime := time.Now().AddDate(0, 0, -TaskExpireDays)
    
    // 删除超过 3 天的 pending 和 processing 任务
    result := w.db.Where("created_at < ? AND status IN ?", expireTime, []string{"pending", "processing"}).
        Delete(&models.EmbeddingTask{})
    
    if result.RowsAffected > 0 {
        log.Printf("[EmbeddingWorker] 清理 %d 个过期任务（超过 %d 天）", result.RowsAffected, TaskExpireDays)
    }
    
    // 清理已完成超过 7 天的任务记录（可选，保持表干净）
    completedExpireTime := time.Now().AddDate(0, 0, -7)
    w.db.Where("completed_at < ? AND status = ?", completedExpireTime, "completed").
        Delete(&models.EmbeddingTask{})
}

// recoverProcessingTasks 恢复 processing 状态的任务
// 应用异常退出时，可能有任务处于 processing 状态，需要恢复为 pending
func (w *EmbeddingWorker) recoverProcessingTasks() {
    result := w.db.Model(&models.EmbeddingTask{}).
        Where("status = ?", "processing").
        Updates(map[string]interface{}{
            "status":     "pending",
            "updated_at": time.Now(),
        })
    
    if result.RowsAffected > 0 {
        log.Printf("[EmbeddingWorker] 恢复 %d 个 processing 状态的任务", result.RowsAffected)
    }
}

// processPendingTasks 处理待处理的任务
func (w *EmbeddingWorker) processPendingTasks() {
    expireTime := time.Now().AddDate(0, 0, -TaskExpireDays)
    
    for {
        // 获取一批待处理的任务（排除过期任务）
        var tasks []models.EmbeddingTask
        w.db.Where("status = ? AND retry_count < ? AND created_at > ?", "pending", w.maxRetries, expireTime).
            Order("created_at ASC").
            Limit(w.batchSize).
            Find(&tasks)
        
        if len(tasks) == 0 {
            break  // 没有更多任务
        }
        
        log.Printf("[EmbeddingWorker] 处理 %d 个任务", len(tasks))
        
        // 标记为处理中
        taskIDs := make([]uint, len(tasks))
        resourceIDs := make([]uint, len(tasks))
        for i, t := range tasks {
            taskIDs[i] = t.ID
            resourceIDs[i] = t.ResourceID
        }
        w.db.Model(&models.EmbeddingTask{}).
            Where("id IN ?", taskIDs).
            Update("status", "processing")
        
        // 批量生成 embedding
        w.processBatch(resourceIDs, taskIDs)
        
        // 批次间隔，避免 API 限流
        time.Sleep(w.batchInterval)
        
        // 检查是否需要停止
        select {
        case <-w.ctx.Done():
            return
        default:
        }
    }
}

// processBatch 处理一批任务
func (w *EmbeddingWorker) processBatch(resourceIDs []uint, taskIDs []uint) {
    // 获取资源信息
    var resources []models.ResourceIndex
    w.db.Where("id IN ?", resourceIDs).Find(&resources)
    
    if len(resources) == 0 {
        // 资源已被删除，直接删除任务
        w.db.Where("id IN ?", taskIDs).Delete(&models.EmbeddingTask{})
        return
    }
    
    // 构建 embedding 文本列表
    texts := make([]string, len(resources))
    for i, r := range resources {
        if r.EmbeddingText != "" {
            texts[i] = r.EmbeddingText
        } else {
            texts[i] = w.embeddingService.BuildEmbeddingText(&r)
        }
    }
    
    // 批量调用 embedding API
    embeddings, err := w.embeddingService.GenerateEmbeddingsBatch(texts)
    if err != nil {
        log.Printf("[EmbeddingWorker] 批量生成失败: %v", err)
        // 标记为失败，增加重试计数
        w.db.Model(&models.EmbeddingTask{}).
            Where("id IN ?", taskIDs).
            Updates(map[string]interface{}{
                "status":        "pending",
                "retry_count":   gorm.Expr("retry_count + 1"),
                "error_message": err.Error(),
                "updated_at":    time.Now(),
            })
        return
    }
    
    // 获取当前使用的模型
    config, _ := w.embeddingService.configService.GetConfigForCapability("embedding")
    modelID := ""
    if config != nil {
        modelID = config.ModelID
    }
    
    // 更新资源的 embedding
    now := time.Now()
    for i, r := range resources {
        w.db.Model(&r).Updates(map[string]interface{}{
            "embedding":            embeddings[i],
            "embedding_text":       texts[i],
            "embedding_model":      modelID,
            "embedding_updated_at": now,
        })
    }
    
    // 标记任务完成
    w.db.Model(&models.EmbeddingTask{}).
        Where("id IN ?", taskIDs).
        Updates(map[string]interface{}{
            "status":       "completed",
            "completed_at": now,
            "updated_at":   now,
        })
    
    log.Printf("[EmbeddingWorker] 完成 %d 个资源的 embedding 生成", len(resources))
}

// SyncAllWorkspaces 同步所有 Workspace 的 embedding（全量同步）
// 对应现有的 Sync All Workspace 功能
func (w *EmbeddingWorker) SyncAllWorkspaces() error {
    log.Println("[EmbeddingWorker] ========== 开始全量同步 ==========")
    
    // 1. 获取所有没有 embedding 的资源
    var resources []models.ResourceIndex
    w.db.Where("embedding IS NULL").Find(&resources)
    
    log.Printf("[EmbeddingWorker] 需要生成 embedding 的资源数: %d", len(resources))
    
    if len(resources) == 0 {
        log.Println("[EmbeddingWorker] 没有需要同步的资源")
        return nil
    }
    
    // 2. 批量创建 embedding 任务
    tasks := make([]models.EmbeddingTask, 0, len(resources))
    for _, r := range resources {
        tasks = append(tasks, models.EmbeddingTask{
            ResourceID:  r.ID,
            WorkspaceID: r.WorkspaceID,
            Status:      "pending",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        })
    }
    
    // 批量插入，忽略重复
    result := w.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&tasks, 1000)
    
    log.Printf("[EmbeddingWorker] 创建 %d 个 embedding 任务", result.RowsAffected)
    log.Println("[EmbeddingWorker] ========== 全量同步任务创建完成 ==========")
    
    return nil
}

// SyncWorkspace 同步指定 Workspace 的 embedding
func (w *EmbeddingWorker) SyncWorkspace(workspaceID string) error {
    log.Printf("[EmbeddingWorker] 同步 Workspace: %s", workspaceID)
    
    // 获取该 Workspace 没有 embedding 的资源
    var resources []models.ResourceIndex
    w.db.Where("workspace_id = ? AND embedding IS NULL", workspaceID).Find(&resources)
    
    if len(resources) == 0 {
        log.Printf("[EmbeddingWorker] Workspace %s 没有需要同步的资源", workspaceID)
        return nil
    }
    
    // 批量创建 embedding 任务
    tasks := make([]models.EmbeddingTask, 0, len(resources))
    for _, r := range resources {
        tasks = append(tasks, models.EmbeddingTask{
            ResourceID:  r.ID,
            WorkspaceID: r.WorkspaceID,
            Status:      "pending",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        })
    }
    
    w.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&tasks, 1000)
    
    log.Printf("[EmbeddingWorker] Workspace %s 创建 %d 个 embedding 任务", workspaceID, len(tasks))
    
    return nil
}

// RebuildWorkspace 重建指定 Workspace 的所有 embedding
func (w *EmbeddingWorker) RebuildWorkspace(workspaceID string) error {
    log.Printf("[EmbeddingWorker] 重建 Workspace: %s", workspaceID)
    
    // 1. 清空该 Workspace 的所有 embedding
    w.db.Model(&models.ResourceIndex{}).
        Where("workspace_id = ?", workspaceID).
        Updates(map[string]interface{}{
            "embedding":            nil,
            "embedding_updated_at": nil,
        })
    
    // 2. 删除该 Workspace 的所有 pending 任务
    w.db.Where("workspace_id = ? AND status = ?", workspaceID, "pending").
        Delete(&models.EmbeddingTask{})
    
    // 3. 重新创建任务
    return w.SyncWorkspace(workspaceID)
}

// GetStatus 获取 worker 状态
func (w *EmbeddingWorker) GetStatus() map[string]interface{} {
    var pendingCount, processingCount, completedCount, failedCount, expiredCount int64
    
    expireTime := time.Now().AddDate(0, 0, -TaskExpireDays)
    
    w.db.Model(&models.EmbeddingTask{}).
        Where("status = ? AND created_at > ?", "pending", expireTime).
        Count(&pendingCount)
    w.db.Model(&models.EmbeddingTask{}).
        Where("status = ?", "processing").
        Count(&processingCount)
    w.db.Model(&models.EmbeddingTask{}).
        Where("status = ?", "completed").
        Count(&completedCount)
    w.db.Model(&models.EmbeddingTask{}).
        Where("status = ? AND retry_count >= ?", "pending", w.maxRetries).
        Count(&failedCount)
    w.db.Model(&models.EmbeddingTask{}).
        Where("created_at < ? AND status IN ?", expireTime, []string{"pending", "processing"}).
        Count(&expiredCount)
    
    return map[string]interface{}{
        "running":          w.running,
        "pending_tasks":    pendingCount,
        "processing_tasks": processingCount,
        "completed_tasks":  completedCount,
        "failed_tasks":     failedCount,
        "expired_tasks":    expiredCount,
        "expire_days":      TaskExpireDays,
    }
}
```

#### 4.2.2 应用启动时初始化

```go
// backend/main.go

func main() {
    // ... 初始化数据库等 ...
    
    // 创建 EmbeddingWorker
    embeddingWorker := services.NewEmbeddingWorker(db)
    
    // 启动守护协程（后台运行）
    ctx, cancel := context.WithCancel(context.Background())
    go embeddingWorker.Start(ctx)
    
    // 优雅关闭
    defer func() {
        cancel()
        embeddingWorker.Stop()
    }()
    
    // ... 启动 HTTP 服务器等 ...
}
```

#### 4.2.3 Sync All Workspace API

```go
// backend/controllers/embedding_controller.go

// SyncAllWorkspaces 同步所有 Workspace 的 embedding
// POST /api/admin/embedding/sync-all
func (c *EmbeddingController) SyncAllWorkspaces(ctx *gin.Context) {
    err := c.worker.SyncAllWorkspaces()
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "code":    500,
            "message": err.Error(),
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code":    200,
        "message": "全量同步任务已创建，后台处理中",
        "data":    c.worker.GetStatus(),
    })
}

// SyncWorkspace 同步指定 Workspace 的 embedding
// POST /api/workspaces/:workspace_id/embedding/sync
func (c *EmbeddingController) SyncWorkspace(ctx *gin.Context) {
    workspaceID := ctx.Param("workspace_id")
    
    err := c.worker.SyncWorkspace(workspaceID)
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "code":    500,
            "message": err.Error(),
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code":    200,
        "message": "同步任务已创建，后台处理中",
    })
}

// RebuildWorkspace 重建指定 Workspace 的 embedding
// POST /api/workspaces/:workspace_id/embedding/rebuild
func (c *EmbeddingController) RebuildWorkspace(ctx *gin.Context) {
    workspaceID := ctx.Param("workspace_id")
    
    err := c.worker.RebuildWorkspace(workspaceID)
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "code":    500,
            "message": err.Error(),
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code":    200,
        "message": "重建任务已创建，后台处理中",
    })
}
```

### 4.3 修改 CMDB 搜索逻辑

```go
// backend/services/ai_cmdb_service.go (修改部分)

// executeQuery 执行单个 CMDB 查询（修改版，支持向量搜索）
func (s *AICMDBService) executeQuery(query CMDBQuery, filters map[string]string, workspaceIDs []string) *CMDBQueryResult {
    result := &CMDBQueryResult{Query: query, Found: false}
    
    // 1. 先尝试精确匹配
    var exactMatch []models.ResourceIndex
    s.db.Where("resource_type = ?", query.Type).
        Where("workspace_id IN ?", workspaceIDs).
        Where("cloud_resource_id = ? OR cloud_resource_name = ?", query.Keyword, query.Keyword).
        Limit(1).Find(&exactMatch)
    
    if len(exactMatch) > 0 {
        log.Printf("[AICMDBService] 精确匹配成功: %s", query.Keyword)
        result.Found = true
        result.Resource = s.convertToResourceInfo(&exactMatch[0])
        return result
    }
    
    // 2. 尝试向量搜索（如果 embedding 服务可用）
    if s.embeddingService != nil {
        vectorResults, err := s.vectorSearch(query, filters, workspaceIDs)
        if err == nil && len(vectorResults) > 0 {
            log.Printf("[AICMDBService] 向量搜索成功: %d 个结果", len(vectorResults))
            result.Found = true
            if len(vectorResults) == 1 {
                result.Resource = &vectorResults[0]
            } else {
                result.Candidates = vectorResults
            }
            return result
        }
        if err != nil {
            log.Printf("[AICMDBService] 向量搜索失败，降级到关键词搜索: %v", err)
        }
    }
    
    // 3. 降级到关键词搜索
    log.Printf("[AICMDBService] 使用关键词搜索: %s", query.Keyword)
    return s.keywordSearch(query, filters, workspaceIDs)
}

// vectorSearch 向量搜索
func (s *AICMDBService) vectorSearch(query CMDBQuery, filters map[string]string, workspaceIDs []string) ([]CMDBResourceInfo, error) {
    // 生成查询向量
    queryVector, err := s.embeddingService.GenerateEmbedding(query.Keyword)
    if err != nil {
        return nil, fmt.Errorf("生成查询向量失败: %w", err)
    }
    
    // 构建 SQL（使用 pgvector 的余弦距离）
    sql := `
        SELECT * FROM resource_index
        WHERE resource_type = ?
          AND workspace_id IN ?
          AND embedding IS NOT NULL
        ORDER BY embedding <#> ?
        LIMIT 10
    `
    
    // 将向量转换为 pgvector 格式
    vectorStr := vectorToString(queryVector)
    
    var resources []models.ResourceIndex
    if err := s.db.Raw(sql, query.Type, workspaceIDs, vectorStr).Scan(&resources).Error; err != nil {
        return nil, fmt.Errorf("向量搜索查询失败: %w", err)
    }
    
    // 转换结果
    results := make([]CMDBResourceInfo, 0, len(resources))
    for _, r := range resources {
        results = append(results, *s.convertToResourceInfo(&r))
    }
    
    return results, nil
}

// vectorToString 将向量转换为 pgvector 格式的字符串
func vectorToString(v []float32) string {
    parts := make([]string, len(v))
    for i, f := range v {
        parts[i] = fmt.Sprintf("%f", f)
    }
    return "[" + strings.Join(parts, ",") + "]"
}

// keywordSearch 关键词搜索（现有逻辑，作为降级方案）
func (s *AICMDBService) keywordSearch(query CMDBQuery, filters map[string]string, workspaceIDs []string) *CMDBQueryResult {
    // ... 现有的关键词搜索逻辑 ...
}
```

### 4.4 Embedding 进度查询 API

```go
// backend/controllers/embedding_controller.go

package controllers

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
    "iac-platform/internal/models"
    "iac-platform/services"
)

// EmbeddingController embedding 控制器
type EmbeddingController struct {
    db     *gorm.DB
    worker *services.EmbeddingWorker
}

// NewEmbeddingController 创建 embedding 控制器
func NewEmbeddingController(db *gorm.DB, worker *services.EmbeddingWorker) *EmbeddingController {
    return &EmbeddingController{
        db:     db,
        worker: worker,
    }
}

// GetWorkspaceEmbeddingStatus 获取 Workspace 的 embedding 状态
// GET /api/workspaces/:workspace_id/embedding-status
func (c *EmbeddingController) GetWorkspaceEmbeddingStatus(ctx *gin.Context) {
    workspaceID := ctx.Param("workspace_id")
    
    // 统计资源总数
    var totalResources int64
    c.db.Model(&models.ResourceIndex{}).
        Where("workspace_id = ?", workspaceID).
        Count(&totalResources)
    
    // 统计有 embedding 的资源数
    var withEmbedding int64
    c.db.Model(&models.ResourceIndex{}).
        Where("workspace_id = ? AND embedding IS NOT NULL", workspaceID).
        Count(&withEmbedding)
    
    // 统计任务状态
    var pendingTasks, processingTasks, failedTasks int64
    c.db.Model(&models.EmbeddingTask{}).
        Where("workspace_id = ? AND status = ?", workspaceID, "pending").
        Count(&pendingTasks)
    c.db.Model(&models.EmbeddingTask{}).
        Where("workspace_id = ? AND status = ?", workspaceID, "processing").
        Count(&processingTasks)
    c.db.Model(&models.EmbeddingTask{}).
        Where("workspace_id = ? AND status = ? AND retry_count >= 3", workspaceID, "pending").
        Count(&failedTasks)
    
    // 计算进度
    var progress float64
    if totalResources > 0 {
        progress = float64(withEmbedding) / float64(totalResources) * 100
    }
    
    // 预估剩余时间（假设每批 100 条，每批 2 秒）
    remainingTasks := pendingTasks + processingTasks
    estimatedSeconds := (remainingTasks / 100) * 2
    estimatedTime := formatDuration(estimatedSeconds)
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 200,
        "data": gin.H{
            "workspace_id":      workspaceID,
            "total_resources":   totalResources,
            "with_embedding":    withEmbedding,
            "pending_tasks":     pendingTasks,
            "processing_tasks":  processingTasks,
            "failed_tasks":      failedTasks,
            "progress":          progress,
            "estimated_time":    estimatedTime,
        },
    })
}

// formatDuration 格式化时间
func formatDuration(seconds int64) string {
    if seconds < 60 {
        return fmt.Sprintf("%d 秒", seconds)
    } else if seconds < 3600 {
        return fmt.Sprintf("%d 分钟", seconds/60)
    } else {
        return fmt.Sprintf("%d 小时 %d 分钟", seconds/3600, (seconds%3600)/60)
    }
}
```

---

## 五、数据变更策略

### 5.1 增量更新（日常运行）

资源变更时，比较 `embedding_text`，有变化才重新生成 embedding：

```go
// SyncResourceIndex 同步资源索引
func (s *CMDBService) SyncResourceIndex(workspaceID string, resources []ResourceInfo) error {
    var newResourceIDs []uint
    
    for _, r := range resources {
        embeddingText := s.embeddingService.BuildEmbeddingText(&r)
        
        var existing models.ResourceIndex
        err := s.db.Where("workspace_id = ? AND terraform_address = ?", 
            workspaceID, r.TerraformAddress).First(&existing).Error
        
        if err == gorm.ErrRecordNotFound {
            // 新资源：创建记录
            newResource := &models.ResourceIndex{
                WorkspaceID:       workspaceID,
                TerraformAddress:  r.TerraformAddress,
                CloudResourceID:   r.CloudResourceID,
                CloudResourceName: r.CloudResourceName,
                EmbeddingText:     embeddingText,
                // Embedding 为 NULL，等待异步生成
            }
            s.db.Create(newResource)
            newResourceIDs = append(newResourceIDs, newResource.ID)
            
        } else if err == nil {
            // 现有资源：检查是否需要更新
            if existing.EmbeddingText != embeddingText {
                s.db.Model(&existing).Updates(map[string]interface{}{
                    "embedding_text": embeddingText,
                    "embedding":      nil,  // 清空旧的 embedding
                })
                newResourceIDs = append(newResourceIDs, existing.ID)
            }
        }
    }
    
    // 批量创建 embedding 任务
    if len(newResourceIDs) > 0 {
        s.createEmbeddingTasks(workspaceID, newResourceIDs)
    }
    
    return nil
}
```

### 5.2 全量初始化（首次部署）

```go
// backend/cmd/generate_embeddings/main.go

func main() {
    // 查找所有没有 embedding 的资源
    var resources []models.ResourceIndex
    db.Where("embedding IS NULL").Find(&resources)
    
    log.Printf("需要生成 embedding 的资源数: %d", len(resources))
    
    // 批量创建任务
    for _, r := range resources {
        task := models.EmbeddingTask{
            ResourceID:  r.ID,
            WorkspaceID: r.WorkspaceID,
            Status:      "pending",
        }
        db.Clauses(clause.OnConflict{DoNothing: true}).Create(&task)
    }
    
    log.Println("任务创建完成，等待 Worker 处理")
}
```

### 5.3 Workspace 级别重建（特殊情况）

```go
// RebuildWorkspaceEmbeddings 重建指定 Workspace 的所有 embedding
func (s *CMDBService) RebuildWorkspaceEmbeddings(workspaceID string) error {
    // 清空该 Workspace 的所有 embedding
    s.db.Model(&models.ResourceIndex{}).
        Where("workspace_id = ?", workspaceID).
        Updates(map[string]interface{}{
            "embedding":            nil,
            "embedding_updated_at": nil,
        })
    
    // 获取该 Workspace 的所有资源 ID
    var resourceIDs []uint
    s.db.Model(&models.ResourceIndex{}).
        Where("workspace_id = ?", workspaceID).
        Pluck("id", &resourceIDs)
    
    // 批量创建 embedding 任务
    s.createEmbeddingTasks(workspaceID, resourceIDs)
    
    return nil
}
```

---

## 六、大规模数据处理

### 6.1 问题分析

50M+ 的 State 文件可能包含数千甚至上万条资源：

| State 大小 | 预估资源数 | Embedding 生成时间 | API 成本 |
|-----------|-----------|-------------------|----------|
| 10M | ~1,000 条 | ~5 分钟 | ~$0.01 |
| 50M | ~5,000 条 | ~25 分钟 | ~$0.05 |
| 100M | ~10,000 条 | ~50 分钟 | ~$0.10 |

### 6.2 解决方案

1. **State 解析是同步的**：快速完成，不阻塞用户
2. **Embedding 生成是异步的**：后台 Worker 慢慢处理
3. **搜索时优雅降级**：没有 embedding 时使用关键词搜索

### 6.3 用户体验

前端显示进度条：

```
┌─────────────────────────────────────────────────────────────────┐
│  Embedding 生成进度                                              │
│                                                                 │
│  ████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  35%                 │
│                                                                 │
│  已完成: 1,750 / 5,000                                          │
│  预估剩余时间: 约 15 分钟                                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 七、成本估算

### 7.1 OpenAI text-embedding-3-large 价格

- **价格**：$0.13 / 1M tokens
- **每条资源**：约 50-100 tokens

### 7.2 成本计算

| 场景 | 资源数 | Token 数 | 成本 |
|------|--------|----------|------|
| 初始化（10,000 条） | 10,000 | ~750K | ~$0.10 |
| 每日增量（100 条） | 100 | ~7.5K | ~$0.001 |
| 用户查询（每次） | 1 | ~50 | ~$0.0000065 |

**结论**：成本极低，可忽略。

---

## 八、实施计划

### 8.1 待做事项

| # | 任务 | 文件 | 工作量 |
|---|------|------|--------|
| 1 | 数据库迁移：添加 embedding 列和索引 | `scripts/add_embedding_columns.sql` | 0.5 天 |
| 2 | 创建 embedding_tasks 队列表 | `scripts/create_embedding_tasks_table.sql` | 0.5 天 |
| 3 | 添加 embedding 能力的 AI 配置 | `scripts/add_embedding_ai_config.sql` | 0.5 天 |
| 4 | 实现 EmbeddingService | `backend/services/embedding_service.go` | 1 天 |
| 5 | 修改 ResourceIndex 模型 | `backend/internal/models/resource_index.go` | 0.5 天 |
| 6 | 创建 EmbeddingTask 模型 | `backend/internal/models/embedding_task.go` | 0.5 天 |
| 7 | 实现 EmbeddingWorker | `backend/services/embedding_worker.go` | 1 天 |
| 8 | 修改 CMDB 搜索逻辑 | `backend/services/ai_cmdb_service.go` | 1 天 |
| 9 | 实现 embedding 进度查询 API | `backend/controllers/embedding_controller.go` | 0.5 天 |
| 10 | 批量生成工具 | `backend/cmd/generate_embeddings/main.go` | 0.5 天 |
| 11 | 测试和优化 | - | 1 天 |

**总计：约 7.5 天**

### 8.2 文件清单

| 文件 | 类型 | 说明 |
|------|------|------|
| `scripts/add_embedding_columns.sql` | 新增 | 数据库迁移脚本 |
| `scripts/create_embedding_tasks_table.sql` | 新增 | 队列表创建脚本 |
| `scripts/add_embedding_ai_config.sql` | 新增 | AI 配置脚本 |
| `backend/services/embedding_service.go` | 新增 | Embedding 服务 |
| `backend/services/embedding_worker.go` | 新增 | 后台 Worker |
| `backend/internal/models/embedding_task.go` | 新增 | 任务模型 |
| `backend/internal/models/resource_index.go` | 修改 | 添加 embedding 字段 |
| `backend/services/ai_cmdb_service.go` | 修改 | 集成向量搜索 |
| `backend/controllers/embedding_controller.go` | 新增 | 进度查询 API |
| `backend/cmd/generate_embeddings/main.go` | 新增 | 批量生成工具 |

---

## 九、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Embedding API 调用失败 | 搜索降级 | 降级到关键词搜索，重试机制 |
| 向量维度不匹配 | 搜索失败 | 记录 embedding_model，切换模型时重新生成 |
| 批量生成耗时长 | 初始化慢 | 异步处理，显示进度 |
| API 限流 | 生成变慢 | 控制批次间隔，指数退避 |
| 成本超预期 | 费用增加 | 监控 API 调用量，设置告警 |

---

## 十、API 设计

### 10.1 接口设计原则

1. **保留现有接口**：现有的 CMDB 搜索接口保持不变
2. **新增向量搜索接口**：提供独立的向量搜索 API
3. **前端切换按钮**：用户可以选择使用哪种搜索方式
4. **默认使用新方案**：向量搜索作为默认选项

### 10.2 接口清单

| 接口 | 方法 | 说明 | 状态 |
|------|------|------|------|
| `/api/cmdb/search` | POST | 现有关键词搜索接口 | 保留 |
| `/api/ai/cmdb/vector-search` | POST | **新增向量搜索接口** | 新增 |
| `/api/ai/embedding/config-status` | GET | Embedding 配置状态查询 | 新增 |
| `/api/admin/embedding/status` | GET | Worker 状态查询（管理员） | 新增 |
| `/api/admin/embedding/sync-all` | POST | 全量同步（管理员） | 新增 |
| `/api/v1/workspaces/:id/embedding-status` | GET | Workspace Embedding 进度查询 | 新增 |
| `/api/v1/workspaces/:id/embedding/sync` | POST | 同步指定 Workspace | 新增 |
| `/api/v1/workspaces/:id/embedding/rebuild` | POST | 重建指定 Workspace | 新增 |
| `/api/ai/form/generate-with-cmdb` | POST | AI + CMDB 配置生成（自动选择搜索方式） | 修改 |

### 10.3 向量搜索接口

```
POST /api/ai/cmdb/vector-search
```

**请求参数**：

```json
{
  "query": "exchange vpc",           // 搜索关键词
  "resource_type": "aws_vpc",        // 资源类型（可选）
  "workspace_ids": ["ws-xxx"],       // Workspace 范围（可选）
  "limit": 10                        // 返回数量（默认 10）
}
```

**响应**：

```json
{
  "code": 200,
  "data": {
    "results": [
      {
        "id": "vpc-0123456789",
        "name": "exchange-vpc-prod",
        "resource_type": "aws_vpc",
        "workspace_id": "ws-xxx",
        "workspace_name": "production",
        "similarity": 0.95,           // 相似度分数
        "tags": {"Environment": "production"}
      }
    ],
    "search_method": "vector",        // 使用的搜索方法
    "total": 1
  }
}
```

### 10.4 AI + CMDB 配置生成接口（修改）

```
POST /api/ai/form/generate-with-cmdb
```

**请求参数**：

```json
{
  "module_id": 1,
  "user_description": "在 exchange vpc 的东京1a区域创建 ec2",
  "user_selections": {},              // 用户选择的资源 ID（多选情况）
  "context_ids": {
    "workspace_id": "ws-xxx",
    "organization_id": "org-xxx"
  }
}
```

**搜索策略**（自动选择，无需手动指定）：
1. 先尝试精确匹配（cloud_resource_id / cloud_resource_name）
2. 如果 embedding 配置可用，使用向量搜索
3. 降级到关键词搜索

### 10.5 前端切换按钮设计

```tsx
// 前端搜索方式切换组件
interface SearchMethodSwitchProps {
  value: 'vector' | 'keyword';
  onChange: (method: 'vector' | 'keyword') => void;
}

const SearchMethodSwitch: React.FC<SearchMethodSwitchProps> = ({ value, onChange }) => {
  return (
    <div className="search-method-switch">
      <span>搜索方式：</span>
      <Switch
        checked={value === 'vector'}
        onChange={(checked) => onChange(checked ? 'vector' : 'keyword')}
        checkedChildren="向量搜索"
        unCheckedChildren="关键词搜索"
      />
      <Tooltip title="向量搜索支持自然语言，关键词搜索更精确">
        <QuestionCircleOutlined />
      </Tooltip>
    </div>
  );
};
```

### 10.6 后端实现

```go
// backend/services/ai_cmdb_service.go

// GenerateConfigWithCMDB 带 CMDB 查询的配置生成（修改版）
func (s *AICMDBService) GenerateConfigWithCMDB(
    userID string,
    moduleID uint,
    userDescription string,
    workspaceID string,
    organizationID string,
    userSelections map[string]string,
    searchMethod string,  // 新增：搜索方法参数
) (*GenerateConfigWithCMDBResponse, error) {
    // ... 意图断言和查询计划生成 ...
    
    // 根据 searchMethod 选择搜索方式
    if searchMethod == "" {
        searchMethod = "vector"  // 默认使用向量搜索
    }
    
    // 执行 CMDB 查询
    cmdbResults, err := s.executeCMDBQueries(userID, queryPlan, searchMethod)
    if err != nil {
        return nil, err
    }
    
    // ... 后续处理 ...
}

// executeCMDBQueries 执行 CMDB 批量查询（修改版）
func (s *AICMDBService) executeCMDBQueries(
    userID string, 
    queryPlan *CMDBQueryPlan,
    searchMethod string,  // 新增：搜索方法参数
) (*CMDBQueryResults, error) {
    results := &CMDBQueryResults{
        Results: make(map[string]*CMDBQueryResult),
    }
    
    workspaceIDs, err := s.getAccessibleWorkspaces(userID)
    if err != nil {
        return nil, err
    }
    
    for _, query := range queryPlan.Queries {
        filters := s.resolveDependencies(query, results)
        
        var queryResult *CMDBQueryResult
        
        switch searchMethod {
        case "vector":
            // 使用向量搜索（默认）
            queryResult = s.executeVectorQuery(query, filters, workspaceIDs)
        case "keyword":
            // 使用关键词搜索
            queryResult = s.executeKeywordQuery(query, filters, workspaceIDs)
        default:
            // 默认使用向量搜索
            queryResult = s.executeVectorQuery(query, filters, workspaceIDs)
        }
        
        key := s.getResourceTypeKey(query.Type)
        results.Results[key] = queryResult
    }
    
    return results, nil
}

// executeVectorQuery 向量搜索（新增）
func (s *AICMDBService) executeVectorQuery(query CMDBQuery, filters map[string]string, workspaceIDs []string) *CMDBQueryResult {
    result := &CMDBQueryResult{Query: query, Found: false}
    
    // 1. 先尝试精确匹配
    var exactMatch []models.ResourceIndex
    s.db.Where("resource_type = ?", query.Type).
        Where("workspace_id IN ?", workspaceIDs).
        Where("cloud_resource_id = ? OR cloud_resource_name = ?", query.Keyword, query.Keyword).
        Limit(1).Find(&exactMatch)
    
    if len(exactMatch) > 0 {
        result.Found = true
        result.Resource = s.convertToResourceInfo(&exactMatch[0])
        return result
    }
    
    // 2. 向量搜索
    if s.embeddingService != nil {
        vectorResults, err := s.vectorSearch(query, filters, workspaceIDs)
        if err == nil && len(vectorResults) > 0 {
            result.Found = true
            if len(vectorResults) == 1 {
                result.Resource = &vectorResults[0]
            } else {
                result.Candidates = vectorResults
            }
            return result
        }
    }
    
    // 3. 降级到关键词搜索
    return s.executeKeywordQuery(query, filters, workspaceIDs)
}

// executeKeywordQuery 关键词搜索（现有逻辑）
func (s *AICMDBService) executeKeywordQuery(query CMDBQuery, filters map[string]string, workspaceIDs []string) *CMDBQueryResult {
    // ... 现有的关键词搜索逻辑 ...
}
```

### 10.7 Controller 修改

```go
// backend/controllers/ai_cmdb_controller.go

// GenerateConfigWithCMDBRequest 请求结构（修改）
type GenerateConfigWithCMDBRequest struct {
    ModuleID        uint              `json:"module_id" binding:"required"`
    UserDescription string            `json:"user_description" binding:"required,max=2000"`
    SearchMethod    string            `json:"search_method,omitempty"`  // 新增：vector 或 keyword，默认 vector
    UserSelections  map[string]string `json:"user_selections,omitempty"`
    ContextIDs      struct {
        WorkspaceID    string `json:"workspace_id,omitempty"`
        OrganizationID string `json:"organization_id,omitempty"`
    } `json:"context_ids,omitempty"`
}

// GenerateConfigWithCMDB 处理函数
func (c *AICMDBController) GenerateConfigWithCMDB(ctx *gin.Context) {
    var req GenerateConfigWithCMDBRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
        return
    }
    
    userID := ctx.GetString("user_id")
    
    // 调用服务，传入 searchMethod
    response, err := c.service.GenerateConfigWithCMDB(
        userID,
        req.ModuleID,
        req.UserDescription,
        req.ContextIDs.WorkspaceID,
        req.ContextIDs.OrganizationID,
        req.UserSelections,
        req.SearchMethod,  // 新增参数
    )
    
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{"code": 200, "data": response})
}
```

### 10.8 路由配置

```go
// backend/internal/router/router_ai.go

func SetupAIRoutes(r *gin.RouterGroup, db *gorm.DB) {
    aiGroup := r.Group("/ai")
    {
        // 现有路由
        aiGroup.POST("/form/generate", aiFormController.GenerateConfig)
        aiGroup.POST("/form/generate-with-cmdb", aiCMDBController.GenerateConfigWithCMDB)
        
        // ... 其他路由 ...
    }
    
    // CMDB 搜索路由
    cmdbGroup := r.Group("/cmdb")
    {
        // 现有关键词搜索接口（保留）
        cmdbGroup.POST("/search", cmdbController.Search)
        
        // 新增向量搜索接口
        cmdbGroup.POST("/vector-search", cmdbController.VectorSearch)
    }
    
    // Embedding 状态路由
    workspaceGroup := r.Group("/workspaces")
    {
        workspaceGroup.GET("/:workspace_id/embedding-status", embeddingController.GetWorkspaceEmbeddingStatus)
    }
}
```

---

## 十一、向量搜索相似度阈值

### 11.1 问题

向量搜索返回的是按相似度排序的结果，但可能返回相似度很低的"噪音"结果。需要设置合理的阈值过滤。

### 11.2 相似度阈值设计

```go
const (
    // SimilarityThreshold 相似度阈值，低于此值的结果将被过滤
    // 余弦相似度范围：-1 到 1，1 表示完全相同
    // 对于 text-embedding-3-large，建议阈值 0.7
    SimilarityThreshold = 0.7
)

// vectorSearch 向量搜索（带相似度阈值）
func (s *AICMDBService) vectorSearch(query CMDBQuery, filters map[string]string, workspaceIDs []string) ([]CMDBResourceInfo, error) {
    queryVector, err := s.embeddingService.GenerateEmbedding(query.Keyword)
    if err != nil {
        return nil, err
    }
    
    // 使用余弦相似度（1 - 余弦距离）
    // <#> 是负内积（用于余弦距离），<=> 是余弦距离
    sql := `
        SELECT *, 1 - (embedding <=> $3) as similarity
        FROM resource_index
        WHERE resource_type = $1
          AND workspace_id = ANY($2)
          AND embedding IS NOT NULL
          AND 1 - (embedding <=> $3) >= $4
        ORDER BY similarity DESC
        LIMIT 10
    `
    
    var results []struct {
        models.ResourceIndex
        Similarity float64 `gorm:"column:similarity"`
    }
    
    if err := s.db.Raw(sql, query.Type, workspaceIDs, vectorToString(queryVector), SimilarityThreshold).Scan(&results).Error; err != nil {
        return nil, err
    }
    
    // 转换结果，包含相似度分数
    infos := make([]CMDBResourceInfo, 0, len(results))
    for _, r := range results {
        info := s.convertToResourceInfo(&r.ResourceIndex)
        info.Similarity = r.Similarity
        infos = append(infos, *info)
    }
    
    return infos, nil
}
```

### 11.3 CMDBResourceInfo 扩展

```go
// CMDBResourceInfo CMDB 资源信息（扩展）
type CMDBResourceInfo struct {
    ID            string            `json:"id"`
    Name          string            `json:"name"`
    ARN           string            `json:"arn,omitempty"`
    Region        string            `json:"region,omitempty"`
    Tags          map[string]string `json:"tags,omitempty"`
    WorkspaceID   string            `json:"workspace_id,omitempty"`
    WorkspaceName string            `json:"workspace_name,omitempty"`
    Similarity    float64           `json:"similarity,omitempty"`  // 新增：相似度分数
}
```

---

## 十二、Embedding 配置检查

### 12.1 问题

如果用户没有配置 embedding 能力的 AI Config，向量搜索功能将不可用。需要提供配置检查和提示。

### 12.2 配置检查 API

```go
// GET /api/ai/embedding/config-status
func (c *EmbeddingController) GetConfigStatus(ctx *gin.Context) {
    // 检查是否有 embedding 配置
    config, err := c.configService.GetConfigForCapability("embedding")
    
    if err != nil || config == nil {
        ctx.JSON(http.StatusOK, gin.H{
            "code": 200,
            "data": gin.H{
                "configured":   false,
                "message":      "未配置 embedding 能力的 AI 配置，向量搜索功能不可用",
                "help":         "请在 AI 配置管理界面添加支持 embedding 能力的配置",
            },
        })
        return
    }
    
    // 检查 API Key 是否已填写
    hasAPIKey := config.APIKey != ""
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 200,
        "data": gin.H{
            "configured":   true,
            "has_api_key":  hasAPIKey,
            "model_id":     config.ModelID,
            "service_type": config.ServiceType,
            "priority":     config.Priority,
            "message":      "embedding 配置已就绪",
        },
    })
}
```

### 12.3 前端提示

```tsx
// 在 AI 助手界面显示配置状态
const EmbeddingConfigStatus: React.FC = () => {
  const { data: status } = useQuery('embeddingConfigStatus', fetchEmbeddingConfigStatus);
  
  if (!status?.configured) {
    return (
      <Alert
        type="warning"
        message="向量搜索未配置"
        description={
          <>
            {status?.message}
            <br />
            <a href="/admin/ai-configs">前往配置</a>
          </>
        }
      />
    );
  }
  
  if (!status?.has_api_key) {
    return (
      <Alert
        type="warning"
        message="API Key 未配置"
        description="请在 AI 配置管理界面填写 OpenAI API Key"
      />
    );
  }
  
  return null;
};
```

---

## 十三、资源删除时的清理

### 13.1 问题

当资源从 `resource_index` 表删除时，需要同步清理 `embedding_tasks` 表中的相关任务。

### 13.2 解决方案

已在 `embedding_tasks` 表设计中使用外键约束：

```sql
resource_id INTEGER NOT NULL REFERENCES resource_index(id) ON DELETE CASCADE
```

当 `resource_index` 中的记录被删除时，`embedding_tasks` 中的相关任务会自动级联删除。

### 13.3 Worker 中的处理

```go
// processBatch 处理一批任务
func (w *EmbeddingWorker) processBatch(resourceIDs []uint, taskIDs []uint) {
    var resources []models.ResourceIndex
    w.db.Where("id IN ?", resourceIDs).Find(&resources)
    
    if len(resources) == 0 {
        // 资源已被删除，直接删除任务（虽然有级联删除，但这里做双重保险）
        w.db.Where("id IN ?", taskIDs).Delete(&models.EmbeddingTask{})
        log.Printf("[EmbeddingWorker] 资源已删除，清理 %d 个任务", len(taskIDs))
        return
    }
    
    // ... 正常处理 ...
}
```

---

## 十四、监控和告警

### 14.1 监控指标

| 指标 | 说明 | 告警阈值 |
|------|------|---------|
| `embedding_tasks_pending` | 待处理任务数 | > 10000 |
| `embedding_tasks_failed` | 失败任务数 | > 100 |
| `embedding_api_latency` | API 调用延迟 | > 5s |
| `embedding_api_errors` | API 调用错误数 | > 10/min |
| `embedding_coverage` | Embedding 覆盖率 | < 80% |

### 14.2 Prometheus 指标

```go
// backend/services/embedding_metrics.go

var (
    embeddingTasksPending = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "embedding_tasks_pending",
        Help: "Number of pending embedding tasks",
    })
    
    embeddingTasksFailed = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "embedding_tasks_failed",
        Help: "Number of failed embedding tasks",
    })
    
    embeddingAPILatency = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "embedding_api_latency_seconds",
        Help:    "Embedding API call latency",
        Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
    })
    
    embeddingAPIErrors = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "embedding_api_errors_total",
        Help: "Total number of embedding API errors",
    })
)

func init() {
    prometheus.MustRegister(embeddingTasksPending)
    prometheus.MustRegister(embeddingTasksFailed)
    prometheus.MustRegister(embeddingAPILatency)
    prometheus.MustRegister(embeddingAPIErrors)
}
```

### 14.3 定期更新指标

```go
// 在 EmbeddingWorker 中定期更新指标
func (w *EmbeddingWorker) updateMetrics() {
    status := w.GetStatus()
    embeddingTasksPending.Set(float64(status["pending_tasks"].(int64)))
    embeddingTasksFailed.Set(float64(status["failed_tasks"].(int64)))
}
```

---

## 十五、相关文档

- [AI + CMDB 集成设计文档](./ai-cmdb-integration-design.md)
- [CMDB 实现总结](../cmdb/cmdb-implementation-summary.md)
- [AI 配置管理](./04-ai-provider-capability-management.md)
