#!/bin/bash
# 监控任务 638 的 plan_hash 和 plan_task_id

echo "监控任务 638..."
echo "按 Ctrl+C 停止监控"
echo ""

while true; do
    clear
    echo "=== 任务 638 状态 ==="
    echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""
    
    docker exec -i iac-platform-postgres psql -U postgres -d iac_platform -c "
    SELECT 
        id,
        task_type,
        status,
        stage,
        plan_task_id,
        CASE 
            WHEN plan_hash IS NULL OR plan_hash = '' THEN '❌ 空'
            ELSE ' ' || LEFT(plan_hash, 16) || '...'
        END as plan_hash_status,
        CASE 
            WHEN plan_task_id IS NULL THEN '❌ 空'
            ELSE ' ' || plan_task_id::text
        END as plan_task_id_status
    FROM workspace_tasks 
    WHERE id = 638;
    "
    
    echo ""
    echo "等待 5 秒后刷新..."
    sleep 5
done
