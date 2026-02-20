package handlers

import (
	"net/http"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PlatformConfigHandler handles platform configuration requests
type PlatformConfigHandler struct {
	db                    *gorm.DB
	platformConfigService *services.PlatformConfigService
}

// NewPlatformConfigHandler creates a new platform config handler
func NewPlatformConfigHandler(db *gorm.DB) *PlatformConfigHandler {
	return &PlatformConfigHandler{
		db:                    db,
		platformConfigService: services.NewPlatformConfigService(db),
	}
}

// NewPlatformConfigHandlerWithService creates a new platform config handler with an existing service
func NewPlatformConfigHandlerWithService(db *gorm.DB, platformConfigService *services.PlatformConfigService) *PlatformConfigHandler {
	return &PlatformConfigHandler{
		db:                    db,
		platformConfigService: platformConfigService,
	}
}

// PlatformConfigResponse represents the platform configuration response
type PlatformConfigResponse struct {
	BaseURL  string `json:"base_url"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	APIPort  string `json:"api_port"`
	CCPort   string `json:"cc_port"`
}

// GetPlatformConfig returns the current platform configuration
// @Summary Get platform configuration
// @Description Get the current platform configuration settings
// @Tags Platform Config
// @Produce json
// @Success 200 {object} PlatformConfigResponse
// @Router /api/v1/admin/platform-config [get]
func (h *PlatformConfigHandler) GetPlatformConfig(c *gin.Context) {
	config := PlatformConfigResponse{}

	// Load each config from database
	var configs []models.SystemConfig
	h.db.Where("key LIKE ?", "platform_%").Find(&configs)

	for _, cfg := range configs {
		value := h.parseJSONString(cfg.Value)
		switch cfg.Key {
		case "platform_base_url":
			config.BaseURL = value
		case "platform_protocol":
			config.Protocol = value
		case "platform_host":
			config.Host = value
		case "platform_api_port":
			config.APIPort = value
		case "platform_cc_port":
			config.CCPort = value
		}
	}

	// Set defaults if not found
	if config.Protocol == "" {
		config.Protocol = "http"
	}
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.APIPort == "" {
		config.APIPort = "8080"
	}
	if config.CCPort == "" {
		config.CCPort = "8081"
	}
	if config.BaseURL == "" {
		config.BaseURL = config.Protocol + "://" + config.Host + ":" + config.APIPort
	}

	c.JSON(http.StatusOK, config)
}

// UpdatePlatformConfigRequest represents the update request
type UpdatePlatformConfigRequest struct {
	BaseURL  *string `json:"base_url"`
	Protocol *string `json:"protocol"`
	Host     *string `json:"host"`
	APIPort  *string `json:"api_port"`
	CCPort   *string `json:"cc_port"`
}

// UpdatePlatformConfig updates the platform configuration
// @Summary Update platform configuration
// @Description Update the platform configuration settings
// @Tags Platform Config
// @Accept json
// @Produce json
// @Param request body UpdatePlatformConfigRequest true "Platform configuration"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/admin/platform-config [put]
func (h *PlatformConfigHandler) UpdatePlatformConfig(c *gin.Context) {
	var req UpdatePlatformConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Update each config if provided
	if req.BaseURL != nil {
		if err := h.upsertConfig("platform_base_url", *req.BaseURL, "平台基础URL，用于Run Task回调和Agent连接"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update base_url", "message": err.Error()})
			return
		}
	}
	if req.Protocol != nil {
		if err := h.upsertConfig("platform_protocol", *req.Protocol, "平台协议（http或https）"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update protocol", "message": err.Error()})
			return
		}
	}
	if req.Host != nil {
		if err := h.upsertConfig("platform_host", *req.Host, "平台主机地址"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update host", "message": err.Error()})
			return
		}
	}
	if req.APIPort != nil {
		if err := h.upsertConfig("platform_api_port", *req.APIPort, "平台API端口"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update api_port", "message": err.Error()})
			return
		}
	}
	if req.CCPort != nil {
		if err := h.upsertConfig("platform_cc_port", *req.CCPort, "Agent控制通道端口"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update cc_port", "message": err.Error()})
			return
		}
	}

	// Refresh the platform config service cache so changes take effect immediately
	if h.platformConfigService != nil {
		if err := h.platformConfigService.RefreshConfig(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to refresh config cache",
				"message": err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "platform configuration updated successfully",
		"note":    "Changes are now effective",
	})
}

// upsertConfig inserts or updates a system config
func (h *PlatformConfigHandler) upsertConfig(key, value, description string) error {
	// Value needs to be JSON formatted (quoted string)
	jsonValue := `"` + value + `"`

	var existing models.SystemConfig
	err := h.db.Where("key = ?", key).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// Insert new config
		config := models.SystemConfig{
			Key:         key,
			Value:       jsonValue,
			Description: description,
			UpdatedAt:   time.Now(),
		}
		return h.db.Create(&config).Error
	}

	if err != nil {
		return err
	}

	// Update existing config using Save (more reliable for JSONB fields)
	existing.Value = jsonValue
	existing.Description = description
	existing.UpdatedAt = time.Now()
	return h.db.Save(&existing).Error
}

// parseJSONString extracts a string value from JSON
func (h *PlatformConfigHandler) parseJSONString(value interface{}) string {
	if value == nil {
		return ""
	}

	str, ok := value.(string)
	if !ok {
		return ""
	}

	// Remove quotes if present (JSON string format)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		return str[1 : len(str)-1]
	}

	return str
}
