#!/bin/bash
# 测试 GetPlanTask API 是否返回 plan_hash 和 agent_id

TASK_ID=642
TOKEN="apt_pool-abcdefghijklmnop_50a26ac346864671cef8c53add6048fae38e6b0d388e065a093ee1bbb198916e"

echo "测试 GetPlanTask API..."
echo "任务 ID: $TASK_ID"
echo ""

curl -s -X GET "http://localhost:8080/api/v1/agents/tasks/$TASK_ID/plan-task" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" | jq '.task | {id, agent_id, plan_hash, plan_task_id}'

echo ""
echo "如果看到 agent_id 和 plan_hash 有值，说明 API 正常"
echo "如果是 null，说明服务器还是旧代码，需要重新部署"
