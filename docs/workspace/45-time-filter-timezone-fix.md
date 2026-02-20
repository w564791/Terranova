# Time Filter Timezone Fix

## 问题描述

在Workspace Detail页面的Runs标签页中，时间过滤器存在严重问题：

### 症状
- 点击"All Time"时，Latest Run正确显示#180（Apply Pending）
- 点击"Today"时，Latest Run显示#156（Cancelled），但#180应该也在今天创建
- Run List中的数据在不同时间过滤器下显示不一致
- "Today"过滤器应该包含"All Time"中的最新数据，但实际上缺失了

### 根本原因

**时区处理问题**：
1. 前端发送ISO 8601格式的UTC时间（如 `2025-10-14T00:00:00.000Z`）
2. 后端数据库存储的是本地时间（无时区信息）
3. 后端直接使用字符串比较时间，导致时区不匹配
4. 这导致时间范围查询结果不正确

## 修复方案

### 1. 前端修复 (WorkspaceDetail.tsx)

#### 问题1: Latest Run状态更新
**原因**：`fetchCurrentRun()`中的ref比较逻辑有缺陷，在更新state之前就更新了ref，导致React可能跳过重新渲染。

**修复**：
```typescript
const fetchCurrentRun = async () => {
  // ... 获取数据 ...
  
  if (needsAttentionTask) {
    // 只在真正改变时更新state
    const prev = prevCurrentRunRef.current;
    if (!prev || prev.id !== needsAttentionTask.id || prev.status !== needsAttentionTask.status) {
      console.log('Updating currentRun to needs attention task:', needsAttentionTask.id);
      setCurrentRun(needsAttentionTask);  // 先更新state
      prevCurrentRunRef.current = needsAttentionTask;  // 再更新ref
    }
    return;
  }
  // ... 其他情况类似处理 ...
};
```

### 2. 后端修复 (workspace_task_controller.go)

#### 问题2: 时间范围过滤不正确
**原因**：后端直接使用字符串比较时间，没有考虑时区转换。

**修复**：
```go
// 时间范围过滤 - 正确处理时区
startDate := ctx.Query("start_date")
if startDate != "" {
    // 解析ISO 8601时间字符串
    startTime, err := time.Parse(time.RFC3339, startDate)
    if err != nil {
        log.Printf("Failed to parse start_date: %v", err)
    } else {
        // 转换为本地时区进行比较
        localStartTime := startTime.Local()
        query = query.Where("created_at >= ?", localStartTime)
        log.Printf("Time filter: start_date=%s (UTC) -> %s (Local)", startDate, localStartTime.Format(time.RFC3339))
    }
}
endDate := ctx.Query("end_date")
if endDate != "" {
    // 解析ISO 8601时间字符串
    endTime, err := time.Parse(time.RFC3339, endDate)
    if err != nil {
        log.Printf("Failed to parse end_date: %v", err)
    } else {
        // 转换为本地时区进行比较
        localEndTime := endTime.Local()
        query = query.Where("created_at <= ?", localEndTime)
        log.Printf("Time filter: end_date=%s (UTC) -> %s (Local)", endDate, localEndTime.Format(time.RFC3339))
    }
}
```

#### 问题3: 辅助函数也需要修复
**修复 `applySearchAndTimeFilters` 函数**：
```go
func applySearchAndTimeFilters(search, startDate, endDate string) func(*gorm.DB) *gorm.DB {
    return func(db *gorm.DB) *gorm.DB {
        if search != "" {
            searchPattern := "%" + search + "%"
            db = db.Where(
                "description LIKE ? OR CAST(id AS TEXT) LIKE ? OR task_type LIKE ?",
                searchPattern, searchPattern, searchPattern,
            )
        }
        if startDate != "" {
            // 解析并转换为本地时区
            if startTime, err := time.Parse(time.RFC3339, startDate); err == nil {
                db = db.Where("created_at >= ?", startTime.Local())
            }
        }
        if endDate != "" {
            // 解析并转换为本地时区
            if endTime, err := time.Parse(time.RFC3339, endDate); err == nil {
                db = db.Where("created_at <= ?", endTime.Local())
            }
        }
        return db
    }
}
```

## 修改的文件

### 前端
- `frontend/src/pages/WorkspaceDetail.tsx`
  - 修复 `fetchCurrentRun()` 函数的ref比较逻辑
  - 添加调试日志以追踪状态更新

### 后端
- `backend/controllers/workspace_task_controller.go`
  - 修复 `GetTasks()` 函数的时间范围过滤逻辑
  - 修复 `applySearchAndTimeFilters()` 辅助函数
  - 添加时区转换和调试日志

## 测试验证

### 测试步骤
1. 访问 `http://localhost:5173/workspaces/12?tab=runs&filter=all&timeFilter=all`
2. 点击"All Time"，验证Latest Run显示#180
3. 点击"Today"，验证Latest Run仍然显示#180（不是#156）
4. 验证Run List中的数据在不同时间过滤器下正确显示
5. 检查浏览器控制台的调试日志，确认状态更新正常
6. 检查后端日志，确认时区转换正确

### 预期结果
- Latest Run始终显示最新的任务（#180），不受时间过滤器影响
- "Today"过滤器正确包含今天创建的所有任务
- 时间范围查询结果准确，不会因为时区问题遗漏数据

## 技术要点

### 时区处理最佳实践
1. **前端**：使用 `toISOString()` 发送UTC时间
2. **后端**：
   - 使用 `time.Parse(time.RFC3339, dateString)` 解析ISO 8601格式
   - 使用 `.Local()` 转换为本地时区
   - 使用转换后的时间对象进行数据库查询
3. **数据库**：GORM会自动处理时间类型的比较

### React状态更新最佳实践
1. 先比较新旧值，确认是否真的需要更新
2. 先调用 `setState()`，再更新ref
3. 使用ref存储上一次的值，避免不必要的重新渲染
4. 添加调试日志以追踪状态变化

## 影响范围

### 直接影响
- Workspace Detail页面的Runs标签页
- Latest Run显示逻辑
- Run List时间过滤功能

### 间接影响
- 所有使用时间范围查询的API端点都应该采用相同的时区处理方式
- 其他页面如果有类似的时间过滤功能，也应该检查是否存在相同问题

## 后续建议

1. **统一时区处理**：在整个项目中统一时区处理方式
2. **添加时区配置**：考虑添加系统级别的时区配置
3. **API文档更新**：在API文档中明确说明时间参数的格式和时区要求
4. **测试覆盖**：添加时区相关的单元测试和集成测试

## 总结

这次修复解决了两个关键问题：
1. **前端状态更新问题**：通过正确的ref比较逻辑，确保Latest Run状态正确更新
2. **后端时区处理问题**：通过正确解析和转换时区，确保时间范围查询准确

修复后，时间过滤器功能完全正常，Latest Run始终显示最新任务，Run List数据准确无误。

## 后续改进：Overview和Runs标签页Latest Run统一

### 问题
- Overview标签页的Latest Run缺少description字段显示
- 两个标签页使用不同的数据源，可能导致不一致

### 解决方案
创建全局Latest Run状态，让两个标签页共享：

1. **添加全局状态**：
```typescript
const [globalLatestRun, setGlobalLatestRun] = useState<any>(null);
const prevGlobalLatestRunRef = React.useRef<any>(null);
```

2. **创建统一获取函数**：
```typescript
const fetchGlobalLatestRun = React.useCallback(async () => {
  // 获取所有任务（不带时间过滤）
  // 应用相同的优先级逻辑
  // 更新全局状态
}, [id]);
```

3. **Overview标签页使用全局Latest Run**：
```typescript
<OverviewTab 
  overview={overview} 
  workspace={workspace} 
  globalLatestRun={globalLatestRun} 
/>
```

4. **显示逻辑完全一致**：
- 支持description字段显示
- 使用相同的状态显示函数
- 使用相同的样式类名（currentRun）

### 效果
-  Overview和Runs标签页Latest Run完全一致
-  支持description字段显示
-  数据源统一，避免不一致
-  定时刷新确保数据同步

---

**修复日期**: 2025-10-14  
**修复人员**: AI Assistant  
**测试状态**:  已验证通过  
**Git Commits**: 
- `b002a84` - 时区处理修复
- `73c5344` - Overview和Runs标签页Latest Run统一
