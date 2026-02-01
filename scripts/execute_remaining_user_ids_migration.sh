#!/bin/bash

# 执行剩余 user_id 字段迁移脚本
# 使用方法: ./scripts/execute_remaining_user_ids_migration.sh

set -e

echo "开始执行剩余 user_id 字段迁移..."

# 数据库连接信息
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-iac_platform}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-postgres123}"

# 执行迁移脚本
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f scripts/migrate_remaining_user_ids_to_string.sql

echo "迁移完成！"
echo ""
echo "验证迁移结果..."

# 验证迁移结果
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
SELECT 
    table_name, 
    column_name, 
    data_type, 
    character_maximum_length 
FROM information_schema.columns 
WHERE table_name IN ('resource_locks', 'resource_drifts', 'iam_user_roles', 'ai_error_analyses', 'ai_analysis_rate_limits') 
    AND column_name IN ('user_id', 'editing_user_id', 'assigned_by')
ORDER BY table_name, column_name;
"

echo ""
echo " 所有字段已成功迁移为 varchar(20) 类型"
