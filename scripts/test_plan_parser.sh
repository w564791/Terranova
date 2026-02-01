#!/bin/bash

# 测试Plan解析功能
# 用法: ./test_plan_parser.sh <task_id>

TASK_ID=${1:-203}

echo "=========================================="
echo "Testing Plan Parser for Task $TASK_ID"
echo "=========================================="
echo ""

# 1. 检查task是否存在
echo "1. Checking if task exists..."
psql -h localhost -U postgres -d iac_platform -t -c "SELECT id, workspace_id, task_type, status, LENGTH(plan_data) as plan_data_size FROM workspace_tasks WHERE id = $TASK_ID;"

# 2. 检查是否已有解析数据
echo ""
echo "2. Checking existing parsed data..."
COUNT=$(psql -h localhost -U postgres -d iac_platform -t -c "SELECT COUNT(*) FROM workspace_task_resource_changes WHERE task_id = $TASK_ID;" | tr -d ' ')
echo "Found $COUNT resource changes"

# 3. 如果没有数据，提示需要重新运行Plan或手动触发解析
if [ "$COUNT" = "0" ]; then
    echo ""
    echo "  No parsed data found for task $TASK_ID"
    echo ""
    echo "Possible reasons:"
    echo "  1. Task was created before the new feature was implemented"
    echo "  2. Async parsing hasn't completed yet"
    echo "  3. Backend service needs restart"
    echo ""
    echo "Solutions:"
    echo "  1. Run a new Plan task to test the feature"
    echo "  2. Restart backend service: cd backend && go run main.go"
    echo "  3. Check backend logs for parsing errors"
else
    echo ""
    echo "3. Showing parsed resource changes..."
    psql -h localhost -U postgres -d iac_platform -c "SELECT id, resource_address, resource_type, action, apply_status FROM workspace_task_resource_changes WHERE task_id = $TASK_ID ORDER BY id LIMIT 20;"
    
    echo ""
    echo "4. Summary statistics..."
    psql -h localhost -U postgres -d iac_platform -c "SELECT action, COUNT(*) as count FROM workspace_task_resource_changes WHERE task_id = $TASK_ID GROUP BY action;"
fi

echo ""
echo "=========================================="
echo "Test completed"
echo "=========================================="
