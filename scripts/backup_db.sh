#!/bin/bash
#
# PostgreSQL 全量备份脚本（通过 Docker 容器内 pg_dump 执行）
# 备份所有数据（结构、索引、函数、触发器、扩展、序列、向量数据），排除 users 表和 task 相关表
#
# Usage:
#   ./scripts/backup_db.sh
#   BACKUP_DIR=/tmp/backups ./scripts/backup_db.sh
#   CONTAINER=my-postgres ./scripts/backup_db.sh

set -euo pipefail

# Docker 容器名
CONTAINER="${CONTAINER:-iac-platform-postgres-pg18}"

# 数据库信息（容器内部连接，host 固定 localhost）
DB_NAME="${DB_NAME:-iac_platform}"
DB_USER="${DB_USER:-postgres}"

# 本地备份目录
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_DIR="${BACKUP_DIR:-./backups}"
BACKUP_FILE="${DB_NAME}_${TIMESTAMP}.sql"
LOCAL_FILE="${BACKUP_DIR}/${BACKUP_FILE}"

mkdir -p "$BACKUP_DIR"

# 排除数据的表（保留表结构，不备份数据）
EXCLUDE_TABLES=(
    "users"
    "workspace_tasks"
    "task_logs"
    "task_comments"
    "task_temporary_permissions"
    "workspace_task_resource_changes"
    "workspace_state_versions"
    "workspace_resources_snapshot"
    "sso_login_logs"
    "user_identities"
    "login_sessions"
    "notification_logs"
    "pool_tokens"
    "cmdb_sync_logs"
    "mfa_tokens"
    "workspace_variables"
    "resource_code_versions"
    "resource_index"
    "run_task_outcomes"
    "run_task_results"
    "workspace_permissions"
    "workspace_resources"
)

# 构建 pg_dump 排除参数
EXCLUDE_ARGS=""
for table in "${EXCLUDE_TABLES[@]}"; do
    EXCLUDE_ARGS="$EXCLUDE_ARGS --exclude-table-data=$table"
done

echo "=========================================="
echo "PostgreSQL 数据库备份"
echo "=========================================="
echo "容器: ${CONTAINER}"
echo "数据库: ${DB_NAME}"
echo "输出: ${LOCAL_FILE}"
echo ""
echo "备份内容: 表结构、索引、约束、函数、触发器、扩展(pgvector等)、序列、全部数据(含向量列)"
echo ""
echo "排除数据的表（仅保留结构）:"
for table in "${EXCLUDE_TABLES[@]}"; do
    echo "  - $table"
done
echo "=========================================="
echo ""
echo "开始备份..."

docker exec "$CONTAINER" pg_dump \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    --format=plain \
    --create \
    --clean \
    --if-exists \
    --no-owner \
    --no-acl \
    $EXCLUDE_ARGS \
    > "$LOCAL_FILE"

FILE_SIZE=$(du -h "$LOCAL_FILE" | cut -f1)

echo ""
echo "=========================================="
echo "备份完成"
echo "文件: ${LOCAL_FILE}"
echo "大小: ${FILE_SIZE}"
echo ""
echo "恢复方式:"
echo "  psql -h <host> -p <port> -U <user> -f ${LOCAL_FILE}"
echo "=========================================="
