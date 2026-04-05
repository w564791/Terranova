package services

import (
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// R1 whitelist: only known risk factors and uncertainty levels are accepted from AI output
var knownRiskFactors = map[string]bool{
	"service_disruption": true, "external_exposure_change": true,
	"dependency_break": true, "resource_deletion": true,
	"permission_scope_change": true, "sensitive_resource_change": true,
	"high_blast_radius": true, "configuration_drift": true,
}
var knownUncertaintyLevels = map[string]bool{"low": true, "medium": true, "high": true}

// AISummaryService AI 执行摘要服务
type AISummaryService struct {
	db             *gorm.DB
	configService  *AIConfigService
	skillAssembler *SkillAssembler
	streamManager  *OutputStreamManager
	riskScorer     *RiskScorer
}

// NewAISummaryService 创建摘要服务
func NewAISummaryService(db *gorm.DB) *AISummaryService {
	return &AISummaryService{
		db:             db,
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
		riskScorer:     NewRiskScorer(db),
	}
}

// NewAISummaryServiceWithStream 创建带 stream 支持的摘要服务
func NewAISummaryServiceWithStream(db *gorm.DB, streamManager *OutputStreamManager) *AISummaryService {
	return &AISummaryService{
		db:             db,
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
		streamManager:  streamManager,
		riskScorer:     NewRiskScorer(db),
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
		"stage: plan\n\n"+
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

	var processLog strings.Builder
	loop.SetObserver(s.buildObserver(taskID, &processLog))

	s.streamStageMarker(taskID, "post_plan_summary", "begin")
	writeStageMarkerToLog(&processLog, "post_plan_summary", "begin")

	// 运行
	ctx, cancel := contextWithTimeout()
	defer cancel()

	result, err := loop.Run(ctx, systemPrompt, userPrompt)
	if err != nil {
		s.streamLog(taskID, fmt.Sprintf("Plan summary failed: %v", err))
		writeStageMarkerToLog(&processLog, "post_plan_summary", "end")
		s.streamStageMarker(taskID, "post_plan_summary", "end")
		summary.ProcessLog = processLog.String()
		s.failSummary(&summary, err, startTime)
		return
	}

	writeStageMarkerToLog(&processLog, "post_plan_summary", "end")
	s.streamStageMarker(taskID, "post_plan_summary", "end")
	summary.ProcessLog = processLog.String()
	s.completePlanSummary(&summary, result, planChangesJSON, startTime, cfg, task.WorkspaceID, taskID, userPrompt)
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
		"stage: apply\n\n"+
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

	var processLog strings.Builder
	loop.SetObserver(s.buildObserver(taskID, &processLog))

	s.streamStageMarker(taskID, "post_apply_summary", "begin")
	writeStageMarkerToLog(&processLog, "post_apply_summary", "begin")

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result, err := loop.Run(ctx, systemPrompt, applyContext)
	if err != nil {
		s.streamLog(taskID, fmt.Sprintf("Apply summary failed: %v", err))
		writeStageMarkerToLog(&processLog, "post_apply_summary", "end")
		s.streamStageMarker(taskID, "post_apply_summary", "end")
		summary.ProcessLog = processLog.String()
		s.failApplySummary(&summary, err, startTime)
		return
	}

	writeStageMarkerToLog(&processLog, "post_apply_summary", "end")
	s.streamStageMarker(taskID, "post_apply_summary", "end")
	summary.ProcessLog = processLog.String()
	s.completeApplySummary(&summary, result, startTime, cfg, task.WorkspaceID, taskID, applyContext)
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

	changes, ok := plan["resource_changes"]
	if !ok {
		return plan
	}

	// 过滤 no-op 资源，只保留有实际变更的资源
	changeList, ok := changes.([]interface{})
	if !ok {
		return changes
	}

	var filtered []interface{}
	for _, item := range changeList {
		change, ok := item.(map[string]interface{})
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		// 检查 change.change.actions，过滤纯 ["no-op"] 的资源
		changeBlock, _ := change["change"].(map[string]interface{})
		if changeBlock == nil {
			filtered = append(filtered, item)
			continue
		}
		actions, ok := changeBlock["actions"].([]interface{})
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		if len(actions) == 1 {
			if action, ok := actions[0].(string); ok && (action == "no-op" || action == "read") {
				continue // 跳过 no-op 和 read（data source）
			}
		}
		filtered = append(filtered, item)
	}

	log.Printf("[AISummaryService] Filtered plan changes: %d/%d resources have actual changes", len(filtered), len(changeList))
	return filtered
}

func (s *AISummaryService) completePlanSummary(summary *models.AIPlanSummary, result *AgentLoopResult, planChangesJSON []byte, startTime time.Time, cfg *models.AIConfig, workspaceID string, taskID uint, userPrompt string) {
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

	// 记录 Skill 使用日志（用 extractJSON 后的结构化输出）
	s.logSummarySkillUsage(cfg, "plan_summary", workspaceID, taskID, userPrompt, outputText, startTime)

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

	// V2 兜底：仅在 riskScorer 不可用时生效
	if s.riskScorer == nil && aiOutput.RiskEvaluation == nil && (summary.RiskLevel == "high" || summary.RiskLevel == "critical") {
		summary.RequiresConfirmation = true
		log.Printf("[AISummaryService] V2 fallback: risk_level=%s, auto-setting requires_confirmation=true for task %d", summary.RiskLevel, summary.TaskID)
	}

	summary.PlanChanges = planChangesJSON

	// Deterministic risk scoring
	if s.riskScorer != nil {
		riskInput, aiComplete, err := s.buildRiskScoringInput(summary, workspaceID, taskID, planChangesJSON)
		summary.AIAnalysisIncomplete = !aiComplete

		if err != nil {
			log.Printf("[RiskScorer] build input failed: %v", err)
		} else {
			scoreResult := s.riskScorer.Score(riskInput)
			scoreResult.AIRiskLevel = summary.RiskLevel
			scoreResult.DivergenceAlert = severityGap(summary.RiskLevel, scoreResult.RiskLevel) >= 2
			if scoreResult.DivergenceAlert {
				log.Printf("[RiskScorer] DIVERGENCE: AI=%s Go=%s for task %d",
					summary.RiskLevel, scoreResult.RiskLevel, taskID)
			}

			summary.RiskScoreValue = &scoreResult.FinalScore
			summary.RiskScoreColor = scoreResult.DecisionColor
			summary.RiskScoreBreakdown, _ = json.Marshal(scoreResult)

			goLevel := scoreResult.RiskLevel
			aiLevel := summary.RiskLevel
			summary.RiskLevel = maxSeverity(aiLevel, goLevel)
			summary.RequiresConfirmation = summary.RiskLevel == "high" || summary.RiskLevel == "critical"

			// When Go scorer upgrades risk level but AI didn't generate decision actions,
			// provide default confirmation options so the UI isn't stuck
			if summary.RequiresConfirmation && len(summary.DecisionActions) == 0 {
				if summary.DecisionTitle == "" {
					summary.DecisionTitle = fmt.Sprintf("Risk Score: %.1f/100 (%s)", scoreResult.FinalScore, summary.RiskLevel)
				}
				// Build risk highlights from deductions
				var highlights []string
				for _, d := range scoreResult.Deductions {
					highlights = append(highlights, fmt.Sprintf("[%s] %s (%d)", d.Category, d.Item, d.Points))
				}
				if len(highlights) > 0 {
					summary.RiskHighlights, _ = json.Marshal(highlights)
				}
				// Default decision actions
				defaultActions := []struct {
					Code  string `json:"code"`
					Label string `json:"label"`
				}{
					{Code: "ACCEPT_RISK", Label: "I have reviewed the risk score and accept the risk"},
				}
				summary.DecisionActions, _ = json.Marshal(defaultActions)
				log.Printf("[RiskScorer] generated default decision actions for task %d (Go upgraded to %s, AI was %s)",
					taskID, goLevel, aiLevel)
			}
		}
	}

	summary.ToolCalls, _ = json.Marshal(result.ToolCalls)
	if len(result.ThinkingContents) > 0 {
		summary.ThinkingContent, _ = json.Marshal(result.ThinkingContents)
	}
	summary.Status = "completed"
	summary.Duration = int(time.Since(startTime).Milliseconds())

	if err := s.db.Save(summary).Error; err != nil {
		log.Printf("[AISummaryService] Failed to save plan summary: %v", err)
		return
	}

	log.Printf("[AISummaryService] Plan summary completed for task %d (id=%s, duration=%dms, steps=%d, thinking=%d, requires_confirmation=%v)",
		summary.TaskID, summary.ID, summary.Duration, result.TotalSteps, len(result.ThinkingContents), summary.RequiresConfirmation)

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

func (s *AISummaryService) completeApplySummary(summary *models.AIApplySummary, result *AgentLoopResult, startTime time.Time, cfg *models.AIConfig, workspaceID string, taskID uint, userPrompt string) {
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

	// 记录 Skill 使用日志（用 extractJSON 后的结构化输出）
	s.logSummarySkillUsage(cfg, "apply_summary", workspaceID, taskID, userPrompt, outputText, startTime)

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
	if len(result.ThinkingContents) > 0 {
		summary.ThinkingContent, _ = json.Marshal(result.ThinkingContents)
	}
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

func (s *AISummaryService) streamLog(taskID uint, line string) {
	if s.streamManager == nil {
		return
	}
	stream := s.streamManager.Get(taskID)
	if stream == nil {
		return
	}
	stream.Broadcast(OutputMessage{
		Type:      "output",
		Line:      line,
		Timestamp: time.Now(),
	})
}

func (s *AISummaryService) streamStageMarker(taskID uint, stage string, status string) {
	if s.streamManager == nil {
		return
	}
	stream := s.streamManager.Get(taskID)
	if stream == nil {
		return
	}
	timestamp := time.Now()
	marker := fmt.Sprintf("========== %s %s at %s ==========",
		strings.ToUpper(stage),
		strings.ToUpper(status),
		timestamp.Format("2006-01-02 15:04:05.000"))
	stream.Broadcast(OutputMessage{
		Type:      "stage_marker",
		Line:      marker,
		Timestamp: timestamp,
		Stage:     stage,
		Status:    status,
	})
}

func writeStageMarkerToLog(processLog *strings.Builder, stage string, status string) {
	processLog.WriteString(fmt.Sprintf("========== %s %s at %s ==========\n",
		strings.ToUpper(stage),
		strings.ToUpper(status),
		time.Now().Format("2006-01-02 15:04:05.000")))
}

func (s *AISummaryService) buildObserver(taskID uint, processLog *strings.Builder) AgentLoopObserver {
	return func(event AgentLoopEvent) {
		now := time.Now().Format("15:04:05.000")
		var msg string
		level := "INFO"
		switch event.Type {
		case "thinking":
			runes := []rune(event.Content)
			content := event.Content
			if len(runes) > 500 {
				content = string(runes[:500]) + "..."
			}
			msg = fmt.Sprintf("[Step %d] Thinking: %s", event.Step, content)
		case "tool_call":
			msg = fmt.Sprintf("[Step %d] Tool call: %s", event.Step, event.ToolName)
		case "tool_result":
			if event.Error != "" {
				level = "WARN"
				msg = fmt.Sprintf("[Step %d] Tool result: %s failed (%dms): %s", event.Step, event.ToolName, event.Duration, event.Error)
			} else {
				msg = fmt.Sprintf("[Step %d] Tool result: %s ok (%dms)", event.Step, event.ToolName, event.Duration)
			}
		case "output":
			msg = fmt.Sprintf("[Step %d] AI output generated", event.Step)
		case "retry":
			level = "WARN"
			msg = fmt.Sprintf("[Step %d] Output validation failed, retrying: %s", event.Step, event.Content)
		default:
			return
		}
		line := fmt.Sprintf("[%s] [%s] %s", now, level, msg)
		processLog.WriteString(line)
		processLog.WriteString("\n")
		s.streamLog(taskID, line)
	}
}

// contextWithTimeout 创建带超时的 context（5 分钟，agent loop 可能多轮调用）
// 注意：cancel 不在此处调用，由调用方在使用完 ctx 后自行管理
// 实际上 agent loop 结束后 ctx 自然过期，不会泄漏
func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Minute)
}

// buildRiskScoringInput extracts all scoring inputs from summary, workspace, task, and plan data.
func (s *AISummaryService) buildRiskScoringInput(summary *models.AIPlanSummary, workspaceID string, taskID uint, planChangesJSON []byte) (RiskScoringInput, bool, error) {
	input := RiskScoringInput{}
	aiComplete := true

	// 1. Workspace: tier + resource count
	var ws models.Workspace
	if err := s.db.Select("tags, resource_count").Where("workspace_id = ?", workspaceID).First(&ws).Error; err != nil {
		return input, false, fmt.Errorf("workspace lookup: %w", err)
	}
	input.WorkspaceResourceCount = ws.ResourceCount
	if ws.Tags != nil {
		var tags map[string]interface{}
		if raw, err := json.Marshal(ws.Tags); err == nil {
			if json.Unmarshal(raw, &tags) == nil {
				if tier, ok := tags["tier"].(string); ok {
					input.WorkspaceTier = tier
				}
			}
		}
	}

	// 2. Task: total changes
	var task models.WorkspaceTask
	if err := s.db.Select("changes_add, changes_change, changes_destroy").First(&task, taskID).Error; err != nil {
		return input, false, fmt.Errorf("task lookup: %w", err)
	}
	input.TotalChanges = task.ChangesAdd + task.ChangesChange + task.ChangesDestroy

	// 3. Parse impact_analysis
	if len(summary.ImpactAnalysis) > 0 {
		input.MaxDirectDependencies, input.MaxDependencyResource,
			input.RiskFactors, input.RiskFactorResourceCounts,
			input.UncertaintyLevel = parseImpactAnalysis(summary.ImpactAnalysis)
	} else {
		aiComplete = false
		input.UncertaintyLevel = "high"
	}

	// 4. Parse affected_resources for cross-workspace count
	if len(summary.AffectedResources) > 0 {
		input.CrossWorkspaceCount = parseCrossWorkspaceCount(summary.AffectedResources, workspaceID)
	}

	// 5. Parse plan changes for module prefixes, query module_hierarchy
	if len(planChangesJSON) > 0 {
		input.AffectedModuleResourceCount = s.calcModuleResourceCount(planChangesJSON, workspaceID)
	}

	// 6. AI completeness check
	if summary.RiskLevel == "" {
		aiComplete = false
	}

	return input, aiComplete, nil
}

// parseImpactAnalysis extracts scoring signals from impact_analysis JSON.
func parseImpactAnalysis(raw json.RawMessage) (maxDeps int, maxDepResource string, factors []string, factorCounts map[string]int, uncertainty string) {
	factorCounts = make(map[string]int)
	type analysisItem struct {
		ResourceAddress    string   `json:"resource_address"`
		Resource           string   `json:"resource"` // fallback field name
		DirectDependencies int      `json:"direct_dependencies"`
		RiskFactors        []string `json:"risk_factors"`
		Uncertainty        *struct {
			Level string `json:"level"`
		} `json:"uncertainty"`
		BlastRadius *struct {
			DirectDependencies int `json:"direct_dependencies"`
		} `json:"blast_radius"`
	}

	// Try parsing as array first, then as {details: [...]} wrapper
	var items []analysisItem
	if err := json.Unmarshal(raw, &items); err != nil {
		var wrapper struct {
			Details []analysisItem `json:"details"`
		}
		if err2 := json.Unmarshal(raw, &wrapper); err2 != nil {
			log.Printf("[RiskScorer] WARN cannot parse impact_analysis: %v", err2)
			return
		}
		items = wrapper.Details
	}

	// Normalize: some AI outputs put direct_dependencies inside blast_radius
	for i := range items {
		if items[i].DirectDependencies == 0 && items[i].BlastRadius != nil {
			items[i].DirectDependencies = items[i].BlastRadius.DirectDependencies
		}
		if items[i].ResourceAddress == "" && items[i].Resource != "" {
			items[i].ResourceAddress = items[i].Resource
		}
	}

	factorSet := make(map[string]bool)
	for _, item := range items {
		if item.DirectDependencies > maxDeps {
			maxDeps = item.DirectDependencies
			maxDepResource = item.ResourceAddress
		}
		for _, f := range item.RiskFactors {
			if !knownRiskFactors[f] {
				log.Printf("[RiskScorer] WARN unrecognized risk_factor: %q - skipped", f)
				continue
			}
			factorCounts[f]++
			if !factorSet[f] {
				factorSet[f] = true
				factors = append(factors, f)
			}
		}
		if item.Uncertainty != nil {
			level := item.Uncertainty.Level
			if knownUncertaintyLevels[level] {
				if severityOrder[level] > severityOrder[uncertainty] {
					uncertainty = level
				}
			} else if level != "" {
				log.Printf("[RiskScorer] WARN unrecognized uncertainty level: %q", level)
			}
		}
	}
	return
}

// parseCrossWorkspaceCount counts unique workspace IDs in affected_resources that differ from current.
func parseCrossWorkspaceCount(raw json.RawMessage, currentWS string) int {
	var items []struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0
	}
	seen := make(map[string]bool)
	for _, item := range items {
		if item.WorkspaceID != "" && item.WorkspaceID != currentWS {
			seen[item.WorkspaceID] = true
		}
	}
	return len(seen)
}

// calcModuleResourceCount extracts module prefixes from plan changes and sums TotalResourceCount.
func (s *AISummaryService) calcModuleResourceCount(planChangesJSON []byte, workspaceID string) int {
	var changes []struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(planChangesJSON, &changes); err != nil {
		return 0
	}

	modulePrefixes := make(map[string]bool)
	for _, c := range changes {
		if prefix := extractModulePrefix(c.Address); prefix != "" {
			modulePrefixes[prefix] = true
		}
	}
	if len(modulePrefixes) == 0 {
		return 0
	}

	paths := make([]string, 0, len(modulePrefixes))
	for p := range modulePrefixes {
		paths = append(paths, p)
	}

	var total int
	s.db.Model(&models.ModuleHierarchy{}).
		Where("workspace_id = ? AND module_path IN ?", workspaceID, paths).
		Select("COALESCE(SUM(total_resource_count), 0)").
		Scan(&total)
	return total
}

// extractModulePrefix extracts the module path from a Terraform resource address.
// "module.vpc.aws_vpc.main" -> "module.vpc"
// "module.vpc.module.subnet.aws_subnet.main" -> "module.vpc.module.subnet"
// "aws_instance.web" -> ""
func extractModulePrefix(address string) string {
	parts := strings.Split(address, ".")
	var moduleParts []string
	for i := 0; i+1 < len(parts); i += 2 {
		if parts[i] != "module" {
			break
		}
		moduleParts = append(moduleParts, parts[i], parts[i+1])
	}
	if len(moduleParts) == 0 {
		return ""
	}
	return strings.Join(moduleParts, ".")
}
