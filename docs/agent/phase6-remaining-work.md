# Phase 6 剩余工作 - Agent Mode plan_data 上传

## 当前状态

###  已完成
1. Phase 6 核心重构（20+ 处）
2. Local 模式 plan_data/plan_json 保存修复
3. Agent 模式 Resource-Changes 实现
4. AgentAPIClient.UploadPlanData() 方法

###  待完成
需要在服务器端添加接收 plan_data 的 API 端点

## 需要添加的代码

### 1. 在 `backend/internal/handlers/agent_handler.go` 添加方法

```go
// UploadPlanData receives plan_data from agent
func (h *AgentHandler) UploadPlanData(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	var taskID uint
	fmt.Sscanf(taskIDStr, "%d", &taskID)

	var req struct {
		PlanData string `json:"plan_data"` // base64 encoded
	}
	c.ShouldBindJSON(&req)

	// Decode base64
	planData, _ := base64.StdEncoding.DecodeString(req.PlanData)

	// Save to database
	h.db.Model(&models.WorkspaceTask{}).Where("id = ?", taskID).Update("plan_data", planData)

	c.JSON(http.StatusOK, gin.H{"message": "plan_data saved"})
}
```

### 2. 在 `backend/internal/router/router_agent.go` 添加路由

```go
agentRoutes.POST("/tasks/:task_id/plan-data", agentHandler.UploadPlanData)
```

### 3. 修改 `GetPlanTask` API 返回 plan_data

```go
func (h *AgentHandler) GetPlanTask(c *gin.Context) {
	// ... 现有代码 ...
	
	// 添加 plan_data 到响应
	c.JSON(http.StatusOK, gin.H{
		"task": gin.H{
			"id": task.ID,
			"workspace_id": task.WorkspaceID,
			"task_type": task.TaskType,
			"context": task.Context,
			"plan_data": task.PlanData, // 添加这行
		},
	})
}
```

## 测试步骤

1. 添加上述代码
2. 重启服务
3. 创建 Plan 任务
4. 验证 plan_data 已保存
5. 执行 Apply 任务
6. 验证 Apply 可以获取 plan_data

## 完成后

Agent 模式将完全支持：
-  Plan 执行
-  Apply 执行（需要 plan_data）
-  Resource-Changes 视图
-  完整的 Plan+Apply 流程
