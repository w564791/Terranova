#!/bin/bash

# AWS Policy Generator Skills 导入脚本
# 将 skill/aws_policy/domain/ 目录下的 6 个 skill 文件插入数据库

set -e

# 数据库连接参数
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-15432}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-iac_platform}"
DB_PASSWORD="${DB_PASSWORD:-postgres123}"

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SKILL_DIR="$PROJECT_ROOT/skill/aws_policy/domain"

echo "=== AWS Policy Skills 导入脚本 ==="
echo "数据库: $DB_HOST:$DB_PORT/$DB_NAME"
echo "Skill 目录: $SKILL_DIR"
echo ""

# 函数：转义 SQL 字符串中的单引号
escape_sql() {
    sed "s/'/''/g"
}

# 函数：插入单个 skill
insert_skill() {
    local id="$1"
    local name="$2"
    local display_name="$3"
    local file_path="$4"
    local description="$5"
    local tags="$6"
    
    echo "正在插入: $name ($id)"
    
    # 读取文件内容并转义
    local content=$(cat "$file_path" | escape_sql)
    
    # 构建 SQL
    local sql="INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
    '$id',
    '$name',
    '$display_name',
    'domain',
    E'$content',
    '1.0.0',
    true,
    100,
    'manual',
    '{\"description\": \"$description\", \"tags\": $tags, \"category\": \"aws_policy\"}',
    'system',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();"
    
    # 执行 SQL
    PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "$sql" > /dev/null
    
    echo "  ✓ 完成"
}

# 1. AWS Policy Core Principles
insert_skill \
    "skill-domain-005" \
    "aws_policy_core_principles" \
    "AWS 策略核心原则" \
    "$SKILL_DIR/aws_policy_core_principles.md" \
    "AWS IAM 和资源策略的核心原则，包括最小权限、显式拒绝、策略结构、验证规则和常见错误" \
    '["domain", "aws", "iam", "policy", "security", "best-practices", "least-privilege"]'

# 2. AWS IAM Policy Patterns
insert_skill \
    "skill-domain-006" \
    "aws_iam_policy_patterns" \
    "AWS IAM 策略模式" \
    "$SKILL_DIR/aws_iam_policy_patterns.md" \
    "AWS IAM 身份策略的常见模式和最佳实践，包括 EC2、S3、Lambda、RDS、CloudFormation 等服务的策略示例" \
    '["domain", "aws", "iam", "policy", "identity-policy", "security", "patterns"]'

# 3. AWS S3 Policy Patterns
insert_skill \
    "skill-domain-007" \
    "aws_s3_policy_patterns" \
    "AWS S3 桶策略模式" \
    "$SKILL_DIR/aws_s3_policy_patterns.md" \
    "AWS S3 桶策略的常见模式和最佳实践，包括 HTTPS 强制、跨账户访问、CloudFront OAI、VPC 端点限制等场景" \
    '["domain", "aws", "s3", "policy", "bucket-policy", "resource-policy", "security", "encryption"]'

# 4. AWS KMS Policy Patterns
insert_skill \
    "skill-domain-008" \
    "aws_kms_policy_patterns" \
    "AWS KMS 密钥策略模式" \
    "$SKILL_DIR/aws_kms_policy_patterns.md" \
    "AWS KMS 密钥策略的常见模式和最佳实践，包括密钥管理员、密钥用户、服务集成、跨账户访问等场景" \
    '["domain", "aws", "kms", "policy", "key-policy", "encryption", "security"]'

# 5. AWS Secrets Manager Policy Patterns
insert_skill \
    "skill-domain-009" \
    "aws_secrets_manager_policy_patterns" \
    "AWS Secrets Manager 策略模式" \
    "$SKILL_DIR/aws_secrets_manager_policy_patterns.md" \
    "AWS Secrets Manager 策略的常见模式和最佳实践，包括基本访问、跨账户、版本控制、Lambda 轮换等场景" \
    '["domain", "aws", "secrets-manager", "policy", "resource-policy", "security", "secrets"]'

# 6. AWS Condition Keys Reference
insert_skill \
    "skill-domain-010" \
    "aws_condition_keys_reference" \
    "AWS 条件键参考" \
    "$SKILL_DIR/aws_condition_keys_reference.md" \
    "AWS IAM 策略常用条件键的快速参考，包括全局条件键、服务特定条件键、条件操作符和使用示例" \
    '["domain", "aws", "iam", "policy", "condition-keys", "security", "reference"]'

echo ""
echo "=== 验证插入结果 ==="
PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "
SELECT id, name, display_name, layer, LENGTH(content) as content_length, is_active 
FROM skills 
WHERE id LIKE 'skill-domain-0%' AND name LIKE 'aws_%'
ORDER BY id;
"

echo ""
echo "=== 导入完成 ==="