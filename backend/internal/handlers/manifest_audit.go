package handlers

import (
	"encoding/json"
	"strconv"

	"gorm.io/gorm"

	"iac-platform/internal/models"
)

// Manifest audit resource_type 常量(对齐 spec §13.2)
const (
	auditResourceManifest           = "MANIFEST"
	auditResourceManifestDraft      = "MANIFEST_DRAFT"
	auditResourceManifestVersion    = "MANIFEST_VERSION"
	auditResourceManifestDeployment = "MANIFEST_DEPLOYMENT"
)

// writeManifestAudit 复用现有 audit_logs 表写一条 manifest 操作审计
//
// 方案 A: ResourceID 置 NULL,manifest 字符串 ID(如 mfd-xxx)塞进 newValues 的 JSONB
// 失败不阻塞业务流程(只 log),与现有 ai_summary_controller.go 行为一致
func writeManifestAudit(
	db *gorm.DB,
	resourceType string,
	action string,
	userID string,
	newValues map[string]interface{},
) {
	var userIDUint *uint
	if uid, err := strconv.ParseUint(userID, 10, 64); err == nil {
		v := uint(uid)
		userIDUint = &v
	}

	raw, _ := json.Marshal(newValues)

	// 不阻塞主流程: 失败仅 log,不返回 error
	_ = db.Create(&models.AuditLog{
		UserID:       userIDUint,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   nil, // 字符串 ID 在 NewValues 里
		NewValues:    string(raw),
	}).Error
}
