# State History显示Bug修复

## 发现的问题

### Bug 1: Download按钮有emoji
前端StatesTab.tsx中下载按钮显示"📥 Download"和"📥"

### Bug 2: 下载State失败
调用`/workspaces/:id/state-versions/:version`返回JSON而不是文件blob

### Bug 3: State数据解析错误
前端显示：
- Version: 4 (应该是1)
- Serial: 3 (错误)
- Resources: 空 (应该显示资源数量)
- Created: NaN天前 (日期解析失败)

## 根本原因

### 后端API返回格式问题

#### 1. GetStateVersions (state-versions列表)
**当前返回：**
```json
{
  "code": 200,
  "data": [...],  // 直接返回数组
  "timestamp": "..."
}
```

**前端期望：**
```json
{
  "items": [...],
  "total": 10
}
```

#### 2. GetCurrentState (current-state)
**当前返回：**
```json
{
  "code": 200,
  "data": {
    "version": 4,  // 这是terraform state的version字段
    "terraform_version": "1.5.7",
    "serial": 3,
    ...完整的terraform state JSON
  }
}
```

**前端期望：**
```json
{
  "id": 1,
  "version": "1",  // 这是我们数据库的version字段
  "serial": 0,
  "terraform_version": "1.5.7",
  "resources_count": 5,
  "created_at": "2025-10-12T15:32:21Z",
  "is_current": true
}
```

#### 3. GetStateVersion (下载)
**当前返回：** JSON格式的state version对象
**应该返回：** 文件blob (application/json)

## 解决方案

### 1. 修复GetStateVersions
- 返回格式改为`{items: [], total: count}`
- 添加total字段统计

### 2. 修复GetCurrentState  
- 查询最新的state version记录
- 从state content中提取terraform_version和resources
- 返回格式化的state version对象

### 3. 修复GetStateVersion (下载)
- 设置Content-Type为application/json
- 设置Content-Disposition为attachment
- 返回state content作为文件

### 4. 移除前端emoji
- 移除"📥 Download"中的emoji
- 移除表格中的"📥"图标

## 实施计划

1. 修复后端state_version_controller.go
2. 修复前端StatesTab.tsx移除emoji
3. 测试验证所有功能
