-- AWS Policy Generator Skills
-- 将 skill/aws_policy/domain/ 目录下的 6 个 skill 文件插入数据库
-- 执行方式: psql -h localhost -p 15432 -U postgres -d iac_platform -f scripts/add_aws_policy_skills.sql

-- 1. AWS Policy Core Principles
INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
    'skill-domain-005',
    'aws_policy_core_principles',
    'AWS 策略核心原则',
    'domain',
    pg_read_file('/Users/ken/go/src/iac-platform/skill/aws_policy/domain/aws_policy_core_principles.md'),
    '1.0.0',
    true,
    100,
    'manual',
    '{"description": "AWS IAM 和资源策略的核心原则，包括最小权限、显式拒绝、策略结构、验证规则和常见错误", "tags": ["domain", "aws", "iam", "policy", "security", "best-practices", "least-privilege"], "category": "aws_policy"}',
    'system',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();

-- 2. AWS IAM Policy Patterns
INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
    'skill-domain-006',
    'aws_iam_policy_patterns',
    'AWS IAM 策略模式',
    'domain',
    pg_read_file('/Users/ken/go/src/iac-platform/skill/aws_policy/domain/aws_iam_policy_patterns.md'),
    '1.0.0',
    true,
    100,
    'manual',
    '{"description": "AWS IAM 身份策略的常见模式和最佳实践，包括 EC2、S3、Lambda、RDS、CloudFormation 等服务的策略示例", "tags": ["domain", "aws", "iam", "policy", "identity-policy", "security", "patterns"], "category": "aws_policy"}',
    'system',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();

-- 3. AWS S3 Policy Patterns
INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
    'skill-domain-007',
    'aws_s3_policy_patterns',
    'AWS S3 桶策略模式',
    'domain',
    pg_read_file('/Users/ken/go/src/iac-platform/skill/aws_policy/domain/aws_s3_policy_patterns.md'),
    '1.0.0',
    true,
    100,
    'manual',
    '{"description": "AWS S3 桶策略的常见模式和最佳实践，包括 HTTPS 强制、跨账户访问、CloudFront OAI、VPC 端点限制等场景", "tags": ["domain", "aws", "s3", "policy", "bucket-policy", "resource-policy", "security", "encryption"], "category": "aws_policy"}',
    'system',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();

-- 4. AWS KMS Policy Patterns
INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
    'skill-domain-008',
    'aws_kms_policy_patterns',
    'AWS KMS 密钥策略模式',
    'domain',
    pg_read_file('/Users/ken/go/src/iac-platform/skill/aws_policy/domain/aws_kms_policy_patterns.md'),
    '1.0.0',
    true,
    100,
    'manual',
    '{"description": "AWS KMS 密钥策略的常见模式和最佳实践，包括密钥管理员、密钥用户、服务集成、跨账户访问等场景", "tags": ["domain", "aws", "kms", "policy", "key-policy", "encryption", "security"], "category": "aws_policy"}',
    'system',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();

-- 5. AWS Secrets Manager Policy Patterns
INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
    'skill-domain-009',
    'aws_secrets_manager_policy_patterns',
    'AWS Secrets Manager 策略模式',
    'domain',
    pg_read_file('/Users/ken/go/src/iac-platform/skill/aws_policy/domain/aws_secrets_manager_policy_patterns.md'),
    '1.0.0',
    true,
    100,
    'manual',
    '{"description": "AWS Secrets Manager 策略的常见模式和最佳实践，包括基本访问、跨账户、版本控制、Lambda 轮换等场景", "tags": ["domain", "aws", "secrets-manager", "policy", "resource-policy", "security", "secrets"], "category": "aws_policy"}',
    'system',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();

-- 6. AWS Condition Keys Reference
INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
    'skill-domain-010',
    'aws_condition_keys_reference',
    'AWS 条件键参考',
    'domain',
    pg_read_file('/Users/ken/go/src/iac-platform/skill/aws_policy/domain/aws_condition_keys_reference.md'),
    '1.0.0',
    true,
    100,
    'manual',
    '{"description": "AWS IAM 策略常用条件键的快速参考，包括全局条件键、服务特定条件键、条件操作符和使用示例", "tags": ["domain", "aws", "iam", "policy", "condition-keys", "security", "reference"], "category": "aws_policy"}',
    'system',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();

-- 验证插入结果
SELECT id, name, display_name, layer, LENGTH(content) as content_length, is_active 
FROM skills 
WHERE id LIKE 'skill-domain-0%' AND name LIKE 'aws_%'
ORDER BY id;