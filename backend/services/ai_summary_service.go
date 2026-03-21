package services

import (
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"
	"log"
	"time"

	"gorm.io/gorm"
)

// AISummaryService AI 执行摘要服务
type AISummaryService struct {
	db             *gorm.DB
	configService  *AIConfigService
	skillAssembler *SkillAssembler
}

// NewAISummaryService 创建摘要服务
func NewAISummaryService(db *gorm.DB) *AISummaryService {
	return &AISummaryService{
		db:             db,
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
	}
}

// defaultPlanSummaryPrompt Plan Summary 默认系统 prompt
const defaultPlanSummaryPrompt = `你是基础设施变更影响分析专家。分析 Terraform plan 的变更内容，评估影响范围和风险等级。

你可以使用提供的工具查询更多信息：
- 如果变更只包含 module 的部分资源，使用 query_module_resources 查看完整 module 资源列表
- 如果需要了解资源的依赖关系，使用 query_cmdb_dependencies 查询谁引用了被变更的资源
- 如果需要查看资源的详细属性，使用 query_resource_attributes

当你收集到足够信息后，请输出最终分析结果，JSON 格式：
{
  "changes_overview": "变更概述（自然语言）",
  "impact_analysis": { "summary": "...", "details": [...] },
  "affected_resources": [ { "address": "...", "type": "...", "impact": "..." } ],
  "risk_level": "low|medium|high|critical"
}`

// defaultApplySummaryPrompt Apply Summary 默认系统 prompt
const defaultApplySummaryPrompt = `你是基础设施执行结果分析专家。分析 Terraform apply 的执行结果，总结变更情况。

你可以使用提供的工具查询更多信息：
- 使用 query_plan_summary 获取 Plan 阶段的影响预测，对比实际执行结果
- 使用 query_cmdb_dependencies 查询受影响资源的依赖方
- 使用 query_state_resources 查看当前资源全貌
- 使用 query_resource_attributes 查看资源详细属性

当你收集到足够信息后，请输出最终分析结果，JSON 格式：
{
  "execution_summary": "执行结果总结（自然语言）",
  "resource_results": [ { "address": "...", "action": "...", "status": "..." } ],
  "impact_confirmation": { "predicted_vs_actual": "...", "unexpected_changes": [...] },
  "affected_resources": [ { "address": "...", "type": "...", "impact": "..." } ]
}`

// GeneratePlanSummary 生成 Plan 阶段摘要（异步调用）
func (s *AISummaryService) GeneratePlanSummary(taskID uint) {
	startTime := time.Now()

	// 前置检查：AI 配置
	cfg, err := s.configService.GetConfigForCapability("summary")
	if err != nil || cfg == nil {
		log.Printf("[AISummaryService] No AI config for 'summary' capability, skipping plan summary for task %d", taskID)
		return
	}

	// 前置检查：任务存在且有变更
	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		log.Printf("[AISummaryService] Task %d not found: %v", taskID, err)
		return
	}

	totalChanges := task.ChangesAdd + task.ChangesChange + task.ChangesDestroy
	if totalChanges == 0 {
		log.Printf("[AISummaryService] Task %d has no changes, skipping plan summary", taskID)
		return
	}

	// 前置检查：防止重复
	var existing models.AIPlanSummary
	if err := s.db.Where("task_id = ?", taskID).First(&existing).Error; err == nil {
		log.Printf("[AISummaryService] Plan summary already exists for task %d (id=%s), skipping", taskID, existing.ID)
		return
	}

	// 生成 ID 并插入 running 记录
	id, err := infrastructure.GeneratePlanSummaryID()
	if err != nil {
		log.Printf("[AISummaryService] Failed to generate plan summary ID: %v", err)
		return
	}

	summary := models.AIPlanSummary{
		ID:          id,
		TaskID:      taskID,
		WorkspaceID: task.WorkspaceID,
		Status:      "running",
		CreatedAt:   time.Now(),
	}
	if err := s.db.Create(&summary).Error; err != nil {
		log.Printf("[AISummaryService] Failed to create plan summary record: %v", err)
		return
	}

	// 提取 plan 变更
	planChangesJSON, _ := json.Marshal(s.extractPlanChanges(task.PlanJSON))

	// 构建 system prompt
	systemPrompt := s.buildSystemPrompt(cfg, "plan")

	// 构建 user prompt
	userPrompt := fmt.Sprintf(
		"请分析以下 Terraform Plan 变更：\n\n"+
			"工作空间: %s\n"+
			"变更统计: 新增 %d, 修改 %d, 删除 %d\n\n"+
			"资源变更详情:\n%s",
		task.WorkspaceID, task.ChangesAdd, task.ChangesChange, task.ChangesDestroy,
		string(planChangesJSON),
	)

	// 创建 Agent Loop
	caller := NewAICallerFromConfig(cfg)
	loop := NewAIAgentLoop(caller, 10)
	loop.RegisterTool(NewQueryModuleResourcesTool(s.db))
	loop.RegisterTool(NewQueryCMDBDependenciesTool(s.db))
	loop.RegisterTool(NewQueryResourceAttributesTool(s.db))
	loop.RegisterTool(NewQueryStateResourcesTool(s.db))

	// 运行
	result, err := loop.Run(contextWithTimeout(), systemPrompt, userPrompt)
	if err != nil {
		s.failSummary(&summary, err, startTime)
		return
	}

	// 解析 AI 输出
	s.completePlanSummary(&summary, result, planChangesJSON, startTime)
}

// GenerateApplySummary 生成 Apply 阶段摘要（异步调用）
func (s *AISummaryService) GenerateApplySummary(taskID uint) {
	startTime := time.Now()

	cfg, err := s.configService.GetConfigForCapability("summary")
	if err != nil || cfg == nil {
		log.Printf("[AISummaryService] No AI config for 'summary' capability, skipping apply summary for task %d", taskID)
		return
	}

	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		log.Printf("[AISummaryService] Task %d not found: %v", taskID, err)
		return
	}

	var existing models.AIApplySummary
	if err := s.db.Where("task_id = ?", taskID).First(&existing).Error; err == nil {
		log.Printf("[AISummaryService] Apply summary already exists for task %d, skipping", taskID)
		return
	}

	id, err := infrastructure.GenerateApplySummaryID()
	if err != nil {
		log.Printf("[AISummaryService] Failed to generate apply summary ID: %v", err)
		return
	}

	summary := models.AIApplySummary{
		ID:          id,
		TaskID:      taskID,
		WorkspaceID: task.WorkspaceID,
		Status:      "running",
		CreatedAt:   time.Now(),
	}
	if err := s.db.Create(&summary).Error; err != nil {
		log.Printf("[AISummaryService] Failed to create apply summary record: %v", err)
		return
	}

	systemPrompt := s.buildSystemPrompt(cfg, "apply")

	// 构建 apply 上下文
	applyContext := fmt.Sprintf(
		"请分析以下 Terraform Apply 执行结果：\n\n"+
			"工作空间: %s\n"+
			"任务 ID: %d\n"+
			"变更统计: 新增 %d, 修改 %d, 删除 %d\n"+
			"执行状态: %s\n",
		task.WorkspaceID, task.ID,
		task.ChangesAdd, task.ChangesChange, task.ChangesDestroy,
		string(task.Status),
	)

	caller := NewAICallerFromConfig(cfg)
	loop := NewAIAgentLoop(caller, 10)
	loop.RegisterTool(NewQueryModuleResourcesTool(s.db))
	loop.RegisterTool(NewQueryCMDBDependenciesTool(s.db))
	loop.RegisterTool(NewQueryResourceAttributesTool(s.db))
	loop.RegisterTool(NewQueryStateResourcesTool(s.db))
	loop.RegisterTool(NewQueryPlanSummaryTool(s.db))

	result, err := loop.Run(contextWithTimeout(), systemPrompt, applyContext)
	if err != nil {
		s.failApplySummary(&summary, err, startTime)
		return
	}

	s.completeApplySummary(&summary, result, startTime)
}

// GetPlanSummary 查询 Plan Summary
func (s *AISummaryService) GetPlanSummary(taskID uint) *models.AIPlanSummary {
	var summary models.AIPlanSummary
	if err := s.db.Where("task_id = ?", taskID).First(&summary).Error; err != nil {
		return nil
	}
	return &summary
}

// GetApplySummary 查询 Apply Summary
func (s *AISummaryService) GetApplySummary(taskID uint) *models.AIApplySummary {
	var summary models.AIApplySummary
	if err := s.db.Where("task_id = ?", taskID).First(&summary).Error; err != nil {
		return nil
	}
	return &summary
}

// RetryPlanSummary 重试 Plan Summary
func (s *AISummaryService) RetryPlanSummary(taskID uint) error {
	var existing models.AIPlanSummary
	if err := s.db.Where("task_id = ?", taskID).First(&existing).Error; err != nil {
		return fmt.Errorf("no plan summary found for task %d", taskID)
	}
	if existing.Status != "failed" {
		return fmt.Errorf("can only retry failed summaries, current status: %s", existing.Status)
	}

	s.db.Delete(&existing)
	go s.GeneratePlanSummary(taskID)
	return nil
}

// RetryApplySummary 重试 Apply Summary
func (s *AISummaryService) RetryApplySummary(taskID uint) error {
	var existing models.AIApplySummary
	if err := s.db.Where("task_id = ?", taskID).First(&existing).Error; err != nil {
		return fmt.Errorf("no apply summary found for task %d", taskID)
	}
	if existing.Status != "failed" {
		return fmt.Errorf("can only retry failed summaries, current status: %s", existing.Status)
	}

	s.db.Delete(&existing)
	go s.GenerateApplySummary(taskID)
	return nil
}

// ========== internal helpers ==========

func (s *AISummaryService) buildSystemPrompt(cfg *models.AIConfig, stage string) string {
	// 检查 capability prompt
	capKey := "summary"
	if prompt, ok := cfg.CapabilityPrompts[capKey]; ok && prompt != "" {
		return prompt
	}

	// skill 模式
	if cfg.Mode == "skill" && s.skillAssembler != nil {
		composition := &cfg.SkillComposition
		if len(composition.FoundationSkills) > 0 || composition.TaskSkill != "" {
			result, err := s.skillAssembler.AssemblePrompt(composition, 0, &DynamicContext{})
			if err == nil && result.Prompt != "" {
				return result.Prompt
			}
		}
	}

	// 默认 prompt
	if stage == "apply" {
		return defaultApplySummaryPrompt
	}
	return defaultPlanSummaryPrompt
}

func (s *AISummaryService) extractPlanChanges(planJSON models.JSONB) interface{} {
	if planJSON == nil {
		return nil
	}

	var plan map[string]interface{}
	data, err := json.Marshal(planJSON)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil
	}

	if changes, ok := plan["resource_changes"]; ok {
		return changes
	}
	return plan
}

func (s *AISummaryService) completePlanSummary(summary *models.AIPlanSummary, result *AgentLoopResult, planChangesJSON []byte, startTime time.Time) {
	// 解析 AI 输出
	var aiOutput struct {
		ChangesOverview   string      `json:"changes_overview"`
		ImpactAnalysis    interface{} `json:"impact_analysis"`
		AffectedResources interface{} `json:"affected_resources"`
		RiskLevel         string      `json:"risk_level"`
	}

	outputText := extractJSON(result.FinalOutput)
	json.Unmarshal([]byte(outputText), &aiOutput)

	summary.ChangesOverview = aiOutput.ChangesOverview
	if aiOutput.ImpactAnalysis != nil {
		summary.ImpactAnalysis, _ = json.Marshal(aiOutput.ImpactAnalysis)
	}
	if aiOutput.AffectedResources != nil {
		summary.AffectedResources, _ = json.Marshal(aiOutput.AffectedResources)
	}
	summary.RiskLevel = aiOutput.RiskLevel
	summary.PlanChanges = planChangesJSON
	summary.ToolCalls, _ = json.Marshal(result.ToolCalls)
	summary.Status = "completed"
	summary.Duration = int(time.Since(startTime).Milliseconds())

	if err := s.db.Save(summary).Error; err != nil {
		log.Printf("[AISummaryService] Failed to save plan summary: %v", err)
	} else {
		log.Printf("[AISummaryService] Plan summary completed for task %d (id=%s, duration=%dms, steps=%d)",
			summary.TaskID, summary.ID, summary.Duration, result.TotalSteps)
	}
}

func (s *AISummaryService) completeApplySummary(summary *models.AIApplySummary, result *AgentLoopResult, startTime time.Time) {
	var aiOutput struct {
		ExecutionSummary    string      `json:"execution_summary"`
		ResourceResults     interface{} `json:"resource_results"`
		ImpactConfirmation  interface{} `json:"impact_confirmation"`
		AffectedResources   interface{} `json:"affected_resources"`
	}

	outputText := extractJSON(result.FinalOutput)
	json.Unmarshal([]byte(outputText), &aiOutput)

	summary.ExecutionSummary = aiOutput.ExecutionSummary
	if aiOutput.ResourceResults != nil {
		summary.ResourceResults, _ = json.Marshal(aiOutput.ResourceResults)
	}
	if aiOutput.ImpactConfirmation != nil {
		summary.ImpactConfirmation, _ = json.Marshal(aiOutput.ImpactConfirmation)
	}
	if aiOutput.AffectedResources != nil {
		summary.AffectedResources, _ = json.Marshal(aiOutput.AffectedResources)
	}
	summary.ToolCalls, _ = json.Marshal(result.ToolCalls)
	summary.Status = "completed"
	summary.Duration = int(time.Since(startTime).Milliseconds())

	if err := s.db.Save(summary).Error; err != nil {
		log.Printf("[AISummaryService] Failed to save apply summary: %v", err)
	} else {
		log.Printf("[AISummaryService] Apply summary completed for task %d (id=%s, duration=%dms)",
			summary.TaskID, summary.ID, summary.Duration)
	}
}

func (s *AISummaryService) failSummary(summary *models.AIPlanSummary, err error, startTime time.Time) {
	summary.Status = "failed"
	summary.ErrorMessage = err.Error()
	summary.Duration = int(time.Since(startTime).Milliseconds())
	s.db.Save(summary)
	log.Printf("[AISummaryService] Plan summary failed for task %d: %v", summary.TaskID, err)
}

func (s *AISummaryService) failApplySummary(summary *models.AIApplySummary, err error, startTime time.Time) {
	summary.Status = "failed"
	summary.ErrorMessage = err.Error()
	summary.Duration = int(time.Since(startTime).Milliseconds())
	s.db.Save(summary)
	log.Printf("[AISummaryService] Apply summary failed for task %d: %v", summary.TaskID, err)
}

// contextWithTimeout 创建带超时的 context（5 分钟，agent loop 可能多轮调用）
func contextWithTimeout() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Minute)
	return ctx
}
