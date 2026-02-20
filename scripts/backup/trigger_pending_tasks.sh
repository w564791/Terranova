#!/bin/bash

# 手动触发pending任务执行的脚本
# 通过调用API来触发workspace的任务执行

WORKSPACE_ID="ws-mb7m9ii5ey"
API_URL="http://localhost:8080"

echo "Triggering task execution for workspace: $WORKSPACE_ID"

# 调用一个会触发TryExecuteNextTask的API
# 例如创建一个新的plan任务会触发
curl -X POST "${API_URL}/api/v1/workspaces/${WORKSPACE_ID}/tasks/plan" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE" \
  -d '{
    "description": "Manual trigger test"
  }'

echo ""
echo "Task execution triggered. Check logs for details."
