-- 添加 CMDB 测试资源
-- 用于测试 AI + CMDB 集成功能
-- 执行方式: psql -U postgres -d iac_platform -f scripts/add_cmdb_test_resources.sql

-- 使用 ken-test workspace
-- workspace_id: ws-5o7movp0e7

-- 1. 添加 VPC 资源
INSERT INTO resource_index (
    workspace_id, terraform_address, resource_type, resource_name, resource_mode,
    cloud_resource_id, cloud_resource_name, cloud_resource_arn, description,
    cloud_provider, cloud_region, attributes, tags, source_type, last_synced_at, created_at
) VALUES 
-- Exchange VPC (生产环境)
(
    'ws-5o7movp0e7', 'module.vpc.aws_vpc.exchange', 'aws_vpc', 'exchange', 'managed',
    'vpc-0123456789abcdef0', 'exchange-vpc-prod', 'arn:aws:ec2:ap-northeast-1:123456789012:vpc/vpc-0123456789abcdef0',
    'Exchange 生产环境 VPC',
    'aws', 'ap-northeast-1',
    '{"cidr_block": "10.0.0.0/16", "enable_dns_hostnames": true, "enable_dns_support": true}'::jsonb,
    '{"Name": "exchange-vpc-prod", "Environment": "production", "Team": "exchange"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- Exchange VPC (开发环境)
(
    'ws-5o7movp0e7', 'module.vpc.aws_vpc.exchange_dev', 'aws_vpc', 'exchange_dev', 'managed',
    'vpc-0987654321fedcba0', 'exchange-vpc-dev', 'arn:aws:ec2:ap-northeast-1:123456789012:vpc/vpc-0987654321fedcba0',
    'Exchange 开发环境 VPC',
    'aws', 'ap-northeast-1',
    '{"cidr_block": "10.1.0.0/16", "enable_dns_hostnames": true, "enable_dns_support": true}'::jsonb,
    '{"Name": "exchange-vpc-dev", "Environment": "development", "Team": "exchange"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- Trading VPC
(
    'ws-5o7movp0e7', 'module.vpc.aws_vpc.trading', 'aws_vpc', 'trading', 'managed',
    'vpc-1111111111111111', 'trading-vpc', 'arn:aws:ec2:ap-northeast-1:123456789012:vpc/vpc-1111111111111111',
    'Trading 系统 VPC',
    'aws', 'ap-northeast-1',
    '{"cidr_block": "10.2.0.0/16", "enable_dns_hostnames": true, "enable_dns_support": true}'::jsonb,
    '{"Name": "trading-vpc", "Environment": "production", "Team": "trading"}'::jsonb,
    'terraform', NOW(), NOW()
)
ON CONFLICT DO NOTHING;

-- 2. 添加子网资源
INSERT INTO resource_index (
    workspace_id, terraform_address, resource_type, resource_name, resource_mode,
    cloud_resource_id, cloud_resource_name, cloud_resource_arn, description,
    cloud_provider, cloud_region, attributes, tags, source_type, last_synced_at, created_at
) VALUES 
-- 东京 1a 私有子网 (Exchange VPC)
(
    'ws-5o7movp0e7', 'module.vpc.aws_subnet.tokyo_1a_private', 'aws_subnet', 'tokyo_1a_private', 'managed',
    'subnet-tokyo1a001', 'tokyo-1a-private', 'arn:aws:ec2:ap-northeast-1:123456789012:subnet/subnet-tokyo1a001',
    '东京 1a 可用区私有子网',
    'aws', 'ap-northeast-1',
    '{"vpc_id": "vpc-0123456789abcdef0", "cidr_block": "10.0.1.0/24", "availability_zone": "ap-northeast-1a", "map_public_ip_on_launch": false}'::jsonb,
    '{"Name": "tokyo-1a-private", "Environment": "production", "Type": "private"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- 东京 1c 私有子网 (Exchange VPC)
(
    'ws-5o7movp0e7', 'module.vpc.aws_subnet.tokyo_1c_private', 'aws_subnet', 'tokyo_1c_private', 'managed',
    'subnet-tokyo1c001', 'tokyo-1c-private', 'arn:aws:ec2:ap-northeast-1:123456789012:subnet/subnet-tokyo1c001',
    '东京 1c 可用区私有子网',
    'aws', 'ap-northeast-1',
    '{"vpc_id": "vpc-0123456789abcdef0", "cidr_block": "10.0.2.0/24", "availability_zone": "ap-northeast-1c", "map_public_ip_on_launch": false}'::jsonb,
    '{"Name": "tokyo-1c-private", "Environment": "production", "Type": "private"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- 东京 1a 公有子网 (Exchange VPC)
(
    'ws-5o7movp0e7', 'module.vpc.aws_subnet.tokyo_1a_public', 'aws_subnet', 'tokyo_1a_public', 'managed',
    'subnet-tokyo1a002', 'tokyo-1a-public', 'arn:aws:ec2:ap-northeast-1:123456789012:subnet/subnet-tokyo1a002',
    '东京 1a 可用区公有子网',
    'aws', 'ap-northeast-1',
    '{"vpc_id": "vpc-0123456789abcdef0", "cidr_block": "10.0.101.0/24", "availability_zone": "ap-northeast-1a", "map_public_ip_on_launch": true}'::jsonb,
    '{"Name": "tokyo-1a-public", "Environment": "production", "Type": "public"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- 新加坡子网
(
    'ws-5o7movp0e7', 'module.vpc.aws_subnet.singapore_1a', 'aws_subnet', 'singapore_1a', 'managed',
    'subnet-sg1a001', 'singapore-1a-private', 'arn:aws:ec2:ap-southeast-1:123456789012:subnet/subnet-sg1a001',
    '新加坡 1a 可用区私有子网',
    'aws', 'ap-southeast-1',
    '{"vpc_id": "vpc-1111111111111111", "cidr_block": "10.2.1.0/24", "availability_zone": "ap-southeast-1a", "map_public_ip_on_launch": false}'::jsonb,
    '{"Name": "singapore-1a-private", "Environment": "production", "Type": "private"}'::jsonb,
    'terraform', NOW(), NOW()
)
ON CONFLICT DO NOTHING;

-- 3. 添加安全组资源
INSERT INTO resource_index (
    workspace_id, terraform_address, resource_type, resource_name, resource_mode,
    cloud_resource_id, cloud_resource_name, cloud_resource_arn, description,
    cloud_provider, cloud_region, attributes, tags, source_type, last_synced_at, created_at
) VALUES 
-- Java Private 安全组
(
    'ws-5o7movp0e7', 'module.security.aws_security_group.java_private', 'aws_security_group', 'java_private', 'managed',
    'sg-java001', 'java-private-sg', 'arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-java001',
    'Java 应用私有安全组，允许内部访问',
    'aws', 'ap-northeast-1',
    '{"vpc_id": "vpc-0123456789abcdef0", "name": "java-private-sg", "ingress": [{"from_port": 8080, "to_port": 8080, "protocol": "tcp", "cidr_blocks": ["10.0.0.0/16"]}]}'::jsonb,
    '{"Name": "java-private-sg", "Application": "java", "Type": "private"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- Java Public 安全组
(
    'ws-5o7movp0e7', 'module.security.aws_security_group.java_public', 'aws_security_group', 'java_public', 'managed',
    'sg-java002', 'java-public-sg', 'arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-java002',
    'Java 应用公有安全组，允许外部访问',
    'aws', 'ap-northeast-1',
    '{"vpc_id": "vpc-0123456789abcdef0", "name": "java-public-sg", "ingress": [{"from_port": 443, "to_port": 443, "protocol": "tcp", "cidr_blocks": ["0.0.0.0/0"]}]}'::jsonb,
    '{"Name": "java-public-sg", "Application": "java", "Type": "public"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- Web 安全组
(
    'ws-5o7movp0e7', 'module.security.aws_security_group.web', 'aws_security_group', 'web', 'managed',
    'sg-web001', 'web-sg', 'arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-web001',
    'Web 服务器安全组',
    'aws', 'ap-northeast-1',
    '{"vpc_id": "vpc-0123456789abcdef0", "name": "web-sg", "ingress": [{"from_port": 80, "to_port": 80, "protocol": "tcp", "cidr_blocks": ["0.0.0.0/0"]}, {"from_port": 443, "to_port": 443, "protocol": "tcp", "cidr_blocks": ["0.0.0.0/0"]}]}'::jsonb,
    '{"Name": "web-sg", "Application": "web", "Type": "public"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- Database 安全组
(
    'ws-5o7movp0e7', 'module.security.aws_security_group.database', 'aws_security_group', 'database', 'managed',
    'sg-db001', 'database-sg', 'arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-db001',
    '数据库安全组，仅允许内部访问',
    'aws', 'ap-northeast-1',
    '{"vpc_id": "vpc-0123456789abcdef0", "name": "database-sg", "ingress": [{"from_port": 3306, "to_port": 3306, "protocol": "tcp", "cidr_blocks": ["10.0.0.0/16"]}, {"from_port": 5432, "to_port": 5432, "protocol": "tcp", "cidr_blocks": ["10.0.0.0/16"]}]}'::jsonb,
    '{"Name": "database-sg", "Application": "database", "Type": "private"}'::jsonb,
    'terraform', NOW(), NOW()
)
ON CONFLICT DO NOTHING;

-- 4. 添加 AMI 资源
INSERT INTO resource_index (
    workspace_id, terraform_address, resource_type, resource_name, resource_mode,
    cloud_resource_id, cloud_resource_name, cloud_resource_arn, description,
    cloud_provider, cloud_region, attributes, tags, source_type, last_synced_at, created_at
) VALUES 
-- Amazon Linux 2 AMI
(
    'ws-5o7movp0e7', 'data.aws_ami.amazon_linux_2', 'aws_ami', 'amazon_linux_2', 'data',
    'ami-0123456789abcdef0', 'amzn2-ami-hvm-2.0', NULL,
    'Amazon Linux 2 AMI',
    'aws', 'ap-northeast-1',
    '{"architecture": "x86_64", "root_device_type": "ebs", "virtualization_type": "hvm"}'::jsonb,
    '{"Name": "amzn2-ami-hvm-2.0"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- Ubuntu 22.04 AMI
(
    'ws-5o7movp0e7', 'data.aws_ami.ubuntu_22_04', 'aws_ami', 'ubuntu_22_04', 'data',
    'ami-ubuntu2204001', 'ubuntu-22.04-amd64', NULL,
    'Ubuntu 22.04 LTS AMI',
    'aws', 'ap-northeast-1',
    '{"architecture": "x86_64", "root_device_type": "ebs", "virtualization_type": "hvm"}'::jsonb,
    '{"Name": "ubuntu-22.04-amd64"}'::jsonb,
    'terraform', NOW(), NOW()
)
ON CONFLICT DO NOTHING;

-- 5. 添加 IAM 角色
INSERT INTO resource_index (
    workspace_id, terraform_address, resource_type, resource_name, resource_mode,
    cloud_resource_id, cloud_resource_name, cloud_resource_arn, description,
    cloud_provider, cloud_region, attributes, tags, source_type, last_synced_at, created_at
) VALUES 
-- EC2 Instance Role
(
    'ws-5o7movp0e7', 'module.iam.aws_iam_role.ec2_instance', 'aws_iam_role', 'ec2_instance', 'managed',
    'ec2-instance-role', 'ec2-instance-role', 'arn:aws:iam::123456789012:role/ec2-instance-role',
    'EC2 实例角色',
    'aws', 'ap-northeast-1',
    '{"name": "ec2-instance-role", "assume_role_policy": "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":\"ec2.amazonaws.com\"},\"Action\":\"sts:AssumeRole\"}]}"}'::jsonb,
    '{"Name": "ec2-instance-role"}'::jsonb,
    'terraform', NOW(), NOW()
),
-- Lambda Execution Role
(
    'ws-5o7movp0e7', 'module.iam.aws_iam_role.lambda_execution', 'aws_iam_role', 'lambda_execution', 'managed',
    'lambda-execution-role', 'lambda-execution-role', 'arn:aws:iam::123456789012:role/lambda-execution-role',
    'Lambda 执行角色',
    'aws', 'ap-northeast-1',
    '{"name": "lambda-execution-role", "assume_role_policy": "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":\"lambda.amazonaws.com\"},\"Action\":\"sts:AssumeRole\"}]}"}'::jsonb,
    '{"Name": "lambda-execution-role"}'::jsonb,
    'terraform', NOW(), NOW()
)
ON CONFLICT DO NOTHING;

-- 显示添加的资源统计
SELECT 
    resource_type,
    COUNT(*) as count
FROM resource_index
WHERE workspace_id = 'ws-5o7movp0e7'
GROUP BY resource_type
ORDER BY count DESC;
