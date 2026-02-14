#!/bin/bash

# 修复 workspace_task_controller.go 中的 workspace ID 解析
# 将所有 ParseUint(ctx.Param("id") 改为直接使用字符串

cd /Users/ken/go/src/iac-platform/backend/controllers

# 备份
cp workspace_task_controller.go workspace_task_controller.go.backup

# 使用 perl 进行更精确的替换
perl -i -pe '
  # 替换 workspaceID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
  # 改为 workspaceID := ctx.Param("id")
  # 并在下一行添加 if workspaceID == "" { 检查
  s/workspaceID, err := strconv\.ParseUint\(ctx\.Param\("id"\), 10, 32\)\n\tif err != nil \{/workspaceID := ctx.Param("id")\n\tif workspaceID == "" {/g;
  
  # 替换 workspaceID, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
  s/workspaceID, _ := strconv\.ParseUint\(ctx\.Param\("id"\), 10, 32\)/workspaceID := ctx.Param("id")/g;
  
  # 替换函数调用中的 uint(workspaceID) 为 workspaceID
  # 但要确保不是在 uint(taskID) 的情况
  s/TryExecuteNextTask\(uint\(workspaceID\)\)/TryExecuteNextTask(workspaceID)/g;
  s/\.First\(&workspace, workspaceID\)/\.Where("workspace_id = ?", workspaceID).First(&workspace)/g;
' workspace_task_controller.go

echo "修复完成！"
echo "请检查编译: cd backend && go build ./..."
