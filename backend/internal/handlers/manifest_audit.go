package handlers

import (
	"encoding/json"

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
// 失败不阻塞业务流程(只 log)
func writeManifestAudit(
	db *gorm.DB,
	resourceType string,
	action string,
	userID string,
	newValues map[string]interface{},
) {
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	raw, _ := json.Marshal(newValues)

	_ = db.Create(&models.AuditLog{
		UserID:       userIDPtr,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   nil,
		NewValues:    string(raw),
	}).Error
}
