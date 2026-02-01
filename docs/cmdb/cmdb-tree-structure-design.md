# IaC平台 CMDB 树状结构功能设计

## 1. 需求分析

### 1.1 当前问题

1. **资源定位困难**：代码和资源分散在多个workspace中，用户难以快速定位资源所在的具体workspace
2. **现有数据不完整**：平台仅保存资源ID，不保存资源名称和详细属性
3. **缺乏层级结构**：现有的`workspace_resources`表是扁平结构，无法展示module嵌套关系
4. **搜索能力不足**：无法基于资源ID或名称进行全局搜索

### 1.2 目标功能

实现树状结构的CMDB功能，支持：
- 按workspace → module → resource的层级展示
- 支持嵌套module（如 `module.ec2-ff.module.sg-module-333`）
- 展示资源详细信息（ID、名称、描述、属性）
- 全局资源搜索（按ID或名称）

### 1.3 期望的树状结构示例

```
workspace: ws-abc123
├── module.ec2-ff (资源名: abc)
│   ├── aws_instance.main
│   │   └── {id: i-1234, name: "web-server", ...}
│   ├── module.sg-module-333
│   │   └── aws_security_group.this
│   │       └── {id: sg-1234, name: "sg-module-333-xxxx", description: "xxx"}
│   └── module.sg-module-444
│       └── aws_security_group.this
│           └── {id: sg-15566, name: null, description: null}
└── module.rds-cluster
    └── aws_rds_cluster.main
        └── {id: rds-xxx, name: "prod-db", ...}
```

## 2. 现有系统分析

### 2.1 Terraform State 结构

从实际state数据分析，Terraform state的资源结构如下：

```json
{
  "resources": [
    {
      "mode": "managed",           // managed 或 data
      "name": "this",              // 资源名称
      "type": "aws_autoscaling_group",  // 资源类型
      "module": "module.AWS_eks-nodegroup-exchang_ddd-64d_clone_570404.module.complete[\"giypeknsix\"].module.self_managed_node_group[\"terraform-managed\"]",
      "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [
        {
          "index_key": 0,          // 可选，用于 count/for_each
          "attributes": {
            "id": "kucoin-ops-nodegroup-example-ken-test-a-20260109081400701500000004",
            "arn": "arn:aws:autoscaling:...",
            "name": "kucoin-ops-nodegroup-example-ken-test-a-20260109081400701500000004",
            "tags": [...],
            // ... 其他属性
          },
          "dependencies": [...]
        }
      ]
    }
  ]
}
```

### 2.2 Module路径解析

Module路径格式：`module.<name>[\"<key>\"].module.<name>[\"<key>\"]...`

示例解析：
- `module.AWS_eks-nodegroup-exchang_ddd-64d_clone_570404` → 顶层module
- `.module.complete["giypeknsix"]` → 子module（带for_each key）
- `.module.self_managed_node_group["terraform-managed"]` → 孙module

### 2.3 现有表结构

当前`workspace_resources`表：
```sql
CREATE TABLE workspace_resources (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL,
    resource_id VARCHAR(100) NOT NULL,      -- 如 "AWS_actions.action-testa"
    resource_type VARCHAR(50) NOT NULL,     -- 如 "AWS_actions"
    resource_name VARCHAR(100) NOT NULL,    -- 如 "action-testa"
    current_version_id INTEGER,
    is_active BOOLEAN DEFAULT true,
    description TEXT,
    tags JSONB,
    ...
);
```

**问题**：
1. `resource_id`存储的是平台自定义格式，不是Terraform地址
2. 没有存储module层级信息
3. 没有存储AWS资源的实际ID（如sg-xxx, i-xxx）

## 3. 数据库设计

### 3.1 新增资源索引表

```sql
-- 资源索引表：存储从Terraform state解析的资源信息
CREATE TABLE resource_index (
    id SERIAL PRIMARY KEY,
    
    -- 基本标识
    workspace_id VARCHAR(50) NOT NULL,
    terraform_address TEXT NOT NULL,        -- 完整Terraform地址，如 module.ec2.aws_instance.main[0]
    
    -- 资源信息
    resource_type VARCHAR(100) NOT NULL,    -- Terraform资源类型，如 aws_security_group
    resource_name VARCHAR(100) NOT NULL,    -- Terraform资源名称，如 main
    resource_mode VARCHAR(20) NOT NULL DEFAULT 'managed',  -- managed 或 data
    index_key TEXT,                         -- count/for_each的key，如 "0" 或 "primary"
    
    -- 云资源信息（从attributes提取）
    cloud_resource_id VARCHAR(255),         -- 云资源ID，如 sg-0123456789
    cloud_resource_name VARCHAR(255),       -- 云资源名称（从name或tags.Name提取）
    cloud_resource_arn TEXT,                -- ARN（如果有）
    description TEXT,                       -- 资源描述
    
    -- Module层级信息
    module_path TEXT,                       -- module路径，如 module.ec2.module.sg
    module_depth INTEGER DEFAULT 0,         -- module嵌套深度
    parent_module_path TEXT,                -- 父module路径
    root_module_name VARCHAR(100),          -- 根module名称（平台资源名）
    
    -- 属性快照
    attributes JSONB,                       -- 资源属性（可选，用于搜索）
    tags JSONB,                             -- 资源标签
    
    -- 元数据
    provider VARCHAR(200),                  -- provider信息
    state_version_id INTEGER,               -- 关联的state版本
    last_synced_at TIMESTAMP DEFAULT NOW(),
    
    -- 索引
    CONSTRAINT uk_resource_index_address UNIQUE (workspace_id, terraform_address)
);

-- 创建索引
CREATE INDEX idx_resource_index_workspace ON resource_index(workspace_id);
CREATE INDEX idx_resource_index_type ON resource_index(resource_type);
CREATE INDEX idx_resource_index_cloud_id ON resource_index(cloud_resource_id);
CREATE INDEX idx_resource_index_cloud_name ON resource_index(cloud_resource_name);
CREATE INDEX idx_resource_index_module_path ON resource_index(module_path);
CREATE INDEX idx_resource_index_root_module ON resource_index(root_module_name);
CREATE INDEX idx_resource_index_mode ON resource_index(resource_mode);

-- 全文搜索索引
CREATE INDEX idx_resource_index_search ON resource_index 
    USING GIN (to_tsvector('english', 
        COALESCE(cloud_resource_id, '') || ' ' || 
        COALESCE(cloud_resource_name, '') || ' ' || 
        COALESCE(description, '')
    ));
```

### 3.2 Module层级表（可选，用于优化树状查询）

```sql
-- Module层级表：存储module的树状结构
CREATE TABLE module_hierarchy (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL,
    module_path TEXT NOT NULL,              -- 完整module路径
    module_name VARCHAR(100) NOT NULL,      -- module名称
    module_key TEXT,                        -- for_each的key
    parent_path TEXT,                       -- 父module路径
    depth INTEGER DEFAULT 0,                -- 嵌套深度
    
    -- 统计信息
    resource_count INTEGER DEFAULT 0,       -- 直接包含的资源数
    total_resource_count INTEGER DEFAULT 0, -- 包含子module的总资源数
    
    -- 元数据
    source VARCHAR(500),                    -- module source（如果能获取）
    last_synced_at TIMESTAMP DEFAULT NOW(),
    
    CONSTRAINT uk_module_hierarchy UNIQUE (workspace_id, module_path)
);

CREATE INDEX idx_module_hierarchy_workspace ON module_hierarchy(workspace_id);
CREATE INDEX idx_module_hierarchy_parent ON module_hierarchy(parent_path);
```

## 4. 资源名称提取策略

### 4.1 提取优先级

不同AWS资源的名称字段不统一，需要按优先级提取：

1. **`name`字段**：直接属性（如安全组的name）
2. **`tags.Name`或`tags_all.Name`**：标签（如EC2的Name标签）
3. **`description`字段**：描述信息
4. **资源类型特定字段**：如bucket名、db_instance_identifier等
5. **云资源ID**：最终fallback

### 4.2 资源类型特定规则

```go
// ResourceNameExtractor 资源名称提取器
type ResourceNameExtractor struct {
    // 资源类型 -> 备选字段列表
    fallbackRules map[string][]string
}

func NewResourceNameExtractor() *ResourceNameExtractor {
    return &ResourceNameExtractor{
        fallbackRules: map[string][]string{
            // EC2相关
            "aws_instance":           {"private_dns", "private_ip"},
            "aws_launch_template":    {"name_prefix"},
            "aws_autoscaling_group":  {"name_prefix"},
            
            // 网络相关
            "aws_vpc":                {"cidr_block"},
            "aws_subnet":             {"cidr_block", "availability_zone"},
            "aws_security_group":     {"name_prefix"},
            "aws_lb":                 {"dns_name"},
            "aws_lb_target_group":    {"name_prefix"},
            
            // 数据库相关
            "aws_db_instance":        {"db_instance_identifier", "endpoint"},
            "aws_rds_cluster":        {"cluster_identifier", "endpoint"},
            
            // 存储相关
            "aws_s3_bucket":          {"bucket"},
            "aws_ebs_volume":         {"availability_zone"},
            
            // IAM相关
            "aws_iam_role":           {"name_prefix"},
            "aws_iam_policy":         {"name_prefix"},
            "aws_iam_instance_profile": {"name_prefix"},
            
            // EKS相关
            "aws_eks_cluster":        {"endpoint"},
            "aws_eks_node_group":     {"node_group_name"},
        },
    }
}

func (e *ResourceNameExtractor) ExtractName(resourceType string, attributes map[string]interface{}) string {
    // 1. 优先提取name字段
    if name := getString(attributes, "name"); name != "" {
        return name
    }
    
    // 2. 尝试从tags中提取Name
    if tags := getMap(attributes, "tags"); tags != nil {
        if name := getString(tags, "Name"); name != "" {
            return name
        }
    }
    if tagsAll := getMap(attributes, "tags_all"); tagsAll != nil {
        if name := getString(tagsAll, "Name"); name != "" {
            return name
        }
    }
    
    // 3. 尝试description字段
    if desc := getString(attributes, "description"); desc != "" {
        // 截取前50个字符作为名称
        if len(desc) > 50 {
            return desc[:50] + "..."
        }
        return desc
    }
    
    // 4. 资源类型特定的fallback
    if fields, ok := e.fallbackRules[resourceType]; ok {
        for _, field := range fields {
            if value := getString(attributes, field); value != "" {
                return value
            }
        }
    }
    
    // 5. 最终使用ID
    if id := getString(attributes, "id"); id != "" {
        return id
    }
    
    return "unnamed"
}
```

## 5. State解析服务

### 5.1 解析流程

```go
// StateParserService State解析服务
type StateParserService struct {
    db            *gorm.DB
    nameExtractor *ResourceNameExtractor
}

// ParseAndSyncState 解析State并同步到资源索引
func (s *StateParserService) ParseAndSyncState(workspaceID string, stateContent map[string]interface{}) error {
    // 1. 解析resources数组
    resources, ok := stateContent["resources"].([]interface{})
    if !ok {
        return nil // 空state
    }
    
    // 2. 解析每个资源
    var indexRecords []ResourceIndex
    moduleSet := make(map[string]bool) // 用于收集所有module路径
    
    for _, res := range resources {
        resMap, ok := res.(map[string]interface{})
        if !ok {
            continue
        }
        
        records := s.parseResource(workspaceID, resMap)
        indexRecords = append(indexRecords, records...)
        
        // 收集module路径
        for _, record := range records {
            if record.ModulePath != "" {
                s.collectModulePaths(record.ModulePath, moduleSet)
            }
        }
    }
    
    // 3. 事务更新数据库
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 删除旧记录
        if err := tx.Where("workspace_id = ?", workspaceID).Delete(&ResourceIndex{}).Error; err != nil {
            return err
        }
        
        // 批量插入新记录
        if len(indexRecords) > 0 {
            if err := tx.CreateInBatches(indexRecords, 100).Error; err != nil {
                return err
            }
        }
        
        // 更新module层级表
        return s.syncModuleHierarchy(tx, workspaceID, moduleSet)
    })
}

// parseResource 解析单个资源
func (s *StateParserService) parseResource(workspaceID string, resMap map[string]interface{}) []ResourceIndex {
    var records []ResourceIndex
    
    mode := getString(resMap, "mode")
    resourceType := getString(resMap, "type")
    resourceName := getString(resMap, "name")
    modulePath := getString(resMap, "module")
    provider := getString(resMap, "provider")
    
    instances, ok := resMap["instances"].([]interface{})
    if !ok || len(instances) == 0 {
        return records
    }
    
    for _, inst := range instances {
        instMap, ok := inst.(map[string]interface{})
        if !ok {
            continue
        }
        
        attributes, _ := instMap["attributes"].(map[string]interface{})
        indexKey := s.getIndexKey(instMap)
        
        // 构建Terraform地址
        address := s.buildTerraformAddress(modulePath, resourceType, resourceName, indexKey)
        
        // 提取云资源信息
        cloudID := getString(attributes, "id")
        cloudName := s.nameExtractor.ExtractName(resourceType, attributes)
        cloudARN := getString(attributes, "arn")
        description := getString(attributes, "description")
        tags := getMap(attributes, "tags")
        
        // 解析module层级
        moduleDepth, parentPath, rootModule := s.parseModulePath(modulePath)
        
        record := ResourceIndex{
            WorkspaceID:       workspaceID,
            TerraformAddress:  address,
            ResourceType:      resourceType,
            ResourceName:      resourceName,
            ResourceMode:      mode,
            IndexKey:          indexKey,
            CloudResourceID:   cloudID,
            CloudResourceName: cloudName,
            CloudResourceARN:  cloudARN,
            Description:       description,
            ModulePath:        modulePath,
            ModuleDepth:       moduleDepth,
            ParentModulePath:  parentPath,
            RootModuleName:    rootModule,
            Attributes:        attributes,
            Tags:              tags,
            Provider:          provider,
            LastSyncedAt:      time.Now(),
        }
        
        records = append(records, record)
    }
    
    return records
}

// buildTerraformAddress 构建完整的Terraform地址
func (s *StateParserService) buildTerraformAddress(modulePath, resourceType, resourceName, indexKey string) string {
    var parts []string
    
    if modulePath != "" {
        parts = append(parts, modulePath)
    }
    
    parts = append(parts, fmt.Sprintf("%s.%s", resourceType, resourceName))
    
    address := strings.Join(parts, ".")
    
    if indexKey != "" {
        address = fmt.Sprintf("%s[%s]", address, indexKey)
    }
    
    return address
}

// parseModulePath 解析module路径
func (s *StateParserService) parseModulePath(modulePath string) (depth int, parentPath, rootModule string) {
    if modulePath == "" {
        return 0, "", ""
    }
    
    // 解析 module.xxx.module.yyy.module.zzz 格式
    parts := strings.Split(modulePath, ".module.")
    depth = len(parts)
    
    if depth > 1 {
        parentPath = strings.Join(parts[:depth-1], ".module.")
        if !strings.HasPrefix(parentPath, "module.") {
            parentPath = "module." + parentPath
        }
    }
    
    // 提取根module名称
    if strings.HasPrefix(modulePath, "module.") {
        firstPart := strings.Split(modulePath[7:], ".")[0]
        // 移除for_each的key
        if idx := strings.Index(firstPart, "["); idx > 0 {
            firstPart = firstPart[:idx]
        }
        rootModule = firstPart
    }
    
    return
}
```

## 6. API设计

### 6.1 获取Workspace资源树

```
GET /api/v1/workspaces/{workspace_id}/resource-tree
```

**响应结构**：
```json
{
  "workspace_id": "ws-abc123",
  "workspace_name": "production",
  "total_resources": 45,
  "tree": [
    {
      "type": "module",
      "name": "ec2-ff",
      "path": "module.ec2-ff",
      "resource_count": 15,
      "children": [
        {
          "type": "resource",
          "terraform_type": "aws_instance",
          "terraform_name": "main",
          "address": "module.ec2-ff.aws_instance.main",
          "cloud_id": "i-0123456789",
          "cloud_name": "web-server",
          "mode": "managed"
        },
        {
          "type": "module",
          "name": "sg-module-333",
          "path": "module.ec2-ff.module.sg-module-333",
          "resource_count": 3,
          "children": [
            {
              "type": "resource",
              "terraform_type": "aws_security_group",
              "terraform_name": "this",
              "address": "module.ec2-ff.module.sg-module-333.aws_security_group.this",
              "cloud_id": "sg-1234",
              "cloud_name": "sg-module-333-xxxx",
              "description": "Security group for web servers",
              "mode": "managed"
            }
          ]
        }
      ]
    }
  ]
}
```

### 6.2 全局资源搜索（支持跳转到资源预览）

```
GET /api/v1/resources/search?q={query}&limit=20
```

**请求参数**：
- `q`: 搜索关键词（支持资源ID、名称、描述）
- `type`: 资源类型过滤（可选）
- `workspace_id`: workspace过滤（可选）
- `limit`: 返回数量限制

**响应结构**：
```json
{
  "total": 3,
  "results": [
    {
      "workspace_id": "ws-abc123",
      "workspace_name": "production",
      "terraform_address": "module.ec2-ff.module.sg-module-333.aws_security_group.this",
      "resource_type": "aws_security_group",
      "cloud_id": "sg-1234",
      "cloud_name": "sg-module-333-xxxx",
      "description": "Security group for web servers",
      "module_path": "module.ec2-ff.module.sg-module-333",
      "root_module": "ec2-ff",
      "platform_resource_id": 80,
      "platform_resource_name": "action-testa",
      "match_field": "cloud_id",
      "match_score": 1.0,
      "jump_url": "/workspaces/ws-abc123/resources/80"
    }
  ]
}
```

**跳转功能说明**：

搜索结果中包含 `platform_resource_id` 和 `jump_url` 字段，用于支持直接跳转到资源预览页面：

1. **`platform_resource_id`**: workspace_resources表中的资源ID（如80）
2. **`jump_url`**: 完整的跳转URL，格式为 `/workspaces/{workspace_id}/resources/{platform_resource_id}`

**资源关联机制**：

resource_index表通过 `root_module_name` 字段与 workspace_resources 表关联：

```sql
-- 查询时关联平台资源
SELECT 
    ri.*,
    wr.id as platform_resource_id,
    wr.resource_name as platform_resource_name,
    CONCAT('/workspaces/', ri.workspace_id, '/resources/', wr.id) as jump_url
FROM resource_index ri
LEFT JOIN workspace_resources wr ON 
    ri.workspace_id = wr.workspace_id 
    AND ri.root_module_name = wr.resource_name
    AND wr.is_active = true
WHERE ri.cloud_resource_id ILIKE '%sg-1234%'
```

### 6.3 获取资源详情

```
GET /api/v1/resources/{workspace_id}/{terraform_address}
```

**响应结构**：
```json
{
  "workspace_id": "ws-abc123",
  "terraform_address": "module.ec2-ff.aws_security_group.main",
  "resource_type": "aws_security_group",
  "resource_name": "main",
  "mode": "managed",
  "cloud_id": "sg-1234",
  "cloud_name": "web-sg",
  "cloud_arn": "arn:aws:ec2:...",
  "description": "Security group for web servers",
  "module_path": "module.ec2-ff",
  "root_module": "ec2-ff",
  "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
  "tags": {
    "Name": "web-sg",
    "Environment": "production"
  },
  "attributes": {
    "id": "sg-1234",
    "name": "web-sg",
    "vpc_id": "vpc-xxx",
    "ingress": [...],
    "egress": [...]
  },
  "last_synced_at": "2026-01-10T12:00:00Z"
}
```

## 7. 同步机制

### 7.1 触发时机

1. **Apply完成后**：在Terraform apply成功后自动触发同步
2. **State上传后**：用户手动上传state后触发同步
3. **定时同步**：每小时扫描所有workspace作为补充

### 7.2 同步流程

```go
// ResourceIndexSyncService 资源索引同步服务
type ResourceIndexSyncService struct {
    db          *gorm.DB
    stateParser *StateParserService
}

// SyncWorkspace 同步单个workspace的资源索引
func (s *ResourceIndexSyncService) SyncWorkspace(workspaceID string) error {
    // 1. 获取最新state
    var stateVersion models.WorkspaceStateVersion
    if err := s.db.Where("workspace_id = ?", workspaceID).
        Order("version DESC").
        First(&stateVersion).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil // 没有state，跳过
        }
        return err
    }
    
    // 2. 解析并同步
    return s.stateParser.ParseAndSyncState(workspaceID, stateVersion.Content)
}

// SyncAllWorkspaces 同步所有workspace（定时任务）
func (s *ResourceIndexSyncService) SyncAllWorkspaces() error {
    var workspaces []models.Workspace
    if err := s.db.Find(&workspaces).Error; err != nil {
        return err
    }
    
    for _, ws := range workspaces {
        if err := s.SyncWorkspace(ws.WorkspaceID); err != nil {
            log.Printf("Failed to sync workspace %s: %v", ws.WorkspaceID, err)
            // 继续处理其他workspace
        }
    }
    
    return nil
}
```

## 8. 前端设计

### 8.1 资源树组件

```tsx
// ResourceTree.tsx
interface TreeNode {
  type: 'module' | 'resource';
  name: string;
  path?: string;
  address?: string;
  cloudId?: string;
  cloudName?: string;
  resourceCount?: number;
  children?: TreeNode[];
}

const ResourceTree: React.FC<{ workspaceId: string }> = ({ workspaceId }) => {
  const [treeData, setTreeData] = useState<TreeNode[]>([]);
  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  
  // 渲染树节点
  const renderTreeNode = (node: TreeNode) => {
    if (node.type === 'module') {
      return (
        <TreeNode
          key={node.path}
          title={
            <span>
              <FolderOutlined /> {node.name}
              <Badge count={node.resourceCount} style={{ marginLeft: 8 }} />
            </span>
          }
        >
          {node.children?.map(renderTreeNode)}
        </TreeNode>
      );
    }
    
    return (
      <TreeNode
        key={node.address}
        title={
          <ResourceNodeTitle
            type={node.terraformType}
            name={node.terraformName}
            cloudId={node.cloudId}
            cloudName={node.cloudName}
          />
        }
      />
    );
  };
  
  return (
    <Tree
      showLine
      expandedKeys={expandedKeys}
      onExpand={setExpandedKeys}
    >
      {treeData.map(renderTreeNode)}
    </Tree>
  );
};
```

### 8.2 全局搜索组件

```tsx
// GlobalResourceSearch.tsx
const GlobalResourceSearch: React.FC = () => {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  
  const handleSearch = async (value: string) => {
    if (!value.trim()) {
      setResults([]);
      return;
    }
    
    setLoading(true);
    try {
      const response = await searchResources(value);
      setResults(response.results);
    } finally {
      setLoading(false);
    }
  };
  
  return (
    <div className="global-search">
      <Input.Search
        placeholder="搜索资源 (ID或名称)"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onSearch={handleSearch}
        loading={loading}
      />
      
      <List
        dataSource={results}
        renderItem={(item) => (
          <List.Item>
            <ResourceSearchResult
              workspaceId={item.workspace_id}
              workspaceName={item.workspace_name}
              address={item.terraform_address}
              type={item.resource_type}
              cloudId={item.cloud_id}
              cloudName={item.cloud_name}
            />
          </List.Item>
        )}
      />
    </div>
  );
};
```

## 9. 实施计划

### Phase 1: 数据层（1-2天）
- [ ] 创建`resource_index`表
- [ ] 创建`module_hierarchy`表（可选）
- [ ] 实现资源名称提取器
- [ ] 实现State解析服务

### Phase 2: 同步机制（1天）
- [ ] 实现单workspace同步功能
- [ ] 在apply完成后触发同步
- [ ] 在state上传后触发同步
- [ ] 实现定时全量同步

### Phase 3: API开发（1-2天）
- [ ] 实现资源树API
- [ ] 实现全局搜索API
- [ ] 实现资源详情API
- [ ] 添加API文档

### Phase 4: 前端开发（2-3天）
- [ ] 实现资源树组件
- [ ] 实现全局搜索组件
- [ ] 集成到workspace详情页
- [ ] 添加导航入口

### Phase 5: 优化（1天）
- [ ] 添加更多资源类型的fallback规则
- [ ] 优化搜索排序逻辑
- [ ] 添加搜索结果缓存
- [ ] 性能测试和优化

## 10. 注意事项

1. **性能考虑**：
   - IaC资源量级通常在几万到几十万，PostgreSQL普通索引即可
   - 大workspace的树状结构可能需要懒加载
   - 搜索结果应该分页返回

2. **数据一致性**：
   - 定时同步作为兜底，防止webhook失败导致的数据不一致
   - 同步失败不应影响平台其他功能

3. **扩展性**：
   - 资源类型的fallback规则应设计为可配置
   - 支持未来添加更多云提供商

4. **安全性**：
   - 资源属性可能包含敏感信息，需要权限控制
   - 搜索结果应该遵循workspace权限
