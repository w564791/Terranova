#!/bin/bash

# 手动触发任务546执行
# 通过创建一个临时任务来触发TryExecuteNextTask

WORKSPACE_ID="ws-mb7m9ii5ey"

echo "Manually triggering task execution for workspace $WORKSPACE_ID"
echo "This will call TryExecuteNextTask which should pick up task 546"

# 使用curl调用创建任务API,这会触发TryExecuteNextTask
# 然后立即取消这个临时任务
curl -X POST "http://localhost:8080/api/v1/workspaces/$WORKSPACE_ID/tasks/plan" \
  -H "Content-Type: application/json" \
  -d '{"description":"trigger"}' 2>/dev/null | jq -r '.id' | while read task_id; do
  if [ ! -z "$task_id" ] && [ "$task_id" != "null" ]; then
    echo "Created temporary task $task_id, cancelling it..."
    curl -X POST "http://localhost:8080/api/v1/workspaces/$WORKSPACE_ID/tasks/$task_id/cancel" 2>/dev/null
  fi
done

echo "Done. Check if task 546 started executing."
