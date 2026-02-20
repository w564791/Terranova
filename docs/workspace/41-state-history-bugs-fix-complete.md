# State History显示Bug修复完成

## 修复日期
2025-10-12

## 修复的Bug

### Bug 1: Download按钮有emoji 
**位置：** `frontend/src/pages/StatesTab.tsx`
**修复：**
- 第148行：`📥 Download` 改为 `Download`
- 第195行：`📥` 改为 `Download`

### Bug 2: 下载State失败 
**位置：** `backend/controllers/state_version_controller.go`
**问题：** GetStateVersion方法返回JSON对象而不是文件blob
**修复：** 第68-110行
```go
// 返回文件内容用于下载
c.Header("Content-Type", "application/json")
c.Header("Content-Disposition", "attachment; filename=terraform-state-v"+strconv.Itoa(version)+".json")
c.JSON(http.StatusOK, stateVersion.Content)
```

### Bug 3: State数据解析错误 

#### 3.1 GetStateVersions返回格式修复
**位置：** `backend/controllers/state_version_controller.go` 第28-52行
**修复前：**
```go
c.JSON(http.StatusOK, gin.H{
    "code": 200,
    "data": versions,  // 直接返回数组
})
```

**修复后：**
```go
// 获取总数
var count int64
svc.db.Model(&models.WorkspaceStateVersion{}).
    Where("workspace_id = ?", workspaceID).
    Count(&count)

c.JSON(http.StatusOK, gin.H{
    "code":      200,
    "items":     versions,
    "total":     count,
    "timestamp": time.Now().Format(time.RFC3339),
})
```

#### 3.2 GetCurrentState返回正确数据
**位置：** `backend/controllers/state_version_controller.go` 第113-145行
**修复前：** 返回`workspace.TFState`（完整的terraform state JSON）
**修复后：** 返回格式化的state version数据
```go
// 查询最新的state version记录
var stateVersion models.WorkspaceStateVersion
if err := svc.db.Where("workspace_id = ?", workspaceID).
    Order("version DESC").
    First(&stateVersion).Error; err != nil {
    // 处理错误
}

// 从content中提取terraform_version和resources_count
var terraformVersion string
var resourcesCount int
var serial int

if stateVersion.Content != nil {
    if tfVer, ok := stateVersion.Content["terraform_version"].(string); ok {
        terraformVersion = tfVer
    }
    if resources, ok := stateVersion.Content["resources"].([]interface{}); ok {
        resourcesCount = len(resources)
    }
    if ser, ok := stateVersion.Content["serial"].(float64); ok {
        serial = int(ser)
    }
}

// 返回格式化的响应
c.JSON(http.StatusOK, gin.H{
    "code": 200,
    "data": gin.H{
        "id":                stateVersion.ID,
        "version":           strconv.Itoa(stateVersion.Version),
        "serial":            serial,
        "terraform_version": terraformVersion,
        "resources_count":   resourcesCount,
        "created_at":        stateVersion.CreatedAt,
        "is_current":        true,
    },
    "timestamp": time.Now().Format(time.RFC3339),
})
```

## 修改的文件

### 后端
- `backend/controllers/state_version_controller.go`
  - GetStateVersions方法：返回items/total格式
  - GetCurrentState方法：返回格式化的state version数据
  - GetStateVersion方法：返回文件blob用于下载

### 前端
- `frontend/src/pages/StatesTab.tsx`
  - 移除Download按钮的emoji

## 预期结果

修复后，State History标签页应该正确显示：

### Current State
- Version: 1（数据库的version字段）
- Serial: 0（从content中提取）
- Terraform Version: 1.5.7（从content中提取）
- Resources: 5（从content.resources数组长度计算）
- Created: 刚刚（正确的相对时间）

### State History列表
- 显示所有历史版本
- 每个版本显示正确的version、serial、terraform_version、resources_count
- Created时间显示正确的相对时间

### 下载功能
- 点击Download按钮正常下载state文件
- 文件名格式：`terraform-state-v{version}.json`
- 文件内容为完整的terraform state JSON

## 测试建议

1. 刷新State History页面，验证Current State显示正确
2. 验证State History列表显示正确的数据
3. 测试下载功能，确认文件正常下载
4. 验证相对时间显示正确（刚刚、X分钟前、X小时前、X天前）

## 注意事项

- 后端已经在运行（端口8080），修改会在下次重启时生效
- 如果需要立即测试，需要重启后端服务
- 前端修改会在页面刷新后生效

## 相关文档
- Bug分析文档：`docs/workspace/40-state-history-bugs-fix.md`
- State管理设计：`docs/workspace/03-state-management.md`
