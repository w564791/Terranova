package services

import (
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ========== Summary 场景 Agent Tools ==========

// AIAgentTaskScope is captured from the database task before an agent loop is
// created. Tool parameters are model-controlled input, so they must never be
// used to choose a workspace or task.
type AIAgentTaskScope struct {
	workspaceID string
	taskID      uint
}

func NewAIAgentTaskScope(workspaceID string, taskID uint) AIAgentTaskScope {
	return AIAgentTaskScope{workspaceID: workspaceID, taskID: taskID}
}

func (scope AIAgentTaskScope) requireWorkspace() error {
	if scope.workspaceID == "" {
		return fmt.Errorf("task workspace scope is required")
	}
	return nil
}

func (scope AIAgentTaskScope) requireTask() error {
	if err := scope.requireWorkspace(); err != nil {
		return err
	}
	if scope.taskID == 0 {
		return fmt.Errorf("task scope is required")
	}
	return nil
}

// QueryModuleResourcesTool 查询 module 完整资源列表
type QueryModuleResourcesTool struct {
	db    *gorm.DB
	scope AIAgentTaskScope
}

func NewQueryModuleResourcesTool(db *gorm.DB, scope AIAgentTaskScope) *QueryModuleResourcesTool {
	return &QueryModuleResourcesTool{db: db, scope: scope}
}

func (t *QueryModuleResourcesTool) Name() string { return "query_module_resources" }
func (t *QueryModuleResourcesTool) Description() string {
	return "查询指定 module 下的完整资源列表（包含子 module）。当 plan 变更只包含部分资源时，使用此工具获取 module 的完整资源视图。"
}
func (t *QueryModuleResourcesTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"module_path": map[string]interface{}{"type": "string", "description": "Module 路径，如 module.vpc"},
		},
		"required": []string{"module_path"},
	}
}

func (t *QueryModuleResourcesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	modulePath, _ := params["module_path"].(string)

	if err := t.scope.requireWorkspace(); err != nil {
		return nil, err
	}
	if modulePath == "" {
		return nil, fmt.Errorf("module_path is required")
	}

	var resources []models.ResourceIndex
	err := t.db.Where(
		"workspace_id = ? AND resource_mode = 'managed' AND (module_path = ? OR module_path LIKE ?)",
		t.scope.workspaceID, modulePath, modulePath+".%",
	).Limit(50).Find(&resources).Error
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	type resourceInfo struct {
		TerraformAddress  string `json:"terraform_address"`
		ResourceType      string `json:"resource_type"`
		CloudResourceID   string `json:"cloud_resource_id,omitempty"`
		CloudResourceName string `json:"cloud_resource_name,omitempty"`
	}

	result := make([]resourceInfo, 0, len(resources))
	for _, r := range resources {
		result = append(result, resourceInfo{
			TerraformAddress:  r.TerraformAddress,
			ResourceType:      r.ResourceType,
			CloudResourceID:   r.CloudResourceID,
			CloudResourceName: r.CloudResourceName,
		})
	}

	log.Printf("[QueryModuleResources] workspace=%s module=%s found=%d", t.scope.workspaceID, modulePath, len(result))
	return map[string]interface{}{
		"module_path":    modulePath,
		"resource_count": len(result),
		"resources":      result,
	}, nil
}

// QueryCMDBDependenciesTool 查询资源依赖方
type QueryCMDBDependenciesTool struct {
	db    *gorm.DB
	scope AIAgentTaskScope
}

func NewQueryCMDBDependenciesTool(db *gorm.DB, scope AIAgentTaskScope) *QueryCMDBDependenciesTool {
	return &QueryCMDBDependenciesTool{db: db, scope: scope}
}

func (t *QueryCMDBDependenciesTool) Name() string { return "query_cmdb_dependencies" }
func (t *QueryCMDBDependenciesTool) Description() string {
	return "查询哪些资源依赖了指定资源。AI 需要判断依赖字段（如 security_group_ids, vpc_id, subnet_id），然后通过此工具在 CMDB 中搜索引用了该资源的其他资源。"
}
func (t *QueryCMDBDependenciesTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"resource_id":      map[string]interface{}{"type": "string", "description": "被依赖资源的 ID（如 sg-123, vpc-456）"},
			"dependency_field": map[string]interface{}{"type": "string", "description": "依赖字段名（如 security_group_ids, vpc_id, subnet_id）"},
		},
		"required": []string{"resource_id", "dependency_field"},
	}
}

func (t *QueryCMDBDependenciesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	start := time.Now()
	resourceID, _ := params["resource_id"].(string)
	depField, _ := params["dependency_field"].(string)

	if err := t.scope.requireWorkspace(); err != nil {
		return nil, err
	}
	if resourceID == "" || depField == "" {
		return nil, fmt.Errorf("resource_id and dependency_field are required")
	}

	// The task workspace is captured at registration time. Do not include the
	// global __external__ pseudo-workspace or accept a workspace from the model.
	query := t.db.Where("workspace_id = ? AND resource_mode = 'managed'", t.scope.workspaceID)

	// 搜索 JSONB 属性：字符串字段精确匹配 + 数组字段包含匹配
	query = query.Where(
		"(attributes->>? ILIKE ? OR attributes->? @> ?)",
		depField, "%"+resourceID+"%",
		depField, fmt.Sprintf(`"%s"`, resourceID),
	)

	var resources []models.ResourceIndex
	if err := query.Limit(50).Find(&resources).Error; err != nil {
		elapsed := time.Since(start)
		go func() {
			searchLog := models.CMDBSearchLog{
				Query:          strings.ToLower(strings.TrimSpace(resourceID)),
				SearchMethod:   "jsonb",
				Source:         "agent",
				TotalCount:     0,
				DurationMs:     int(elapsed.Milliseconds()),
				FallbackReason: "query error: " + err.Error(),
			}
			if dbErr := t.db.Create(&searchLog).Error; dbErr != nil {
				log.Printf("[SearchLog] write failed: %v", dbErr)
			}
		}()
		return nil, fmt.Errorf("query failed: %w", err)
	}

	type depInfo struct {
		TerraformAddress string `json:"terraform_address"`
		ResourceType     string `json:"resource_type"`
		WorkspaceID      string `json:"workspace_id"`
		CloudResourceID  string `json:"cloud_resource_id,omitempty"`
	}

	result := make([]depInfo, 0, len(resources))
	for _, r := range resources {
		result = append(result, depInfo{
			TerraformAddress: r.TerraformAddress,
			ResourceType:     r.ResourceType,
			WorkspaceID:      r.WorkspaceID,
			CloudResourceID:  r.CloudResourceID,
		})
	}

	elapsed := time.Since(start)
	log.Printf("[QueryCMDBDependencies] resource=%s field=%s found=%d elapsed=%dms", resourceID, depField, len(result), elapsed.Milliseconds())

	// 异步写入搜索日志
	go func() {
		searchLog := models.CMDBSearchLog{
			Query:          strings.ToLower(strings.TrimSpace(resourceID)),
			SearchMethod:   "jsonb",
			Source:         "agent",
			TotalCount:     len(result),
			DurationMs:     int(elapsed.Milliseconds()),
			FallbackReason: "dep_field:" + depField,
		}
		if err := t.db.Create(&searchLog).Error; err != nil {
			log.Printf("[SearchLog] write failed: %v", err)
		}
	}()

	return map[string]interface{}{
		"resource_id":      resourceID,
		"dependency_field": depField,
		"dependent_count":  len(result),
		"dependents":       result,
	}, nil
}

// QueryResourceAttributesTool 查询资源完整属性（复用 CMDB 关键字搜索）
type QueryResourceAttributesTool struct {
	db          *gorm.DB
	cmdbService *CMDBService
	scope       AIAgentTaskScope
}

func NewQueryResourceAttributesTool(db *gorm.DB, scope AIAgentTaskScope) *QueryResourceAttributesTool {
	return &QueryResourceAttributesTool{db: db, cmdbService: NewCMDBService(db), scope: scope}
}

func (t *QueryResourceAttributesTool) Name() string { return "query_resource_attributes" }
func (t *QueryResourceAttributesTool) Description() string {
	return "在当前任务的工作空间内搜索并查询资源完整属性。支持 cloud_resource_id、terraform_address 或关键词模糊搜索。"
}
func (t *QueryResourceAttributesTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "搜索关键词：cloud_resource_id、terraform_address、资源名称等，支持模糊匹配"},
		},
		"required": []string{"query"},
	}
}

func (t *QueryResourceAttributesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	start := time.Now()
	q, _ := params["query"].(string)
	if err := t.scope.requireWorkspace(); err != nil {
		return nil, err
	}
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}

	// The task workspace is the only data scope; SearchResources adds an exact
	// workspace predicate, so platform-global external records cannot match.
	results, err := t.cmdbService.SearchResources(q, t.scope.workspaceID, "", 1)
	if err != nil {
		elapsed := time.Since(start)
		go func() {
			searchLog := models.CMDBSearchLog{
				Query:          strings.ToLower(strings.TrimSpace(q)),
				SearchMethod:   "keyword",
				Source:         "agent",
				TotalCount:     0,
				DurationMs:     int(elapsed.Milliseconds()),
				FallbackReason: "search error: " + err.Error(),
			}
			if dbErr := t.db.Create(&searchLog).Error; dbErr != nil {
				log.Printf("[SearchLog] write failed: %v", dbErr)
			}
		}()
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		// 记录零结果搜索日志
		elapsed := time.Since(start)
		go func() {
			searchLog := models.CMDBSearchLog{
				Query:        strings.ToLower(strings.TrimSpace(q)),
				SearchMethod: "keyword",
				Source:       "agent",
				TotalCount:   0,
				KeywordCount: 0,
				DurationMs:   int(elapsed.Milliseconds()),
			}
			if err := t.db.Create(&searchLog).Error; err != nil {
				log.Printf("[SearchLog] write failed: %v", err)
			}
		}()
		return map[string]interface{}{"found": false, "message": "resource not found", "query": q}, nil
	}

	// 用搜到的精确 ID 取完整属性（含 attributes、tags）
	hit := results[0]
	var resource models.ResourceIndex
	if err := t.db.Where("workspace_id = ? AND cloud_resource_id = ?", t.scope.workspaceID, hit.CloudResourceID).
		First(&resource).Error; err != nil {
		// 搜索能找到但取属性失败，返回搜索结果的基本信息
		elapsed := time.Since(start)
		log.Printf("[QueryResourceAttributes] search hit but attributes fetch failed: %v", err)
		go func() {
			searchLog := models.CMDBSearchLog{
				Query:          strings.ToLower(strings.TrimSpace(q)),
				ResourceType:   hit.ResourceType,
				SearchMethod:   "keyword",
				Source:         "agent",
				TotalCount:     1,
				KeywordCount:   1,
				DurationMs:     int(elapsed.Milliseconds()),
				FallbackReason: "attributes fetch failed",
			}
			if err := t.db.Create(&searchLog).Error; err != nil {
				log.Printf("[SearchLog] write failed: %v", err)
			}
		}()
		return map[string]interface{}{
			"found":             true,
			"workspace_id":      hit.WorkspaceID,
			"terraform_address": hit.TerraformAddress,
			"resource_type":     hit.ResourceType,
			"cloud_resource_id": hit.CloudResourceID,
		}, nil
	}

	var attrs map[string]interface{}
	if len(resource.Attributes) > 0 {
		json.Unmarshal(resource.Attributes, &attrs)
	}
	var tags map[string]interface{}
	if len(resource.Tags) > 0 {
		json.Unmarshal(resource.Tags, &tags)
	}

	elapsed := time.Since(start)
	log.Printf("[QueryResourceAttributes] query=%s found=%s workspace=%s elapsed=%dms", q, resource.CloudResourceID, resource.WorkspaceID, elapsed.Milliseconds())

	// 异步写入搜索日志
	go func() {
		searchLog := models.CMDBSearchLog{
			Query:        strings.ToLower(strings.TrimSpace(q)),
			ResourceType: resource.ResourceType,
			SearchMethod: "keyword",
			Source:       "agent",
			TotalCount:   1,
			KeywordCount: 1,
			DurationMs:   int(elapsed.Milliseconds()),
		}
		if err := t.db.Create(&searchLog).Error; err != nil {
			log.Printf("[SearchLog] write failed: %v", err)
		}
	}()

	result := map[string]interface{}{
		"found":              true,
		"workspace_id":       resource.WorkspaceID,
		"terraform_address":  resource.TerraformAddress,
		"resource_type":      resource.ResourceType,
		"cloud_resource_id":  resource.CloudResourceID,
		"cloud_resource_arn": resource.CloudResourceARN,
		"tags":               tags,
	}
	// 有摘要时优先返回摘要（省 token），无摘要时 fallback 返回原始 attributes
	if resource.ResourceSummary != "" {
		result["resource_summary"] = resource.ResourceSummary
	} else {
		result["attributes"] = attrs
	}
	return result, nil
}

// QueryStateResourcesTool 查询工作空间完整资源概览
type QueryStateResourcesTool struct {
	db    *gorm.DB
	scope AIAgentTaskScope
}

func NewQueryStateResourcesTool(db *gorm.DB, scope AIAgentTaskScope) *QueryStateResourcesTool {
	return &QueryStateResourcesTool{db: db, scope: scope}
}

func (t *QueryStateResourcesTool) Name() string { return "query_state_resources" }
func (t *QueryStateResourcesTool) Description() string {
	return "查询工作空间当前的完整资源列表概览。用于了解整体资源全貌。"
}
func (t *QueryStateResourcesTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *QueryStateResourcesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if err := t.scope.requireWorkspace(); err != nil {
		return nil, err
	}

	var resources []models.ResourceIndex
	err := t.db.Where("workspace_id = ? AND resource_mode = 'managed'", t.scope.workspaceID).
		Limit(100).Find(&resources).Error
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// 按 resource_type 分组
	groups := make(map[string][]map[string]string)
	for _, r := range resources {
		groups[r.ResourceType] = append(groups[r.ResourceType], map[string]string{
			"address":  r.TerraformAddress,
			"cloud_id": r.CloudResourceID,
		})
	}

	// 构建摘要
	type groupInfo struct {
		ResourceType string              `json:"resource_type"`
		Count        int                 `json:"count"`
		Resources    []map[string]string `json:"resources"`
	}

	summary := make([]groupInfo, 0, len(groups))
	for rt, items := range groups {
		summary = append(summary, groupInfo{
			ResourceType: rt,
			Count:        len(items),
			Resources:    items,
		})
	}

	log.Printf("[QueryStateResources] workspace=%s total=%d types=%d", t.scope.workspaceID, len(resources), len(groups))
	return map[string]interface{}{
		"workspace_id":    t.scope.workspaceID,
		"total_resources": len(resources),
		"resource_types":  len(groups),
		"summary":         summary,
	}, nil
}

// QueryPlanSummaryTool 查询 Plan Summary（仅 apply summary 阶段使用）
type QueryPlanSummaryTool struct {
	db    *gorm.DB
	scope AIAgentTaskScope
}

func NewQueryPlanSummaryTool(db *gorm.DB, scope AIAgentTaskScope) *QueryPlanSummaryTool {
	return &QueryPlanSummaryTool{db: db, scope: scope}
}

func (t *QueryPlanSummaryTool) Name() string { return "query_plan_summary" }
func (t *QueryPlanSummaryTool) Description() string {
	return "查询对应任务的 Plan 阶段影响分析结果。仅在 Apply Summary 阶段可用，用于对比预测与实际结果。"
}
func (t *QueryPlanSummaryTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *QueryPlanSummaryTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if err := t.scope.requireTask(); err != nil {
		return nil, err
	}

	var summary models.AIPlanSummary
	err := t.db.Where("task_id = ? AND workspace_id = ? AND status = 'completed'", t.scope.taskID, t.scope.workspaceID).First(&summary).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return map[string]interface{}{"found": false, "message": "no completed plan summary found"}, nil
		}
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// 解析 JSONB 字段
	var impactAnalysis, affectedResources interface{}
	if len(summary.ImpactAnalysis) > 0 {
		json.Unmarshal(summary.ImpactAnalysis, &impactAnalysis)
	}
	if len(summary.AffectedResources) > 0 {
		json.Unmarshal(summary.AffectedResources, &affectedResources)
	}

	return map[string]interface{}{
		"found":              true,
		"changes_overview":   summary.ChangesOverview,
		"impact_analysis":    impactAnalysis,
		"affected_resources": affectedResources,
		"risk_level":         summary.RiskLevel,
	}, nil
}

// QueryResourceCodeDiffTool 查询资源代码在上次 apply 后的真实变更
type QueryResourceCodeDiffTool struct {
	db    *gorm.DB
	scope AIAgentTaskScope
}

func NewQueryResourceCodeDiffTool(db *gorm.DB, scope AIAgentTaskScope) *QueryResourceCodeDiffTool {
	return &QueryResourceCodeDiffTool{db: db, scope: scope}
}

func (t *QueryResourceCodeDiffTool) Name() string { return "query_resource_code_diff" }
func (t *QueryResourceCodeDiffTool) Description() string {
	return "查询当前任务工作空间内资源代码自上次 apply 以来的真实变更。用于分析 after_unknown 字段；输入资源 resource_id。"
}
func (t *QueryResourceCodeDiffTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"resource_id": map[string]interface{}{"type": "string", "description": "CMDB 资源标识（如 AWS_s3-bucket.xxx）"},
		},
		"required": []string{"resource_id"},
	}
}

func (t *QueryResourceCodeDiffTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	resourceID, _ := params["resource_id"].(string)
	if err := t.scope.requireWorkspace(); err != nil {
		return nil, err
	}
	if resourceID == "" {
		return nil, fmt.Errorf("resource_id is required")
	}
	workspaceID := t.scope.workspaceID

	// 1. 找到资源
	var resource models.WorkspaceResource
	if err := t.db.Where("workspace_id = ? AND resource_id = ?", workspaceID, resourceID).
		First(&resource).Error; err != nil {
		return map[string]interface{}{"found": false, "message": "resource not found"}, nil
	}

	// 自动感知：manifest-managed 资源走 manifest 分支
	if resource.ManifestDeploymentID != nil && *resource.ManifestDeploymentID != "" {
		return t.executeManifestCodeDiff(workspaceID, resourceID, resource)
	}

	// 2. 获取当前最新版本（module 分支）
	var currentVersion models.ResourceCodeVersion
	if err := t.db.Where("resource_id = ? AND is_latest = true", resource.ID).
		First(&currentVersion).Error; err != nil {
		return map[string]interface{}{"found": false, "message": "no current version found"}, nil
	}

	// 3. 查找上次 apply 时的代码版本号
	//    优先从 workspace_task_resource_changes.applied_code_version 查询（快速路径）
	//    Fallback: 扫描 workspace_tasks.snapshot_resource_versions JSONB
	moduleAddress := "module." + strings.ReplaceAll(resourceID, ".", "_")
	var appliedVersionNum int
	var appliedTaskID uint

	var rc models.WorkspaceTaskResourceChange
	if err := t.db.Select("task_id, applied_code_version").
		Where("workspace_id = ? AND module_address = ? AND apply_status = 'completed' AND applied_code_version IS NOT NULL",
			workspaceID, moduleAddress).
		Order("task_id DESC").
		First(&rc).Error; err == nil && rc.AppliedCodeVersion != nil {
		appliedVersionNum = *rc.AppliedCodeVersion
		appliedTaskID = rc.TaskID
		log.Printf("[query_resource_code_diff] fast path: resource=%s version=%d task=%d", resourceID, appliedVersionNum, appliedTaskID)
	} else {
		// Fallback: JSONB scan on snapshot_resource_versions
		var task models.WorkspaceTask
		if err := t.db.Select("id, snapshot_resource_versions").
			Where("workspace_id = ? AND status = 'applied' AND jsonb_exists(snapshot_resource_versions, ?)", workspaceID, resourceID).
			Order("id DESC").
			First(&task).Error; err != nil {
			return map[string]interface{}{
				"found":           true,
				"message":         "no apply history found for this resource",
				"current_version": currentVersion.Version,
				"current_code":    currentVersion.TFCode,
			}, nil
		}
		appliedTaskID = task.ID
		if snap, ok := task.SnapshotResourceVersions[resourceID]; ok {
			if snapMap, ok := snap.(map[string]interface{}); ok {
				if v, ok := snapMap["version"].(float64); ok {
					appliedVersionNum = int(v)
				}
			}
		}
		log.Printf("[query_resource_code_diff] fallback path: resource=%s version=%d task=%d", resourceID, appliedVersionNum, appliedTaskID)
	}

	if appliedVersionNum == 0 {
		return map[string]interface{}{
			"found":           true,
			"message":         "could not extract version from snapshot",
			"current_version": currentVersion.Version,
			"current_code":    currentVersion.TFCode,
		}, nil
	}

	// 当前版本和 apply 版本相同，没有变更
	if appliedVersionNum == currentVersion.Version {
		return map[string]interface{}{
			"found":           true,
			"has_changes":     false,
			"message":         "code unchanged since last apply",
			"current_version": currentVersion.Version,
			"applied_version": appliedVersionNum,
			"applied_task_id": appliedTaskID,
		}, nil
	}

	// 4. 获取 apply 时的版本代码
	var appliedVersion models.ResourceCodeVersion
	if err := t.db.Where("resource_id = ? AND version = ?", resource.ID, appliedVersionNum).
		First(&appliedVersion).Error; err != nil {
		return map[string]interface{}{
			"found":           true,
			"message":         "applied version record not found",
			"current_version": currentVersion.Version,
			"applied_version": appliedVersionNum,
			"current_code":    currentVersion.TFCode,
		}, nil
	}

	// 5. 计算精简 diff：只输出变更的字段
	diff := computeJSONDiff(appliedVersion.TFCode, currentVersion.TFCode)

	return map[string]interface{}{
		"found":           true,
		"has_changes":     true,
		"applied_version": appliedVersionNum,
		"current_version": currentVersion.Version,
		"applied_task_id": appliedTaskID,
		"diff":            diff,
	}, nil
}

// executeManifestCodeDiff 处理 manifest-managed 资源的代码变更分析。
// 对比上次 apply 的 manifest 版本与当前版本中，指定 resource/module block 的 HCL 源码差异。
func (t *QueryResourceCodeDiffTool) executeManifestCodeDiff(
	workspaceID, resourceID string,
	resource models.WorkspaceResource,
) (interface{}, error) {
	// 1. 查 workspace 获取 manifest 软链接 + subpath
	var ws struct {
		ManifestDeploymentID *string
		ManifestSubpath      *string
	}
	if err := t.db.Table("workspaces").
		Select("manifest_deployment_id, manifest_subpath").
		Where("workspace_id = ?", workspaceID).
		Scan(&ws).Error; err != nil {
		return map[string]interface{}{"found": false, "message": "workspace not found"}, nil
	}
	if ws.ManifestDeploymentID == nil || *ws.ManifestDeploymentID == "" {
		return map[string]interface{}{"found": false, "message": "workspace has no manifest deployment"}, nil
	}

	// 2. 查 manifest_deployments 获取 manifest_id + 当前 version_id
	var dep models.ManifestDeployment
	if err := t.db.Select("manifest_id, version_id").
		Where("id = ?", *ws.ManifestDeploymentID).
		First(&dep).Error; err != nil {
		return map[string]interface{}{"found": false, "message": "manifest deployment not found"}, nil
	}

	// 获取当前版本名称
	var currentVersion models.ManifestVersion
	if err := t.db.Select("version").
		Where("id = ?", dep.VersionID).
		First(&currentVersion).Error; err != nil {
		return map[string]interface{}{"found": false, "message": "current manifest version not found"}, nil
	}

	// 3. 查上次 apply 成功的 task 获取 snapshot_manifest_version_id（旧版本）
	// 排除后台任务（drift_check 等），仅取用户发起的正常流程任务。
	var appliedTask models.WorkspaceTask
	if err := t.db.Select("id, snapshot_manifest_version_id").
		Where("workspace_id = ? AND status = 'applied' AND snapshot_manifest_version_id IS NOT NULL AND is_background = false", workspaceID).
		Order("id DESC").
		First(&appliedTask).Error; err != nil {
		return map[string]interface{}{
			"found":           true,
			"source_type":     "manifest",
			"message":         "no apply history with manifest version found",
			"current_version": currentVersion.Version,
		}, nil
	}
	if appliedTask.SnapshotManifestVersionID == nil || *appliedTask.SnapshotManifestVersionID == "" {
		return map[string]interface{}{
			"found":           true,
			"source_type":     "manifest",
			"message":         "no manifest version in apply snapshot",
			"current_version": currentVersion.Version,
		}, nil
	}

	// 获取旧版本名称
	var appliedVersion models.ManifestVersion
	if err := t.db.Select("version").
		Where("id = ?", *appliedTask.SnapshotManifestVersionID).
		First(&appliedVersion).Error; err != nil {
		return map[string]interface{}{
			"found":           true,
			"source_type":     "manifest",
			"message":         "applied manifest version record not found",
			"current_version": currentVersion.Version,
		}, nil
	}

	// 4. 新旧版本相同，无变更
	if *appliedTask.SnapshotManifestVersionID == dep.VersionID {
		return map[string]interface{}{
			"found":           true,
			"source_type":     "manifest",
			"has_changes":     false,
			"message":         "manifest version unchanged since last apply",
			"applied_version": appliedVersion.Version,
			"current_version": currentVersion.Version,
			"applied_task_id": appliedTask.ID,
		}, nil
	}

	// 5. 拉两个版本的 manifest_files
	subpath := ""
	if ws.ManifestSubpath != nil {
		subpath = *ws.ManifestSubpath
	}

	appliedFiles := t.loadManifestFiles(dep.ManifestID, *appliedTask.SnapshotManifestVersionID)
	currentFiles := t.loadManifestFiles(dep.ManifestID, dep.VersionID)

	if len(appliedFiles) == 0 && len(currentFiles) == 0 {
		return map[string]interface{}{
			"found":           true,
			"source_type":     "manifest",
			"message":         "no manifest files found in either version",
			"applied_version": appliedVersion.Version,
			"current_version": currentVersion.Version,
		}, nil
	}

	// 6. 解析 resource_id → (kind, typeName, name)
	kind, typeName, name := parseResourceIDForManifest(resourceID)

	// 7. 从两个版本提取该 resource 的 HCL block
	appliedScope := manifestFilesToScope(appliedFiles)
	currentScope := manifestFilesToScope(currentFiles)

	appliedPath, appliedHCL, appliedFound := ExtractResourceBlock(appliedScope, kind, typeName, name, subpath)
	currentPath, currentHCL, currentFound := ExtractResourceBlock(currentScope, kind, typeName, name, subpath)

	// 8. 分析变更情况
	result := map[string]interface{}{
		"found":           true,
		"source_type":     "manifest",
		"applied_version": appliedVersion.Version,
		"current_version": currentVersion.Version,
		"applied_task_id": appliedTask.ID,
	}

	switch {
	case !appliedFound && !currentFound:
		result["has_changes"] = false
		result["message"] = "resource block not found in either version"
	case !appliedFound && currentFound:
		result["has_changes"] = true
		result["change_type"] = "added"
		result["file"] = currentPath
		result["new_hcl"] = currentHCL
	case appliedFound && !currentFound:
		result["has_changes"] = true
		result["change_type"] = "removed"
		result["file"] = appliedPath
		result["old_hcl"] = appliedHCL
	case appliedFound && currentFound:
		if appliedHCL == currentHCL {
			result["has_changes"] = false
			result["message"] = "resource block unchanged"
			result["file"] = currentPath
		} else {
			result["has_changes"] = true
			result["change_type"] = "modified"
			result["file"] = currentPath
			result["old_hcl"] = appliedHCL
			result["new_hcl"] = currentHCL
			if appliedPath != currentPath {
				result["file_moved"] = true
				result["old_file"] = appliedPath
			}
		}
	}

	log.Printf("[query_resource_code_diff] manifest: resource=%s applied=%s current=%s has_changes=%v",
		resourceID, appliedVersion.Version, currentVersion.Version, result["has_changes"])

	return result, nil
}

// loadManifestFiles 加载指定 manifest 版本的所有文件
func (t *QueryResourceCodeDiffTool) loadManifestFiles(manifestID, versionID string) []models.ManifestFile {
	var files []models.ManifestFile
	if err := t.db.Select("path, content").
		Where("manifest_id = ? AND version_id = ?", manifestID, versionID).
		Find(&files).Error; err != nil {
		log.Printf("[query_resource_code_diff] failed to load manifest files (manifest=%s version=%s): %v",
			manifestID, versionID, err)
	}
	return files
}

// manifestFilesToScope 将 ManifestFile 切片转换为 path→content 的 scope map
func manifestFilesToScope(files []models.ManifestFile) map[string][]byte {
	scope := make(map[string][]byte, len(files))
	for _, f := range files {
		scope[f.Path] = f.Content
	}
	return scope
}

// parseResourceIDForManifest 将 resource_id 解析为 HCL block 的 (kind, typeName, name)。
// resource_id 格式:
//   - module 块: "module.<name>" → kind="module", typeName="", name="<name>"
//   - resource 块: "<type>.<name>" → kind="resource", typeName="<type>", name="<name>"
func parseResourceIDForManifest(resourceID string) (kind, typeName, name string) {
	if strings.HasPrefix(resourceID, "module.") {
		return "module", "", strings.TrimPrefix(resourceID, "module.")
	}
	parts := strings.SplitN(resourceID, ".", 2)
	if len(parts) == 2 {
		return "resource", parts[0], parts[1]
	}
	// fallback: 整个 resourceID 当 name 用
	return "resource", "", resourceID
}

// computeJSONDiff 计算两个 JSON 对象的精简差异，只返回变更部分
func computeJSONDiff(oldCode, newCode map[string]interface{}) []map[string]interface{} {
	var diffs []map[string]interface{}
	computeJSONDiffRecursive("", oldCode, newCode, &diffs)
	return diffs
}

func computeJSONDiffRecursive(prefix string, oldObj, newObj map[string]interface{}, diffs *[]map[string]interface{}) {
	allKeys := make(map[string]bool)
	for k := range oldObj {
		allKeys[k] = true
	}
	for k := range newObj {
		allKeys[k] = true
	}

	for key := range allKeys {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		oldVal, oldExists := oldObj[key]
		newVal, newExists := newObj[key]

		if !oldExists {
			*diffs = append(*diffs, map[string]interface{}{
				"path": path,
				"type": "added",
				"new":  newVal,
			})
			continue
		}
		if !newExists {
			*diffs = append(*diffs, map[string]interface{}{
				"path": path,
				"type": "removed",
				"old":  oldVal,
			})
			continue
		}

		// 都存在，递归比较
		oldMap, oldIsMap := oldVal.(map[string]interface{})
		newMap, newIsMap := newVal.(map[string]interface{})
		if oldIsMap && newIsMap {
			computeJSONDiffRecursive(path, oldMap, newMap, diffs)
			continue
		}

		// 非 map 类型直接比较 JSON 序列化
		oldJSON, _ := json.Marshal(oldVal)
		newJSON, _ := json.Marshal(newVal)
		if string(oldJSON) != string(newJSON) {
			*diffs = append(*diffs, map[string]interface{}{
				"path": path,
				"type": "modified",
				"old":  oldVal,
				"new":  newVal,
			})
		}
	}
}
