package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ResourceSummaryService CMDB 资源摘要服务
type ResourceSummaryService struct {
	db            *gorm.DB
	configService *AIConfigService
}

// NewResourceSummaryService 创建资源摘要服务
func NewResourceSummaryService(db *gorm.DB) *ResourceSummaryService {
	return &ResourceSummaryService{
		db:            db,
		configService: NewAIConfigService(db),
	}
}

const defaultResourceSummaryPrompt = `根据资源属性生成配置摘要。

严格规则：
- 纯文本输出，禁止使用 markdown 标题（#）、代码块、列表符号
- 第一行：资源类型中文名 + 属性中的实际名称或 ID（从 name/id/bucket 等字段取，不要用占位符）
- 只描述属性中实际存在的配置，不要推测、不要给建议、不要列出缺失项
- 0.0.0.0/0 或 ::/0 标注[公网暴露]
- deletion_protection=false 标注[删除保护未启用]
- backup_retention_period=0 标注[无备份]
- 不超过 200 字

提取维度（仅输出有数据的）：
网络：入站/出站规则（协议+端口+CIDR）、公网IP、VPC/子网/可用区
安全：加密方式、删除保护、公开访问阻止、IAM权限
备份：备份保留期、多可用区、版本控制
规格：实例类型、引擎版本、存储大小
标签：Environment、team 等业务标签

资源类型: %s
属性:
%s`

// GenerateSummaries 为指定工作空间的资源生成摘要
// 同步执行，在调用方的 goroutine 内运行
func (s *ResourceSummaryService) GenerateSummaries(ctx context.Context, workspaceID string) error {
	return s.generateSummariesForResources(ctx, workspaceID, "workspace_id = ? AND resource_mode = 'managed'", workspaceID)
}

// GenerateSummariesForExternalSource 为外置 CMDB 数据源的资源生成摘要
func (s *ResourceSummaryService) GenerateSummariesForExternalSource(ctx context.Context, sourceID string) error {
	return s.generateSummariesForResources(ctx, sourceID, "external_source_id = ? AND resource_mode = 'managed'", sourceID)
}

// CompensateMissingSummaries 补偿启动时检查：找到 attributes 有值但 summary 缺失或 hash 不匹配的资源
func (s *ResourceSummaryService) CompensateMissingSummaries(ctx context.Context) error {
	return s.generateSummariesForResources(ctx, "compensation",
		"resource_mode = 'managed' AND attributes IS NOT NULL AND (resource_summary IS NULL OR resource_summary = '' OR summary_hash IS NULL OR summary_hash = '')")
}

func (s *ResourceSummaryService) generateSummariesForResources(ctx context.Context, logID string, where string, args ...interface{}) error {
	// 检查能力开关
	featureService := NewAIFeatureService(s.db)
	if !featureService.IsFeatureEnabled("cmdb_resource_summary") {
		log.Printf("[ResourceSummary] CMDB resource summary feature disabled, skipping for %s", logID)
		return nil
	}

	// 检查 AI 配置
	cfg, err := s.configService.GetConfigForCapability("cmdb_resource_summary")
	if err != nil || cfg == nil {
		log.Printf("[ResourceSummary] No AI config for 'cmdb_resource_summary', skipping for %s", logID)
		return nil
	}

	// 获取 prompt
	prompt := defaultResourceSummaryPrompt
	if customPrompt, ok := cfg.CapabilityPrompts["cmdb_resource_summary"]; ok && customPrompt != "" {
		prompt = customPrompt
	}

	// 查询资源
	var resources []models.ResourceIndex
	if err := s.db.Where(where, args...).Find(&resources).Error; err != nil {
		return fmt.Errorf("failed to query resources: %w", err)
	}

	if len(resources) == 0 {
		return nil
	}

	// 创建 AI caller
	caller := NewAICallerFromConfig(cfg)
	rateLimitSleep := time.Duration(cfg.RateLimitSeconds) * time.Second
	if rateLimitSleep < time.Second {
		rateLimitSleep = time.Second
	}

	generated := 0
	skipped := 0
	failed := 0

	for _, resource := range resources {
		if ctx.Err() != nil {
			log.Printf("[ResourceSummary] Context cancelled for %s, processed %d/%d", logID, generated+skipped+failed, len(resources))
			break
		}

		// 构建 prompt：优先用 attributes，无 attributes 则用元数据
		var contentForHash string
		var userPrompt string
		hasAttributes := len(resource.Attributes) > 0 && string(resource.Attributes) != "null" && string(resource.Attributes) != "{}"

		if hasAttributes {
			contentForHash = string(resource.Attributes)
			attributesStr := truncateAttributes(resource.Attributes)
			userPrompt = fmt.Sprintf(prompt, resource.ResourceType, attributesStr)
		} else {
			meta := fmt.Sprintf("name: %s\ncloud_resource_id: %s\ndescription: %s\ntags: %s",
				resource.ResourceName, resource.CloudResourceID, resource.Description, string(resource.Tags))
			contentForHash = meta
			userPrompt = fmt.Sprintf(prompt, resource.ResourceType, meta)
		}

		// 计算 hash，对比跳过未变更的资源
		hash := computeContentHash(contentForHash)
		if hash == resource.SummaryHash && resource.ResourceSummary != "" {
			skipped++
			continue
		}

		// 清掉旧 hash，确保 AI 失败后补偿能捡到
		if resource.SummaryHash != "" && resource.SummaryHash != hash {
			s.db.Model(&models.ResourceIndex{}).Where("id = ?", resource.ID).
				Updates(map[string]interface{}{
					"summary_hash":              "",
					"summary_assessment_status": "",
				})
		}

		// 如果有 regeneration hint（质量反馈），追加到 prompt
		if resource.SummaryRegenerationHint != "" {
			userPrompt += fmt.Sprintf("\n\n上一次生成的摘要存在以下问题，请在本次生成中修正：\n%s", resource.SummaryRegenerationHint)
		}

		// 调 AI（单个资源 30 秒超时）
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		messages := []AgentMessage{
			{Role: "user", Content: userPrompt},
		}
		response, err := caller.ChatWithTools(callCtx, messages, nil)
		callCancel()
		if err != nil {
			log.Printf("[ResourceSummary] AI call failed for resource %d (%s): %v", resource.ID, resource.TerraformAddress, err)
			failed++
			continue
		}

		summary := strings.TrimSpace(response.Content)
		if summary == "" {
			log.Printf("[ResourceSummary] AI returned empty summary for resource %d (%s)", resource.ID, resource.TerraformAddress)
			failed++
			continue
		}

		// 写入 resource_summary + summary_hash（GORM）
		if err := s.db.Model(&models.ResourceIndex{}).Where("id = ?", resource.ID).Updates(map[string]interface{}{
			"resource_summary":           summary,
			"summary_hash":               hash,
			"summary_assessment_status":  "pending",
			"summary_regeneration_hint":  "",
		}).Error; err != nil {
			log.Printf("[ResourceSummary] Failed to save summary for resource %d: %v", resource.ID, err)
			failed++
			continue
		}

		generated++

		// Rate limiting
		if rateLimitSleep > 0 {
			time.Sleep(rateLimitSleep)
		}
	}

	log.Printf("[ResourceSummary] Completed for %s: generated=%d, skipped=%d, failed=%d, total=%d",
		logID, generated, skipped, failed, len(resources))

	return nil
}

// computeContentHash 计算内容的 MD5 hash
func computeContentHash(content string) string {
	if content == "" {
		return ""
	}
	hash := md5.Sum([]byte(content))
	return hex.EncodeToString(hash[:])
}

// truncateAttributes 智能截断 attributes，保留安全相关 key
func truncateAttributes(attributes json.RawMessage) string {
	const maxLen = 8000

	if len(attributes) <= maxLen {
		return string(attributes)
	}

	// 解析 JSON
	var attrs map[string]interface{}
	if err := json.Unmarshal(attributes, &attrs); err != nil {
		// 无法解析，直接截断
		return string(attributes[:maxLen]) + "..."
	}

	// 低价值的大字段，优先移除
	lowValueKeys := []string{
		"after_unknown", "after_sensitive", "before_sensitive",
		"after_identity", "timeouts",
	}
	for _, key := range lowValueKeys {
		delete(attrs, key)
	}

	result, _ := json.Marshal(attrs)
	if len(result) <= maxLen {
		return string(result)
	}

	// 仍然超限，保留高价值 key
	highValueKeys := map[string]bool{
		"id": true, "arn": true, "name": true, "tags": true, "tags_all": true,
		"ingress": true, "egress": true, "policy": true, "assume_role_policy": true,
		"encryption": true, "server_side_encryption_configuration": true,
		"public_access": true, "block_public_acls": true, "block_public_policy": true,
		"deletion_protection": true, "skip_final_snapshot": true, "force_destroy": true,
		"backup_retention_period": true, "multi_az": true,
		"versioning": true, "lifecycle_rule": true, "lifecycle_configuration": true,
		"instance_type": true, "engine": true, "engine_version": true,
		"vpc_id": true, "subnet_id": true, "subnet_ids": true,
		"security_group_ids": true, "vpc_security_group_ids": true,
		"publicly_accessible": true, "associate_public_ip_address": true,
		"cidr_block": true, "cidr_blocks": true,
		"rule": true, "inbound_rule": true, "outbound_rule": true,
		"bucket": true, "region": true, "status": true,
	}

	filtered := make(map[string]interface{})
	for key, val := range attrs {
		if highValueKeys[key] {
			filtered[key] = val
		}
	}

	result, _ = json.Marshal(filtered)
	if len(result) > maxLen {
		return string(result[:maxLen]) + "..."
	}
	return string(result)
}
