## 完整方案：AI Config Batch Embedding 配置功能

---

### 一、数据库层面

**新增字段到 `ai_configs` 表：**

```sql
-- scripts/add_embedding_batch_fields.sql
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS embedding_batch_enabled BOOLEAN DEFAULT false;
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS embedding_batch_size INTEGER DEFAULT 10;

COMMENT ON COLUMN ai_configs.embedding_batch_enabled IS '是否启用批量 embedding（仅 embedding 能力使用）';
COMMENT ON COLUMN ai_configs.embedding_batch_size IS '批量大小（建议 10-50）';
```

---

### 二、后端 Model 更新

**修改 `backend/internal/models/ai_config.go`：**

```go
// Vector 搜索配置（仅 embedding 能力使用）
TopK                  int     `gorm:"default:50" json:"top_k"`
SimilarityThreshold   float64 `gorm:"default:0.3" json:"similarity_threshold"`
EmbeddingBatchEnabled bool    `gorm:"default:false" json:"embedding_batch_enabled"` // 新增
EmbeddingBatchSize    int     `gorm:"default:10" json:"embedding_batch_size"`       // 新增
```

---

### 三、前端 AI Config 编辑界面

**修改 `frontend/src/pages/AIConfigForm.tsx`：**

在现有的 "Vector 搜索配置（Embedding 专用）" 区块中添加：

```tsx
{/* 在 similarity_threshold 之后添加 */}

<div className={styles.formGroup} style={{ marginBottom: '12px' }}>
  <label className={styles.checkboxLabel}>
    <input
      type="checkbox"
      checked={formData.embedding_batch_enabled}
      onChange={(e) => setFormData({ ...formData, embedding_batch_enabled: e.target.checked })}
    />
    <span>启用 Batch Embedding</span>
  </label>
  <div className={styles.hint}>
    批量处理多个文本，提升 embedding 生成效率（Titan V2、OpenAI 等模型支持）
  </div>
</div>

{formData.embedding_batch_enabled && (
  <div className={styles.formGroup} style={{ marginBottom: '0' }}>
    <label className={styles.label}>批量大小（Batch Size）</label>
    <input
      type="number"
      className={styles.select}
      value={formData.embedding_batch_size}
      onChange={(e) => setFormData({ ...formData, embedding_batch_size: parseInt(e.target.value) || 10 })}
      min="1"
      max="100"
      required
    />
    <div className={styles.hint}>
      每批处理的文本数量（建议：10-50，过大可能导致 API 超时）
    </div>
  </div>
)}
```

**更新 formData 初始状态：**

```tsx
const [formData, setFormData] = useState({
  // ... 现有字段
  top_k: 50,
  similarity_threshold: 0.3,
  embedding_batch_enabled: false,  // 新增
  embedding_batch_size: 10,        // 新增
});
```

---

### 四、后端 Embedding Service

**修改 `backend/services/embedding_service.go`：**

1. **新增 `callBedrockEmbeddingBatch` 方法：**

```go
// callBedrockEmbeddingBatch 批量调用 Bedrock Embedding API（Titan V2 支持）
func (s *EmbeddingService) callBedrockEmbeddingBatch(aiConfig *models.AIConfig, texts []string) ([][]float32, error) {
    // 构建批量请求
    titanRequest := map[string]interface{}{
        "inputText": texts,  // 数组格式
    }
    requestBody, err := json.Marshal(titanRequest)
    if err != nil {
        return nil, fmt.Errorf("无法序列化请求: %w", err)
    }

    // ... 调用 Bedrock API ...

    // 解析批量响应
    var titanResponse struct {
        Embeddings [][]float32 `json:"embeddings"`
    }
    if err := json.Unmarshal(output.Body, &titanResponse); err != nil {
        return nil, fmt.Errorf("无法解析 Titan batch embedding 响应: %w", err)
    }
    
    return titanResponse.Embeddings, nil
}
```

2. **修改 `GenerateEmbeddingsBatch` 方法：**

```go
func (s *EmbeddingService) GenerateEmbeddingsBatch(texts []string) ([][]float32, error) {
    aiConfig, err := s.configService.GetConfigForCapability("embedding")
    if err != nil {
        return nil, err
    }

    // 根据配置决定是否使用批量 API
    if aiConfig.EmbeddingBatchEnabled {
        switch aiConfig.ServiceType {
        case "openai":
            return s.callOpenAIEmbeddingBatch(aiConfig, texts)
        case "bedrock":
            if strings.Contains(aiConfig.ModelID, "titan-embed") {
                return s.callBedrockEmbeddingBatch(aiConfig, texts)
            }
            // 其他 Bedrock 模型不支持批量，回退到逐个调用
        }
    }

    // 不支持批量或未启用，逐个调用
    embeddings := make([][]float32, len(texts))
    for i, text := range texts {
        embedding, err := s.GenerateEmbedding(text)
        if err != nil {
            return nil, err
        }
        embeddings[i] = embedding
    }
    return embeddings, nil
}
```

---

### 五、后端 Embedding Worker

**修改 `backend/services/embedding_worker.go` 的 `processBatch` 方法：**

```go
func (w *EmbeddingWorker) processBatch(resourceIDs []uint, taskIDs []uint) {
    // 获取配置
    config, _ := w.embeddingService.configService.GetConfigForCapability("embedding")
    
    // 如果启用了批量处理
    if config != nil && config.EmbeddingBatchEnabled {
        // 批量生成 embedding
        texts := make([]string, len(resources))
        for i, r := range resources {
            texts[i] = w.embeddingService.BuildEmbeddingText(&r)
        }
        
        embeddings, err := w.embeddingService.GenerateEmbeddingsBatch(texts)
        if err == nil {
            // 批量更新数据库
            // ...
            return
        }
        log.Printf("[EmbeddingWorker] 批量处理失败，回退到逐个处理: %v", err)
    }
    
    // 逐个处理（原有逻辑）
    // ...
}
```

---

### 六、前端 Service 更新

**修改 `frontend/src/services/ai.ts`：**

```typescript
export interface AIConfig {
  // ... 现有字段
  top_k: number;
  similarity_threshold: number;
  embedding_batch_enabled: boolean;  // 新增
  embedding_batch_size: number;      // 新增
}
```

---

### 七、实现步骤总结

| 步骤 | 文件 | 修改内容 |
|------|------|----------|
| 1 | `scripts/add_embedding_batch_fields.sql` | 新增数据库字段 |
| 2 | `backend/internal/models/ai_config.go` | 更新 Model 结构体 |
| 3 | `frontend/src/services/ai.ts` | 更新 TypeScript 类型 |
| 4 | `frontend/src/pages/AIConfigForm.tsx` | 添加 UI 配置项 |
| 5 | `backend/services/embedding_service.go` | 实现 `callBedrockEmbeddingBatch` |
| 6 | `backend/services/embedding_service.go` | 修改 `GenerateEmbeddingsBatch` |
| 7 | `backend/services/embedding_worker.go` | 修改 `processBatch` 使用批量 API |

---

### 八、预期效果

- **UI 体验**：当选择 embedding 能力时，自动显示 "启用 Batch Embedding" 开关
- **灵活配置**：用户可以根据模型能力决定是否启用批量处理
- **性能提升**：启用后，embedding 生成效率提升 10-50 倍
- **向后兼容**：默认不启用，不影响现有功能

---

需要我帮你实现这个方案吗？如果需要，请 toggle to Act mode。

---

## 九、实现评估报告

> 评估时间：2026-01-26

### 9.1 Batch Embedding 核心功能变更（符合文档预期）

| 步骤 | 文件 | 状态 | 评估 |
|------|------|------|------|
| 1 | `scripts/add_embedding_batch_fields.sql` | ✅ | 与文档完全一致 |
| 2 | `backend/internal/models/ai_config.go` | ✅ | 与文档完全一致 |
| 3 | `frontend/src/services/ai.ts` | ✅ | 与文档完全一致 |
| 4 | `frontend/src/pages/AIConfigForm.tsx` | ✅ | 与文档完全一致 |
| 5 | `backend/services/embedding_service.go` | ✅ | 实现比文档更完善 |
| 6 | `backend/services/embedding_worker.go` | ✅ | 实现比文档更完善 |

### 9.2 实现亮点

**embedding_service.go 改进：**
- 新增 `generateEmbeddingsSequentially()` 辅助方法，代码更清晰
- `callBedrockEmbeddingBatch()` 实现了完整的 Titan V2 批量 API 调用
- 支持 inference profile 配置
- 包含详细的日志和错误处理

**embedding_worker.go 改进：**
- 新增 `processBatchWithBatchAPI()` 方法
- 批量处理失败时自动回退到逐个处理
- 完整的数据库更新和任务状态管理

### 9.3 配套优化变更

以下变更不在原文档预期范围内，但是**必要的配套优化**：

| 文件 | 变更目的 |
|------|----------|
| `backend/services/cmdb_external_source_service.go` | 使用 `Updates` 而不是 `Save`，保留已生成的 embedding 数据 |
| `backend/services/cmdb_service.go` | 改为增量更新模式，保留已有的 embedding 数据 |
| `backend/services/task_queue_manager.go` | Apply 完成后自动同步 CMDB（统一处理 Local/Agent/K8s Agent） |
| `backend/services/terraform_executor.go` | 移除重复的 CMDB 同步，改由 TaskQueueManager 统一处理 |

### 9.4 总结

| 类别 | 文件数 | 状态 |
|------|--------|------|
| Batch Embedding 核心功能 | 6 | ✅ 完全符合文档预期 |
| 配套优化（保留 embedding 数据） | 2 | ✅ 必要的配套改进 |
| 独立功能改进（CMDB 同步） | 2 |  独立功能，与 Batch Embedding 间接相关 |

**结论**: Batch Embedding 核心功能的实现**完全符合文档预期**，且实现质量比文档描述更完善。额外的配套优化确保了 embedding 数据在 CMDB 同步时不会丢失。
