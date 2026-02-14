#!/bin/bash

# 修复最后的编译错误

cd /Users/ken/go/src/iac-platform/backend

echo "修复 workspace_task_controller.go..."
# 将 uint(workspaceID) 改为 workspaceID (因为现在是 string)
sed -i.bak 's/WorkspaceID:   uint(workspaceID),/WorkspaceID:   workspaceID,/g' controllers/workspace_task_controller.go
sed -i.bak 's/TryExecuteNextTask(uint(workspaceID))/TryExecuteNextTask(workspaceID)/g' controllers/workspace_task_controller.go

echo "修复 workspace_variable_controller.go..."
sed -i.bak 's/WorkspaceID:  uint(workspaceID),/WorkspaceID:  workspaceID,/g' controllers/workspace_variable_controller.go

echo "修复 state_version_controller.go..."
# 需要手动检查 Line 105

echo "完成！请运行: cd backend && go build ."
