# 资源编辑页面强制刷新问题修复报告

## 问题描述

在 `http://localhost:5173/workspaces/12/resources/33/edit` 页面，用户反馈每隔一段时间会出现强制刷新，然后提示是否恢复草稿，导致用户体验极差。

## 问题根因分析

通过代码审查发现，问题出在 `frontend/src/pages/EditResource.tsx` 的状态轮询机制：

### 1. 过于激进的轮询逻辑

原代码每 **5秒** 轮询一次编辑状态，检查当前会话是否仍然有效：

```typescript
statusPollTimerRef.current = window.setInterval(async () => {
  const status = await ResourceEditingService.getEditingStatus(...);
  
  // 问题：一旦找不到当前session，立即判定为被接管
  if (status.editors.length > 0 && !currentSession && !editingDisabled) {
    setEditingDisabled(true);
    showToast('编辑已被其他窗口接管', 'warning');
  }
}, 5000); // 5秒轮询
```

### 2. 缺乏容错机制

原代码存在以下问题：

- **没有重试机制**：网络抖动或临时的后端问题会立即触发"被接管"警告
- **轮询间隔过短**：5秒一次的轮询给服务器带来不必要的压力
- **单次失败即判定**：没有考虑临时性故障，一次失败就认为session丢失

### 3. 用户体验问题

当状态轮询失败或检测到session不存在时：
- 页面会禁用编辑功能
- 显示"编辑已被其他窗口接管"的警告
- 用户被迫刷新页面
- 刷新后会提示恢复草稿

这个循环导致了"强制刷新"的用户体验。

## 解决方案

### 1. 增加重试机制

引入 `consecutiveFailures` 计数器，只有连续多次失败才认为session真的丢失：

```typescript
const MAX_CONSECUTIVE_FAILURES = 3; // 连续3次失败才认为session丢失
let consecutiveFailures = 0; // 必须在setInterval外部声明

statusPollTimerRef.current = window.setInterval(async () => {
  try {
    const status = await ResourceEditingService.getEditingStatus(...);
    
    // 成功时重置失败计数
    consecutiveFailures = 0;
    
    // ... 正常处理逻辑
  } catch (error) {
    consecutiveFailures++;
    
    // 只有连续多次失败才认为session真的丢失了
    if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES && !editingDisabled) {
      setEditingDisabled(true);
      showToast('编辑会话已断开,请刷新页面重新编辑', 'warning');
    }
  }
}, 10000); // 改为10秒轮询一次
```

### 2. 延长轮询间隔

将轮询间隔从 **5秒** 延长到 **10秒**：
- 减少服务器压力
- 降低因临时网络问题导致的误判
- 仍然能及时检测到真正的session冲突

### 3. 改进错误提示

将错误提示从"编辑已被其他窗口接管"改为更准确的"编辑会话已断开,请刷新页面重新编辑"，让用户明白这可能是网络问题而不是真的被接管。

## 修改的文件

- `frontend/src/pages/EditResource.tsx`

## 关键改进点

1. **容错性提升**：从单次失败判定改为连续3次失败才判定
2. **性能优化**：轮询间隔从5秒延长到10秒
3. **用户体验**：减少误判导致的编辑中断
4. **错误提示**：更准确的错误信息

## 测试建议

1. **正常编辑流程**：验证正常编辑不会被中断
2. **网络抖动测试**：模拟短暂的网络中断，验证不会触发误判
3. **真实接管测试**：在另一个窗口打开同一资源，验证接管机制仍然有效
4. **长时间编辑**：验证长时间编辑不会出现异常刷新

## 预期效果

- 用户在正常编辑时不会再遇到"强制刷新"问题
- 临时的网络问题不会导致编辑中断
- 真正的session冲突仍然能被正确检测
- 服务器负载降低（轮询频率减半）

## 后续优化建议

1. 考虑使用 WebSocket 替代轮询，实现实时的编辑状态同步
2. 增加用户可配置的轮询间隔选项
3. 在网络恢复后自动重新建立session，而不是要求用户刷新
4. 添加更详细的日志记录，便于排查问题
