package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// 模拟的CMDB资源数据
type CMDBResource struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Region      string                 `json:"region"`
	AccountID   string                 `json:"account_id"`
	AccountName string                 `json:"account_name"`
	ARN         string                 `json:"arn"`
	Description string                 `json:"description"`
	Tags        map[string]string      `json:"tags"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"` // 资源属性（如 vpc_id, availability_zone 等）
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// API响应结构
type APIResponse struct {
	Success bool           `json:"success"`
	Data    []CMDBResource `json:"data"`
	Total   int            `json:"total"`
	Message string         `json:"message,omitempty"`
}

// 固定的测试Token
const TEST_TOKEN = "test-cmdb-token-12345"

// 生成模拟数据
func generateMockData() []CMDBResource {
	return []CMDBResource{
		// ==================== VPC 资源 ====================
		{
			ID:          "vpc-0a1b2c3d4e5f67890",
			Name:        "exchange-vpc",
			Type:        "aws_vpc",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:vpc/vpc-0a1b2c3d4e5f67890",
			Description: "Exchange 生产环境 VPC",
			Tags: map[string]string{
				"Name":        "exchange-vpc",
				"Environment": "production",
				"Team":        "exchange",
			},
			CreatedAt: "2024-01-01T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpc-0b2c3d4e5f678901a",
			Name:        "exchange-vpc-dev",
			Type:        "aws_vpc",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Development Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:vpc/vpc-0b2c3d4e5f678901a",
			Description: "Exchange 开发环境 VPC",
			Tags: map[string]string{
				"Name":        "exchange-vpc-dev",
				"Environment": "development",
				"Team":        "exchange",
			},
			CreatedAt: "2024-01-02T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpc-0c3d4e5f6789012ab",
			Name:        "trading-vpc",
			Type:        "aws_vpc",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:vpc/vpc-0c3d4e5f6789012ab",
			Description: "Trading 系统 VPC",
			Tags: map[string]string{
				"Name":        "trading-vpc",
				"Environment": "production",
				"Team":        "trading",
			},
			CreatedAt: "2024-01-03T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== 子网资源 ====================
		{
			ID:          "subnet-0d1e2f3a4b5c67890",
			Name:        "tokyo-1a-private",
			Type:        "aws_subnet",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:subnet/subnet-0d1e2f3a4b5c67890",
			Description: "东京 1a 可用区私有子网 (Exchange VPC)",
			Tags: map[string]string{
				"Name":             "tokyo-1a-private",
				"Environment":      "production",
				"Type":             "private",
				"AvailabilityZone": "ap-northeast-1a",
				"VPC":              "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":            "vpc-0a1b2c3d4e5f67890",
				"availability_zone": "ap-northeast-1a",
				"cidr_block":        "10.0.1.0/24",
				"map_public_ip":     false,
			},
			CreatedAt: "2024-01-05T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "subnet-0e2f3a4b5c678901d",
			Name:        "tokyo-1c-private",
			Type:        "aws_subnet",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:subnet/subnet-0e2f3a4b5c678901d",
			Description: "东京 1c 可用区私有子网 (Exchange VPC)",
			Tags: map[string]string{
				"Name":             "tokyo-1c-private",
				"Environment":      "production",
				"Type":             "private",
				"AvailabilityZone": "ap-northeast-1c",
				"VPC":              "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":            "vpc-0a1b2c3d4e5f67890",
				"availability_zone": "ap-northeast-1c",
				"cidr_block":        "10.0.3.0/24",
				"map_public_ip":     false,
			},
			CreatedAt: "2024-01-05T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "subnet-0f3a4b5c6789012ef",
			Name:        "tokyo-1a-public",
			Type:        "aws_subnet",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:subnet/subnet-0f3a4b5c6789012ef",
			Description: "东京 1a 可用区公有子网 (Exchange VPC)",
			Tags: map[string]string{
				"Name":             "tokyo-1a-public",
				"Environment":      "production",
				"Type":             "public",
				"AvailabilityZone": "ap-northeast-1a",
				"VPC":              "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":            "vpc-0a1b2c3d4e5f67890",
				"availability_zone": "ap-northeast-1a",
				"cidr_block":        "10.0.0.0/24",
				"map_public_ip":     true,
			},
			CreatedAt: "2024-01-05T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "subnet-01bc9ccfe9259b6e7",
			Name:        "exchange-public-1b",
			Type:        "aws_subnet",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:subnet/subnet-01bc9ccfe9259b6e7",
			Description: "东京 1b 可用区公有子网 (Exchange VPC) - 自动分配公网 IP",
			Tags: map[string]string{
				"Name":             "exchange-public-1b",
				"Environment":      "production",
				"Type":             "public",
				"AvailabilityZone": "ap-northeast-1b",
				"VPC":              "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":                          "vpc-0a1b2c3d4e5f67890",
				"availability_zone":               "ap-northeast-1b",
				"cidr_block":                      "10.0.2.0/24",
				"map_public_ip_on_launch":         true,
				"assign_ipv6_address_on_creation": false,
			},
			CreatedAt: "2024-01-06T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== 安全组资源 ====================
		{
			ID:          "sg-0f9635f40c4b29f6d",
			Name:        "nodegroup-sg",
			Type:        "aws_security_group",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-0f9635f40c4b29f6d",
			Description: "Java 应用私有安全组，允许内部访问",
			Tags: map[string]string{
				"Name":        "java-private-sg",
				"Application": "java",
				"Type":        "private",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id": "vpc-0a1b2c3d4e5f67890",
				"ingress": []map[string]interface{}{
					{"from_port": 8080, "to_port": 8080, "protocol": "tcp", "cidr_blocks": []string{"10.0.0.0/16"}},
					{"from_port": 443, "to_port": 443, "protocol": "tcp", "cidr_blocks": []string{"10.0.0.0/16"}},
				},
				"egress": []map[string]interface{}{
					{"from_port": 0, "to_port": 0, "protocol": "-1", "cidr_blocks": []string{"0.0.0.0/0"}},
				},
			},
			CreatedAt: "2024-01-10T09:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		}, {
			ID:          "sg-020756ecf1930143e",
			Name:        "nodegroup-sg-classic",
			Type:        "aws_security_group",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-020756ecf1930143e",
			Description: "Java 应用私有安全组，允许内部访问",
			Tags: map[string]string{
				"Name":        "java-private-sg",
				"Application": "java",
				"Type":        "private",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id": "vpc-0a1b2c3d4e5f67890",
			},
			CreatedAt: "2024-01-10T09:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		}, {
			ID:          "sg-0a1b2c3d4e5f67890",
			Name:        "java-private",
			Type:        "aws_security_group",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-0a1b2c3d4e5f67890",
			Description: "Java 应用私有安全组，允许内部访问",
			Tags: map[string]string{
				"Name":        "java-private-sg",
				"Application": "java",
				"Type":        "private",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id": "vpc-0a1b2c3d4e5f67890",
			},
			CreatedAt: "2024-01-10T09:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "sg-0b2c3d4e5f678901a",
			Name:        "java-public",
			Type:        "aws_security_group",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-0b2c3d4e5f678901a",
			Description: "Java 应用公有安全组，允许外部访问",
			Tags: map[string]string{
				"Name":        "java-public-sg",
				"Application": "java",
				"Type":        "public",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id": "vpc-0a1b2c3d4e5f67890",
			},
			CreatedAt: "2024-01-10T09:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "sg-0c3d4e5f6789012ab",
			Name:        "web-sg",
			Type:        "aws_security_group",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-0c3d4e5f6789012ab",
			Description: "Web 服务器安全组",
			Tags: map[string]string{
				"Name":        "web-sg",
				"Application": "web",
				"Type":        "public",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id": "vpc-0a1b2c3d4e5f67890",
				"ingress": []map[string]interface{}{
					{"from_port": 80, "to_port": 80, "protocol": "tcp", "cidr_blocks": []string{"0.0.0.0/0"}},
					{"from_port": 443, "to_port": 443, "protocol": "tcp", "cidr_blocks": []string{"0.0.0.0/0"}},
					{"from_port": 22, "to_port": 22, "protocol": "tcp", "cidr_blocks": []string{"0.0.0.0/0"}},
				},
				"egress": []map[string]interface{}{
					{"from_port": 0, "to_port": 0, "protocol": "-1", "cidr_blocks": []string{"0.0.0.0/0"}},
				},
			},
			CreatedAt: "2024-01-10T09:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "sg-0d4e5f67890123abc",
			Name:        "database-sg",
			Type:        "aws_security_group",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-0d4e5f67890123abc",
			Description: "数据库安全组，仅允许内部访问",
			Tags: map[string]string{
				"Name":        "database-sg",
				"Application": "database",
				"Type":        "private",
				"VPC":         "exchange-vpc",
				"Environment": "production",
			},
			Attributes: map[string]interface{}{
				"vpc_id": "vpc-0a1b2c3d4e5f67890",
				"ingress": []map[string]interface{}{
					{"from_port": 3306, "to_port": 3306, "protocol": "tcp", "cidr_blocks": []string{"10.0.0.0/16"}},
					{"from_port": 5432, "to_port": 5432, "protocol": "tcp", "cidr_blocks": []string{"0.0.0.0/0"}},
				},
				"egress": []map[string]interface{}{
					{"from_port": 0, "to_port": 0, "protocol": "-1", "cidr_blocks": []string{"0.0.0.0/0"}},
				},
			},
			CreatedAt: "2024-01-10T09:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== EC2 实例 ====================
		{
			ID:          "i-0123456789abcdef0",
			Name:        "web-server-01",
			Type:        "aws_instance",
			Status:      "running",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:instance/i-0123456789abcdef0",
			Description: "Production web server",
			Tags: map[string]string{
				"Name":        "web-server-01",
				"Environment": "production",
				"Application": "web",
				"Team":        "platform",
			},
			Attributes: map[string]interface{}{
				"instance_type":               "t3.large",
				"ami":                         "ami-0abc123def456789",
				"availability_zone":           "ap-northeast-1a",
				"subnet_id":                   "subnet-0d1e2f3a4b5c67890",
				"vpc_security_group_ids":      []string{"sg-0c3d4e5f6789012ab"},
				"associate_public_ip_address": true,
				"public_ip":                   "54.199.123.45",
				"private_ip":                  "10.0.1.100",
				"iam_instance_profile":        "ec2-instance-role",
				"ebs_optimized":               true,
				"monitoring":                  true,
				"disable_api_termination":     false,
			},
			CreatedAt: "2024-01-15T10:30:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "i-0123456789abcdef1",
			Name:        "api-server-01",
			Type:        "aws_instance",
			Status:      "running",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:instance/i-0123456789abcdef1",
			Description: "Production API server",
			Tags: map[string]string{
				"Name":        "api-server-01",
				"Environment": "production",
				"Application": "api",
				"Team":        "backend",
			},
			Attributes: map[string]interface{}{
				"instance_type":               "c6i.xlarge",
				"ami":                         "ami-0abc123def456789",
				"availability_zone":           "ap-northeast-1c",
				"subnet_id":                   "subnet-0e2f3a4b5c678901d",
				"vpc_security_group_ids":      []string{"sg-0a1b2c3d4e5f67890"},
				"associate_public_ip_address": false,
				"private_ip":                  "10.0.3.50",
				"iam_instance_profile":        "ec2-instance-role",
				"disable_api_termination":     true,
			},
			CreatedAt: "2024-01-15T11:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== S3 存储桶 ====================
		{
			ID:          "bucket-prod-data-001",
			Name:        "prod-data-bucket",
			Type:        "aws_s3_bucket",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:s3:::prod-data-bucket",
			Description: "Production data storage bucket",
			Tags: map[string]string{
				"Name":        "prod-data-bucket",
				"Environment": "production",
				"DataType":    "application",
			},
			Attributes: map[string]interface{}{
				"bucket":     "prod-data-bucket",
				"region":     "ap-northeast-1",
				"versioning": map[string]interface{}{"enabled": true},
				"server_side_encryption_configuration": map[string]interface{}{
					"rule": map[string]interface{}{
						"apply_server_side_encryption_by_default": map[string]interface{}{
							"sse_algorithm":     "aws:kms",
							"kms_master_key_id": "arn:aws:kms:ap-northeast-1:123456789012:key/mrk-abc123",
						},
					},
				},
				"block_public_acls":       true,
				"block_public_policy":     true,
				"ignore_public_acls":      true,
				"restrict_public_buckets": true,
				"force_destroy":           false,
			},
			CreatedAt: "2024-01-05T14:30:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== RDS 数据库 ====================
		{
			ID:          "db-prod-mysql-001",
			Name:        "production-mysql",
			Type:        "aws_db_instance",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:rds:ap-northeast-1:123456789012:db:production-mysql",
			Description: "Production MySQL database",
			Tags: map[string]string{
				"Name":        "production-mysql",
				"Environment": "production",
				"Database":    "mysql",
				"Version":     "8.0",
			},
			Attributes: map[string]interface{}{
				"engine":                  "mysql",
				"engine_version":          "8.0.35",
				"instance_class":          "db.r6g.xlarge",
				"allocated_storage":       100,
				"multi_az":                true,
				"publicly_accessible":     false,
				"deletion_protection":     true,
				"storage_encrypted":       true,
				"kms_key_id":              "arn:aws:kms:ap-northeast-1:123456789012:key/mrk-abc123",
				"backup_retention_period": 7,
				"vpc_security_group_ids":  []string{"sg-0d4e5f67890123abc"},
				"db_subnet_group_name":    "prod-db-subnet-group",
				"skip_final_snapshot":     false,
			},
			CreatedAt: "2024-01-08T16:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== 额外安全组 ====================
		{
			ID:          "sg-1234566",
			Name:        "api-endpoint-sg",
			Type:        "aws_security_group",
			Status:      "active",
			Region:      "ap-northeast-1",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:123456789012:security-group/sg-1234566",
			Description: "API Endpoint 安全组，允许 HTTPS 入站",
			Tags: map[string]string{
				"Name":        "api-endpoint-sg",
				"Application": "api",
				"Type":        "private",
				"VPC":         "exchange-vpc",
				"Environment": "production",
			},
			Attributes: map[string]interface{}{
				"vpc_id": "vpc-0a1b2c3d4e5f67890",
				"ingress": []map[string]interface{}{
					{"from_port": 443, "to_port": 443, "protocol": "tcp", "cidr_blocks": []string{"10.0.0.0/16"}},
				},
				"egress": []map[string]interface{}{
					{"from_port": 0, "to_port": 0, "protocol": "-1", "cidr_blocks": []string{"0.0.0.0/0"}},
				},
			},
			CreatedAt: "2024-02-01T10:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== VPC Endpoint ====================
		{
			ID:          "vpce-0a1b2c3d4e5f67890",
			Name:        "exchange-api-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0a1b2c3d4e5f67890",
			Description: "Exchange API VPC Endpoint，绑定 api-endpoint-sg 安全组和 exchange-public-1b 子网",
			Tags: map[string]string{
				"Name":        "exchange-api-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.execute-api",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.execute-api",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-01bc9ccfe9259b6e7"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T10:30:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== VPC Endpoints (常用服务) ====================
		{
			ID:          "vpce-0b2c3d4e5f6789001",
			Name:        "s3-gateway-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789001",
			Description: "S3 Gateway Endpoint，所有 VPC 内流量直达 S3 无需经过 NAT",
			Tags: map[string]string{
				"Name":        "s3-gateway-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.s3",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":            "vpc-0a1b2c3d4e5f67890",
				"service_name":     "com.amazonaws.ap-northeast-1.s3",
				"vpc_endpoint_type": "Gateway",
				"route_table_ids":  []string{"rtb-0a1b2c3d4e5f67890"},
				"state":            "available",
			},
			CreatedAt: "2024-02-01T11:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789002",
			Name:        "dynamodb-gateway-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789002",
			Description: "DynamoDB Gateway Endpoint",
			Tags: map[string]string{
				"Name":        "dynamodb-gateway-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.dynamodb",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":            "vpc-0a1b2c3d4e5f67890",
				"service_name":     "com.amazonaws.ap-northeast-1.dynamodb",
				"vpc_endpoint_type": "Gateway",
				"route_table_ids":  []string{"rtb-0a1b2c3d4e5f67890"},
				"state":            "available",
			},
			CreatedAt: "2024-02-01T11:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789003",
			Name:        "ecr-api-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789003",
			Description: "ECR API Interface Endpoint，EKS 拉取镜像走私网",
			Tags: map[string]string{
				"Name":        "ecr-api-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.ecr.api",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.ecr.api",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T11:30:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789004",
			Name:        "ecr-dkr-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789004",
			Description: "ECR Docker Interface Endpoint，EKS 拉取镜像层走私网",
			Tags: map[string]string{
				"Name":        "ecr-dkr-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.ecr.dkr",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.ecr.dkr",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T11:30:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789005",
			Name:        "sts-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789005",
			Description: "STS Interface Endpoint，IRSA/Pod Identity 走私网获取临时凭证",
			Tags: map[string]string{
				"Name":        "sts-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.sts",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.sts",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T12:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789006",
			Name:        "logs-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789006",
			Description: "CloudWatch Logs Interface Endpoint，日志推送走私网",
			Tags: map[string]string{
				"Name":        "logs-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.logs",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.logs",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T12:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789007",
			Name:        "monitoring-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789007",
			Description: "CloudWatch Monitoring Interface Endpoint",
			Tags: map[string]string{
				"Name":        "monitoring-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.monitoring",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.monitoring",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T12:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789008",
			Name:        "secretsmanager-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789008",
			Description: "Secrets Manager Interface Endpoint，应用获取密钥走私网",
			Tags: map[string]string{
				"Name":        "secretsmanager-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.secretsmanager",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.secretsmanager",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T12:30:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789009",
			Name:        "kms-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789009",
			Description: "KMS Interface Endpoint，加解密操作走私网",
			Tags: map[string]string{
				"Name":        "kms-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.kms",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.kms",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T12:30:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789010",
			Name:        "sqs-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789010",
			Description: "SQS Interface Endpoint，消息队列走私网",
			Tags: map[string]string{
				"Name":        "sqs-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.sqs",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.sqs",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T13:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "vpce-0b2c3d4e5f6789011",
			Name:        "sns-endpoint",
			Type:        "aws_vpc_endpoint",
			Status:      "available",
			Region:      "ap-northeast-1",
			AccountID:   "072107099780",
			AccountName: "Production Account",
			ARN:         "arn:aws:ec2:ap-northeast-1:072107099780:vpc-endpoint/vpce-0b2c3d4e5f6789011",
			Description: "SNS Interface Endpoint，通知服务走私网",
			Tags: map[string]string{
				"Name":        "sns-endpoint",
				"Service":     "com.amazonaws.ap-northeast-1.sns",
				"Environment": "production",
				"VPC":         "exchange-vpc",
			},
			Attributes: map[string]interface{}{
				"vpc_id":              "vpc-0a1b2c3d4e5f67890",
				"service_name":        "com.amazonaws.ap-northeast-1.sns",
				"vpc_endpoint_type":   "Interface",
				"subnet_ids":          []string{"subnet-0d1e2f3a4b5c67890", "subnet-0e2f3a4b5c678901d"},
				"security_group_ids":  []string{"sg-0024b8192276e71e0"},
				"private_dns_enabled": true,
				"state":               "available",
			},
			CreatedAt: "2024-02-01T13:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},

		// ==================== IAM 角色 ====================
		{
			ID:          "ec2-instance-role",
			Name:        "ec2-instance-role",
			Type:        "aws_iam_role",
			Status:      "active",
			Region:      "global",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:iam::123456789012:role/ec2-instance-role",
			Description: "EC2 实例角色",
			Tags: map[string]string{
				"Name": "ec2-instance-role",
			},
			CreatedAt: "2024-01-01T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:          "lambda-execution-role",
			Name:        "lambda-execution-role",
			Type:        "aws_iam_role",
			Status:      "active",
			Region:      "global",
			AccountID:   "123456789012",
			AccountName: "Production Account",
			ARN:         "arn:aws:iam::123456789012:role/lambda-execution-role",
			Description: "Lambda 执行角色",
			Tags: map[string]string{
				"Name": "lambda-execution-role",
			},
			CreatedAt: "2024-01-01T00:00:00Z",
			UpdatedAt: time.Now().Format(time.RFC3339),
		},
	}
}

// Token验证中间件
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-API-Token")
		if token == "" {
			token = c.GetHeader("Authorization")
			if token != "" && len(token) > 7 && token[:7] == "Bearer " {
				token = token[7:]
			}
		}

		if token != TEST_TOKEN {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid or missing API token",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func main() {
	// 设置Gin为release模式
	gin.SetMode(gin.ReleaseMode)

	r := gin.Default()

	// CORS中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 健康检查端点（无需认证）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "CMDB Test Server",
			"version": "1.0.0",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	// API信息端点（无需认证）
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":     "CMDB Test Server",
			"version":     "1.0.0",
			"description": "Mock CMDB API for testing external data source integration",
			"endpoints": map[string]string{
				"GET /health":           "Health check",
				"GET /api/v1/resources": "List all resources (requires X-API-Token header)",
			},
			"authentication": map[string]string{
				"type":   "API Token",
				"header": "X-API-Token",
				"token":  TEST_TOKEN,
			},
			"example_curl": fmt.Sprintf("curl -H 'X-API-Token: %s' http://localhost:11112/api/v1/resources", TEST_TOKEN),
		})
	})

	// API路由组（需要认证）
	api := r.Group("/api/v1")
	api.Use(authMiddleware())
	{
		// 获取所有资源
		api.GET("/resources", func(c *gin.Context) {
			resources := generateMockData()

			// 支持类型过滤
			resourceType := c.Query("type")
			if resourceType != "" {
				filtered := []CMDBResource{}
				for _, r := range resources {
					if r.Type == resourceType {
						filtered = append(filtered, r)
					}
				}
				resources = filtered
			}

			response := APIResponse{
				Success: true,
				Data:    resources,
				Total:   len(resources),
				Message: "Resources retrieved successfully",
			}

			c.JSON(http.StatusOK, response)
		})

		// 获取单个资源
		api.GET("/resources/:id", func(c *gin.Context) {
			id := c.Param("id")
			resources := generateMockData()

			for _, r := range resources {
				if r.ID == id {
					c.JSON(http.StatusOK, gin.H{
						"success": true,
						"data":    r,
					})
					return
				}
			}

			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Resource not found",
			})
		})
	}

	// 启动服务器
	port := "11112"
	log.Printf("🚀 CMDB Test Server starting on port %s", port)
	log.Printf("📝 API Token: %s", TEST_TOKEN)
	log.Printf("🔗 Test URL: http://localhost:%s", port)
	log.Printf("📊 Resources endpoint: http://localhost:%s/api/v1/resources", port)
	log.Printf("💡 Example: curl -H 'X-API-Token: %s' http://localhost:%s/api/v1/resources", TEST_TOKEN, port)

	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
