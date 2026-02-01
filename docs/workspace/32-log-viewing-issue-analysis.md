# 日志查看问题深度分析

## 问题描述
从用户截图看到：
1. 任务状态显示为"CURRENT"（running）
2. 显示的是StageLogViewer的Tab（Pending, Fetching, Init等）
3. 所有Tab都是灰色（disabled状态）
4. 黑色日志区域没有内容

## 问题分析

### 可能的原因

#### 原因1: SmartLogViewer状态判断问题
```tsx
// SmartLogViewer.tsx
if (taskStatus === 'running' || taskStatus === 'pending' || 
    taskStatus === 'waiting' || taskStatus === 'apply_pending') {
  return <TerraformOutputViewer taskId={taskId} />; // 应该显示这个
}

// 否则显示
return <StageLogViewer taskId={taskId} taskType={taskType} />; // 但实际显示了这个
```

**可能性**: taskStatus没有正确获取或更新

#### 原因2: 页面没有刷新
用户Confirm Apply后，页面可能没有重新渲染SmartLogViewer

#### 原因3: WebSocket连接问题
TerraformOutputViewer的WebSocket可能没有连接成功

#### 原因4: 任务状态更新延迟
数据库中任务状态已更新为running，但前端还没有获取到最新状态

## 调试步骤

### Step 1: 检查SmartLogViewer的状态获取
```tsx
// 在SmartLogViewer中添加调试日志
console.log('SmartLogViewer - taskStatus:', taskStatus);
console.log('SmartLogViewer - taskType:', taskType);
console.log('SmartLogViewer - using viewer:', 
  (taskStatus === 'running') ? 'TerraformOutputViewer' : 'StageLogViewer'
);
```

### Step 2: 检查TaskDetail的刷新逻辑
```tsx
// handleConfirmApply中
await api.post(...);
fetchTask(); // 这个会刷新task状态
// 但SmartLogViewer是否会重新渲染？
```

### Step 3: 检查WebSocket连接
```tsx
// useTerraformOutput hook
console.log('WebSocket URL:', wsUrl);
console.log('WebSocket connected:', isConnected);
console.log('Lines received:', lines.length);
```

## 解决方案

### 方案1: 强制刷新SmartLogViewer
在TaskDetail的fetchTask后，强制重新挂载SmartLogViewer：

```tsx
const [logViewerKey, setLogViewerKey] = useState(0);

const fetchTask = async () => {
  // ... 现有代码 ...
  setTask(taskData);
  setLogViewerKey(prev => prev + 1); // 强制重新挂载
};

// 在渲染时
<SmartLogViewer key={logViewerKey} taskId={parseInt(taskId!)} />
```

### 方案2: 改进SmartLogViewer的状态轮询
```tsx
useEffect(() => {
  fetchTaskStatus();
  
  // 更频繁的轮询
  const interval = setInterval(() => {
    fetchTaskStatus();
  }, 2000); // 从5秒改为2秒

  return () => clearInterval(interval);
}, [taskId]); // 移除taskStatus依赖
```

### 方案3: 添加手动刷新按钮
在日志查看器中添加刷新按钮，让用户可以手动刷新：

```tsx
<button onClick={() => window.location.reload()}>
  🔄 刷新页面
</button>
```

### 方案4: 使用React Context共享状态
创建TaskContext，在TaskDetail和SmartLogViewer之间共享任务状态：

```tsx
// TaskContext.tsx
const TaskContext = createContext<{
  task: Task | null;
  refreshTask: () => void;
}>(null);

// TaskDetail.tsx
<TaskContext.Provider value={{ task, refreshTask: fetchTask }}>
  <SmartLogViewer taskId={taskId} />
</TaskContext.Provider>

// SmartLogViewer.tsx
const { task } = useContext(TaskContext);
const taskStatus = task?.status || '';
```

## 推荐方案

### 立即实施：方案1 + 方案2

1. **在TaskDetail中强制刷新SmartLogViewer**
   - 添加key prop
   - fetchTask后更新key

2. **改进SmartLogViewer的轮询**
   - 缩短轮询间隔到2秒
   - 移除taskStatus依赖避免轮询停止

3. **添加调试日志**
   - 在SmartLogViewer中打印状态
   - 在控制台查看实际状态

## 临时解决方案

用户可以：
1. Confirm Apply后，手动刷新页面（F5）
2. 或者重新进入任务详情页

## 根本解决方案

需要实现：
1. TaskDetail和SmartLogViewer之间的状态同步
2. 更可靠的状态轮询机制
3. WebSocket连接状态监控
4. 自动重连机制

## 测试验证

### 测试场景1: Plan阶段
1. 创建plan_and_apply任务
2. 立即进入任务详情页
3. 检查是否显示TerraformOutputViewer
4. 检查WebSocket是否连接
5. 检查是否有实时日志

### 测试场景2: Confirm Apply后
1. Plan完成后点击Confirm Apply
2. 检查页面是否自动刷新
3. 检查是否切换到TerraformOutputViewer
4. 检查Apply日志是否实时显示

### 测试场景3: Apply完成后
1. Apply完成
2. 检查是否切换到StageLogViewer
3. 检查所有Tab是否可点击
4. 检查历史日志是否完整

## 下一步行动

1. 实施方案1：添加key prop强制刷新
2. 实施方案2：改进轮询机制
3. 添加调试日志
4. 测试验证
5. 如果还有问题，考虑方案4（Context）
