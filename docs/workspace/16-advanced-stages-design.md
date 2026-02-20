# 高级执行阶段设计（未来扩展）

> **文档版本**: v1.0  
> **创建日期**: 2025-10-11  
> **状态**: 设计文档（未来实施）  
> **优先级**: 低（非刚需）  
> **前置阅读**: [15-terraform-execution-detail.md](./15-terraform-execution-detail.md)

## 📋 概述

本文档详细设计三个高级执行阶段：OPA策略检查、成本估算、Sentinel策略检查。这些功能不是当前的刚需，但为未来扩展预留了完整的设计方案。

## 🎯 功能优先级

| 功能 | 优先级 | 实施阶段 | 依赖 |
|------|--------|----------|------|
| OPA Policy Check | 低 | Phase 4+ | Plan JSON输出 |
| Cost Estimation | 低 | Phase 4+ | Plan JSON输出 |
| Sentinel Policy Check | 低 | Phase 4+ | Plan JSON输出 |

## 1️⃣ OPA Policy Check Stage（OPA策略检查）

### 功能概述

Open Policy Agent (OPA) 是一个开源的策略引擎，用于统一的策略执行。在Terraform执行流程中，OPA可以检查Plan是否符合组织的安全和合规要求。

### 执行时机

```
Plan Stage → Post-Plan Stage → OPA Policy Check → Cost Estimation → ...
```

### 策略类型

#### Mandatory Policy（强制策略）
- 失败会阻止运行继续
- 必须通过才能进入下一阶段
- 不可覆盖

#### Advisory Policy（建议策略）
- 失败不会阻止运行
- 会显示警告信息
- 可以继续执行

### 数据库设计

```sql
-- OPA策略表
CREATE TABLE opa_policies (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    policy_type VARCHAR(20) NOT NULL, -- mandatory, advisory
    rego_code TEXT NOT NULL, -- OPA Rego语言代码
    enabled BOOLEAN DEFAULT true,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 策略集表
CREATE TABLE opa_policy_sets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    workspace_id INTEGER REFERENCES workspaces(id),
    global BOOLEAN DEFAULT false, -- 全局策略集
    created_at TIMESTAMP DEFAULT NOW()
);

-- 策略集关联表
CREATE TABLE opa_policy_set_policies (
    policy_set_id INTEGER REFERENCES opa_policy_sets(id) ON DELETE CASCADE,
    policy_id INTEGER REFERENCES opa_policies(id) ON DELETE CASCADE,
    PRIMARY KEY (policy_set_id, policy_id)
);

-- 策略检查结果表
CREATE TABLE opa_policy_check_results (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    policy_id INTEGER REFERENCES opa_policies(id),
    passed BOOLEAN NOT NULL,
    violations JSONB, -- 违规详情
    execution_time INTEGER, -- 执行时间（毫秒）
    created_at TIMESTAMP DEFAULT NOW()
);
```

### 实现设计

```go
// OPA策略服务
type OPAPolicyService struct {
    db     *gorm.DB
    client *opa.Client
}

// 策略检查请求
type PolicyCheckRequest struct {
    PlanJSON map[string]interface{} `json:"plan_json"`
    Workspace *models.Workspace     `json:"workspace"`
}

// 策略检查结果
type PolicyCheckResult struct {
    PolicyID   uint                   `json:"policy_id"`
    PolicyName string                 `json:"policy_name"`
    PolicyType string                 `json:"policy_type"`
    Passed     bool                   `json:"passed"`
    Violations []PolicyViolation      `json:"violations"`
    Duration   int                    `json:"duration"` // 毫秒
}

type PolicyViolation struct {
    Message  string                 `json:"message"`
    Resource string                 `json:"resource"`
    Severity string                 `json:"severity"` // high, medium, low
    Details  map[string]interface{} `json:"details"`
}

// 执行OPA策略检查
func (s *OPAPolicyService) CheckPolicies(
    ctx context.Context,
    task *models.WorkspaceTask,
) ([]PolicyCheckResult, error) {
    // 1. 获取适用的策略集
    policySets, err := s.getApplicablePolicySets(task.WorkspaceID)
    if err != nil {
        return nil, fmt.Errorf("failed to get policy sets: %w", err)
    }
    
    if len(policySets) == 0 {
        log.Printf("No OPA policies configured for workspace %d", task.WorkspaceID)
        return []PolicyCheckResult{}, nil
    }
    
    // 2. 获取所有策略
    var policies []models.OPAPolicy
    for _, ps := range policySets {
        var setPolicies []models.OPAPolicy
        s.db.Joins("JOIN opa_policy_set_policies ON opa_policies.id = opa_policy_set_policies.policy_id").
            Where("opa_policy_set_policies.policy_set_id = ? AND opa_policies.enabled = true", ps.ID).
            Find(&setPolicies)
        policies = append(policies, setPolicies...)
    }
    
    // 3. 执行每个策略
    results := make([]PolicyCheckResult, 0)
    for _, policy := range policies {
        result, err := s.evaluatePolicy(ctx, task, &policy)
        if err != nil {
            log.Printf("Failed to evaluate policy %s: %v", policy.Name, err)
            // 策略执行失败视为策略不通过
            result = &PolicyCheckResult{
                PolicyID:   policy.ID,
                PolicyName: policy.Name,
                PolicyType: policy.PolicyType,
                Passed:     false,
                Violations: []PolicyViolation{{
                    Message:  fmt.Sprintf("Policy evaluation failed: %v", err),
                    Severity: "high",
                }},
            }
        }
        
        results = append(results, *result)
        
        // 4. 保存检查结果
        s.saveCheckResult(task.ID, result)
    }
    
    return results, nil
}

// 评估单个策略
func (s *OPAPolicyService) evaluatePolicy(
    ctx context.Context,
    task *models.WorkspaceTask,
    policy *models.OPAPolicy,
) (*PolicyCheckResult, error) {
    startTime := time.Now()
    
    // 1. 准备输入数据
    input := map[string]interface{}{
        "plan": task.PlanJSON,
        "workspace": map[string]interface{}{
            "id":   task.WorkspaceID,
            "name": task.Context["workspace"].(*models.Workspace).Name,
        },
    }
    
    // 2. 执行OPA评估
    result, err := s.client.Evaluate(ctx, policy.RegoCode, input)
    if err != nil {
        return nil, err
    }
    
    duration := time.Since(startTime)
    
    // 3. 解析结果
    passed := result["allow"].(bool)
    violations := make([]PolicyViolation, 0)
    
    if !passed {
        if violationData, ok := result["violations"].([]interface{}); ok {
            for _, v := range violationData {
                violation := v.(map[string]interface{})
                violations = append(violations, PolicyViolation{
                    Message:  violation["message"].(string),
                    Resource: violation["resource"].(string),
                    Severity: violation["severity"].(string),
                    Details:  violation["details"].(map[string]interface{}),
                })
            }
        }
    }
    
    return &PolicyCheckResult{
        PolicyID:   policy.ID,
        PolicyName: policy.Name,
        PolicyType: policy.PolicyType,
        Passed:     passed,
        Violations: violations,
        Duration:   int(duration.Milliseconds()),
    }, nil
}

// 判断是否可以继续
func (s *OPAPolicyService) CanContinue(results []PolicyCheckResult) (bool, string) {
    mandatoryFailed := false
    advisoryFailed := false
    failedPolicies := make([]string, 0)
    
    for _, result := range results {
        if !result.Passed {
            failedPolicies = append(failedPolicies, result.PolicyName)
            
            if result.PolicyType == "mandatory" {
                mandatoryFailed = true
            } else {
                advisoryFailed = true
            }
        }
    }
    
    if mandatoryFailed {
        return false, fmt.Sprintf("Mandatory policies failed: %s", 
            strings.Join(failedPolicies, ", "))
    }
    
    if advisoryFailed {
        return true, fmt.Sprintf("Advisory policies failed (warning): %s", 
            strings.Join(failedPolicies, ", "))
    }
    
    return true, ""
}
```

### OPA策略示例

#### 1. 禁止公开S3 Bucket

```rego
package terraform.policies.s3

# 禁止创建公开的S3 Bucket
deny[msg] {
    resource := input.plan.resource_changes[_]
    resource.type == "aws_s3_bucket"
    resource.change.after.acl == "public-read"
    
    msg := {
        "message": "S3 bucket cannot be public",
        "resource": resource.address,
        "severity": "high",
        "details": {
            "acl": resource.change.after.acl
        }
    }
}

# 必须启用版本控制
deny[msg] {
    resource := input.plan.resource_changes[_]
    resource.type == "aws_s3_bucket"
    not resource.change.after.versioning[_].enabled
    
    msg := {
        "message": "S3 bucket must enable versioning",
        "resource": resource.address,
        "severity": "medium",
        "details": {}
    }
}

# 允许规则
allow {
    count(deny) == 0
}
```

#### 2. 强制标签要求

```rego
package terraform.policies.tags

required_tags := ["Environment", "Owner", "Project"]

# 检查必需标签
deny[msg] {
    resource := input.plan.resource_changes[_]
    resource.change.actions[_] == "create"
    
    # 获取资源标签
    tags := object.get(resource.change.after, "tags", {})
    
    # 检查每个必需标签
    required_tag := required_tags[_]
    not tags[required_tag]
    
    msg := {
        "message": sprintf("Missing required tag: %s", [required_tag]),
        "resource": resource.address,
        "severity": "high",
        "details": {
            "required_tags": required_tags,
            "current_tags": tags
        }
    }
}

allow {
    count(deny) == 0
}
```

### API接口设计

```go
// 创建OPA策略
POST /api/v1/opa-policies
{
    "name": "s3-security-policy",
    "description": "S3 bucket security requirements",
    "policy_type": "mandatory",
    "rego_code": "package terraform.policies.s3\n..."
}

// 获取策略列表
GET /api/v1/opa-policies

// 更新策略
PUT /api/v1/opa-policies/:id

// 删除策略
DELETE /api/v1/opa-policies/:id

// 测试策略
POST /api/v1/opa-policies/:id/test
{
    "plan_json": {...}
}

// 创建策略集
POST /api/v1/opa-policy-sets
{
    "name": "production-policies",
    "workspace_id": 1,
    "policy_ids": [1, 2, 3]
}

// 获取任务的策略检查结果
GET /api/v1/workspace-tasks/:id/policy-check-results
```

## 2️⃣ Cost Estimation Stage（成本估算）

### 功能概述

成本估算功能分析Terraform Plan，预测基础设施变更对月度成本的影响。

### 执行时机

```
Post-Plan Stage → Cost Estimation → Policy Check → ...
```

### 数据库设计

```sql
-- 成本估算结果表
CREATE TABLE cost_estimates (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    
    -- 成本数据
    prior_monthly_cost DECIMAL(10, 2), -- 变更前月度成本
    proposed_monthly_cost DECIMAL(10, 2), -- 变更后月度成本
    monthly_cost_delta DECIMAL(10, 2), -- 月度成本变化
    
    -- 资源成本明细
    resources JSONB, -- 每个资源的成本详情
    
    -- 元数据
    currency VARCHAR(3) DEFAULT 'USD',
    estimation_method VARCHAR(50), -- 估算方法
    confidence_level VARCHAR(20), -- 置信度：high, medium, low
    
    created_at TIMESTAMP DEFAULT NOW()
);

-- 资源价格表（可选，用于离线估算）
CREATE TABLE resource_prices (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(20) NOT NULL, -- aws, azure, gcp
    region VARCHAR(50) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    instance_type VARCHAR(50),
    pricing_data JSONB NOT NULL, -- 价格详情
    effective_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(provider, region, resource_type, instance_type, effective_date)
);
```

### 实现设计

```go
// 成本估算服务
type CostEstimationService struct {
    db           *gorm.DB
    pricingAPI   PricingAPIClient // AWS/Azure/GCP价格API客户端
    cacheService *CacheService
}

// 成本估算结果
type CostEstimate struct {
    PriorMonthlyCost    float64              `json:"prior_monthly_cost"`
    ProposedMonthlyCost float64              `json:"proposed_monthly_cost"`
    MonthlyCostDelta    float64              `json:"monthly_cost_delta"`
    Resources           []ResourceCostDetail `json:"resources"`
    Currency            string               `json:"currency"`
    ConfidenceLevel     string               `json:"confidence_level"`
}

type ResourceCostDetail struct {
    Address         string  `json:"address"`
    Type            string  `json:"type"`
    Action          string  `json:"action"` // create, update, delete
    PriorCost       float64 `json:"prior_cost"`
    ProposedCost    float64 `json:"proposed_cost"`
    CostDelta       float64 `json:"cost_delta"`
    PricingDetails  map[string]interface{} `json:"pricing_details"`
}

// 执行成本估算
func (s *CostEstimationService) EstimateCost(
    ctx context.Context,
    task *models.WorkspaceTask,
) (*CostEstimate, error) {
    // 1. 解析Plan JSON
    planJSON := task.PlanJSON
    if planJSON == nil {
        return nil, fmt.Errorf("plan JSON is missing")
    }
    
    // 2. 提取资源变更
    resourceChanges, ok := planJSON["resource_changes"].([]interface{})
    if !ok {
        return nil, fmt.Errorf("invalid plan JSON format")
    }
    
    // 3. 估算每个资源的成本
    estimate := &CostEstimate{
        Resources: make([]ResourceCostDetail, 0),
        Currency:  "USD",
    }
    
    for _, rc := range resourceChanges {
        change := rc.(map[string]interface{})
        resourceCost, err := s.estimateResourceCost(ctx, change)
        if err != nil {
            log.Printf("Failed to estimate cost for resource: %v", err)
            continue
        }
        
        estimate.Resources = append(estimate.Resources, *resourceCost)
        estimate.PriorMonthlyCost += resourceCost.PriorCost
        estimate.ProposedMonthlyCost += resourceCost.ProposedCost
    }
    
    // 4. 计算总成本变化
    estimate.MonthlyCostDelta = estimate.ProposedMonthlyCost - estimate.PriorMonthlyCost
    
    // 5. 评估置信度
    estimate.ConfidenceLevel = s.assessConfidenceLevel(estimate.Resources)
    
    // 6. 保存估算结果
    if err := s.saveCostEstimate(task.ID, estimate); err != nil {
        return nil, fmt.Errorf("failed to save cost estimate: %w", err)
    }
    
    return estimate, nil
}

// 估算单个资源成本
func (s *CostEstimationService) estimateResourceCost(
    ctx context.Context,
    change map[string]interface{},
) (*ResourceCostDetail, error) {
    resourceType := change["type"].(string)
    address := change["address"].(string)
    actions := change["change"].(map[string]interface{})["actions"].([]interface{})
    
    detail := &ResourceCostDetail{
        Address: address,
        Type:    resourceType,
        Action:  actions[0].(string),
    }
    
    // 根据资源类型估算成本
    switch resourceType {
    case "aws_instance":
        return s.estimateEC2Cost(ctx, change, detail)
    case "aws_s3_bucket":
        return s.estimateS3Cost(ctx, change, detail)
    case "aws_rds_instance":
        return s.estimateRDSCost(ctx, change, detail)
    default:
        // 未知资源类型，返回0成本
        return detail, nil
    }
}

// 估算EC2实例成本
func (s *CostEstimationService) estimateEC2Cost(
    ctx context.Context,
    change map[string]interface{},
    detail *ResourceCostDetail,
) (*ResourceCostDetail, error) {
    after := change["change"].(map[string]interface{})["after"]
    if after == nil {
        return detail, nil
    }
    
    config := after.(map[string]interface{})
    instanceType := config["instance_type"].(string)
    region := s.getRegionFromConfig(config)
    
    // 从价格API获取价格
    pricing, err := s.pricingAPI.GetEC2Pricing(ctx, region, instanceType)
    if err != nil {
        return nil, err
    }
    
    // 计算月度成本（假设24/7运行）
    hoursPerMonth := 730.0
    detail.ProposedCost = pricing.HourlyRate * hoursPerMonth
    detail.CostDelta = detail.ProposedCost - detail.PriorCost
    detail.PricingDetails = map[string]interface{}{
        "instance_type": instanceType,
        "region":        region,
        "hourly_rate":   pricing.HourlyRate,
        "hours_per_month": hoursPerMonth,
    }
    
    return detail, nil
}
```

### 成本估算API

```go
// 获取任务的成本估算
GET /api/v1/workspace-tasks/:id/cost-estimate

// 响应示例
{
    "prior_monthly_cost": 150.00,
    "proposed_monthly_cost": 280.50,
    "monthly_cost_delta": 130.50,
    "currency": "USD",
    "confidence_level": "high",
    "resources": [
        {
            "address": "aws_instance.web",
            "type": "aws_instance",
            "action": "create",
            "prior_cost": 0,
            "proposed_cost": 73.00,
            "cost_delta": 73.00,
            "pricing_details": {
                "instance_type": "t3.medium",
                "region": "us-east-1",
                "hourly_rate": 0.10,
                "hours_per_month": 730
            }
        }
    ]
}
```

### 价格数据源

#### 选项1: 云厂商API
- AWS Price List API
- Azure Pricing API
- GCP Cloud Billing API

#### 选项2: 第三方服务
- Infracost
- Cloud Custodian
- CloudHealth

#### 选项3: 自维护价格表
- 定期更新resource_prices表
- 适用于离线环境

## 3️⃣ Sentinel Policy Check Stage（Sentinel策略检查）

### 功能概述

Sentinel是HashiCorp的策略即代码框架，专为Terraform设计。支持更复杂的策略逻辑和更细粒度的控制。

### 执行时机

```
Cost Estimation → Sentinel Policy Check → Pre-Apply → ...
```

### 策略类型

#### Hard-Mandatory（硬强制）
- 失败会阻止运行继续
- 不可覆盖
- 直接进入`plan_errored`状态

#### Soft-Mandatory（软强制）
- 失败会暂停运行
- 可以被授权用户覆盖
- 进入`policy_override`状态

#### Advisory（建议）
- 失败不会阻止运行
- 显示警告信息
- 可以继续执行

### 数据库设计

```sql
-- Sentinel策略表
CREATE TABLE sentinel_policies (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    enforcement_level VARCHAR(20) NOT NULL, -- hard-mandatory, soft-mandatory, advisory
    sentinel_code TEXT NOT NULL, -- Sentinel语言代码
    enabled BOOLEAN DEFAULT true,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Sentinel策略集表
CREATE TABLE sentinel_policy_sets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    workspace_id INTEGER REFERENCES workspaces(id),
    global BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

-- 策略集关联表
CREATE TABLE sentinel_policy_set_policies (
    policy_set_id INTEGER REFERENCES sentinel_policy_sets(id) ON DELETE CASCADE,
    policy_id INTEGER REFERENCES sentinel_policies(id) ON DELETE CASCADE,
    PRIMARY KEY (policy_set_id, policy_id)
);

-- Sentinel检查结果表
CREATE TABLE sentinel_policy_check_results (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    policy_id INTEGER REFERENCES sentinel_policies(id),
    passed BOOLEAN NOT NULL,
    violations JSONB,
    execution_time INTEGER,
    overridden BOOLEAN DEFAULT false, -- 是否被覆盖
    overridden_by INTEGER REFERENCES users(id),
    overridden_at TIMESTAMP,
    override_reason TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

### 实现设计

```go
// Sentinel策略服务
type SentinelPolicyService struct {
    db       *gorm.DB
    executor *sentinel.Executor
}

// 执行Sentinel策略检查
func (s *SentinelPolicyService) CheckPolicies(
    ctx context.Context,
    task *models.WorkspaceTask,
) ([]SentinelCheckResult, error) {
    // 1. 获取适用的策略集
    policySets, err := s.getApplicablePolicySets(task.WorkspaceID)
    if err != nil {
        return nil, err
    }
    
    // 2. 获取所有策略
    var policies []models.SentinelPolicy
    for _, ps := range policySets {
        var setPolicies []models.SentinelPolicy
        s.db.Joins("JOIN sentinel_policy_set_policies ON sentinel_policies.id = sentinel_policy_set_policies.policy_id").
            Where("sentinel_policy_set_policies.policy_set_id = ? AND sentinel_policies.enabled = true", ps.ID).
            Find(&setPolicies)
        policies = append(policies, setPolicies...)
    }
    
    // 3. 执行每个策略
    results := make([]SentinelCheckResult, 0)
    for _, policy := range policies {
        result, err := s.evaluatePolicy(ctx, task, &policy)
        if err != nil {
            log.Printf("Failed to evaluate policy %s: %v", policy.Name, err)
            continue
        }
        
        results = append(results, *result)
        s.saveCheckResult(task.ID, result)
    }
    
    return results, nil
}

// 判断是否可以继续
func (s *SentinelPolicyService) CanContinue(results []SentinelCheckResult) (bool, bool, string) {
    hardMandatoryFailed := false
    softMandatoryFailed := false
    failedPolicies := make([]string, 0)
    
    for _, result := range results {
        if !result.Passed && !result.Overridden {
            failedPolicies = append(failedPolicies, result.PolicyName)
            
            switch result.EnforcementLevel {
            case "hard-mandatory":
                hardMandatoryFailed = true
            case "soft-mandatory":
                softMandatoryFailed = true
            }
        }
    }
    
    if hardMandatoryFailed {
        return false, false, fmt.Sprintf("Hard-mandatory policies failed: %s", 
            strings.Join(failedPolicies, ", "))
    }
    
    if softMandatoryFailed {
        return false, true, fmt.Sprintf("Soft-mandatory policies failed (can override): %s", 
            strings.Join(failedPolicies, ", "))
    }
    
    return true, false, ""
}

// 覆盖策略失败
func (s *SentinelPolicyService) OverridePolicy(
    resultID uint,
    userID uint,
    reason string,
) error {
    // 1. 检查用户权限
    if !s.hasOverridePermission(userID) {
        return fmt.Errorf("user does not have override permission")
    }
    
    // 2. 更新结果
    now := time.Now()
    return s.db.Model(&models.SentinelPolicyCheckResult{}).
        Where("id = ?", resultID).
        Updates(map[string]interface{}{
            "overridden":      true,
            "overridden_by":   userID,
            "overridden_at":   now,
            "override_reason": reason,
        }).Error
}
```

### Sentinel策略示例

#### 1. 限制实例类型

```sentinel
import "tfplan/v2" as tfplan

# 允许的实例类型
allowed_instance_types = [
    "t3.micro",
    "t3.small",
    "t3.medium",
]

# 检查所有EC2实例
main = rule {
    all tfplan.resource_changes as _, rc {
        rc.type is "aws_instance" and
        rc.change.actions contains "create" implies
        rc.change.after.instance_type in allowed_instance_types
    }
}
```

#### 2. 强制加密

```sentinel
import "tfplan/v2" as tfplan

# 检查S3 Bucket加密
s3_encryption = rule {
    all tfplan.resource_changes as _, rc {
        rc.type is "aws_s3_bucket" and
        rc.change.actions contains "create" implies
        rc.change.after.server_side_encryption_configuration is not null
    }
}

# 检查EBS卷加密
ebs_encryption = rule {
    all tfplan.resource_changes as _, rc {
        rc.type is "aws_ebs_volume" and
        rc.change.actions contains "create" implies
        rc.change.after.encrypted is true
    }
}

main = rule {
    s3_encryption and ebs_encryption
}
```

### API接口设计

```go
// 创建Sentinel策略
POST /api/v1/sentinel-policies
{
    "name": "instance-type-restriction",
    "description": "Restrict EC2 instance types",
    "enforcement_level": "soft-mandatory",
    "sentinel_code": "import \"tfplan/v2\" as tfplan\n..."
}

// 覆盖策略失败
POST /api/v1/sentinel-policy-check-results/:id/override
{
    "reason": "Emergency deployment approved by CTO"
}

// 获取任务的Sentinel检查结果
GET /api/v1/workspace-tasks/:id/sentinel-check-results
```

## 🔄 集成到执行流程

### 更新HandleCostEstimationStage

```go
func (s *TerraformExecutor) HandleCostEstimationStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    task.Stage = StageCostEstimation
    task.State = StateCostEstimating
    s.db.Save(task)
    
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.CostEstimation.Enabled {
        log.Printf("Task %d: Cost Estimation skipped", task.ID)
        return s.TransitionToPolicyCheck(task)
    }
    
    // 执行成本估算
    estimate, err := s.costEstimationService.EstimateCost(ctx, task)
    if err != nil {
        log.Printf("Cost estimation failed: %v", err)
        // 不阻塞流程
    } else {
        task.Context["cost_estimate"] = estimate
        
        // 更新状态
        task.State = StateCostEstimated
        s.db.Save(task)
        
        log.Printf("Task %d: Cost estimate - Delta: $%.2f/month", 
            task.ID, estimate.MonthlyCostDelta)
        
        // 发送成本通知（如果变化显著）
        if math.Abs(estimate.MonthlyCostDelta) > 100 {
            s.notifySystem.Notify(models.EventCostEstimated, workspace, task)
        }
    }
    
    // 进入下一阶段
    return s.TransitionToPolicyCheck(task)
}
```

### 更新HandlePolicyCheckStage（OPA + Sentinel）

```go
func (s *TerraformExecutor) HandlePolicyCheckStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    task.Stage = StagePolicyCheck
    task.State = StatePolicyCheck
    s.db.Save(task)
    
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.PolicyCheck.Enabled {
        log.Printf("Task %d: Policy Check skipped", task.ID)
        return s.TransitionToPreApply(task)
    }
    
    // 1. 执行OPA策略检查
    opaResults, err := s.opaPolicyService.CheckPolicies(ctx, task)
    if err != nil {
        log.Printf("OPA policy check failed: %v", err)
    }
    
    canContinue, message := s.opaPolicyService.CanContinue(opaResults)
    if !canContinue {
        task.State = StatePlanErrored
        task.ErrorMessage = message
        s.db.Save(task)
        return fmt.Errorf("OPA policy check failed: %s", message)
    }
    
    // 2. 执行Sentinel策略检查
    sentinelResults, err := s.sentinelPolicyService.CheckPolicies(ctx, task)
    if err != nil {
        log.Printf("Sentinel policy check failed: %v", err)
    }
    
    canContinue, needsOverride, message := s.sentinelPolicyService.CanContinue(sentinelResults)
    
    if !canContinue {
        if needsOverride {
            // 软强制失败，等待覆盖
            task.State = StatePolicyOverride
            task.ErrorMessage = message
            s.db.Save(task)
            
            // 发送通知，等待授权用户覆盖
            s.notifySystem.Notify(models.EventPolicyOverrideRequired, workspace, task)
            
            return fmt.Errorf("policy override required: %s", message)
        } else {
            // 硬强制失败
            task.State = StatePlanErrored
            task.ErrorMessage = message
            s.db.Save(task)
            return fmt.Errorf("hard-mandatory policy failed: %s", message)
        }
    }
    
    // 3. 策略检查通过
    task.State = StatePolicyChecked
    s.db.Save(task)
    
    log.Printf("Task %d: Policy Check passed", task.ID)
    
    // 4. 进入下一阶段
    return s.TransitionToPreApply(task)
}
```

## 🎨 前端UI设计

### 1. OPA策略管理页面

```typescript
// pages/OPAPolicies.tsx
interface OPAPolicy {
    id: number;
    name: string;
    description: string;
    policy_type: 'mandatory' | 'advisory';
    rego_code: string;
    enabled: boolean;
}

// 功能：
// - 策略列表展示
// - 创建/编辑策略（Monaco Editor编辑Rego代码）
// - 测试策略（使用示例Plan JSON）
// - 启用/禁用策略
// - 删除策略
```

### 2. 成本估算展示

```typescript
// components/CostEstimateCard.tsx
interface CostEstimate {
    prior_monthly_cost: number;
    proposed_monthly_cost: number;
    monthly_cost_delta: number;
    currency: string;
    confidence_level: string;
    resources: ResourceCostDetail[];
}

// 展示内容：
// - 成本变化总览（卡片）
// - 资源成本明细表
// - 成本趋势图表
// - 置信度指示器
```

### 3. 策略检查结果展示

```typescript
// components/PolicyCheckResults.tsx
interface PolicyCheckResult {
    policy_name: string;
    policy_type: string;
    passed: boolean;
    violations: PolicyViolation[];
}

// 展示内容：
// - 策略检查状态（通过/失败）
// - 违规详情列表
// - 覆盖按钮（软强制策略）
// - 违规资源高亮
```

## 📊 实施路线图

### Phase 1: 基础功能（当前）
-  核心执行流程
-  Plan和Apply
-  State管理
-  基础日志

### Phase 2: 扩展钩子（近期）
- Pre-Plan钩子
- Post-Plan钩子
- Pre-Apply钩子
- Post-Apply钩子

### Phase 3: OPA策略（中期）
- OPA策略管理
- 策略集配置
- 策略检查执行
- 结果展示

### Phase 4: 成本估算（中期）
- 价格数据集成
- 成本计算引擎
- 成本趋势分析
- 预算告警

### Phase 5: Sentinel策略（远期）
- Sentinel策略管理
- 策略覆盖机制
- 高级策略逻辑
- 审计日志

## 🔗 第三方集成

### OPA集成

```bash
# 安装OPA
go get github.com/open-policy-agent/opa/sdk

# 启动OPA服务器
opa run --server --addr localhost:8181
```

### Infracost集成（成本估算）

```bash
# 安装Infracost
brew install infracost

# 使用Infracost估算成本
infracost breakdown --path plan.json --format json
```

### Sentinel集成

```bash
# 安装Sentinel
# 需要HashiCorp Enterprise许可证
```

## 📝 配置示例

### Workspace配置启用高级功能

```json
{
  "run_config": {
    "cost_estimation": {
      "enabled": true,
      "timeout": 300,
      "hooks": ["cost-alert"],
      "metadata": {
        "alert_threshold": 100,
        "use_infracost": true
      }
    },
    "policy_check": {
      "enabled": true,
      "timeout": 600,
      "hooks": ["policy-report"],
      "metadata": {
        "opa_enabled": true,
        "sentinel_enabled": false,
        "fail_on_violation": true
      }
    }
  }
}
```

## 🎯 总结

### 核心价值

1. **OPA策略检查** - 开源、灵活、易于集成
2. **成本估算** - 预测成本变化，避免意外支出
3. **Sentinel策略** - 企业级策略管理，细粒度控制

### 实施建议

1. **优先级**: 先实现核心执行流程，再考虑高级功能
2. **OPA优先**: OPA是开源的，实施成本低
3. **成本估算**: 可以先集成Infracost，后续自建
4. **Sentinel**: 需要企业许可证，最后考虑

### 技术选型

| 功能 | 推荐方案 | 理由 |
|------|----------|------|
| 策略检查 | OPA | 开源、社区活跃、易于集成 |
| 成本估算 | Infracost | 成熟、准确、支持多云 |
| 企业策略 | Sentinel | HashiCorp官方、功能强大 |

---

**注意**: 这些功能都是未来扩展，当前不是刚需。建议先完成核心执行流程，再根据实际需求逐步添加。
