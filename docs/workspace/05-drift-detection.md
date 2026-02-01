# Workspace模块 - AI漂移检测

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 未来扩展（第二版功能）

## 📘 概述

Drift检测是指检测实际基础设施状态与Terraform配置的差异。AI漂移检测通过人工智能分析这些差异，提供智能化的风险评估和修复建议。

## 🎯 核心功能

### 1. 周期性检测

**触发方式**:
- 定时任务（如每小时、每天）
- 手动触发
- 事件触发（如配置变更）

**检测流程**:
```
1. 调度器触发检测任务
   ↓
2. 执行terraform plan -refresh-only
   ↓
3. 对比实际状态与期望状态
   ↓
4. 识别漂移资源
   ↓
5. AI分析漂移原因和影响
   ↓
6. 生成检测报告
   ↓
7. 发送通知（on_drift_detected事件）
```

### 2. AI分析

**分析维度**:
- **漂移类型**: 配置漂移、资源删除、未授权变更
- **影响范围**: 单个资源、资源组、整个环境
- **风险等级**: 低、中、高、严重
- **根因分析**: 手动变更、自动扩缩容、外部系统

**AI模型**:
- 使用GPT-4或Claude进行自然语言分析
- 基于历史数据训练的分类模型
- 异常检测算法

### 3. 智能报告

**报告内容**:
- 漂移摘要
- 详细变更列表
- 风险评估
- 修复建议
- 历史趋势

**报告格式**:
```json
{
  "workspace_id": 1,
  "detection_time": "2025-10-09T10:00:00Z",
  "drift_detected": true,
  "summary": {
    "total_resources": 50,
    "drifted_resources": 3,
    "risk_level": "medium"
  },
  "drifts": [
    {
      "resource": "aws_instance.web",
      "type": "configuration_drift",
      "changes": {
        "instance_type": {
          "expected": "t2.micro",
          "actual": "t2.small"
        }
      },
      "risk_level": "medium",
      "ai_analysis": {
        "cause": "Manual change via AWS Console",
        "impact": "Increased cost by $10/month",
        "recommendation": "Revert to t2.micro or update Terraform config"
      }
    }
  ],
  "ai_summary": "3 resources have drifted from their expected state..."
}
```

## 🔄 检测流程详解

### 1. 调度配置

**数据模型**:
```go
type DriftDetectionConfig struct {
    ID          uint      `json:"id"`
    WorkspaceID uint      `json:"workspace_id"`
    Enabled     bool      `json:"enabled"`
    Schedule    string    `json:"schedule"` // Cron表达式
    LastRun     time.Time `json:"last_run"`
    NextRun     time.Time `json:"next_run"`
}
```

**Cron示例**:
```
0 * * * *     # 每小时
0 0 * * *     # 每天午夜
0 0 * * 0     # 每周日
0 0 1 * *     # 每月1号
```

### 2. 检测执行

**执行逻辑**:
```go
func (s *DriftDetectionService) RunDetection(workspaceID uint) (*DriftReport, error) {
    // 1. 获取Workspace
    workspace, err := s.workspaceService.GetWorkspace(workspaceID)
    if err != nil {
        return nil, err
    }
    
    // 2. 执行terraform plan -refresh-only
    executor := s.selectExecutor(workspace.ExecutionMode)
    result, err := executor.ExecutePlan(&WorkspaceTask{
        WorkspaceID: workspaceID,
        TaskType:    TaskTypeDriftCheck,
        Options:     map[string]interface{}{"refresh_only": true},
    })
    
    if err != nil {
        return nil, err
    }
    
    // 3. 解析Plan输出，识别漂移
    drifts := s.parseDrifts(result.PlanJSON)
    
    if len(drifts) == 0 {
        return &DriftReport{DriftDetected: false}, nil
    }
    
    // 4. AI分析
    aiAnalysis, err := s.aiService.AnalyzeDrifts(drifts)
    if err != nil {
        log.Error("AI analysis failed:", err)
        // 继续执行，不阻塞
    }
    
    // 5. 生成报告
    report := &DriftReport{
        WorkspaceID:    workspaceID,
        DetectionTime:  time.Now(),
        DriftDetected:  true,
        Drifts:         drifts,
        AIAnalysis:     aiAnalysis,
    }
    
    // 6. 保存报告
    s.db.Create(report)
    
    // 7. 发送通知
    s.notificationService.Send("drift_detected", report)
    
    return report, nil
}
```

### 3. AI分析实现

**AI服务接口**:
```go
type AIService interface {
    AnalyzeDrifts(drifts []Drift) (*AIAnalysis, error)
    SuggestFix(drift Drift) (string, error)
    PredictImpact(drift Drift) (*ImpactPrediction, error)
}
```

**GPT-4分析示例**:
```go
func (s *GPT4Service) AnalyzeDrifts(drifts []Drift) (*AIAnalysis, error) {
    prompt := fmt.Sprintf(`
Analyze the following infrastructure drifts and provide:
1. Root cause analysis
2. Risk assessment
3. Remediation recommendations

Drifts:
%s
`, formatDrifts(drifts))
    
    response, err := s.client.CreateChatCompletion(
        context.Background(),
        openai.ChatCompletionRequest{
            Model: openai.GPT4,
            Messages: []openai.ChatCompletionMessage{
                {
                    Role:    openai.ChatMessageRoleSystem,
                    Content: "You are an infrastructure expert analyzing Terraform drift.",
                },
                {
                    Role:    openai.ChatMessageRoleUser,
                    Content: prompt,
                },
            },
        },
    )
    
    if err != nil {
        return nil, err
    }
    
    return parseAIResponse(response.Choices[0].Message.Content), nil
}
```

## 📊 漂移类型

### 1. 配置漂移

**定义**: 资源存在但配置与期望不符

**示例**:
```
Resource: aws_instance.web
Expected: instance_type = "t2.micro"
Actual:   instance_type = "t2.small"
```

**常见原因**:
- 手动修改
- 自动扩缩容
- 外部系统变更

### 2. 资源删除

**定义**: Terraform管理的资源被删除

**示例**:
```
Resource: aws_s3_bucket.data
Expected: exists
Actual:   not found
```

**常见原因**:
- 误删除
- 清理脚本
- 权限问题

### 3. 未授权资源

**定义**: 存在未在Terraform中定义的资源

**示例**:
```
Resource: aws_instance.unknown
Expected: not managed
Actual:   exists with tag "managed-by: terraform"
```

**常见原因**:
- 手动创建
- 其他工具创建
- 配置遗漏

## 🎯 风险评估

### 风险等级

**低风险**:
- 标签变更
- 描述变更
- 非关键配置

**中风险**:
- 实例类型变更
- 安全组规则变更
- 网络配置变更

**高风险**:
- 数据库配置变更
- 加密设置变更
- 访问控制变更

**严重风险**:
- 资源删除
- 数据丢失风险
- 安全漏洞

### 评估算法

```go
func (s *DriftDetectionService) AssessRisk(drift Drift) RiskLevel {
    score := 0
    
    // 资源类型权重
    if drift.ResourceType == "aws_rds_instance" {
        score += 30
    } else if drift.ResourceType == "aws_s3_bucket" {
        score += 20
    }
    
    // 变更类型权重
    if drift.ChangeType == "delete" {
        score += 50
    } else if drift.ChangeType == "modify" {
        score += 20
    }
    
    // 属性权重
    for attr := range drift.Changes {
        if attr == "encryption" || attr == "public_access" {
            score += 30
        }
    }
    
    // 评级
    if score >= 80 {
        return RiskCritical
    } else if score >= 50 {
        return RiskHigh
    } else if score >= 20 {
        return RiskMedium
    }
    return RiskLow
}
```

## 🔧 修复建议

### 自动修复

**适用场景**:
- 低风险漂移
- 可逆变更
- 已验证的修复方案

**实现**:
```go
func (s *DriftDetectionService) AutoFix(driftID uint) error {
    drift, err := s.GetDrift(driftID)
    if err != nil {
        return err
    }
    
    // 只自动修复低风险漂移
    if drift.RiskLevel != RiskLow {
        return errors.New("auto-fix only available for low-risk drifts")
    }
    
    // 创建Apply任务
    task := &WorkspaceTask{
        WorkspaceID: drift.WorkspaceID,
        TaskType:    TaskTypeApply,
        Message:     fmt.Sprintf("Auto-fix drift: %s", drift.Resource),
    }
    
    return s.taskService.CreateTask(task)
}
```

### 手动修复

**修复选项**:
1. **更新Terraform配置**: 接受实际状态
2. **执行Apply**: 恢复到期望状态
3. **忽略**: 标记为已知漂移

## 📈 历史趋势

### 趋势分析

**指标**:
- 漂移频率
- 漂移类型分布
- 风险等级分布
- 修复时间

**可视化**:
```
漂移趋势图:
  ┌─────────────────────────────────┐
  │ 30 ┤                        ●    │
  │ 25 ┤                   ●         │
  │ 20 ┤              ●              │
  │ 15 ┤         ●                   │
  │ 10 ┤    ●                        │
  │  5 ┤●                            │
  └─────────────────────────────────┘
    Mon Tue Wed Thu Fri Sat Sun
```

## 🔔 通知事件

### on_drift_detected

**触发时机**: 检测到漂移

**Payload**:
```json
{
  "event": "drift_detected",
  "workspace_id": 1,
  "workspace_name": "production-infra",
  "detection_time": "2025-10-09T10:00:00Z",
  "drift_count": 3,
  "risk_level": "medium",
  "report_url": "https://platform.example.com/workspaces/1/drift-reports/123"
}
```

### on_drift_resolved

**触发时机**: 漂移已修复

**Payload**:
```json
{
  "event": "drift_resolved",
  "workspace_id": 1,
  "resolution_time": "2025-10-09T11:00:00Z",
  "resolution_method": "apply",
  "resolved_by": "admin@example.com"
}
```

## 🚀 未来扩展

### 1. 预测性检测

使用机器学习预测可能的漂移

### 2. 自动修复策略

基于规则的自动修复决策

### 3. 成本影响分析

计算漂移对成本的影响

### 4. 合规性检查

检查漂移是否违反合规策略

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [04-task-workflow.md](./04-task-workflow.md) - 任务工作流
- [06-notification-system.md](./06-notification-system.md) - 通知系统
