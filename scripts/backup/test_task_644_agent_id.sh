#!/bin/bash

# 测试任务 644 的 agent_id 数据流

echo "========== 1. 检查数据库中任务 644 的 agent_id =========="
psql -U postgres -d iac_platform -c "SELECT id, task_type, status, agent_id, plan_task_id FROM workspace_tasks WHERE id = 644;"

echo ""
echo "========== 2. 测试 GetPlanTask API (任务 644) =========="
curl -s -X GET "http://localhost:8080/api/agent/tasks/644/plan" \
  -H "Authorization: Bearer YOUR_POOL_TOKEN" | jq '.'

echo ""
echo "========== 3. 检查 Agent 日志中的 GetPlanTask 调用 =========="
echo "请查看 Agent 日志，搜索 'GetPlanTask' 和任务 644"
