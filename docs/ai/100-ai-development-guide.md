# AI开发指导文档

## 📋 项目概览

**项目名称**: IaC平台 (Infrastructure as Code Platform)
**技术栈**: Go + React + PostgreSQL
**核心功能**: 通过AI解析Terraform Module自动生成表单化界面

## 🎯 已完成的设计文档

###  核心文档清单
- `docs/development-guide.md` - 完整开发指南
- `docs/database-schema.sql` - 数据库设计
- `docs/api-specification.md` - API接口规范  
- `docs/agent-architecture.md` - Agent执行架构
- `docs/vcs-integration.md` - 版本控制集成
- `docs/ai-development-guide.md` - 本文档

###  已设计的核心功能模块

#### 1. 用户认证模块
- 登录/注册/登出
- JWT Token认证
- 角色权限管理 (admin/user)

#### 2. 模块管理模块  
- Terraform Module CRUD
- VCS集成 (GitHub/GitLab)
- 自动同步和Webhook

#### 3. Schema管理模块
- AI解析生成Schema
- Schema版本管理
- 表单验证规则

#### 4. 工作空间模块
- 工作空间CRUD
- 变量管理 (敏感/系统变量)
- 多种状态后端 (local/s3/remote)

#### 5. 部署执行模块
- 三种执行模式 (Server/Agent/K8s)
- Terraform执行引擎
- 实时状态推送

#### 6. Agent管理模块
- Agent池管理
- 动态Agent注册
- 负载均衡和故障恢复

#### 7. 检测集成模块
- 安全检测
- 成本分析
- 合规检查

#### 8. 后台管理模块
- 用户管理
- 系统配置
- 审计日志

## 🚀 开发任务分解

### Phase 1: 基础架构 (优先级: 高)
```
[ ] 数据库初始化
    - 执行 database-schema.sql
    - 创建默认管理员用户
    
[ ] 后端API框架
    - Go + Gin + GORM 基础框架
    - JWT认证中间件
    - 基础CRUD接口
    
[ ] 前端项目初始化  
    - React + TypeScript + Ant Design
    - 路由和状态管理
    - 基础布局组件
```

### Phase 2: 核心功能 (优先级: 高)
```
[ ] 用户认证系统
    - 登录/注册接口实现
    - 前端登录页面
    - Token管理
    
[ ] 模块管理功能
    - Module CRUD接口
    - 前端模块列表页面
    - VCS集成基础功能
    
[ ] Schema管理功能
    - Schema CRUD接口
    - AI解析引擎集成
    - Schema编辑器组件
```

### Phase 3: 高级功能 (优先级: 中)
```
[ ] 动态表单系统
    - 基于Schema的表单渲染
    - 无限嵌套组件
    - 表单验证和联动
    
[ ] 工作空间管理
    - 工作空间CRUD
    - 变量管理界面
    - 权限控制
    
[ ] Terraform执行引擎
    - Server模式执行
    - 状态管理
    - 日志收集
```

### Phase 4: 扩展功能 (优先级: 低)
```
[ ] Agent系统
    - Agent注册和管理
    - K8s动态Pod执行
    - 负载均衡
    
[ ] 检测集成
    - 安全扫描
    - 成本估算
    - 合规检查
    
[ ] 监控运维
    - 指标收集
    - 告警规则
    - 日志分析
```

## 🔍 开发上下文检查清单

### 在开始任何开发任务前，请确认：

####  功能是否已设计
1. 检查相关功能是否在上述"已设计的核心功能模块"中
2. 如果已设计，参考对应的文档进行实现
3. 如果未设计，先询问是否需要设计该功能

####  API接口是否已定义
1. 查看 `docs/api-specification.md` 确认接口是否已定义
2. 如果已定义，严格按照接口规范实现
3. 如果未定义，先设计API接口再实现

####  数据库表是否已设计
1. 查看 `docs/database-schema.sql` 确认表结构
2. 如果已设计，使用现有表结构
3. 如果需要新表，先更新数据库设计

####  避免重复开发
1. 检查功能是否已经实现
2. 检查是否有类似的现有代码可以复用
3. 确认当前任务的具体范围和边界

## 💡 开发指导原则

### 1. 遵循现有设计
- 严格按照已有的API规范实现
- 使用已定义的数据库表结构
- 遵循既定的技术架构

### 2. 最小化实现
- 只实现当前任务必需的功能
- 避免过度设计和冗余代码
- 专注于核心业务逻辑

### 3. 保持一致性
- 使用统一的错误处理方式
- 遵循统一的代码风格
- 保持API响应格式一致

### 4. 安全优先
- 敏感数据加密存储
- 实现适当的权限检查
- 输入验证和SQL注入防护

## 🛠️ 技术实现要点

### 后端开发 (Go)
```go
// 统一的响应格式
type APIResponse struct {
    Code      int         `json:"code"`
    Message   string      `json:"message"`
    Data      interface{} `json:"data,omitempty"`
    Timestamp time.Time   `json:"timestamp"`
}

// 统一的错误处理
func HandleError(c *gin.Context, code int, message string) {
    c.JSON(code, APIResponse{
        Code:      code,
        Message:   message,
        Timestamp: time.Now(),
    })
}

// JWT认证中间件
func JWTAuthMiddleware() gin.HandlerFunc {
    return gin.HandlerFunc(func(c *gin.Context) {
        // 实现JWT验证逻辑
    })
}
```

### 前端开发 (React)
```typescript
// 统一的API调用
interface APIResponse<T> {
  code: number;
  message: string;
  data?: T;
  timestamp: string;
}

// 统一的错误处理
const handleAPIError = (error: any) => {
  message.error(error.response?.data?.message || '操作失败');
};

// 统一的表单验证
const validateForm = (values: any, schema: Schema) => {
  // 基于Schema的表单验证逻辑
};
```

## 📝 开发任务模板

### 开始新任务时，请按以下格式确认：

```
任务: [具体功能描述]

 上下文检查:
- [ ] 功能已在设计文档中定义
- [ ] API接口已在规范中定义  
- [ ] 数据库表结构已确认
- [ ] 无重复开发风险

📋 实现范围:
- 具体要实现的功能点1
- 具体要实现的功能点2
- 具体要实现的功能点3

🔗 相关文档:
- docs/xxx.md (相关设计文档)
- API: POST /api/v1/xxx (相关接口)
- 表: xxx_table (相关数据表)

 注意事项:
- 特殊的业务逻辑要求
- 安全考虑
- 性能要求
```

## 🚨 常见问题避免

### ❌ 避免这些行为:
1. **重复设计已有功能** - 先检查文档再开发
2. **偏离API规范** - 严格按照已定义的接口实现
3. **创建不必要的表** - 使用现有数据库设计
4. **过度实现** - 只实现当前任务需要的功能
5. **忽略安全考虑** - 敏感数据必须加密处理

###  推荐做法:
1. **先读文档后编码** - 理解整体设计再实现
2. **小步快跑** - 分阶段实现，及时验证
3. **复用现有代码** - 查找类似功能进行复用
4. **保持简洁** - 代码简洁易懂，避免过度抽象
5. **及时沟通** - 遇到设计问题及时确认

## 📚 快速参考

### 核心文件路径
```
docs/
├── development-guide.md      # 完整开发指南
├── database-schema.sql       # 数据库设计
├── api-specification.md      # API接口规范
├── agent-architecture.md     # Agent架构设计
├── vcs-integration.md        # VCS集成设计
└── ai-development-guide.md   # 本文档

src/
├── backend/                  # Go后端代码
├── frontend/                 # React前端代码
└── agent/                    # Agent代码
```

### 关键技术栈
- **后端**: Go 1.21+ + Gin + GORM + PostgreSQL
- **前端**: React 18+ + TypeScript + Ant Design + Vite
- **数据库**: PostgreSQL 15+ with JSONB
- **认证**: JWT Bearer Token
- **容器**: Docker + Docker Compose

---

**记住**: 这个文档的目的是帮助AI开发者避免重复工作和偏离设计。每次开始新任务时，请先参考这个指南进行上下文检查！