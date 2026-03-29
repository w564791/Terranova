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

// QueryModuleResourcesTool 查询 module 完整资源列表
type QueryModuleResourcesTool struct {
	db *gorm.DB
}

func NewQueryModuleResourcesTool(db *gorm.DB) *QueryModuleResourcesTool {
	return &QueryModuleResourcesTool{db: db}
}

func (t *QueryModuleResourcesTool) Name() string { return "query_module_resources" }
func (t *QueryModuleResourcesTool) Description() string {
	return "查询指定 module 下的完整资源列表（包含子 module）。当 plan 变更只包含部分资源时，使用此工具获取 module 的完整资源视图。"
}
func (t *QueryModuleResourcesTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"workspace_id": map[string]interface{}{"type": "string", "description": "工作空间 ID"},
			"module_path":  map[string]interface{}{"type": "string", "description": "Module 路径，如 module.vpc"},
		},
		"required": []string{"workspace_id", "module_path"},
	}
}

func (t *QueryModuleResourcesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	workspaceID, _ := params["workspace_id"].(string)
	modulePath, _ := params["module_path"].(string)

	if workspaceID == "" || modulePath == "" {
		return nil, fmt.Errorf("workspace_id and module_path are required")
	}

	var resources []models.ResourceIndex
	err := t.db.Where(
		"workspace_id = ? AND resource_mode = 'managed' AND (module_path = ? OR module_path LIKE ?)",
		workspaceID, modulePath, modulePath+".%",
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

	log.Printf("[QueryModuleResources] workspace=%s module=%s found=%d", workspaceID, modulePath, len(result))
	return map[string]interface{}{
		"module_path":    modulePath,
		"resource_count": len(result),
		"resources":      result,
	}, nil
}

// QueryCMDBDependenciesTool 查询资源依赖方
type QueryCMDBDependenciesTool struct {
	db *gorm.DB
}

func NewQueryCMDBDependenciesTool(db *gorm.DB) *QueryCMDBDependenciesTool {
	return &QueryCMDBDependenciesTool{db: db}
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

	if resourceID == "" || depField == "" {
		return nil, fmt.Errorf("resource_id and dependency_field are required")
	}

	// 全局查询，不限定 workspace，确保 __external__ CMDB 数据也能被搜索到
	query := t.db.Where("resource_mode = 'managed'")

	// 搜索 JSONB 属性：字符串字段精确匹配 + 数组字段包含匹配
	query = query.Where(
		"(attributes->>? ILIKE ? OR attributes->? @> ?)",
		depField, "%"+resourceID+"%",
		depField, fmt.Sprintf(`"%s"`, resourceID),
	)

	var resources []models.ResourceIndex
	if err := query.Limit(50).Find(&resources).Error; err != nil {
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
			Query:        strings.ToLower(strings.TrimSpace(resourceID)),
			ResourceType: depField,
			SearchMethod: "jsonb",
			Source:       "agent",
			TotalCount:   len(result),
			DurationMs:   int(elapsed.Milliseconds()),
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
}

func NewQueryResourceAttributesTool(db *gorm.DB) *QueryResourceAttributesTool {
	return &QueryResourceAttributesTool{db: db, cmdbService: NewCMDBService(db)}
}

func (t *QueryResourceAttributesTool) Name() string { return "query_resource_attributes" }
func (t *QueryResourceAttributesTool) Description() string {
	return "搜索并查询资源的完整属性信息。支持通过 cloud_resource_id、terraform_address 或关键词模糊搜索，自动跨 workspace 查询（含外部 CMDB 数据）。"
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
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}

	// 复用 CMDB 关键字搜索（支持模糊匹配，自动跨 workspace 含外部 CMDB）
	results, err := t.cmdbService.SearchResources(q, "", "", 1)
	if err != nil {
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
	if err := t.db.Where("workspace_id = ? AND cloud_resource_id = ?", hit.WorkspaceID, hit.CloudResourceID).
		First(&resource).Error; err != nil {
		// 搜索能找到但取属性失败，返回搜索结果的基本信息
		log.Printf("[QueryResourceAttributes] search hit but attributes fetch failed: %v", err)
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

	return map[string]interface{}{
		"found":              true,
		"workspace_id":       resource.WorkspaceID,
		"terraform_address":  resource.TerraformAddress,
		"resource_type":      resource.ResourceType,
		"cloud_resource_id":  resource.CloudResourceID,
		"cloud_resource_arn": resource.CloudResourceARN,
		"attributes":         attrs,
		"tags":               tags,
	}, nil
}

// QueryStateResourcesTool 查询工作空间完整资源概览
type QueryStateResourcesTool struct {
	db *gorm.DB
}

func NewQueryStateResourcesTool(db *gorm.DB) *QueryStateResourcesTool {
	return &QueryStateResourcesTool{db: db}
}

func (t *QueryStateResourcesTool) Name() string { return "query_state_resources" }
func (t *QueryStateResourcesTool) Description() string {
	return "查询工作空间当前的完整资源列表概览。用于了解整体资源全貌。"
}
func (t *QueryStateResourcesTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"workspace_id": map[string]interface{}{"type": "string", "description": "工作空间 ID"},
		},
		"required": []string{"workspace_id"},
	}
}

func (t *QueryStateResourcesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	workspaceID, _ := params["workspace_id"].(string)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	var resources []models.ResourceIndex
	err := t.db.Where("workspace_id = ? AND resource_mode = 'managed'", workspaceID).
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

	log.Printf("[QueryStateResources] workspace=%s total=%d types=%d", workspaceID, len(resources), len(groups))
	return map[string]interface{}{
		"workspace_id":   workspaceID,
		"total_resources": len(resources),
		"resource_types":  len(groups),
		"summary":         summary,
	}, nil
}

// QueryPlanSummaryTool 查询 Plan Summary（仅 apply summary 阶段使用）
type QueryPlanSummaryTool struct {
	db *gorm.DB
}

func NewQueryPlanSummaryTool(db *gorm.DB) *QueryPlanSummaryTool {
	return &QueryPlanSummaryTool{db: db}
}

func (t *QueryPlanSummaryTool) Name() string { return "query_plan_summary" }
func (t *QueryPlanSummaryTool) Description() string {
	return "查询对应任务的 Plan 阶段影响分析结果。仅在 Apply Summary 阶段可用，用于对比预测与实际结果。"
}
func (t *QueryPlanSummaryTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "number", "description": "任务 ID"},
		},
		"required": []string{"task_id"},
	}
}

func (t *QueryPlanSummaryTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	taskIDFloat, _ := params["task_id"].(float64)
	taskID := uint(taskIDFloat)
	if taskID == 0 {
		return nil, fmt.Errorf("task_id is required")
	}

	var summary models.AIPlanSummary
	err := t.db.Where("task_id = ? AND status = 'completed'", taskID).First(&summary).Error
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
