package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditConfigHandler 审计配置处理器
type AuditConfigHandler struct {
	db *gorm.DB
}

// NewAuditConfigHandler 创建审计配置处理器实例
func NewAuditConfigHandler(db *gorm.DB) *AuditConfigHandler {
	return &AuditConfigHandler{
		db: db,
	}
}

// AuditConfig 审计配置
type AuditConfig struct {
	Enabled        bool `json:"enabled"`
	IncludeBody    bool `json:"include_body"`
	IncludeHeaders bool `json:"include_headers"`
}

// GetAuditConfig 获取审计配置
// @Summary 获取审计配置
// @Tags IAM-Audit
// @Produce json
// @Success 200 {object} AuditConfig
// @Router /api/v1/iam/audit/config [get]
func (h *AuditConfigHandler) GetAuditConfig(c *gin.Context) {
	var config AuditConfig

	// 从system_configs表读取配置（value是JSONB类型，存储为字符串）
	var enabledValue string
	err := h.db.Table("system_configs").
		Select("value::text").
		Where("key = ?", "audit_log_enabled").
		Scan(&enabledValue).Error

	if err != nil || enabledValue == "" {
		// 默认启用
		config.Enabled = true
	} else {
		// 去掉JSON字符串的引号
		enabledValue = enabledValue[1 : len(enabledValue)-1]
		config.Enabled = enabledValue == "true"
	}

	// 读取body配置
	var bodyValue string
	h.db.Table("system_configs").
		Select("value::text").
		Where("key = ?", "audit_log_include_body").
		Scan(&bodyValue)
	if bodyValue != "" {
		bodyValue = bodyValue[1 : len(bodyValue)-1]
		config.IncludeBody = bodyValue == "true"
	}

	// 读取headers配置
	var headersValue string
	h.db.Table("system_configs").
		Select("value::text").
		Where("key = ?", "audit_log_include_headers").
		Scan(&headersValue)
	if headersValue != "" {
		headersValue = headersValue[1 : len(headersValue)-1]
		config.IncludeHeaders = headersValue == "true"
	}

	c.JSON(http.StatusOK, config)
}

// UpdateAuditConfig 更新审计配置
// @Summary 更新审计配置
// @Tags IAM-Audit
// @Accept json
// @Produce json
// @Param config body AuditConfig true "审计配置"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/audit/config [put]
func (h *AuditConfigHandler) UpdateAuditConfig(c *gin.Context) {
	var config AuditConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新或插入配置
	enabledValue := "false"
	if config.Enabled {
		enabledValue = "true"
	}

	if err := h.db.Exec(`
		INSERT INTO system_configs (key, value, description, updated_at)
		VALUES ('audit_log_enabled', ?::jsonb, '审计日志启用状态', NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, `"`+enabledValue+`"`).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update enabled config: " + err.Error()})
		return
	}

	bodyValue := "false"
	if config.IncludeBody {
		bodyValue = "true"
	}
	if err := h.db.Exec(`
		INSERT INTO system_configs (key, value, description, updated_at)
		VALUES ('audit_log_include_body', ?::jsonb, '审计日志记录请求体', NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, `"`+bodyValue+`"`).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update body config: " + err.Error()})
		return
	}

	headersValue := "false"
	if config.IncludeHeaders {
		headersValue = "true"
	}
	if err := h.db.Exec(`
		INSERT INTO system_configs (key, value, description, updated_at)
		VALUES ('audit_log_include_headers', ?::jsonb, '审计日志记录请求头', NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, `"`+headersValue+`"`).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update headers config: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "审计配置已更新",
		"config":  config,
	})
}
