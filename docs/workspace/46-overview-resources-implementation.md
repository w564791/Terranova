# Overview标签页Resources部分实现

## 概述

在Overview标签页的Resources部分实现了实际资源列表数据的显示，替换了原有的占位符。

## 实现时间

2025-10-14

## 需求

在Overview标签页的Resources部分：
1. 获取并显示前5个资源
2. 复用ResourcesTab的数据结构和显示逻辑
3. 添加"View all resources"链接跳转到Resources标签页
4. 保持与Resources标签页一致的显示样式

## 技术实现

### 1. 状态管理

在`WorkspaceDetail.tsx`主组件中添加了全局Resources状态：

```typescript
// 全局Resources状态（Overview标签页使用）
const [globalResources, setGlobalResources] = useState<any[]>([]);
const [globalResourcesTotal, setGlobalResourcesTotal] = useState<number>(0);
```

### 2. 数据获取

创建了`fetchGlobalResources`函数：

```typescript
const fetchGlobalResources = React.useCallback(async () => {
  if (!id) return;
  
  try {
    // 获取前5个资源
    const params = new URLSearchParams({
      page: '1',
      page_size: '5',
      sort_by: 'created_at',
      sort_order: 'desc',
      include_inactive: 'false',
    });
    
    const response = await api.get(`/workspaces/${id}/resources?${params.toString()}`);
    const data = response.data || response;
    setGlobalResources(data.resources || []);
    setGlobalResourcesTotal(data.pagination?.total || 0);
  } catch (error) {
    console.error('Failed to fetch global resources:', error);
  }
}, [id]);
```

### 3. API端点

```
GET /workspaces/${workspaceId}/resources?page=1&page_size=5&sort_by=created_at&sort_order=desc&include_inactive=false
```

**响应结构**：
```json
{
  "resources": [
    {
      "id": 1,
      "resource_name": "...",
      "resource_type": "...",
      "current_version": {
        "version": 1,
        "is_latest": true,
        "change_summary": "..."
      },
      "is_active": true,
      "created_at": "..."
    }
  ],
  "pagination": {
    "total": 9,
    "page": 1,
    "page_size": 5
  }
}
```

### 4. 显示逻辑

#### 资源数量显示

在三个位置显示资源总数：

1. **全局头部**：
```typescript
<span className={styles.metaItem}>
  Resources {globalResourcesTotal}
</span>
```

2. **Overview Resources标题**：
```typescript
<span className={styles.resourceCount}>{globalResourcesTotal}</span>
```

3. **View all按钮**：
```typescript
View all {globalResourcesTotal} resources →
```

#### 资源列表显示

```typescript
{globalResources.length > 0 ? (
  <div className={styles.resourcesList}>
    <div className={styles.resourcesTable}>
      <div className={styles.tableHeader}>
        <div>NAME</div>
        <div>TYPE</div>
        <div>VERSION</div>
        <div>STATUS</div>
        <div>CREATED</div>
      </div>
      <div className={styles.tableBody}>
        {globalResources.map((resource) => (
          <div className={styles.resourceRow} onClick={...}>
            {/* 资源详细信息 */}
          </div>
        ))}
        {globalResourcesTotal > 5 && (
          <div className={styles.viewAllRow}>
            <button onClick={() => onTabChange('resources')}>
              View all {globalResourcesTotal} resources →
            </button>
          </div>
        )}
      </div>
    </div>
  </div>
) : (
  <div className={styles.emptyState}>
    <p>No resources managed</p>
  </div>
)}
```

### 5. CSS样式

#### 表格布局

```css
.tableHeader {
  display: grid;
  grid-template-columns: 2fr 1fr 1.5fr 1fr 1fr;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background: var(--color-gray-50);
  border-bottom: 1px solid var(--color-gray-200);
  font-size: 12px;
  font-weight: 600;
  color: var(--color-gray-600);
  text-transform: uppercase;
}

.resourceRow {
  display: grid;
  grid-template-columns: 2fr 1fr 1.5fr 1fr 1fr;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  border-bottom: 1px solid var(--color-gray-100);
  align-items: center;
  transition: background 0.2s;
}
```

#### 资源名称换行

```css
.resourceName {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  overflow: hidden;
}

.resourceNameText {
  font-weight: 500;
  color: var(--color-gray-900);
  font-size: 14px;
  word-break: break-word;
  overflow-wrap: break-word;
  line-height: 1.4;
}
```

### 6. 交互功能

#### 资源行点击

```typescript
onClick={() => navigate(`/workspaces/${workspaceId}/resources/${resource.id}`)}
```

#### View all按钮

```typescript
onClick={(e) => {
  e.stopPropagation();
  onTabChange('resources');
}}
```

### 7. 自动刷新

集成到现有的5秒定时刷新机制中：

```typescript
const interval = setInterval(() => {
  const refreshData = async () => {
    // ...
    await fetchGlobalResources();
  };
  refreshData();
}, 5000);
```

## 问题修复记录

### 问题1：资源列表不显示

**原因**：显示条件依赖`overview.resource_count > 0`，但该字段值为0
**解决**：改为基于`globalResources.length > 0`判断

### 问题2：资源数量显示为0

**原因**：使用了`overview.resource_count`或`globalResources.length`
**解决**：使用API返回的`pagination.total`字段（`globalResourcesTotal`）

### 问题3：资源名称过长占用其他列

**原因**：缺少换行和溢出处理
**解决**：添加`word-break: break-word`和`overflow-wrap: break-word`

### 问题4：View all按钮不跳转

**原因**：使用`navigate()`但没有触发标签页切换
**解决**：改为调用`onTabChange('resources')`

### 问题5：表头和数据列不对齐

**原因**：CSS中有两个`.tableHeader`定义冲突，States的`display: flex`覆盖了Resources的`display: grid`
**解决**：将States的表头类名改为`.statesTableHeader`

## 文件修改

### 修改的文件

1. `frontend/src/pages/WorkspaceDetail.tsx`
   - 添加`globalResources`和`globalResourcesTotal`状态
   - 创建`fetchGlobalResources`函数
   - 更新Overview标签页显示逻辑
   - 传递`onTabChange`函数给OverviewTab

2. `frontend/src/pages/WorkspaceDetail.module.css`
   - 添加资源表格样式
   - 添加资源行样式
   - 添加View all按钮样式
   - 修复表头对齐问题（重命名States表头类）

## 数据流

```
初始加载 & 每5秒刷新
  ↓
fetchGlobalResources()
  ↓
API: GET /workspaces/12/resources?page=1&page_size=5&...
  ↓
Response: {
  resources: [前5个资源],
  pagination: { total: X }
}
  ↓
State更新:
- globalResources = [前5个资源]
- globalResourcesTotal = X
  ↓
显示:
- 全局头部: "Resources X"
- Overview标题: "Resources X"
- 资源列表: 显示前5个
- View all: "View all X resources →" (当X > 5)
```

## 测试验证

-  资源列表正确显示前5个资源
-  资源数量准确显示（来自API的pagination.total）
-  资源名称过长时自动换行
-  表头和数据列完美对齐
-  点击资源行跳转到详情页
-  点击View all按钮跳转到Resources标签页
-  自动刷新功能正常工作
-  与Resources标签页样式一致

## 总结

成功实现了Overview标签页Resources部分的实际数据显示，提供了良好的用户体验和与其他标签页一致的交互方式。通过解决多个技术问题（数据源、换行、对齐、跳转等），最终实现了完整且稳定的功能。
