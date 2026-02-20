#!/bin/bash

# 临时脚本：将S3 demo schema插入数据库
# 注意：这只是开发测试用的临时脚本

echo "=== 插入S3 Demo Schema到数据库 ==="

# 数据库连接信息
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_NAME=${DB_NAME:-iac_platform}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-postgres123}

# 先确保module存在
echo "1. 确保S3 module记录存在..."
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME <<EOF
-- 插入或更新module记录
INSERT INTO modules (id, name, provider, description, import_type, source_url, sync_status, created_at, updated_at)
VALUES (
    6,
    's3-bucket',
    'aws', 
    'AWS S3 bucket module - Demo for testing (80+ parameters)',
    'url',
    'https://github.com/terraform-aws-modules/terraform-aws-s3-bucket',
    'completed',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE
SET 
    name = EXCLUDED.name,
    provider = EXCLUDED.provider,
    description = EXCLUDED.description,
    updated_at = NOW();
EOF

echo "2. 读取生成的schema JSON..."
# 获取schema.json的内容（只取schema部分）
SCHEMA_JSON=$(cd backend/cmd/generate_s3_schema && go run main.go 2>/dev/null | grep -A 100000 '"schema":' | sed '1s/.*"schema"://' | sed 's/^}/}/')

# 如果上面的方法失败，尝试直接读取文件
if [ -z "$SCHEMA_JSON" ]; then
    echo "从文件读取schema..."
    if [ -f "backend/cmd/generate_s3_schema/s3_schema.json" ]; then
        SCHEMA_JSON=$(cat backend/cmd/generate_s3_schema/s3_schema.json | jq '.schema')
    else
        echo "错误：找不到s3_schema.json文件"
        echo "请先运行: cd backend/cmd/generate_s3_schema && go run main.go"
        exit 1
    fi
fi

echo "3. 插入schema到数据库..."
# 使用psql插入数据
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME <<EOF
-- 删除旧的schema（如果存在）
DELETE FROM schemas WHERE module_id = 6;

-- 插入新的schema
INSERT INTO schemas (module_id, schema_data, version, status, ai_generated, created_by)
VALUES (
    6,
    '$SCHEMA_JSON'::jsonb,
    '2.0.0',
    'active',
    false,
    1
);
EOF

echo "4. 验证插入结果..."
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT id, module_id, version, status, jsonb_pretty(schema_data) as schema_preview FROM schemas WHERE module_id = 6 LIMIT 1;"

echo "=== 完成！S3 Demo Schema已插入数据库 ==="
echo ""
echo "提示："
echo "1. 启动后端服务: cd backend && go run main.go"
echo "2. 启动前端服务: cd frontend && npm run dev"
echo "3. 访问 http://localhost:5173/modules/6/schema 查看效果"
