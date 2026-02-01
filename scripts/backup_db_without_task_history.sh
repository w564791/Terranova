#!/bin/bash

# PostgreSQL数据库备份脚本
# 排除task历史记录表

# 数据库连接信息
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-iac_platform}"
DB_USER="${DB_USER:-postgres}"
DB_PASSWORD="${DB_PASSWORD:-postgres123}"

# 备份文件名（带时间戳）
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_DIR="${BACKUP_DIR:-./backups}"
BACKUP_FILE="${BACKUP_DIR}/iac_platform_backup_${TIMESTAMP}.sql"

# 创建备份目录
mkdir -p "$BACKUP_DIR"

# 需要排除的task历史记录表
EXCLUDE_TABLES=(
    # Task执行历史记录
    "workspace_tasks"
    "task_logs"
    "task_comments"
    "task_temporary_permissions"
    "workspace_task_resource_changes"
    "workspace_state_versions"
    "workspace_resources_snapshot"
    
    # Run Task执行结果
    "run_task_outcomes"
    "run_task_results"
    
    # AI分析历史
    "ai_error_analyses"
    "ai_parse_tasks"
    "ai_analysis_rate_limits"
    
    # 扫描结果
    "scan_results"
    
    # 日志类表
    "access_logs"
    "agent_access_logs"
    "audit_logs"
    "permission_audit_logs"
    "webhook_logs"
)

# 构建排除表的参数
EXCLUDE_ARGS=""
for table in "${EXCLUDE_TABLES[@]}"; do
    EXCLUDE_ARGS="$EXCLUDE_ARGS --exclude-table-data=$table"
done

echo "=========================================="
echo "PostgreSQL 数据库备份"
echo "=========================================="
echo "数据库: $DB_NAME"
echo "主机: $DB_HOST:$DB_PORT"
echo "备份文件: $BACKUP_FILE"
echo ""
echo "排除的表（仅排除数据，保留表结构）:"
for table in "${EXCLUDE_TABLES[@]}"; do
    echo "  - $table"
done
echo "=========================================="

# 设置密码环境变量
export PGPASSWORD="$DB_PASSWORD"

# 执行备份
echo ""
echo "开始备份..."
pg_dump \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    --no-owner \
    --no-acl \
    $EXCLUDE_ARGS \
    -f "$BACKUP_FILE"

# 检查备份结果
if [ $? -eq 0 ]; then
    # 获取文件大小
    FILE_SIZE=$(ls -lh "$BACKUP_FILE" | awk '{print $5}')
    echo ""
    echo "=========================================="
    echo "备份成功!"
    echo "文件: $BACKUP_FILE"
    echo "大小: $FILE_SIZE"
    echo "=========================================="
    
    # 可选：压缩备份文件
    echo ""
    echo "正在压缩备份文件..."
    gzip -f "$BACKUP_FILE"
    COMPRESSED_FILE="${BACKUP_FILE}.gz"
    COMPRESSED_SIZE=$(ls -lh "$COMPRESSED_FILE" | awk '{print $5}')
    echo "压缩后文件: $COMPRESSED_FILE"
    echo "压缩后大小: $COMPRESSED_SIZE"
else
    echo ""
    echo "=========================================="
    echo "备份失败!"
    echo "=========================================="
    exit 1
fi

# 清理密码环境变量
unset PGPASSWORD

echo ""
echo "备份完成!"
