# Resource List 优化方案

## 当前问题分析

### 1. 前端问题
**文件**: `frontend/src/pages/ResourcesTab.tsx`

当前实现：
```typescript
const fetchResources = async () => {
  const response = await api.get(
    `/workspaces/${workspaceId}/resources?include_inactive=${includeInactive}`
  );
  setResources(data.resources || []);
};
```

**问题**:
- ❌ 一次性加载所有资源
- ❌ 没有分页功能
- ❌ 前端过滤（搜索）效率低
- ❌ 加载所有字段数据（包括完整的tf_code、variables等大字段）

### 2. 后端问题
**文件**: `backend/controllers/resource_controller.go`

当前实现：
```go
func (c *ResourceController) GetResources(ctx *gin.Context) {
    resources, err := c.service.GetResources(uint(workspaceID), includeInactive)
    ctx.JSON(http.StatusOK, gin.H{
        "resources": resources,
        "total":     len(resources),
    })
}
```

**问题**:
- ❌ 返回所有资源记录
- ❌ 返回完整的Resource对象（包括tf_code、variables等大字段）
- ❌ 没有分页参数
- ❌ 没有搜索过滤
- ❌ 数据库查询效率低

## 优化方案

### 方案概述
参考 Runs List 的成功实现，为 Resources List 添加：
1. **后端分页支持**
2. **字段选择（只返回列表必需字段）**
3. **搜索过滤**
4. **排序功能**

### 详细设计

#### 1. 后端API优化

##### 1.1 新增查询参数
```
GET /api/v1/workspaces/:id/resources?page=1&page_size=10&search=bucket&sort_by=created_at&sort_order=desc&include_inactive=false
```

**参数说明**:
- `page`: 页码（默认1）
- `page_size`: 每页数量（默认10，可选10/20/50/100）
- `search`: 搜索关键词（搜索resource_name、resource_type、description）
- `sort_by`: 排序字段（created_at、updated_at、resource_name）
- `sort_order`: 排序方向（asc、desc，默认desc）
- `include_inactive`: 是否包含已删除资源（默认false）

##### 1.2 返回数据结构
```json
{
  "resources": [
    {
      "id": 1,
      "workspace_id": 12,
      "resource_type": "aws_s3_bucket",
      "resource_name": "my_bucket",
      "resource_id": "aws_s3_bucket.my_bucket",
      "is_active": true,
      "created_at": "2025-10-14T10:00:00Z",
      "updated_at": "2025-10-14T11:00:00Z",
      "current_version": {
        "version": 2,
        "is_latest": true,
        "change_summary": "Updated bucket configuration"
      }
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 10,
    "total": 45,
    "total_pages": 5
  }
}
```

**注意**: 列表API **不返回** `tf_code` 和 `variables` 字段，这些大字段只在详情API中返回。

##### 1.3 后端实现伪代码

**Controller层** (`backend/controllers/resource_controller.go`):
```go
func (c *ResourceController) GetResources(ctx *gin.Context) {
    workspaceID, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
    
    // 解析分页参数
    page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))
    
    // 解析搜索和过滤参数
    search := ctx.Query("search")
    sortBy := ctx.DefaultQuery("sort_by", "created_at")
    sortOrder := ctx.DefaultQuery("sort_order", "desc")
    includeInactive := ctx.Query("include_inactive") == "true"
    
    // 调用Service层
    result, err := c.service.GetResourcesPaginated(
        uint(workspaceID),
        page,
        pageSize,
        search,
        sortBy,
        sortOrder,
        includeInactive,
    )
    
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    ctx.JSON(http.StatusOK, result)
}
```

**Service层** (`backend/services/resource_service.go`):
```go
type ResourceListItem struct {
    ID            uint      `json:"id"`
    WorkspaceID   uint      `json:"workspace_id"`
    ResourceType  string    `json:"resource_type"`
    ResourceName  string    `json:"resource_name"`
    ResourceID    string    `json:"resource_id"`
    IsActive      bool      `json:"is_active"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
    CurrentVersion *struct {
        Version       int    `json:"version"`
        IsLatest      bool   `json:"is_latest"`
        ChangeSummary string `json:"change_summary"`
    } `json:"current_version,omitempty"`
}

func (s *ResourceService) GetResourcesPaginated(
    workspaceID uint,
    page, pageSize int,
    search, sortBy, sortOrder string,
    includeInactive bool,
) (map[string]interface{}, error) {
    
    // 构建查询
    query := s.db.Model(&models.WorkspaceResource{}).
        Where("workspace_id = ?", workspaceID)
    
    // 过滤已删除资源
    if !includeInactive {
        query = query.Where("is_active = ?", true)
    }
    
    // 搜索过滤
    if search != "" {
        query = query.Where(
            "resource_name LIKE ? OR resource_type LIKE ? OR description LIKE ?",
            "%"+search+"%", "%"+search+"%", "%"+search+"%",
        )
    }
    
    // 计算总数
    var total int64
    query.Count(&total)
    
    // 排序
    orderClause := sortBy + " " + sortOrder
    query = query.Order(orderClause)
    
    // 分页
    offset := (page - 1) * pageSize
    query = query.Offset(offset).Limit(pageSize)
    
    // 只查询必要字段（不包括tf_code和variables）
    var resources []models.WorkspaceResource
    err := query.Select(
        "id", "workspace_id", "resource_type", "resource_name",
        "resource_id", "is_active", "created_at", "updated_at",
        "current_version_id",
    ).Preload("CurrentVersion", func(db *gorm.DB) *gorm.DB {
        return db.Select("id", "version", "is_latest", "change_summary")
    }).Find(&resources).Error
    
    if err != nil {
        return nil, err
    }
    
    // 转换为列表项格式
    items := make([]ResourceListItem, len(resources))
    for i, r := range resources {
        items[i] = ResourceListItem{
            ID:           r.ID,
            WorkspaceID:  r.WorkspaceID,
            ResourceType: r.ResourceType,
            ResourceName: r.ResourceName,
            ResourceID:   r.ResourceID,
            IsActive:     r.IsActive,
            CreatedAt:    r.CreatedAt,
            UpdatedAt:    r.UpdatedAt,
        }
        
        if r.CurrentVersion != nil {
            items[i].CurrentVersion = &struct {
                Version       int    `json:"version"`
                IsLatest      bool   `json:"is_latest"`
                ChangeSummary string `json:"change_summary"`
            }{
                Version:       r.CurrentVersion.Version,
                IsLatest:      r.CurrentVersion.IsLatest,
                ChangeSummary: r.CurrentVersion.ChangeSummary,
            }
        }
    }
    
    // 计算总页数
    totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
    
    return map[string]interface{}{
        "resources": items,
        "pagination": map[string]interface{}{
            "page":        page,
            "page_size":   pageSize,
            "total":       total,
            "total_pages": totalPages,
        },
    }, nil
}
```

#### 2. 前端优化

##### 2.1 添加分页状态
```typescript
const [page, setPage] = useState(1);
const [pageSize, setPageSize] = useState(10);
const [total, setTotal] = useState(0);
const [sortBy, setSortBy] = useState('created_at');
const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
```

##### 2.2 更新fetchResources函数
```typescript
const fetchResources = async () => {
  try {
    setLoading(true);
    
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString(),
      sort_by: sortBy,
      sort_order: sortOrder,
      include_inactive: includeInactive.toString(),
    });
    
    if (searchTerm) {
      params.append('search', searchTerm);
    }
    
    const response = await api.get(
      `/workspaces/${workspaceId}/resources?${params.toString()}`
    );
    
    const data = response.data || response;
    setResources(data.resources || []);
    setTotal(data.pagination?.total || 0);
  } catch (error) {
    showToast(extractErrorMessage(error), 'error');
  } finally {
    setLoading(false);
  }
};
```

##### 2.3 添加分页控件
```typescript
{total > 0 && (
  <div className={styles.paginationContainer}>
    <div className={styles.paginationLeft}>
      <div className={styles.paginationInfo}>
        Showing {Math.min((page - 1) * pageSize + 1, total)} to{' '}
        {Math.min(page * pageSize, total)} of {total} resources
      </div>
      <div className={styles.pageSizeSelector}>
        <label>Per page:</label>
        <select 
          value={pageSize} 
          onChange={(e) => {
            setPageSize(Number(e.target.value));
            setPage(1);
          }}
        >
          <option value={10}>10</option>
          <option value={20}>20</option>
          <option value={50}>50</option>
          <option value={100}>100</option>
        </select>
      </div>
    </div>
    <div className={styles.paginationControls}>
      <button
        onClick={() => setPage(page - 1)}
        disabled={page === 1}
      >
        ← Previous
      </button>
      <span>Page {page} of {Math.ceil(total / pageSize)}</span>
      <button
        onClick={() => setPage(page + 1)}
        disabled={page >= Math.ceil(total / pageSize)}
      >
        Next →
      </button>
    </div>
  </div>
)}
```

## 性能对比

### 优化前
- **数据库查询**: 查询所有字段（包括tf_code、variables等大字段）
- **网络传输**: 传输所有资源的完整数据
- **前端渲染**: 渲染所有资源
- **示例**: 100个资源，每个资源tf_code约10KB
  - 数据库读取: ~1MB
  - 网络传输: ~1MB
  - 前端内存: ~1MB

### 优化后
- **数据库查询**: 只查询列表必需字段，使用LIMIT/OFFSET
- **网络传输**: 只传输当前页的精简数据
- **前端渲染**: 只渲染当前页
- **示例**: 100个资源，每页10个，每个资源列表项约0.5KB
  - 数据库读取: ~5KB（当前页）
  - 网络传输: ~5KB
  - 前端内存: ~5KB

**性能提升**: ~200倍

## 实施步骤

### Phase 1: 后端实现（优先）
1.  在`resource_service.go`中添加`GetResourcesPaginated`方法
2.  创建`ResourceListItem`结构体（精简字段）
3.  更新`resource_controller.go`的`GetResources`方法
4.  添加单元测试

### Phase 2: 前端实现
1.  更新`ResourcesTab.tsx`添加分页状态
2.  更新`fetchResources`函数支持分页参数
3.  添加分页控件UI
4.  添加排序功能
5.  更新URL参数同步

### Phase 3: 测试验证
1.  测试分页功能
2.  测试搜索功能
3.  测试排序功能
4.  性能测试（对比优化前后）

## 兼容性考虑

### 向后兼容
- 保持原有API路径不变
- 如果不传分页参数，默认返回第1页，每页10条
- 前端逐步迁移，不影响现有功能

### 数据库索引
建议添加以下索引以提升查询性能：
```sql
CREATE INDEX idx_workspace_resources_workspace_id ON workspace_resources(workspace_id);
CREATE INDEX idx_workspace_resources_is_active ON workspace_resources(is_active);
CREATE INDEX idx_workspace_resources_created_at ON workspace_resources(created_at);
CREATE INDEX idx_workspace_resources_resource_name ON workspace_resources(resource_name);
```

## 参考实现

可以参考以下文件的成功实现：
- `backend/controllers/workspace_task_controller.go` - Runs List分页实现
- `frontend/src/pages/WorkspaceDetail.tsx` - RunsTab组件的分页UI

## 预期收益

1. **性能提升**: 
   - 数据库查询时间减少 ~95%
   - 网络传输时间减少 ~95%
   - 页面加载时间减少 ~90%

2. **用户体验**:
   - 页面响应更快
   - 支持大量资源（1000+）
   - 更好的搜索和过滤体验

3. **系统稳定性**:
   - 降低数据库负载
   - 降低网络带宽消耗
   - 降低前端内存占用

## 建议

建议优先实施此优化方案，因为：
1.  参考了成功的Runs List实现
2.  技术方案成熟可靠
3.  性能提升显著
4.  用户体验改善明显
5.  实施风险低
