# AI 配置生成实时进度优化方案

## 文档信息

| 项目 | 内容 |
|------|------|
| 文档版本 | 1.0.0 |
| 创建日期 | 2026-02-01 |
| 状态 | 待审核 |
| 作者 | AI Assistant |
| 相关接口 | `/api/v1/ai/form/generate-with-cmdb-skill` |

---

## 1. 背景与问题

### 1.1 当前实现

当前的进度展示是**前端模拟**的，与后端实际执行进度不同步：

```typescript
// frontend/src/components/OpenAPIFormRenderer/AIFormAssistant/AIConfigGenerator.tsx

const PROGRESS_STEPS = {
  cmdb: [
    { key: 'parse', label: '解析需求', duration: 800 },
    { key: 'cmdb', label: '查询CMDB', duration: 1500 },
    { key: 'skill', label: '组装Skill', duration: 600 },
    { key: 'ai', label: 'AI生成', duration: 0 },  // 最后一步持续到完成
  ],
  normal: [
    { key: 'parse', label: '解析需求', duration: 800 },
    { key: 'ai', label: 'AI生成', duration: 0 },
  ],
};
```

**工作原理**：
1. 用户点击生成后，前端开始计时
2. 按照预设的 `duration` 时间自动切换步骤
3. 最后一步（AI生成）持续到请求完成

### 1.2 存在的问题

| 问题 | 影响 |
|------|------|
| 进度是假的 | 用户看到的进度与实际不符，体验差 |
| 步骤硬编码在前端 | 后端增加/删除步骤时，前端需要同步修改 |
| 无法反映真实耗时 | 如果后端执行快，进度条还在前面；如果后端慢，进度条早就到了最后 |
| 维护成本高 | 前后端需要保持步骤定义同步 |

### 1.3 后端实际执行流程

从 `ai_cmdb_skill_service.go` 可以看到，后端有详细的步骤：

```
优化版流程（GenerateConfigWithCMDBSkillOptimized）：
├── 步骤 1: 获取 AI 配置
├── 步骤 2: 意图断言检查
├── 步骤 3: 并行执行
│   ├── CMDB 评估 + 查询
│   └── Domain Skill 选择
├── 步骤 4: 获取 Schema 数据
├── 步骤 5: 组装 Skill Prompt
├── 步骤 6: 调用 AI 生成配置
└── 步骤 7: 解析 AI 响应
```

---

## 2. 优化目标

1. **真实进度**：前端显示的进度与后端实际执行同步
2. **后端控制**：步骤定义完全由后端控制，前端自适应
3. **可扩展**：后端增加/删除步骤时，前端无需修改
4. **UI 不变**：保持现有的进度条样式

---

## 3. 技术方案

### 3.1 方案选型

| 方案 | 优点 | 缺点 | 适用场景 |
|------|------|------|----------|
| **SSE (Server-Sent Events)** | 实现简单、单向推送、HTTP 协议 | 需要处理认证 | ✅ 进度推送 |
| WebSocket | 双向通信 | 实现复杂、需要维护连接 | 需要双向交互 |
| 轮询 | 实现最简单 | 延迟高、服务器负载大 | 不推荐 |

**选择 SSE 的原因**：
- 进度推送是单向的（服务端 → 客户端）
- 基于 HTTP 协议，与现有架构兼容
- 使用 `fetch` + `ReadableStream` 实现，支持 Authorization Header
- 实现简单，维护成本低

### 3.2 架构设计

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              前端                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  AIConfigGenerator.tsx                                           │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │   │
│  │  │ fetch +         │  │ ProgressState   │  │ Progress UI     │  │   │
│  │  │ ReadableStream  │→ │ (动态状态)      │→ │ (自适应渲染)    │  │   │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↑
                                    │ SSE 事件流
                                    │
┌─────────────────────────────────────────────────────────────────────────┐
│                              后端                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Controller (SSE Endpoint)                                       │   │
│  │  ┌─────────────────┐                                             │   │
│  │  │ /generate-sse   │ ← 新增 SSE 端点                              │   │
│  │  └────────┬────────┘                                             │   │
│  │           │                                                       │   │
│  │           ↓                                                       │   │
│  │  ┌─────────────────────────────────────────────────────────────┐ │   │
│  │  │  Service (带进度回调)                                        │ │   │
│  │  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │ │   │
│  │  │  │ Step 1  │→ │ Step 2  │→ │ Step 3  │→ │ Step N  │        │ │   │
│  │  │  │ 意图断言 │  │ CMDB查询│  │ Skill选择│  │ AI生成  │        │ │   │
│  │  │  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘        │ │   │
│  │  │       │            │            │            │              │ │   │
│  │  │       └────────────┴────────────┴────────────┘              │ │   │
│  │  │                         │                                    │ │   │
│  │  │                         ↓                                    │ │   │
│  │  │              ProgressCallback(step, total, name)             │ │   │
│  │  └─────────────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 4. 详细设计

### 4.1 数据结构

#### 4.1.1 进度事件（后端 → 前端）

```go
// backend/services/progress_event.go

// ProgressEvent 进度事件
type ProgressEvent struct {
    Type       string `json:"type"`        // 事件类型: "progress" | "complete" | "error" | "need_selection"
    Step       int    `json:"step"`        // 当前步骤（从 1 开始）
    TotalSteps int    `json:"total_steps"` // 总步骤数
    StepName   string `json:"step_name"`   // 步骤名称（中文）
    Message    string `json:"message"`     // 详细消息（可选）
    ElapsedMs  int64  `json:"elapsed_ms"`  // 已耗时（毫秒）
    
    // 完成时的数据
    Config      map[string]interface{} `json:"config,omitempty"`       // 生成的配置
    CMDBLookups []CMDBLookupResult     `json:"cmdb_lookups,omitempty"` // CMDB 查询结果
    
    // 错误时的数据
    Error string `json:"error,omitempty"` // 错误信息
}
```

#### 4.1.2 前端状态

```typescript
// frontend/src/services/aiForm.ts

interface ProgressEvent {
  type: 'progress' | 'complete' | 'error' | 'need_selection';
  step: number;
  total_steps: number;
  step_name: string;
  message?: string;
  elapsed_ms: number;
  config?: Record<string, unknown>;
  cmdb_lookups?: CMDBLookupResult[];
  error?: string;
}
```

### 4.2 SSE 事件流示例

#### 4.2.1 正常流程

```
event: progress
data: {"type":"progress","step":1,"total_steps":5,"step_name":"意图断言","message":"正在检查请求安全性...","elapsed_ms":0}

event: progress
data: {"type":"progress","step":2,"total_steps":5,"step_name":"查询CMDB","message":"正在查询 CMDB 资源...","elapsed_ms":523}

event: progress
data: {"type":"progress","step":3,"total_steps":5,"step_name":"选择Skills","message":"正在智能选择 Domain Skills...","elapsed_ms":1847}

event: progress
data: {"type":"progress","step":4,"total_steps":5,"step_name":"组装Prompt","message":"正在组装 AI 提示词...","elapsed_ms":2156}

event: progress
data: {"type":"progress","step":5,"total_steps":5,"step_name":"AI生成","message":"正在调用 AI 生成配置...","elapsed_ms":2234}

event: complete
data: {"type":"complete","step":5,"total_steps":5,"step_name":"完成","config":{...},"elapsed_ms":4523}
```

#### 4.2.2 需要用户选择

```
event: progress
data: {"type":"progress","step":1,"total_steps":5,"step_name":"意图断言",...}

event: progress
data: {"type":"progress","step":2,"total_steps":5,"step_name":"查询CMDB",...}

event: need_selection
data: {"type":"need_selection","step":2,"total_steps":5,"step_name":"需要选择","cmdb_lookups":[...],"elapsed_ms":1847}
```

#### 4.2.3 错误情况

```
event: progress
data: {"type":"progress","step":1,"total_steps":5,"step_name":"意图断言",...}

event: error
data: {"type":"error","step":1,"total_steps":5,"step_name":"意图断言","error":"请求被安全系统拦截","elapsed_ms":523}
```

### 4.3 后端步骤定义

步骤完全在后端定义，前端不需要知道具体有哪些步骤：

```go
// backend/services/ai_cmdb_skill_service.go

// 步骤定义（可根据需要调整）
var progressSteps = []struct {
    Name    string
    Message string
}{
    {"意图断言", "正在检查请求安全性..."},
    {"查询CMDB", "正在查询 CMDB 资源..."},
    {"选择Skills", "正在智能选择 Domain Skills..."},
    {"组装Prompt", "正在组装 AI 提示词..."},
    {"AI生成", "正在调用 AI 生成配置..."},
}

// 如果后端增加步骤，只需修改这里
// 前端会自动适应
```

### 4.4 API 设计

有两种方案可选：

#### 方案 A：新增 SSE 端点（推荐）

新增一个独立的 SSE 端点，保留原有 POST 端点不变。

| 端点 | 方法 | 用途 |
|------|------|------|
| `/api/v1/ai/form/generate-with-cmdb-skill` | POST | 原有端点，保持不变 |
| `/api/v1/ai/form/generate-with-cmdb-skill-sse` | GET | 新增 SSE 端点 |

**优点**：
- 不影响现有功能，风险最低
- 前端可以根据需要选择使用哪个端点
- 降级逻辑简单：SSE 失败时调用 POST

**缺点**：
- 需要维护两个端点
- 前端需要知道两个端点的存在

#### 方案 B：改造现有端点

直接改造现有 POST 端点，通过 `Accept` Header 判断返回格式。

| Accept Header | 返回格式 |
|---------------|----------|
| `text/event-stream` | SSE 事件流 |
| 其他（默认） | JSON |

**优点**：
- 只有一个端点，维护简单
- 前端只需修改 Header

**缺点**：
- 改动现有端点，风险较高
- POST 请求返回 SSE 不太符合 RESTful 规范
- 需要修改现有的 Controller 逻辑

#### 推荐方案

**推荐方案 A**，原因：
1. 风险最低，不影响现有功能
2. 降级逻辑清晰
3. 符合 RESTful 规范（GET 用于 SSE）

---

#### 4.4.1 SSE 端点设计（方案 A）

```
GET /api/v1/ai/form/generate-with-cmdb-skill-sse
```

**请求参数**（Query String）：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| module_id | int | 是 | Module ID |
| user_description | string | 是 | 用户描述 |
| workspace_id | string | 否 | Workspace ID |
| organization_id | string | 否 | Organization ID |
| mode | string | 否 | 模式：new / refine |
| use_optimized | bool | 否 | 是否使用优化版 |
| user_selections | string | 否 | 用户选择的资源（JSON 格式） |
| resource_info_map | string | 否 | 完整资源信息（JSON 格式） |

**响应**：SSE 事件流

**响应头**：
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

#### 4.4.2 原有端点（保持不变）

```
POST /api/v1/ai/form/generate-with-cmdb-skill
```

作为降级方案，当 SSE 不可用时使用

### 4.5 前端适配

#### 4.5.1 删除硬编码步骤

```typescript
// 删除这段代码
const PROGRESS_STEPS = {
  cmdb: [...],
  normal: [...],
};
```

#### 4.5.2 动态渲染进度

```typescript
// 前端只负责渲染，不定义步骤
{loading && progress && (
  <div className={styles.loadingProgressInline}>
    <Spin size="small" />
    <span className={styles.loadingTextInline}>
      <span className={styles.progressStepCurrent}>
        {progress.step_name}
      </span>
      <span className={styles.progressInfo}>
        ({progress.step}/{progress.total_steps})
      </span>
      {progress.message && (
        <span className={styles.progressMessage}>
          {progress.message}
        </span>
      )}
    </span>
  </div>
)}
```

---

## 5. 兼容性设计

### 5.1 降级策略

如果 SSE 不可用（如浏览器不支持 ReadableStream、网络问题），自动降级到原有的 POST 请求：

```typescript
const generateConfig = async (params: GenerateParams, onProgress?: (event: ProgressEvent) => void) => {
  // 检查 ReadableStream 支持
  if (typeof ReadableStream !== 'undefined' && onProgress) {
    try {
      return await generateWithSSE(params, onProgress);
    } catch (error) {
      console.warn('SSE failed, falling back to POST:', error);
      return generateWithPost(params);
    }
  } else {
    // 降级到 POST 请求
    return generateWithPost(params);
  }
};
```

### 5.2 超时处理

使用 AbortController 实现超时控制：

```typescript
const SSE_TIMEOUT = 120000; // 120 秒（AI 生成可能较慢）

const generateWithSSE = async (params: GenerateParams, onProgress: (event: ProgressEvent) => void) => {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), SSE_TIMEOUT);

  try {
    const response = await fetch(url, {
      headers: { 'Authorization': `Bearer ${getToken()}` },
      signal: controller.signal,
    });
    // 处理响应...
  } finally {
    clearTimeout(timeoutId);
  }
};
```

### 5.3 错误处理

fetch + ReadableStream 的错误处理：

```typescript
const generateWithSSE = async (params: GenerateParams, onProgress: (event: ProgressEvent) => void) => {
  try {
    const response = await fetch(url, { /* ... */ });
    
    if (!response.ok) {
      // HTTP 错误，降级到 POST
      throw new Error(`HTTP ${response.status}`);
    }
    
    // 读取流...
  } catch (error) {
    if (error.name === 'AbortError') {
      console.warn('SSE timeout, falling back to POST');
    }
    // 降级到 POST
    return generateWithPost(params);
  }
};
```

---

## 6. 实现计划

### 6.1 阶段划分

| 阶段 | 任务 | 预计时间 | 依赖 |
|------|------|----------|------|
| 1 | 后端：定义 ProgressEvent 结构 | 0.5h | 无 |
| 2 | 后端：实现 SSE 端点 | 1.5h | 阶段 1 |
| 3 | 后端：在服务层添加进度回调 | 1.5h | 阶段 2 |
| 4 | 前端：删除硬编码的 PROGRESS_STEPS | 0.5h | 无 |
| 5 | 前端：实现 SSE 客户端 | 1h | 阶段 2 |
| 6 | 前端：适配动态进度渲染 | 0.5h | 阶段 4, 5 |
| 7 | 测试和调试 | 1h | 阶段 3, 6 |

**总计：约 6.5 小时**

### 6.2 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `backend/services/progress_event.go` | 新增 | 进度事件结构定义 |
| `backend/controllers/ai_cmdb_skill_controller.go` | 修改 | 添加 SSE 端点 |
| `backend/services/ai_cmdb_skill_service.go` | 修改 | 添加进度回调支持 |
| `backend/routes/routes.go` | 修改 | 注册 SSE 路由 |
| `frontend/src/services/aiForm.ts` | 修改 | 添加 SSE 客户端 |
| `frontend/src/components/.../AIConfigGenerator.tsx` | 修改 | 删除硬编码步骤，适配动态渲染 |

---

## 7. 测试计划

### 7.1 单元测试

| 测试项 | 说明 |
|--------|------|
| ProgressEvent 序列化 | 验证 JSON 序列化正确 |
| 进度回调触发 | 验证每个步骤都触发回调 |
| 错误处理 | 验证错误时正确推送 error 事件 |

### 7.2 集成测试

| 测试项 | 说明 |
|--------|------|
| SSE 连接建立 | 验证 SSE 连接正常建立 |
| 进度事件推送 | 验证进度事件按顺序推送 |
| 完成事件 | 验证完成时推送 complete 事件 |
| need_selection 事件 | 验证需要选择时推送正确事件 |

### 7.3 前端测试

| 测试项 | 说明 |
|--------|------|
| 进度渲染 | 验证进度 UI 正确渲染 |
| 步骤自适应 | 验证后端增加步骤时前端自动适应 |
| 降级处理 | 验证 SSE 不可用时降级到 POST |
| 超时处理 | 验证超时后正确处理 |

---

## 8. 风险与缓解

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|----------|
| SSE 连接不稳定 | 进度中断 | 低 | 自动降级到 POST |
| 浏览器不支持 SSE | 功能不可用 | 极低 | 降级到 POST |
| 后端推送延迟 | 进度不实时 | 低 | 优化后端处理 |
| 并发连接过多 | 服务器压力 | 中 | 限制并发数 |

---

## 9. 代码改动影响分析

### 9.1 改动范围

| 文件 | 改动类型 | 改动量 | 影响现有功能 |
|------|----------|--------|--------------|
| `backend/services/progress_event.go` | **新增** | ~50 行 | ❌ 无影响（新文件） |
| `backend/controllers/ai_cmdb_skill_controller.go` | 修改 | ~80 行 | ❌ 无影响（新增端点，不修改现有端点） |
| `backend/services/ai_cmdb_skill_service.go` | 修改 | ~100 行 | ⚠️ 低风险（添加进度回调参数） |
| `backend/routes/routes.go` | 修改 | ~5 行 | ❌ 无影响（新增路由） |
| `frontend/src/services/aiForm.ts` | 修改 | ~50 行 | ❌ 无影响（新增函数） |
| `frontend/src/components/.../AIConfigGenerator.tsx` | 修改 | ~100 行 | ⚠️ 低风险（修改进度渲染逻辑） |

**总改动量**：约 400 行代码

### 9.2 对现有功能的影响

#### 后端

| 现有功能 | 影响 | 说明 |
|----------|------|------|
| `POST /generate-with-cmdb-skill` | ❌ 无影响 | 保持不变，作为降级方案 |
| `GenerateConfigWithCMDBSkill()` | ❌ 无影响 | 保持不变 |
| `GenerateConfigWithCMDBSkillOptimized()` | ⚠️ 低风险 | 添加可选的进度回调参数 |

**后端改动策略**：
- 新增 SSE 端点，不修改现有 POST 端点
- 服务层方法添加**可选**的进度回调参数，默认为 nil（不影响现有调用）

```go
// 改动前
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkillOptimized(...) (*Response, error)

// 改动后（向后兼容）
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkillOptimized(
    ...,
    progressCallback func(ProgressEvent),  // 可选参数，nil 时不推送进度
) (*Response, error)
```

#### 前端

| 现有功能 | 影响 | 说明 |
|----------|------|------|
| 配置生成 | ❌ 无影响 | 核心逻辑不变 |
| 进度显示 | ⚠️ 低风险 | 从模拟改为真实，UI 样式不变 |
| CMDB 选择 | ❌ 无影响 | 逻辑不变 |
| 错误处理 | ❌ 无影响 | 逻辑不变 |

**前端改动策略**：
- 优先使用 SSE，失败时自动降级到 POST
- 进度 UI 样式保持不变，只是数据源改变

### 9.3 风险评估

| 风险等级 | 说明 |
|----------|------|
| 🟢 低风险 | 新增代码为主，现有代码改动最小化 |
| 🟢 低风险 | 保留原有 POST 端点作为降级方案 |
| 🟢 低风险 | 前端自动降级机制确保功能可用 |

### 9.4 测试策略

为确保不影响现有功能，建议：

1. **回归测试**：测试原有 POST 端点功能正常
2. **新功能测试**：测试 SSE 端点功能正常
3. **降级测试**：模拟 SSE 失败，验证自动降级到 POST

---

## 10. 回滚方案

如果优化后出现问题，可以通过以下方式回滚：

1. **前端回滚**：恢复 `PROGRESS_STEPS` 硬编码，使用原有的模拟进度
2. **后端回滚**：禁用 SSE 端点，前端自动降级到 POST

**回滚时间**：< 5 分钟（只需修改前端代码或禁用路由）

---

## 11. 认证与安全

### 11.1 SSE 端点认证

原生 `EventSource` API 不支持自定义 Header，但我们可以使用 `fetch` + `ReadableStream` 来实现 SSE，这样就可以继续使用现有的 Authorization Header 认证方式。

| 方案 | 优点 | 缺点 | 推荐 |
|------|------|------|------|
| **EventSource** | 浏览器原生支持、自动重连 | 不支持自定义 Header | ❌ 不适用 |
| **fetch + ReadableStream** | 支持自定义 Header、与现有认证一致 | 需要手动处理重连 | ✅ 推荐 |
| **Query String Token** | 实现简单 | Token 可能被日志记录 | ⚠️ 备选 |

**推荐方案**：使用 `fetch` + `ReadableStream`，保持现有的 Authorization Header 认证

```typescript
// 前端：使用 fetch 实现 SSE，携带 Authorization Header
const generateWithSSE = async (params: GenerateParams, onProgress: (event: ProgressEvent) => void) => {
  const url = new URL('/api/v1/ai/form/generate-with-cmdb-skill-sse', window.location.origin);
  // 将参数添加到 URL
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined) {
      url.searchParams.set(key, typeof value === 'object' ? JSON.stringify(value) : String(value));
    }
  });

  const response = await fetch(url.toString(), {
    method: 'GET',
    headers: {
      'Authorization': `Bearer ${getToken()}`,  // 使用现有的 token 获取方式
      'Accept': 'text/event-stream',
    },
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const reader = response.body?.getReader();
  const decoder = new TextDecoder();

  if (!reader) {
    throw new Error('ReadableStream not supported');
  }

  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    
    // 解析 SSE 事件
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';  // 保留未完成的行

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        const data = line.slice(6);
        try {
          const event = JSON.parse(data) as ProgressEvent;
          onProgress(event);
        } catch (e) {
          console.error('Failed to parse SSE event:', e);
        }
      }
    }
  }
};
```

**后端保持不变**：继续使用现有的认证中间件

```go
// 后端：使用现有的认证中间件，从 Authorization Header 获取 token
// 路由配置
aiGroup.GET("/form/generate-with-cmdb-skill-sse", authMiddleware.RequireAuth(), controller.GenerateConfigWithCMDBSkillSSE)
```

### 11.2 CORS 配置

SSE 需要正确配置 CORS：

```go
// 后端：CORS 配置
func CORSMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "http://localhost:5173")  // 前端地址
        c.Header("Access-Control-Allow-Credentials", "true")  // 允许携带 Cookie
        c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        // ...
    }
}
```

### 11.3 请求限流

防止 SSE 连接被滥用：

```go
// 后端：限流配置
const (
    MaxConcurrentSSEConnections = 100  // 最大并发 SSE 连接数
    SSEConnectionTimeout        = 120 * time.Second  // 单个连接最大时长
)
```

---

## 12. 监控与日志

### 12.1 监控指标

| 指标 | 说明 | 类型 |
|------|------|------|
| `sse_connections_active` | 当前活跃 SSE 连接数 | Gauge |
| `sse_connections_total` | SSE 连接总数 | Counter |
| `sse_events_sent_total` | 发送的 SSE 事件总数 | Counter |
| `sse_connection_duration_seconds` | SSE 连接持续时间 | Histogram |
| `sse_errors_total` | SSE 错误总数 | Counter |

### 12.2 日志记录

```go
// 后端：关键日志点
log.Printf("[SSE] 连接建立: user_id=%s, module_id=%d", userID, moduleID)
log.Printf("[SSE] 进度推送: step=%d/%d, step_name=%s", step, total, stepName)
log.Printf("[SSE] 连接关闭: user_id=%s, duration=%dms", userID, duration)
log.Printf("[SSE] 错误: user_id=%s, error=%v", userID, err)
```

---

## 13. 性能考虑

### 13.1 连接管理

- **连接池**：限制最大并发连接数，防止资源耗尽
- **超时机制**：单个连接最长 120 秒，超时自动关闭
- **心跳检测**：每 30 秒发送心跳，检测连接是否存活

### 13.2 内存优化

- **流式写入**：使用 `ctx.Writer.Flush()` 及时刷新缓冲区
- **避免缓存**：不缓存 SSE 响应，减少内存占用

### 13.3 并发处理

- **Goroutine 管理**：每个 SSE 连接使用独立 goroutine
- **Context 取消**：客户端断开时及时取消后端任务

---

## 14. 与未来 Pipeline 方案的兼容性

### 14.1 场景理解

**当前场景**：单个资源配置生成（同步 SSE）
```
用户请求 → SSE 连接 → 实时进度推送 → 返回结果
           ├── 步骤1: 意图断言
           ├── 步骤2: CMDB查询
           ├── 步骤3: Skill选择
           ├── 步骤4: AI生成
           └── 返回配置
```

**未来 Pipeline 场景**：多个资源配置生成（**后台异步任务**）
```
用户提交任务 → 立即返回任务ID → 后台执行 → 轮询/WebSocket 查询进度
                                    │
                                    ├── 资源1: EC2 实例（后台生成）
                                    ├── 资源2: S3 存储桶（后台生成）
                                    └── 资源3: RDS 数据库（后台生成）
```

### 14.2 两种场景的本质区别

| 维度 | 当前方案（SSE） | Pipeline 方案（后台任务） |
|------|----------------|--------------------------|
| **执行模式** | 同步，前端等待 | 异步，后台执行 |
| **连接方式** | SSE 长连接 | 提交后断开，轮询查询 |
| **进度存储** | 内存中，实时推送 | 数据库持久化 |
| **用户体验** | 必须保持页面打开 | 可以离开页面，稍后查看 |
| **适用场景** | 单资源，快速生成 | 多资源，长时间任务 |

### 14.3 两种方案是否冲突？

**结论：不冲突，是两个独立的功能**

| 场景 | 使用方案 | 说明 |
|------|----------|------|
| 用户在表单中生成单个资源配置 | **当前 SSE 方案** | 实时反馈，体验好 |
| 用户提交 Pipeline 任务生成多个资源 | **Pipeline 后台任务** | 异步执行，不阻塞 |

### 14.4 当前方案对 Pipeline 的影响

**当前 SSE 方案不会影响 Pipeline 方案**，原因：

1. **接口不同**：
   - 当前：`/api/v1/ai/form/generate-with-cmdb-skill-sse`（SSE 实时）
   - Pipeline：`/api/v1/pipeline/submit`（提交任务）+ `/api/v1/pipeline/{id}/status`（查询进度）

2. **进度机制不同**：
   - 当前：SSE 实时推送，进度在内存中
   - Pipeline：进度存储在数据库，前端轮询或 WebSocket 查询

3. **服务层可复用**：
   - 当前 SSE 方案的服务层逻辑（`GenerateConfigWithCMDBSkillOptimized`）可以被 Pipeline 复用
   - Pipeline 只需要将进度写入数据库，而不是 SSE 推送

### 14.5 服务层复用设计

```go
// 当前方案：SSE 进度回调
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkillOptimized(
    ...,
    progressCallback func(ProgressEvent),  // SSE 推送
) (*Response, error)

// Pipeline 方案：数据库进度回调
func (s *PipelineService) ExecuteTask(taskID string, resource Resource) {
    s.aiCMDBSkillService.GenerateConfigWithCMDBSkillOptimized(
        ...,
        func(event ProgressEvent) {
            // 写入数据库，而不是 SSE 推送
            s.db.UpdateTaskProgress(taskID, event)
        },
    )
}
```

### 14.6 结论

1. **当前 SSE 方案**：适用于单资源实时生成，用户在页面等待
2. **Pipeline 方案**：适用于多资源后台生成，用户可以离开页面
3. **两者独立**：不会相互影响，不需要重构
4. **服务层复用**：Pipeline 可以复用当前的服务层逻辑，只需更换进度回调实现

**建议**：当前方案按计划实现，不需要为 Pipeline 做额外预留

---

## 15. 后端 SSE 实现示例

### 15.1 Controller 实现

```go
// backend/controllers/ai_cmdb_skill_controller.go

// GenerateConfigWithCMDBSkillSSE SSE 端点
func (c *AICMDBSkillController) GenerateConfigWithCMDBSkillSSE(ctx *gin.Context) {
    // 设置 SSE 响应头
    ctx.Header("Content-Type", "text/event-stream")
    ctx.Header("Cache-Control", "no-cache")
    ctx.Header("Connection", "keep-alive")
    ctx.Header("X-Accel-Buffering", "no")  // 禁用 nginx 缓冲

    // 解析请求参数
    moduleID, _ := strconv.Atoi(ctx.Query("module_id"))
    userDescription := ctx.Query("user_description")
    // ... 其他参数

    // 获取 ResponseWriter 的 Flusher
    flusher, ok := ctx.Writer.(http.Flusher)
    if !ok {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
        return
    }

    // 创建进度回调
    startTime := time.Now()
    progressCallback := func(event services.ProgressEvent) {
        event.ElapsedMs = time.Since(startTime).Milliseconds()
        data, _ := json.Marshal(event)
        fmt.Fprintf(ctx.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
        flusher.Flush()
    }

    // 调用服务层
    result, err := c.aiCMDBSkillService.GenerateConfigWithCMDBSkillOptimized(
        ctx.Request.Context(),
        moduleID,
        userDescription,
        // ... 其他参数
        progressCallback,
    )

    if err != nil {
        // 推送错误事件
        progressCallback(services.ProgressEvent{
            Type:    "error",
            Error:   err.Error(),
        })
        return
    }

    // 推送完成事件
    progressCallback(services.ProgressEvent{
        Type:   "complete",
        Config: result.Config,
    })
}
```

### 15.2 Service 层改动

```go
// backend/services/ai_cmdb_skill_service.go

// GenerateConfigWithCMDBSkillOptimized 带进度回调的优化版
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkillOptimized(
    ctx context.Context,
    moduleID int,
    userDescription string,
    // ... 其他参数
    progressCallback func(ProgressEvent),  // 新增：进度回调（可选）
) (*GenerateConfigResponse, error) {
    
    // 辅助函数：安全地推送进度
    reportProgress := func(step int, totalSteps int, stepName, message string) {
        if progressCallback != nil {
            progressCallback(ProgressEvent{
                Type:       "progress",
                Step:       step,
                TotalSteps: totalSteps,
                StepName:   stepName,
                Message:    message,
            })
        }
    }

    totalSteps := 5

    // 步骤 1: 意图断言
    reportProgress(1, totalSteps, "意图断言", "正在检查请求安全性...")
    if err := s.checkIntentAssertion(ctx, userDescription); err != nil {
        return nil, err
    }

    // 步骤 2: 查询 CMDB
    reportProgress(2, totalSteps, "查询CMDB", "正在查询 CMDB 资源...")
    cmdbResult, err := s.queryCMDB(ctx, moduleID, userDescription)
    if err != nil {
        return nil, err
    }

    // 步骤 3: 选择 Skills
    reportProgress(3, totalSteps, "选择Skills", "正在智能选择 Domain Skills...")
    skills, err := s.selectDomainSkills(ctx, moduleID, userDescription)
    if err != nil {
        return nil, err
    }

    // 步骤 4: 组装 Prompt
    reportProgress(4, totalSteps, "组装Prompt", "正在组装 AI 提示词...")
    prompt := s.assemblePrompt(cmdbResult, skills)

    // 步骤 5: AI 生成
    reportProgress(5, totalSteps, "AI生成", "正在调用 AI 生成配置...")
    config, err := s.callAI(ctx, prompt)
    if err != nil {
        return nil, err
    }

    return &GenerateConfigResponse{Config: config}, nil
}
```

---

## 16. 审核记录

| 日期 | 审核人 | 状态 | 备注 |
|------|--------|------|------|
| 2026-02-01 | - | 待审核 | 初稿完成 |

---

## 17. 变更历史

| 版本 | 日期 | 作者 | 变更内容 |
|------|------|------|----------|
| 1.0.0 | 2026-02-01 | AI Assistant | 初始版本 |