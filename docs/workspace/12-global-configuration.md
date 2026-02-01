# Workspace模块 - 全局配置

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整设计

## 📘 概述

本文档定义Workspace模块依赖的全局配置，包括Agent Pool配置、K8s配置、Terraform版本配置等。

## 🎯 配置类型

### 1. Agent Pool配置（全局）

**用途**: 定义Agent池，Workspace创建时选择使用

**数据模型**:
```go
type AgentPool struct {
    ID                 uint              `json:"id"`
    Name               string            `json:"name"`
    Description        string            `json:"description"`
    PoolType           string            `json:"pool_type"` // static, dynamic
    SelectionStrategy  string            `json:"selection_strategy"` // round_robin, least_busy, random, label_match
    RequiredLabels     []string          `json:"required_labels"`
    AgentIDs           []string          `json:"agent_ids"`
    Metadata           map[string]interface{} `json:"metadata"`
}
```

**配置示例**:
```json
{
  "name": "production-pool",
  "description": "生产环境Agent池",
  "pool_type": "static",
  "selection_strategy": "least_busy",
  "required_labels": ["production", "aws"],
  "agent_ids": ["agent-01", "agent-02", "agent-03"]
}
```

**Workspace依赖**:
```go
type Workspace struct {
    ExecutionMode string `json:"execution_mode"` // "agent"
    AgentPoolID   *uint  `json:"agent_pool_id"`  // 引用Agent Pool
}
```

### 2. K8s配置（全局）

**用途**: 定义K8s集群配置，Workspace创建时选择使用

**数据模型**:
```go
type K8sConfig struct {
    ID                  uint              `json:"id"`
    Name                string            `json:"name"`
    Description         string            `json:"description"`
    Kubeconfig          string            `json:"kubeconfig"` // base64编码
    ContextName         string            `json:"context_name"`
    Namespace           string            `json:"namespace"`
    PodTemplate         map[string]interface{} `json:"pod_template"`
    ServiceAccountName  string            `json:"service_account_name"`
    ImagePullSecrets    []string          `json:"image_pull_secrets"`
    IsDefault           bool              `json:"is_default"`
}
```

**配置示例**:
```json
{
  "name": "prod-k8s",
  "description": "生产环境K8s集群",
  "namespace": "terraform",
  "pod_template": {
    "image": "hashicorp/terraform:1.6.0",
    "resources": {
      "requests": {"cpu": "500m", "memory": "512Mi"},
      "limits": {"cpu": "1000m", "memory": "1Gi"}
    }
  },
  "service_account_name": "terraform-runner",
  "is_default": true
}
```

**Workspace依赖**:
```go
type Workspace struct {
    ExecutionMode string `json:"execution_mode"` // "k8s"
    K8sConfigID   *uint  `json:"k8s_config_id"`  // 引用K8s配置
}
```

### 3. Terraform版本配置（全局）

**用途**: 管理可用的Terraform版本，包括下载链接和校验和

**数据模型**:
```go
type TerraformVersion struct {
    ID          uint      `json:"id"`
    Version     string    `json:"version"`      // 例如: "1.6.0"
    DownloadURL string    `json:"download_url"` // 下载链接
    Checksum    string    `json:"checksum"`     // SHA256校验和
    Platform    string    `json:"platform"`     // linux_amd64, darwin_amd64, etc.
    IsDefault   bool      `json:"is_default"`
    IsActive    bool      `json:"is_active"`
    CreatedAt   time.Time `json:"created_at"`
}
```

**配置示例**:
```json
{
  "version": "1.6.0",
  "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
  "checksum": "sha256:d117883fd98b960c5d0f012b0d4b21801e1c3f4f",
  "platform": "linux_amd64",
  "is_default": true,
  "is_active": true
}
```

**下载链接格式**:
```
https://releases.hashicorp.com/terraform/{version}/terraform_{version}_{platform}.zip
```

**支持的平台**:
- `linux_amd64`
- `linux_arm64`
- `darwin_amd64` (macOS Intel)
- `darwin_arm64` (macOS Apple Silicon)
- `windows_amd64`

**Workspace依赖**:
```go
type Workspace struct {
    TerraformVersion string `json:"terraform_version"` // 例如: "1.6.0"
}
```

### 4. 系统配置（全局）

**用途**: 平台级别的系统配置

**数据模型**:
```go
type SystemConfig struct {
    ID    uint   `json:"id"`
    Key   string `json:"key"`
    Value string `json:"value"`
    Type  string `json:"type"` // string, int, bool, json
}
```

**配置项**:
```json
{
  "terraform.default_version": "1.6.0",
  "terraform.auto_update": "false",
  "workspace.default_execution_mode": "local",
  "workspace.max_concurrent_tasks": "10",
  "agent.heartbeat_timeout": "60",
  "agent.task_lock_duration": "300",
  "state.retention_days": "30",
  "log.retention_days": "30"
}
```

## 📊 配置管理API

### Agent Pool配置

```http
# 创建Agent Pool
POST /api/v1/agent-pools
{
  "name": "production-pool",
  "pool_type": "static",
  "selection_strategy": "least_busy",
  "required_labels": ["production"]
}

# 获取Agent Pool列表
GET /api/v1/agent-pools

# 获取Agent Pool详情
GET /api/v1/agent-pools/:id

# 更新Agent Pool
PUT /api/v1/agent-pools/:id

# 删除Agent Pool
DELETE /api/v1/agent-pools/:id
```

### K8s配置

```http
# 创建K8s配置
POST /api/v1/k8s-configs
{
  "name": "prod-k8s",
  "namespace": "terraform",
  "pod_template": {...}
}

# 获取K8s配置列表
GET /api/v1/k8s-configs

# 设置默认配置
POST /api/v1/k8s-configs/:id/set-default

# 测试K8s连接
POST /api/v1/k8s-configs/:id/test
```

### Terraform版本配置

```http
# 获取可用版本列表
GET /api/v1/terraform-versions

# 添加新版本
POST /api/v1/terraform-versions
{
  "version": "1.6.0",
  "platform": "linux_amd64"
}

# 设置默认版本
POST /api/v1/terraform-versions/:id/set-default

# 启用/禁用版本
POST /api/v1/terraform-versions/:id/toggle
```

### 系统配置

```http
# 获取所有系统配置
GET /api/v1/system-configs

# 更新系统配置
PUT /api/v1/system-configs/:key
{
  "value": "1.6.0"
}
```

## 🔧 配置服务实现

### TerraformVersionService

```go
type TerraformVersionService struct {
    db *gorm.DB
}

// GetAvailableVersions 获取可用版本列表
func (s *TerraformVersionService) GetAvailableVersions() ([]TerraformVersion, error) {
    var versions []TerraformVersion
    err := s.db.Where("is_active = ?", true).
        Order("version DESC").
        Find(&versions).Error
    return versions, err
}

// GetDefaultVersion 获取默认版本
func (s *TerraformVersionService) GetDefaultVersion() (*TerraformVersion, error) {
    var version TerraformVersion
    err := s.db.Where("is_default = ? AND is_active = ?", true, true).
        First(&version).Error
    return &version, err
}

// DownloadTerraform 下载Terraform二进制文件
func (s *TerraformVersionService) DownloadTerraform(version string, platform string) error {
    // 1. 获取版本信息
    var tfVersion TerraformVersion
    err := s.db.Where("version = ? AND platform = ?", version, platform).
        First(&tfVersion).Error
    if err != nil {
        return err
    }
    
    // 2. 下载文件
    resp, err := http.Get(tfVersion.DownloadURL)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // 3. 验证校验和
    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return err
    }
    
    hash := sha256.Sum256(data)
    checksum := hex.EncodeToString(hash[:])
    
    if checksum != tfVersion.Checksum {
        return errors.New("checksum mismatch")
    }
    
    // 4. 解压并安装
    return s.installTerraform(data, version)
}

// SyncVersionsFromHashiCorp 从HashiCorp同步版本列表
func (s *TerraformVersionService) SyncVersionsFromHashiCorp() error {
    // 1. 获取版本列表
    resp, err := http.Get("https://releases.hashicorp.com/terraform/index.json")
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    var releases map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
        return err
    }
    
    // 2. 解析并保存版本信息
    for version, data := range releases {
        // 解析版本数据
        // 保存到数据库
    }
    
    return nil
}
```

## 🔗 Workspace依赖关系

### 创建Workspace时的依赖检查

```go
func (s *WorkspaceService) CreateWorkspace(workspace *Workspace) error {
    // 1. 验证执行模式配置
    switch workspace.ExecutionMode {
    case "agent":
        if workspace.AgentPoolID == nil {
            return errors.New("agent mode requires agent_pool_id")
        }
        
        // 检查Agent Pool是否存在
        var pool AgentPool
        if err := s.db.First(&pool, workspace.AgentPoolID).Error; err != nil {
            return errors.New("agent pool not found")
        }
        
    case "k8s":
        if workspace.K8sConfigID == nil {
            return errors.New("k8s mode requires k8s_config_id")
        }
        
        // 检查K8s配置是否存在
        var config K8sConfig
        if err := s.db.First(&config, workspace.K8sConfigID).Error; err != nil {
            return errors.New("k8s config not found")
        }
    }
    
    // 2. 验证Terraform版本
    if workspace.TerraformVersion == "" {
        // 使用默认版本
        defaultVersion, err := s.tfVersionService.GetDefaultVersion()
        if err != nil {
            return err
        }
        workspace.TerraformVersion = defaultVersion.Version
    } else {
        // 验证版本是否可用
        var version TerraformVersion
        err := s.db.Where("version = ? AND is_active = ?", 
            workspace.TerraformVersion, true).First(&version).Error
        if err != nil {
            return errors.New("terraform version not available")
        }
    }
    
    // 3. 创建Workspace
    return s.db.Create(workspace).Error
}
```

## 📝 前端集成

### 获取配置选项

```typescript
// 获取Agent Pool列表
const fetchAgentPools = async () => {
  const response = await api.get('/agent-pools');
  setAgentPools(response.data);
};

// 获取K8s配置列表
const fetchK8sConfigs = async () => {
  const response = await api.get('/k8s-configs');
  setK8sConfigs(response.data);
};

// 获取Terraform版本列表
const fetchTerraformVersions = async () => {
  const response = await api.get('/terraform-versions');
  setTerraformVersions(response.data.map(v => v.version));
};

// 在组件加载时获取
useEffect(() => {
  fetchAgentPools();
  fetchK8sConfigs();
  fetchTerraformVersions();
}, []);
```

### 表单中使用配置

```tsx
{/* 执行模式选择 */}
<select value={executionMode} onChange={handleModeChange}>
  <option value="local">Local</option>
  <option value="agent">Agent</option>
  <option value="k8s">K8s</option>
</select>

{/* Agent Pool选择（仅在agent模式显示） */}
{executionMode === 'agent' && (
  <select value={agentPoolId} onChange={handlePoolChange}>
    <option value="">选择Agent Pool</option>
    {agentPools.map(pool => (
      <option key={pool.id} value={pool.id}>
        {pool.name} ({pool.selection_strategy})
      </option>
    ))}
  </select>
)}

{/* K8s配置选择（仅在k8s模式显示） */}
{executionMode === 'k8s' && (
  <select value={k8sConfigId} onChange={handleK8sChange}>
    <option value="">选择K8s配置</option>
    {k8sConfigs.map(config => (
      <option key={config.id} value={config.id}>
        {config.name} ({config.namespace})
      </option>
    ))}
  </select>
)}

{/* Terraform版本选择 */}
<select value={terraformVersion} onChange={handleVersionChange}>
  {terraformVersions.map(version => (
    <option key={version} value={version}>
      {version}
    </option>
  ))}
</select>
```

## 🔒 配置安全

### 1. Kubeconfig加密

```go
// 加密Kubeconfig
func (s *K8sConfigService) EncryptKubeconfig(kubeconfig string) (string, error) {
    encrypted, err := encrypt([]byte(kubeconfig), encryptionKey)
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(encrypted), nil
}

// 解密Kubeconfig
func (s *K8sConfigService) DecryptKubeconfig(encrypted string) (string, error) {
    data, err := base64.StdEncoding.DecodeString(encrypted)
    if err != nil {
        return "", err
    }
    decrypted, err := decrypt(data, encryptionKey)
    return string(decrypted), err
}
```

### 2. Agent Token管理

```go
// Token已在AgentService中实现
// 使用32字节随机数 + base64编码
// 有效期1年
// 支持重新生成和撤销
```

## 📊 配置监控

### 配置使用统计

```sql
-- Agent Pool使用情况
SELECT 
    ap.name,
    COUNT(w.id) as workspace_count
FROM agent_pools ap
LEFT JOIN workspaces w ON w.agent_pool_id = ap.id
GROUP BY ap.id, ap.name;

-- K8s配置使用情况
SELECT 
    kc.name,
    COUNT(w.id) as workspace_count
FROM k8s_configs kc
LEFT JOIN workspaces w ON w.k8s_config_id = kc.id
GROUP BY kc.id, kc.name;

-- Terraform版本使用情况
SELECT 
    terraform_version,
    COUNT(*) as workspace_count
FROM workspaces
GROUP BY terraform_version
ORDER BY workspace_count DESC;
```

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [02-agent-k8s-implementation.md](./02-agent-k8s-implementation.md) - Agent/K8s实现
- [11-frontend-design.md](./11-frontend-design.md) - 前端设计
