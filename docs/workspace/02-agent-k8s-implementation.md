# Agent和K8s执行模式设计文档

> **文档版本**: v1.0  
> **最后更新**: 2025-10-09  
> **状态**: 设计阶段

## 📋 概述

本文档详细说明Agent执行模式和K8s执行模式的设计，包括全局配置、Token管理、Agent CRUD功能等。

## 🎯 设计目标

### Agent模式
1. **Agent管理**: 支持Agent的CRUD操作
2. **Token管理**: 为Agent生成和管理访问Token
3. **标签系统**: 支持Agent标签，用于任务分配
4. **能力匹配**: 根据Agent能力分配任务
5. **状态监控**: 实时监控Agent状态

### K8s模式
1. **全局配置**: 配置K8s集群连接信息
2. **Pod模板**: 定义Pod创建模板
3. **ServiceAccount**: 配置Pod使用的ServiceAccount
4. **资源限制**: 配置CPU/内存限制
5. **镜像管理**: 配置Terraform镜像

## 📊 数据库设计

### 1. agents表

```sql
CREATE TABLE agents (
    id SERIAL PRIMARY KEY,
    
    -- Agent唯一标识（由Agent自己生成，如hostname+uuid）
    agent_id VARCHAR(255) NOT NULL UNIQUE,
    
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- Agent类型
    agent_type VARCHAR(50) NOT NULL, -- 'remote', 'k8s'
    
    -- 状态
    status VARCHAR(50) NOT NULL DEFAULT 'offline', -- 'online', 'offline', 'busy', 'error'
    
    -- 标签（JSON数组）[ERROR] Failed to process response: Too many requests, please wait before trying again. You have sent too many requests.  Wait before trying again.

### 2. agent_pools表

```sql
CREATE TABLE agent_pools (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- 池类型
    pool_type VARCHAR(50) NOT NULL, -- 'static', 'dynamic'
    
    -- 选择策略
    selection_strategy VARCHAR(50) DEFAULT 'round_robin', 
    -- 'round_robin', 'least_busy', 'random', 'label_match'
    
    -- 标签要求（JSON数组）
    required_labels JSONB DEFAULT '[]',
    
    -- 关联的Agent ID列表（JSON数组）
    agent_ids JSONB DEFAULT '[]',
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);
```

### 3. k8s_configs表（全局K8s配置）

```sql
CREATE TABLE k8s_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- K8s集群配置
    kubeconfig TEXT, -- base64编码的kubeconfig
    context_name VARCHAR(255), -- 使用的context
    namespace VARCHAR(255) DEFAULT 'default',
    
    -- Pod模板配置
    pod_template JSONB NOT NULL,
    -- 例如:
    -- {
    --   "image": "hashicorp/terraform:1.6.0",
    --   "serviceAccountName": "terraform-runner",
    --   "resources": {
    --     "requests": {"cpu": "500m", "memory": "512Mi"},
    --     "limits": {"cpu": "1000m", "memory": "1Gi"}
    --   },
    --   "env": [
    --     {"name": "TF_LOG", "value": "INFO"}
    --   ],
    --   "volumeMounts": [...],
    --   "securityContext": {...}
    -- }
    
    -- ServiceAccount配置
    service_account_name VARCHAR(255) DEFAULT 'default',
    
    -- 镜像拉取密钥
    image_pull_secrets JSONB DEFAULT '[]',
    
    -- 是否为默认配置
    is_default BOOLEAN DEFAULT false,
    
    -- 状态
    status VARCHAR(50) DEFAULT 'active', -- 'active', 'inactive'
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_k8s_configs_is_default ON k8s_configs(is_default);
```

### 4. 更新workspace_tasks表

```sql
ALTER TABLE workspace_tasks ADD COLUMN agent_id INTEGER REFERENCES agents(id);
ALTER TABLE workspace_tasks ADD COLUMN k8s_config_id INTEGER REFERENCES k8s_configs(id);
ALTER TABLE workspace_tasks ADD COLUMN k8s_pod_name VARCHAR(255);
ALTER TABLE workspace_tasks ADD COLUMN execution_node VARCHAR(255); -- 执行节点标识

CREATE INDEX idx_workspace_tasks_agent_id ON workspace_tasks(agent_id);
CREATE INDEX idx_workspace_tasks_k8s_pod_name ON workspace_tasks(k8s_pod_name);
```

## 🔧 Go模型定义

### Agent模型

```go
package models

import (
    "time"
    "database/sql/driver"
    "encoding/json"
)

type AgentType string

const (
    AgentTypeRemote AgentType = "remote"
    AgentTypeK8s    AgentType = "k8s"
)

type AgentStatus string

const (
    AgentStatusOnline  AgentStatus = "online"
    AgentStatusOffline AgentStatus = "offline"
    AgentStatusBusy    AgentStatus = "busy"
    AgentStatusError   AgentStatus = "error"
)

type Agent struct {
    ID          int       `json:"id" gorm:"primaryKey"`
    Name        string    `json:"name" gorm:"uniqueIndex;not null"`
    Description string    `json:"description"`
    AgentType   AgentType `json:"agent_type" gorm:"not null"`
    Status      AgentStatus `json:"status" gorm:"default:offline"`
    
    // 标签和能力
    Labels       JSONArray  `json:"labels" gorm:"type:jsonb;default:'[]'"`
    Capabilities JSONObject `json:"capabilities" gorm:"type:jsonb;default:'{}'"`
    
    // Token
    Token          string     `json:"token,omitempty" gorm:"uniqueIndex"`
    TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
    
    // 连接信息
    Endpoint string `json:"endpoint"`
    
    // 心跳
    LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
    
    // 统计
    TotalTasks   int `json:"total_tasks" gorm:"default:0"`
    SuccessTasks int `json:"success_tasks" gorm:"default:0"`
    FailedTasks  int `json:"failed_tasks" gorm:"default:0"`
    
    // 元数据
    Metadata JSONObject `json:"metadata" gorm:"type:jsonb;default:'{}'"`
    
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
    DeletedAt *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// JSONArray 自定义类型
type JSONArray []string

func (j JSONArray) Value() (driver.Value, error) {
    return json.Marshal(j)
}

func (j *JSONArray) Scan(value interface{}) error {
    bytes, ok := value.([]byte)
    if !ok {
        return nil
    }
    return json.Unmarshal(bytes, j)
}

// JSONObject 自定义类型
type JSONObject map[string]interface{}

func (j JSONObject) Value() (driver.Value, error) {
    return json.Marshal(j)
}

func (j *JSONObject) Scan(value interface{}) error {
    bytes, ok := value.([]byte)
    if !ok {
        return nil
    }
    return json.Unmarshal(bytes, j)
}
```

### AgentPool模型

```go
type PoolType string

const (
    PoolTypeStatic  PoolType = "static"
    PoolTypeDynamic PoolType = "dynamic"
)

type SelectionStrategy string

const (
    StrategyRoundRobin SelectionStrategy = "round_robin"
    StrategyLeastBusy  SelectionStrategy = "least_busy"
    StrategyRandom     SelectionStrategy = "random"
    StrategyLabelMatch SelectionStrategy = "label_match"
)

type AgentPool struct {
    ID                 int               `json:"id" gorm:"primaryKey"`
    Name               string            `json:"name" gorm:"uniqueIndex;not null"`
    Description        string            `json:"description"`
    PoolType           PoolType          `json:"pool_type" gorm:"not null"`
    SelectionStrategy  SelectionStrategy `json:"selection_strategy" gorm:"default:round_robin"`
    RequiredLabels     JSONArray         `json:"required_labels" gorm:"type:jsonb;default:'[]'"`
    AgentIDs           JSONArray         `json:"agent_ids" gorm:"type:jsonb;default:'[]'"`
    Metadata           JSONObject        `json:"metadata" gorm:"type:jsonb;default:'{}'"`
    CreatedAt          time.Time         `json:"created_at"`
    UpdatedAt          time.Time         `json:"updated_at"`
    DeletedAt          *time.Time        `json:"deleted_at,omitempty" gorm:"index"`
}
```

### K8sConfig模型

```go
type K8sConfig struct {
    ID                  int        `json:"id" gorm:"primaryKey"`
    Name                string     `json:"name" gorm:"uniqueIndex;not null"`
    Description         string     `json:"description"`
    Kubeconfig          string     `json:"kubeconfig,omitempty"` // base64编码
    ContextName         string     `json:"context_name"`
    Namespace           string     `json:"namespace" gorm:"default:default"`
    PodTemplate         JSONObject `json:"pod_template" gorm:"type:jsonb;not null"`
    ServiceAccountName  string     `json:"service_account_name" gorm:"default:default"`
    ImagePullSecrets    JSONArray  `json:"image_pull_secrets" gorm:"type:jsonb;default:'[]'"`
    IsDefault           bool       `json:"is_default" gorm:"default:false"`
    Status              string     `json:"status" gorm:"default:active"`
    Metadata            JSONObject `json:"metadata" gorm:"type:jsonb;default:'{}'"`
    CreatedAt           time.Time  `json:"created_at"`
    UpdatedAt           time.Time  `json:"updated_at"`
    DeletedAt           *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}
```

## 🔌 API接口设计

### Agent管理API

```
# Agent CRUD
GET    /api/v1/agents                    # 获取Agent列表
POST   /api/v1/agents                    # 创建Agent
GET    /api/v1/agents/:id                # 获取Agent详情
PUT    /api/v1/agents/:id                # 更新Agent
DELETE /api/v1/agents/:id                # 删除Agent

# Token管理
POST   /api/v1/agents/:id/regenerate-token  # 重新生成Token
POST   /api/v1/agents/:id/revoke-token      # 撤销Token

# Agent状态
POST   /api/v1/agents/:id/heartbeat      # Agent心跳
GET    /api/v1/agents/:id/status         # 获取Agent状态
GET    /api/v1/agents/:id/tasks          # 获取Agent任务列表

# Agent Pool
GET    /api/v1/agent-pools               # 获取Pool列表
POST   /api/v1/agent-pools               # 创建Pool
GET    /api/v1/agent-pools/:id           # 获取Pool详情
PUT    /api/v1/agent-pools/:id           # 更新Pool
DELETE /api/v1/agent-pools/:id           # 删除Pool
POST   /api/v1/agent-pools/:id/agents    # 添加Agent到Pool
DELETE /api/v1/agent-pools/:id/agents/:agent_id  # 从Pool移除Agent
```

### K8s配置API

```
# K8s配置CRUD
GET    /api/v1/k8s-configs               # 获取K8s配置列表
POST   /api/v1/k8s-configs               # 创建K8s配置
GET    /api/v1/k8s-configs/:id           # 获取K8s配置详情
PUT    /api/v1/k8s-configs/:id           # 更新K8s配置
DELETE /api/v1/k8s-configs/:id           # 删除K8s配置

# 配置测试
POST   /api/v1/k8s-configs/:id/test      # 测试K8s连接
POST   /api/v1/k8s-configs/:id/set-default  # 设置为默认配置

# Pod模板管理
GET    /api/v1/k8s-configs/:id/pod-template  # 获取Pod模板
PUT    /api/v1/k8s-configs/:id/pod-template  # 更新Pod模板
```

## 🎯 核心服务实现

### AgentService

```go
package services

type AgentService struct {
    db *gorm.DB
}

// CreateAgent 创建Agent并生成Token
func (s *AgentService) CreateAgent(agent *models.Agent) error {
    // 生成Token
    token, err := generateSecureToken()
    if err != nil {
        return err
    }
    
    agent.Token = token
    agent.TokenExpiresAt = time.Now().Add(365 * 24 * time.Hour) // 1年有效期
    agent.Status = models.AgentStatusOffline
    
    return s.db.Create(agent).Error
}

// RegenerateToken 重新生成Token
func (s *AgentService) RegenerateToken(agentID int) (string, error) {
    token, err := generateSecureToken()
    if err != nil {
        return "", err
    }
    
    expiresAt := time.Now().Add(365 * 24 * time.Hour)
    
    err = s.db.Model(&models.Agent{}).
        Where("id = ?", agentID).
        Updates(map[string]interface{}{
            "token": token,
            "token_expires_at": expiresAt,
        }).Error
    
    return token, err
}

// SelectAgent 根据策略选择Agent
func (s *AgentService) SelectAgent(poolID int, labels []string) (*models.Agent, error) {
    var pool models.AgentPool
    if err := s.db.First(&pool, poolID).Error; err != nil {
        return nil, err
    }
    
    switch pool.SelectionStrategy {
    case models.StrategyRoundRobin:
        return s.selectRoundRobin(pool.AgentIDs)
    case models.StrategyLeastBusy:
        return s.selectLeastBusy(pool.AgentIDs)
    case models.StrategyLabelMatch:
        return s.selectByLabels(pool.AgentIDs, labels)
    default:
        return s.selectRandom(pool.AgentIDs)
    }
}

// Heartbeat 处理Agent心跳
func (s *AgentService) Heartbeat(agentID int) error {
    return s.db.Model(&models.Agent{}).
        Where("id = ?", agentID).
        Updates(map[string]interface{}{
            "last_heartbeat_at": time.Now(),
            "status": models.AgentStatusOnline,
        }).Error
}
```

### K8sExecutorService

```go
package services

import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)

type K8sExecutorService struct {
    db *gorm.DB
}

// ExecuteTask 在K8s中执行任务
func (s *K8sExecutorService) ExecuteTask(task *models.WorkspaceTask, workspace *models.Workspace) error {
    // 1. 获取K8s配置
    config, err := s.getK8sConfig(workspace.K8sConfigID)
    if err != nil {
        return err
    }
    
    // 2. 创建K8s客户端
    client, err := s.createK8sClient(config)
    if err != nil {
        return err
    }
    
    // 3. 创建Pod
    pod, err := s.createPod(client, config, task, workspace)
    if err != nil {
        return err
    }
    
    // 4. 更新任务信息
    task.K8sPodName = pod.Name
    task.ExecutionNode = pod.Spec.NodeName
    s.db.Save(task)
    
    // 5. 监控Pod状态
    go s.monitorPod(client, config.Namespace, pod.Name, task.ID)
    
    return nil
}

// createPod 根据模板创建Pod
func (s *K8sExecutorService) createPod(client *kubernetes.Clientset, config *models.K8sConfig, task *models.WorkspaceTask, workspace *models.Workspace) (*corev1.Pod, error) {
    // 从pod_template构建Pod定义
    podTemplate := config.PodTemplate
    
    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("terraform-%d-%d", workspace.ID, task.ID),
            Labels: map[string]string{
                "app": "terraform-runner",
                "workspace-id": fmt.Sprintf("%d", workspace.ID),
                "task-id": fmt.Sprintf("%d", task.ID),
            },
        },
        Spec: corev1.PodSpec{
            ServiceAccountName: config.ServiceAccountName,
            RestartPolicy: corev1.RestartPolicyNever,
            Containers: []corev1.Container{
                {
                    Name:  "terraform",
                    Image: podTemplate["image"].(string),
                    // ... 其他配置从podTemplate读取
                },
            },
        },
    }
    
    return client.CoreV1().Pods(config.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
}
```

## 📝 使用流程

### Agent模式流程

```
1. 管理员创建Agent
   POST /api/v1/agents
   {
     "name": "agent-01",
     "agent_type": "remote",
     "labels": ["prod", "us-west"],
     "endpoint": "https://agent-01.example.com"
   }
   
2. 系统生成Token
   Response: {
     "id": 1,
     "token": "agt_xxxxxxxxxxxxx",
     "token_expires_at": "2026-10-09T00:00:00Z"
   }
   
3. 管理员将Token配置到Agent机器
   
4. Agent启动并发送心跳
   POST /api/v1/agents/1/heartbeat
   Headers: Authorization: Bearer agt_xxxxxxxxxxxxx
   
5. 用户创建Workspace并选择Agent Pool
   
6. 用户创建Plan任务
   
7. TaskWorker选择Agent并分配任务
   
8. Agent执行任务并返回结果
```

### K8s模式流程

```
1. 管理员创建K8s配置
   POST /api/v1/k8s-configs
   {
     "name": "prod-k8s",
     "kubeconfig": "base64_encoded_kubeconfig",
     "namespace": "terraform",
     "pod_template": {
       "image": "hashicorp/terraform:1.6.0",
       "serviceAccountName": "terraform-runner",
       "resources": {...}
     }
   }
   
2. 测试K8s连接
   POST /api/v1/k8s-configs/1/test
   
3. 设置为默认配置
   POST /api/v1/k8s-configs/1/set-default
   
4. 用户创建Workspace并选择K8s模式
   
5. 用户创建Plan任务
   
6. TaskWorker在K8s中创建Pod执行任务
   
7. 监控Pod状态并更新任务结果
   
8. 任务完成后清理Pod
```

## 🔒 安全考虑

### Token安全
1. Token使用加密存储
2. Token有过期时间
3. 支持Token撤销
4. Token使用HTTPS传输

### K8s安全
1. Kubeconfig加密存储
2. 使用专用ServiceAccount
3. RBAC权限控制
4. Pod安全策略

### Agent安全
1. Agent认证使用Token
2. 心跳超时自动下线
3. 任务执行隔离
4. 审计日志记录

## 📊 监控和告警

### Agent监控
- Agent在线状态
- 任务执行统计
- 心跳超时告警
- 任务失败率告警

### K8s监控
- Pod创建成功率
- Pod执行时间
- 资源使用情况
- Pod失败告警

---

**文档版本**: v1.0  
**最后更新**: 2025-10-09  
**下一步**: 实现Agent和K8s执行模式
