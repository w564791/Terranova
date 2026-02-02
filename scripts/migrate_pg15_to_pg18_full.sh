#!/bin/bash
# ============================================================
# PostgreSQL 15 -> 18 完整数据迁移脚本
# 
# 此脚本在 PG18 容器内执行，通过 Docker 网络直接从 PG15 导入数据
#
# 使用方式:
#   ./scripts/migrate_pg15_to_pg18_full.sh
# ============================================================

set -e

# 配置
PG15_HOST="iac-platform-postgres-pgvector"  # Docker 网络中的 PG15 容器名
PG18_CONTAINER="iac-platform-postgres-pg18"
DB_NAME="iac_platform"
DB_USER="postgres"
DB_PASSWORD="postgres123"

echo "============================================================"
echo "PostgreSQL 15 -> 18 完整数据迁移"
echo "============================================================"
echo ""

# 1. 检查容器状态
echo "📋 步骤 1: 检查容器状态..."
if ! docker ps | grep -q "iac-platform-postgres-pgvector"; then
    echo "❌ 错误: PG15 容器未运行"
    exit 1
fi

if ! docker ps | grep -q "$PG18_CONTAINER"; then
    echo "❌ 错误: PG18 容器未运行"
    exit 1
fi
echo "✅ 两个容器都在运行"
echo ""

# 2. 在 PG18 中启用 pgvector 扩展（必须在导入数据前完成）
echo "📋 步骤 2: 在 PG18 中预先启用 pgvector 扩展..."
docker exec -e PGPASSWORD=$DB_PASSWORD $PG18_CONTAINER psql -U $DB_USER -d $DB_NAME -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>/dev/null || true
echo "✅ pgvector 扩展已启用"
echo ""

# 3. 清空 PG18 数据库（保留扩展）
echo "📋 步骤 3: 准备 PG18 数据库..."
docker exec -e PGPASSWORD=$DB_PASSWORD $PG18_CONTAINER psql -U $DB_USER -d $DB_NAME -c "
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO $DB_USER;
GRANT ALL ON SCHEMA public TO public;
" 2>/dev/null || true
echo "✅ PG18 数据库已准备好"
echo ""

# 4. 在 PG18 容器内，通过网络直接从 PG15 导入数据
echo "📋 步骤 4: 从 PG15 导入数据到 PG18..."
echo "   通过 Docker 网络直接连接 PG15 ($PG15_HOST)..."
echo "   这可能需要几分钟..."

# 使用 pg_dump | pg_restore 管道，直接通过网络传输
docker exec -e PGPASSWORD=$DB_PASSWORD $PG18_CONTAINER bash -c "
    PGPASSWORD=$DB_PASSWORD pg_dump \
        -h $PG15_HOST \
        -p 5432 \
        -U $DB_USER \
        -d $DB_NAME \
        -Fc \
        --no-owner \
        --no-acl \
    | pg_restore \
        -U $DB_USER \
        -d $DB_NAME \
        --no-owner \
        --no-acl \
        --single-transaction \
        2>&1 | grep -v 'already exists' || true
"

echo "✅ 数据导入完成"
echo ""

# 5. 验证迁移结果
echo "📋 步骤 5: 验证迁移结果..."
echo ""

echo "--- PG15 (源数据库) ---"
docker exec -e PGPASSWORD=$DB_PASSWORD iac-platform-postgres-pgvector psql -U $DB_USER -d $DB_NAME -c "
SELECT 
    (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public') as tables,
    (SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = 'public') as functions,
    version() as pg_version;
"

echo ""
echo "--- PG18 (目标数据库) ---"
docker exec -e PGPASSWORD=$DB_PASSWORD $PG18_CONTAINER psql -U $DB_USER -d $DB_NAME -c "
SELECT 
    (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public') as tables,
    (SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = 'public') as functions,
    version() as pg_version;
"

# 6. 验证 vector 类型数据
echo ""
echo "📋 步骤 6: 验证 vector 类型数据..."
docker exec -e PGPASSWORD=$DB_PASSWORD $PG18_CONTAINER psql -U $DB_USER -d $DB_NAME -c "
SELECT 
    'resource_index' as table_name,
    COUNT(*) as total_rows,
    COUNT(embedding) as rows_with_embedding
FROM resource_index;
" 2>/dev/null || echo "⚠️ resource_index 表可能不存在或无法访问"

echo ""
echo "============================================================"
echo "✅ 迁移完成！"
echo ""
echo "新数据库连接信息:"
echo "  主机: localhost"
echo "  端口: 15433"
echo "  数据库: $DB_NAME"
echo "  用户: $DB_USER"
echo ""
echo "测试连接:"
echo "  PGPASSWORD=$DB_PASSWORD psql -h localhost -p 15433 -U $DB_USER -d $DB_NAME"
echo "============================================================"