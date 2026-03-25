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

	// 前置检查：能力开关
	featureService := NewAIFeatureService(s.db)
	if !featureService.IsFeatureEnabled("execute_summary") {
		log.Printf("[AISummaryService] Execute summary feature disabled, skipping plan summary for task %d", taskID)
		return
	}

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
	loop.SetOutputValidator(planSummaryValidator)

	// 运行
	ctx, cancel := contextWithTimeout()
	defer cancel()

	result, err := loop.Run(ctx, systemPrompt, userPrompt)
	if err != nil {
		s.failSummary(&summary, err, startTime)
		return
	}

	// 记录 Skill 使用日志
	s.logSummarySkillUsage(cfg, "plan_summary", task.WorkspaceID, taskID, userPrompt, result.FinalOutput, startTime)

	// 解析 AI 输出
	s.completePlanSummary(&summary, result, planChangesJSON, startTime)
}

// GenerateApplySummary 生成 Apply 阶段摘要（异步调用）
func (s *AISummaryService) GenerateApplySummary(taskID uint) {
	startTime := time.Now()

	featureService := NewAIFeatureService(s.db)
	if !featureService.IsFeatureEnabled("execute_summary") {
		log.Printf("[AISummaryService] Execute summary feature disabled, skipping apply summary for task %d", taskID)
		return
	}

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
	loop.SetOutputValidator(applySummaryValidator)

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result, err := loop.Run(ctx, systemPrompt, applyContext)
	if err != nil {
		s.failApplySummary(&summary, err, startTime)
		return
	}

	// 记录 Skill 使用日志
	s.logSummarySkillUsage(cfg, "apply_summary", task.WorkspaceID, taskID, applyContext, result.FinalOutput, startTime)

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

	if err := s.db.Delete(&existing).Error; err != nil {
		return fmt.Errorf("failed to delete old plan summary: %w", err)
	}
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

	if err := s.db.Delete(&existing).Error; err != nil {
		return fmt.Errorf("failed to delete old apply summary: %w", err)
	}
	go s.GenerateApplySummary(taskID)
	return nil
}

// ========== internal helpers ==========

// planSummaryValidator 验证 Plan Summary 的 AI 输出（兼容 V2 和 V3）
var planSummaryValidator OutputValidator = func(output string) error {
	text := extractJSON(output)
	var parsed struct {
		ChangesOverview string `json:"changes_overview"`
		RiskLevel       string `json:"risk_level"` // V2
		RiskEvaluation  *struct {
			RiskLevel string `json:"risk_level"`
		} `json:"risk_evaluation"` // V3
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return fmt.Errorf("输出不是有效的 JSON 格式，请确保输出纯 JSON（不要包含 markdown 代码块标记）")
	}
	if parsed.ChangesOverview == "" {
		return fmt.Errorf("缺少 changes_overview 字段，请确保包含变更概述")
	}
	riskLevel := parsed.RiskLevel
	if parsed.RiskEvaluation != nil && parsed.RiskEvaluation.RiskLevel != "" {
		riskLevel = parsed.RiskEvaluation.RiskLevel
	}
	if riskLevel == "" {
		return fmt.Errorf("缺少 risk_level 字段，请确保包含风险等级（low/medium/high/critical）")
	}
	return nil
}

// applySummaryValidator 验证 Apply Summary 的 AI 输出
var applySummaryValidator OutputValidator = func(output string) error {
	text := extractJSON(output)
	var parsed struct {
		ExecutionSummary string `json:"execution_summary"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return fmt.Errorf("输出不是有效的 JSON 格式，请确保输出纯 JSON（不要包含 markdown 代码块标记）")
	}
	if parsed.ExecutionSummary == "" {
		return fmt.Errorf("缺少 execution_summary 字段，请确保包含执行总结")
	}
	return nil
}

func (s *AISummaryService) buildSystemPrompt(cfg *models.AIConfig, stage string) string {
	// skill 模式优先
	if cfg.Mode == "skill" && s.skillAssembler != nil {
		composition := &cfg.SkillComposition
		// 如果没有配置 task skill，使用默认的 execute_summary_workflow
		if composition.TaskSkill == "" && len(composition.FoundationSkills) == 0 {
			composition = s.getDefaultSummarySkillComposition()
		}
		if len(composition.FoundationSkills) > 0 || composition.TaskSkill != "" {
			result, err := s.skillAssembler.AssemblePrompt(composition, 0, &DynamicContext{
				UseCMDB: true,
				ExtraContext: map[string]interface{}{
					"stage": stage,
				},
			})
			if err == nil && result.Prompt != "" {
				log.Printf("[AISummaryService] Using skill mode prompt (skills: %v, stage: %s)", result.UsedSkillNames, stage)
				return result.Prompt
			}
			log.Printf("[AISummaryService] Skill assembly failed, falling back to prompt mode: %v", err)
		}
	}

	// prompt 模式：检查 capability prompt
	if prompt, ok := cfg.CapabilityPrompts["summary"]; ok && prompt != "" {
		return prompt
	}

	// 默认 prompt
	if stage == "apply" {
		return defaultApplySummaryPrompt
	}
	return defaultPlanSummaryPrompt
}

// getDefaultSummarySkillComposition 获取默认的 Summary Skill 组合配置
// 注意：foundation_skills 由用户在 AI Config UI 上配置，这里只提供 task skill 默认值
func (s *AISummaryService) getDefaultSummarySkillComposition() *models.SkillComposition {
	return &models.SkillComposition{
		FoundationSkills: []string{},
		TaskSkill:        "execute_summary_workflow",
	}
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
	// 解析 AI 输出（V3 结构：risk_evaluation 嵌套）
	var aiOutput struct {
		ChangesOverview   string      `json:"changes_overview"`
		ImpactAnalysis    interface{} `json:"impact_analysis"`
		AffectedResources interface{} `json:"affected_resources"`
		RiskLevel         string      `json:"risk_level"` // V2 兼容
		RiskEvaluation    *struct {
			RiskLevel                string          `json:"risk_level"`
			Confidence               string          `json:"confidence"`
			RequiresHumanConfirmation bool            `json:"requires_human_confirmation"`
			DecisionHintsRaw         json.RawMessage `json:"decision_hints"`
		} `json:"risk_evaluation"`
	}

	log.Printf("[AISummaryService] AI raw output for task %d (len=%d): %.500s", summary.TaskID, len(result.FinalOutput), result.FinalOutput)

	outputText := extractJSON(result.FinalOutput)
	if err := json.Unmarshal([]byte(outputText), &aiOutput); err != nil {
		log.Printf("[AISummaryService] Failed to parse AI output as JSON for task %d: %v, extracted text (len=%d): %.300s", summary.TaskID, err, len(outputText), outputText)
		aiOutput.ChangesOverview = result.FinalOutput
	}

	summary.ChangesOverview = aiOutput.ChangesOverview
	if aiOutput.ImpactAnalysis != nil {
		summary.ImpactAnalysis, _ = json.Marshal(aiOutput.ImpactAnalysis)
	}
	if aiOutput.AffectedResources != nil {
		summary.AffectedResources, _ = json.Marshal(aiOutput.AffectedResources)
	}

	// 提取 risk_level（V3 嵌套结构优先，V2 顶层兼容）
	if aiOutput.RiskEvaluation != nil && aiOutput.RiskEvaluation.RiskLevel != "" {
		summary.RiskLevel = aiOutput.RiskEvaluation.RiskLevel
	} else if aiOutput.RiskLevel != "" {
		summary.RiskLevel = aiOutput.RiskLevel
	}

	// 提取决策字段
	if aiOutput.RiskEvaluation != nil {
		summary.RequiresConfirmation = aiOutput.RiskEvaluation.RequiresHumanConfirmation
		if len(aiOutput.RiskEvaluation.DecisionHintsRaw) > 0 {
			s.parseDecisionHints(summary, aiOutput.RiskEvaluation.DecisionHintsRaw)
		}
	}

	// V2 兜底：如果 AI 没有返回 risk_evaluation 结构（V2 格式），
	// 根据 risk_level 自动判断是否需要人工确认
	if aiOutput.RiskEvaluation == nil && (summary.RiskLevel == "high" || summary.RiskLevel == "critical") {
		summary.RequiresConfirmation = true
		log.Printf("[AISummaryService] V2 fallback: risk_level=%s, auto-setting requires_confirmation=true for task %d", summary.RiskLevel, summary.TaskID)
	}

	summary.PlanChanges = planChangesJSON
	summary.ToolCalls, _ = json.Marshal(result.ToolCalls)
	summary.Status = "completed"
	summary.Duration = int(time.Since(startTime).Milliseconds())

	if err := s.db.Save(summary).Error; err != nil {
		log.Printf("[AISummaryService] Failed to save plan summary: %v", err)
		return
	}

	log.Printf("[AISummaryService] Plan summary completed for task %d (id=%s, duration=%dms, steps=%d, requires_confirmation=%v)",
		summary.TaskID, summary.ID, summary.Duration, result.TotalSteps, summary.RequiresConfirmation)

	// plan-only 任务不需要决策确认（没有 apply 阶段）
	var task models.WorkspaceTask
	if err := s.db.Select("task_type").First(&task, summary.TaskID).Error; err == nil {
		if task.TaskType != "plan_and_apply" {
			summary.RequiresConfirmation = false
			s.db.Model(&models.AIPlanSummary{}).Where("id = ?", summary.ID).
				Update("requires_confirmation", false)
		}
	}

	// 如果需要人工确认，将 task 从 apply_pending 改为 decision_required（CAS）
	if summary.RequiresConfirmation {
		result := s.db.Model(&models.WorkspaceTask{}).
			Where("id = ? AND status = ?", summary.TaskID, string(models.TaskStatusApplyPending)).
			Updates(map[string]interface{}{
				"status": string(models.TaskStatusDecisionRequired),
				"stage":  "decision_required",
			})
		if result.RowsAffected > 0 {
			log.Printf("[AISummaryService] Task %d status changed to decision_required", summary.TaskID)
		}
	}
}

// parseDecisionHints 解析 decision_hints（兼容 V4 对象格式和 V3 数组格式）
func (s *AISummaryService) parseDecisionHints(summary *models.AIPlanSummary, raw json.RawMessage) {
	// V4 格式：对象 {title, risk_highlights, recommended_actions}
	var v4Hint struct {
		Title              string   `json:"title"`
		RiskHighlights     []string `json:"risk_highlights"`
		RecommendedActions []struct {
			Code  string `json:"code"`
			Label string `json:"label"`
		} `json:"recommended_actions"`
	}
	if err := json.Unmarshal(raw, &v4Hint); err == nil && v4Hint.Title != "" {
		summary.DecisionTitle = v4Hint.Title
		if len(v4Hint.RiskHighlights) > 0 {
			summary.RiskHighlights, _ = json.Marshal(v4Hint.RiskHighlights)
		}
		if len(v4Hint.RecommendedActions) > 0 {
			summary.DecisionActions, _ = json.Marshal(v4Hint.RecommendedActions)
		}
		return
	}

	// V3 格式：数组 [{scenario, title, recommended_actions}]
	var v3Hints []struct {
		Scenario           string `json:"scenario"`
		Title              string `json:"title"`
		RecommendedActions []struct {
			Code  string `json:"code"`
			Label string `json:"label"`
		} `json:"recommended_actions"`
	}
	if err := json.Unmarshal(raw, &v3Hints); err == nil && len(v3Hints) > 0 {
		hint := v3Hints[0]
		summary.DecisionScenario = hint.Scenario
		summary.DecisionTitle = hint.Title
		if len(hint.RecommendedActions) > 0 {
			summary.DecisionActions, _ = json.Marshal(hint.RecommendedActions)
		}
	}
}

func (s *AISummaryService) completeApplySummary(summary *models.AIApplySummary, result *AgentLoopResult, startTime time.Time) {
	var aiOutput struct {
		ExecutionSummary    string      `json:"execution_summary"`
		ResourceResults     interface{} `json:"resource_results"`
		ImpactConfirmation  interface{} `json:"impact_confirmation"`
		AffectedResources   interface{} `json:"affected_resources"`
	}

	log.Printf("[AISummaryService] AI raw output for apply task %d (len=%d): %.500s", summary.TaskID, len(result.FinalOutput), result.FinalOutput)

	outputText := extractJSON(result.FinalOutput)
	if err := json.Unmarshal([]byte(outputText), &aiOutput); err != nil {
		log.Printf("[AISummaryService] Failed to parse AI output as JSON for apply task %d: %v", summary.TaskID, err)
		aiOutput.ExecutionSummary = result.FinalOutput
	}

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

// logSummarySkillUsage 记录 Summary 的 Skill 使用日志
func (s *AISummaryService) logSummarySkillUsage(cfg *models.AIConfig, capability string, workspaceID string, taskID uint, userPrompt string, aiOutput string, startTime time.Time) {
	// 获取 task skill 信息
	composition := &cfg.SkillComposition
	if composition.TaskSkill == "" && len(composition.FoundationSkills) == 0 {
		composition = s.getDefaultSummarySkillComposition()
	}

	taskSkillName := composition.TaskSkill
	taskSkillContent := ""
	if taskSkillName != "" {
		if skill, err := s.skillAssembler.GetSkillByName(taskSkillName); err == nil && skill != nil {
			taskSkillContent = skill.Content
		}
	}

	// 收集使用的 skill IDs
	var skillIDs []string
	if result, err := s.skillAssembler.AssemblePrompt(composition, 0, &DynamicContext{
		UseCMDB: true,
		ExtraContext: map[string]interface{}{"stage": capability},
	}); err == nil {
		skillIDs = result.UsedSkillIDs
	}

	inputSnapshot, _ := json.Marshal(map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": workspaceID,
		"capability":   capability,
		"user_prompt":  userPrompt,
	})
	outputSnapshot := json.RawMessage(fmt.Sprintf("%q", aiOutput))
	// 尝试解析为 JSON，如果成功用结构化格式
	if json.Valid([]byte(aiOutput)) {
		outputSnapshot = json.RawMessage(aiOutput)
	}

	executionTimeMs := int(time.Since(startTime).Milliseconds())
	moduleID := uint(0)

	if _, err := s.skillAssembler.LogSkillUsage(LogSkillUsageParams{
		SkillIDs:         skillIDs,
		Capability:       capability,
		WorkspaceID:      workspaceID,
		UserID:           "system",
		ModuleID:         &moduleID,
		AIModel:          cfg.ModelID,
		ExecutionTimeMs:  executionTimeMs,
		InputSnapshot:    inputSnapshot,
		OutputSnapshot:   outputSnapshot,
		TaskSkillName:    taskSkillName,
		TaskSkillContent: taskSkillContent,
	}); err != nil {
		log.Printf("[AISummaryService] Failed to log skill usage for %s task %d: %v", capability, taskID, err)
	}
}

// contextWithTimeout 创建带超时的 context（5 分钟，agent loop 可能多轮调用）
// 注意：cancel 不在此处调用，由调用方在使用完 ctx 后自行管理
// 实际上 agent loop 结束后 ctx 自然过期，不会泄漏
func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Minute)
}
